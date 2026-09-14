import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { NetworkConfig, NetworkStatus } from "../api/types";
import { t } from "../i18n";

const modeName = (mode: NetworkConfig["mode"]) => ({ local: t("Local only"), direct_https: t("Direct HTTPS"), trusted_proxy: t("Trusted reverse proxy"), legacy: t("Startup flags") })[mode];
const errorText = (reason: unknown) => reason instanceof Error ? reason.message : t("Request failed");

export function NetworkSettingsPanel({ backendOnline, administrator }: { backendOnline: boolean; administrator: boolean }) {
  const [status, setStatus] = useState<NetworkStatus | null>(null);
  const [draft, setDraft] = useState<NetworkConfig | null>(null);
  const [validated, setValidated] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  useEffect(() => {
    if (!administrator || !backendOnline) return;
    const abort = new AbortController();
    void api.networkStatus(abort.signal).then((next) => {
      if (abort.signal.aborted) return;
      setStatus(next);
      const current = next.saved ?? next.active;
      setDraft(current.mode === "legacy" ? { mode: "local", listen_address: current.listen_address, public_url: "", trusted_proxies: [], tls_certificate: "", tls_private_key: "" } : current);
    }).catch((reason: unknown) => { if (!abort.signal.aborted) setError(errorText(reason)); });
    return () => abort.abort();
  }, [administrator, backendOnline]);

  if (!administrator) return <section className="group"><h3 className="group-title">{t("LAN and WAN access")}</h3><p className="hint-block">{t("An administrator manages the installation's network address and certificates. Your account can observe shared content; controlling motion requires a separate permission.")}</p></section>;
  const locked = !backendOnline || busy || !draft;
  const patch = (change: Partial<NetworkConfig>) => {
    setDraft((current) => current && { ...current, ...change });
    setValidated(""); setMessage(""); setError("");
  };
  const changeMode = (mode: NetworkConfig["mode"]) => patch({ mode, trusted_proxies: [], tls_certificate: "", tls_private_key: "", ...(mode === "local" ? { listen_address: "127.0.0.1:49717", public_url: "" } : {}) });
  const validate = async () => {
    if (!draft) return;
    setBusy(true); setError(""); setMessage("");
    try {
      const result = await api.validateNetwork(draft);
      setDraft(result.config); setValidated(JSON.stringify(result.config)); setMessage(result.message);
    } catch (reason) { setValidated(""); setError(errorText(reason)); }
    finally { setBusy(false); }
  };
  const save = async () => {
    if (!draft || validated !== JSON.stringify(draft)) return;
    setBusy(true); setError("");
    try {
      await api.saveNetwork(draft, password);
      setStatus(await api.networkStatus());
      setMessage(t("Saved for the next app restart. Keep this session available while you test the new address from another device."));
    } catch (reason) { setError(errorText(reason)); }
    finally { setPassword(""); setBusy(false); }
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
  return <section className="group network-settings">
    <h3 className="group-title">{t("LAN and WAN access")}</h3>
    <p className="hint-block">{t("Choose an explicit HTTPS address for other devices. Changes apply after restarting MagicHandy. Creating a public route, opening a firewall, and enrolling client trust remain under your control.")}</p>
    {status && <p className="form-status">{t("Running:")} <strong>{modeName(status.active.mode)}</strong> · {status.active.public_url || status.active.listen_address}{status.restart_required && <> · <strong>{t("Restart required")}</strong></>}</p>}
    {draft && <>
      <div className="settings-grid two">
        <label className="field"><span className="label">{t("Access mode")}</span><select value={draft.mode} disabled={locked} onChange={(event) => changeMode(event.target.value as NetworkConfig["mode"])}><option value="local">{t("Local only")}</option><option value="direct_https">{t("Direct HTTPS")}</option><option value="trusted_proxy">{t("Trusted reverse proxy")}</option></select></label>
        <label className="field"><span className="label">{t("Listen address and port")}</span><input type="text" value={draft.listen_address} list="network-interfaces" disabled={locked} spellCheck={false} onChange={(event) => patch({ listen_address: event.target.value })} /><datalist id="network-interfaces">{status?.interfaces.map((item) => <option key={`${item.name}/${item.address}`} value={`${item.address.includes(":") ? `[${item.address}]` : item.address}:49717`}>{item.name}</option>)}</datalist></label>
        {draft.mode !== "local" && <label className="field"><span className="label">{t("Public HTTPS URL")}</span><input type="text" value={draft.public_url} disabled={locked} spellCheck={false} onChange={(event) => patch({ public_url: event.target.value })} /><span className="hint">{t("The exact address clients use, including a nonstandard port. No path or credentials.")}</span></label>}
        {draft.mode === "trusted_proxy" && <label className="field"><span className="label">{t("Trusted proxy peers")}</span><input type="text" value={(draft.trusted_proxies ?? []).join(", ")} disabled={locked} spellCheck={false} onChange={(event) => patch({ trusted_proxies: event.target.value.split(",").map((part) => part.trim()) })} /><span className="hint">{t("Only these immediate proxy IPs or CIDRs may reach this listener. The proxy must replace forwarded headers.")}</span></label>}
        {draft.mode === "direct_https" && <>
          <label className="field"><span className="label">{t("Certificate chain path on this host")}</span><input type="text" value={draft.tls_certificate} disabled={locked} spellCheck={false} onChange={(event) => patch({ tls_certificate: event.target.value })} /></label>
          <label className="field"><span className="label">{t("Private key path on this host")}</span><input type="text" value={draft.tls_private_key} disabled={locked} spellCheck={false} onChange={(event) => patch({ tls_private_key: event.target.value })} /><span className="hint">{t("PEM files readable by the account running MagicHandy. Keep the key private.")}</span></label>
        </>}
      </div>
      {status?.certificate && <p className="hint-block">{t("Certificate expires:")} {new Date(status.certificate.not_after).toLocaleString()}{status.certificate.renewal_due && <> · {t("Renewal due")}</>}{status.certificate.reload_error && <> · {t("The replacement could not be loaded; the previous valid certificate is still in use.")}</>}</p>}
      {draft.mode === "trusted_proxy" && <p className="hint-block">{t("Use a loopback or private backend listener. Configure one HTTPS public origin, disable response buffering/caching, and keep worker ports private. Forwarded requests cannot open host file dialogs or bootstrap accounts.")}</p>}
      {status?.authentication_required && <label className="field"><span className="label">{t("Current administrator password")}</span><input type="password" autoComplete="current-password" value={password} disabled={locked} onChange={(event) => setPassword(event.target.value)} /><span className="hint">{t("Required when saving remote-access changes.")}</span></label>}
      <div className="row-actions"><button className="btn btn-secondary" type="button" disabled={locked} onClick={() => void validate()}>{t("Validate configuration")}</button><button className="btn btn-primary" type="button" disabled={locked || !validated || (status?.authentication_required && !password)} onClick={() => void save()}>{t("Save for restart")}</button><button className="btn btn-secondary" type="button" disabled={locked} onClick={() => void report()}>{t("Download connection report")}</button></div>
      <p className="hint-block">{t("If remote setup fails, start MagicHandy on this host with -network-mode local. Account protection remains enabled, and you can repair the saved network settings here.")}</p>
    </>}
    {error && <p className="form-status auth-error" role="alert">{error}</p>}
    {message && <p className="form-status" role="status">{message}</p>}
  </section>;
}
