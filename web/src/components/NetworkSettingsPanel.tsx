import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { CertificatePreparation, InternetDiscovery, NetworkConfig, NetworkStatus } from "../api/types";
import { t } from "../i18n";
import { AccessScopeChoices, configForScope, NetworkSetupFields, networkScope, type NetworkScope } from "./NetworkSetupFields";

const modeName = (mode: NetworkConfig["mode"]) => ({ local: t("Local only"), direct_https: t("Direct HTTPS"), trusted_proxy: t("Trusted reverse proxy"), legacy: t("Startup flags") })[mode];
const errorText = (reason: unknown) => reason instanceof Error ? reason.message : t("Request failed");

function pause(signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const abort = () => { clearTimeout(timer); reject(new DOMException("Aborted", "AbortError")); };
    const timer = setTimeout(() => { signal.removeEventListener("abort", abort); resolve(); }, 1500);
    if (signal.aborted) abort();
    else signal.addEventListener("abort", abort, { once: true });
  });
}

export function NetworkSettingsPanel({ backendOnline, administrator, initialScope, onReadyChange }: {
  backendOnline: boolean; administrator: boolean; initialScope?: NetworkScope; onReadyChange?: (ready: boolean) => void;
}) {
  const [status, setStatus] = useState<NetworkStatus | null>(null);
  const [draft, setDraft] = useState<NetworkConfig | null>(null);
  const [validated, setValidated] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [detecting, setDetecting] = useState(false);
  const [discovery, setDiscovery] = useState<InternetDiscovery | null>(null);
  const [preparation, setPreparation] = useState<CertificatePreparation | null>(null);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const lifetime = useRef(new AbortController());
  useEffect(() => {
    const abort = new AbortController(); lifetime.current = abort;
    setBusy(false); setDetecting(false); setDraft(null); setStatus(null); setPassword(""); setValidated("");
    if (administrator && backendOnline) void api.networkStatus(abort.signal).then((next) => {
      if (abort.signal.aborted) return;
      setStatus(next); setPreparation(next.preparation ?? null);
      const current = next.saved ?? next.active;
      setDraft(configForScope(initialScope ?? networkScope(current), current, next));
    }).catch((reason: unknown) => { if (!abort.signal.aborted) setError(errorText(reason)); });
    return () => abort.abort();
  }, [administrator, backendOnline, initialScope]);

  useEffect(() => {
    onReadyChange?.(Boolean(!busy && draft && status && JSON.stringify(draft) === JSON.stringify(status.saved ?? status.active)));
  }, [busy, draft, status, onReadyChange]);

  // Reopening setup can find a job started by an earlier page. Observe it
  // without reissuing a certificate or retaining that page's password.
  useEffect(() => {
    if (preparation?.state !== "running" || busy) return;
    const abort = new AbortController();
    void (async () => {
      try {
        let next: CertificatePreparation;
        do {
          await pause(abort.signal);
          next = await api.certificatePreparation(abort.signal);
          if (!abort.signal.aborted) setPreparation(next);
        } while (next.state === "running" && !abort.signal.aborted);
      } catch (reason) { if (!abort.signal.aborted) setError(errorText(reason)); }
    })();
    return () => abort.abort();
  }, [preparation?.state, busy]);

  const detect = async () => {
    const signal = lifetime.current.signal;
    setDetecting(true); setError("");
    try {
      const result = await api.discoverInternet(signal);
      if (signal.aborted) return;
      setDiscovery(result);
      setDraft((current) => current?.certificate_mode === "automatic_public" ? { ...current,
        public_url: current.public_url || (result.public_ip ? `https://${result.public_ip.includes(":") ? `[${result.public_ip}]` : result.public_ip}` : ""),
        accepted_terms: current.accepted_terms === result.terms_url ? current.accepted_terms : undefined,
      } : current);
      setValidated("");
    } catch (reason) { if (!signal.aborted) setError(errorText(reason)); }
    finally { if (!signal.aborted) setDetecting(false); }
  };
  const wantsPublicCertificate = draft?.certificate_mode === "automatic_public";
  useEffect(() => { if (wantsPublicCertificate) void detect(); }, [wantsPublicCertificate]);

  if (!administrator) return <section className="group"><h3 className="group-title">{t("LAN and WAN access")}</h3><p className="hint-block">{t("An administrator manages the installation's network address and certificates. Your account can observe shared content; controlling motion requires a separate permission.")}</p></section>;
  const locked = !backendOnline || busy || detecting || !draft;
  const patch = (change: Partial<NetworkConfig>) => {
    setDraft((current) => current && { ...current, ...change });
    setValidated(""); setMessage(""); setError("");
  };
  const validate = async () => {
    if (!draft) return;
    const signal = lifetime.current.signal;
    setBusy(true); setError(""); setMessage("");
    try {
      const result = await api.validateNetwork(draft);
      if (signal.aborted) return;
      setDraft(result.config); setValidated(JSON.stringify(result.config)); setMessage(result.message);
    } catch (reason) { if (!signal.aborted) { setValidated(""); setError(errorText(reason)); } }
    finally { if (!signal.aborted) setBusy(false); }
  };
  const save = async (prepare = false) => {
    if (!draft || (!prepare && validated !== JSON.stringify(draft))) return;
    const signal = lifetime.current.signal;
    setBusy(true); setError(""); setMessage("");
    try {
      if (prepare) {
        let next = await api.prepareNetworkCertificate(draft, password, signal);
        setPreparation(next);
        const deadline = Date.now() + 5 * 60_000;
        while (next.state === "running" && Date.now() < deadline) {
          await pause(signal);
          next = await api.certificatePreparation(signal);
          if (!signal.aborted) setPreparation(next);
        }
        if (next.state !== "ready") throw new Error(next.message || t("Certificate preparation did not finish. Check its status before retrying."));
      }
      if (signal.aborted) return;
      const checked = await api.validateNetwork(draft);
      if (signal.aborted) return;
      await api.saveNetwork(checked.config, password);
      if (signal.aborted) return;
      const refreshed = await api.networkStatus(signal);
      if (signal.aborted) return;
      setDraft(checked.config); setStatus(refreshed);
      setMessage(t("Saved for the next app restart. Keep this session available while you test the new address from another device."));
    } catch (reason) { if (!signal.aborted) setError(errorText(reason)); }
    finally { if (!signal.aborted) { setPassword(""); setBusy(false); } }
  };
  const report = async () => {
    setBusy(true); setError("");
    try {
      const report = await api.networkReport();
      const link = document.createElement("a");
      const url = URL.createObjectURL(new Blob([JSON.stringify(report, null, 2) + "\n"], { type: "application/json" }));
      link.href = url; link.download = "magichandy-connection-report.json"; link.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
      setMessage(t("Connection report downloaded. Share it with the developer when reporting a connection problem."));
    } catch (reason) { setError(errorText(reason)); }
    finally { setBusy(false); }
  };
  const needsPassword = status?.authentication_required && !password;
  const preparedForDraft = preparation?.state === "ready" && preparation.config?.public_url === draft?.public_url && preparation.config?.certificate_mode === draft?.certificate_mode;
  return <section className="group network-settings">
    {!initialScope && <h3 className="group-title">{t("LAN and WAN access")}</h3>}
    {status && <p className="form-status">{t("Running:")} <strong>{modeName(status.active.mode)}</strong> · {status.active.public_url || status.active.listen_address}{status.restart_required && <> · <strong>{t("Restart required")}</strong></>}</p>}
    {draft && status && <>
      {!initialScope && <AccessScopeChoices value={networkScope(draft)} disabled={locked} onChange={(scope) => { setDraft(configForScope(scope, draft, status)); setValidated(""); setError(""); setMessage(""); }} />}
      <NetworkSetupFields draft={draft} status={status} locked={locked} discovery={discovery} detecting={detecting} patch={patch} detect={() => void detect()} />
      {status.certificate && <p className="hint-block">{t("Certificate expires:")} {new Date(status.certificate.not_after).toLocaleString()}{status.certificate.renewal_due && <> · {t("Renewal due")}</>}{status.certificate.reload_error && <> · {t("Certificate renewal or reload failed. Check HTTPS setup; an expired certificate will not be served.")}</>}</p>}
      {status.authentication_required && <label className="field"><span className="label">{t("Current administrator password")}</span><input type="password" autoComplete="current-password" value={password} disabled={locked} onChange={(event) => setPassword(event.target.value)} /><span className="hint">{t("Required when saving remote-access changes.")}</span></label>}
      <div className="row-actions">
        {draft.certificate_mode ? <button className="btn btn-primary" type="button" disabled={locked || !status.authentication_required || Boolean(needsPassword) || (wantsPublicCertificate && !draft.accepted_terms)} onClick={() => void save(true)}>{busy ? t("Preparing HTTPS...") : t("Set up HTTPS and save")}</button> : <>
          <button className="btn btn-secondary" type="button" disabled={locked} onClick={() => void validate()}>{t("Validate configuration")}</button>
          <button className="btn btn-primary" type="button" disabled={locked || !validated || Boolean(needsPassword)} onClick={() => void save()}>{t("Save for restart")}</button>
        </>}
        {draft.certificate_mode === "local_ca" && (preparedForDraft || status.active.certificate_mode === "local_ca") && <a className="btn btn-secondary" href="/api/network/local-trust" download="magichandy-local-trust.crt">{t("Download local trust certificate")}</a>}
        {!initialScope && <button className="btn btn-secondary" type="button" disabled={locked} onClick={() => void report()}>{t("Download connection report")}</button>}
      </div>
      {preparation?.state === "running" && <p className="form-status" role="status">{t("Preparing the certificate. Public validation can take a few minutes; the current app address stays available.")}</p>}
      {preparedForDraft && <p className="form-status">{t("Certificate ready.")}</p>}
      {preparation?.state === "failed" && !error && <p className="form-status auth-error" role="alert">{preparation.message}</p>}
      {status.restart_required && status.saved?.public_url && <p className="network-result">{t("After restart:")} <a href={status.saved.public_url} target="_blank" rel="noreferrer">{status.saved.public_url}</a></p>}
      <p className="hint-block">{t("If remote setup fails, start MagicHandy on this host with -network-mode local. Account protection remains enabled, and you can repair the saved network settings here.")}</p>
    </>}
    {error && <p className="form-status auth-error" role="alert">{error}</p>}
    {message && <p className="form-status" role="status">{message}</p>}
  </section>;
}
