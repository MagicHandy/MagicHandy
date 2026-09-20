import { useEffect, useRef, useState, type FormEvent } from "react";
import { api } from "../api/client";
import type { IssuedRecoveryCodes, RecoveryCodeStatus } from "../api/access-types";
import { t, translateKnown } from "../i18n";
import { ChevronUpIcon } from "../shell/icons";

export function RecoveryCodesPanel({ backendOnline }: { backendOnline: boolean }) {
  const [expanded, setExpanded] = useState(false);
  const [status, setStatus] = useState<RecoveryCodeStatus | null>(null);
  const [codes, setCodes] = useState<string[] | null>(null);
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const active = useRef<{ controller: AbortController; timer: number } | null>(null);

  const cancel = () => {
    const previous = active.current;
    active.current = null;
    if (previous) { window.clearTimeout(previous.timer); previous.controller.abort(); }
  };
  const run = async (operation: "read" | "replace" | "remove") => {
    cancel();
    const controller = new AbortController();
    const request = { controller, timer: 0 };
    request.timer = window.setTimeout(() => {
      if (active.current !== request) return;
      active.current = null;
      controller.abort();
      setBusy(false); setStatus(null); setCodes(null);
      setError(t("The recovery request timed out. Refresh its status before trying again."));
    }, 10_000);
    active.current = request;
    setBusy(true); setError("");
    if (operation !== "read") setCodes(null);
    try {
      const result = operation === "read" ? await api.recoveryCodeStatus(controller.signal)
        : operation === "replace" ? await api.replaceRecoveryCodes(password, controller.signal)
          : await api.removeRecoveryCodes(password, controller.signal);
      if (active.current !== request) return;
      if (!Number.isInteger(result.remaining) || result.remaining < 0 || result.remaining > 8 || result.limit !== 8) {
        throw new Error(t("Invalid recovery response."));
      }
      if (operation === "replace") {
        const issued = result as IssuedRecoveryCodes;
        if (!Array.isArray(issued.codes) || issued.codes.length !== 8 ||
            issued.codes.some(code => typeof code !== "string" || !/^[A-F0-9]{4}(?:-[A-F0-9]{4}){7}$/.test(code))) {
          throw new Error(t("Invalid recovery response."));
        }
        setCodes(issued.codes);
      } else if (status?.created_at !== result.created_at) {
        setCodes(null);
      }
      // Keep the one-time secret out of the retained status object.
      setStatus({ remaining: result.remaining, limit: result.limit, created_at: result.created_at });
      if (operation !== "read") setPassword("");
    } catch (reason) {
      if (active.current !== request) return;
      setError(controller.signal.aborted ? t("The recovery request timed out. Refresh its status before trying again.")
        : reason instanceof Error ? translateKnown(reason.message) : t("Request failed"));
    } finally {
      window.clearTimeout(request.timer);
      if (active.current === request) { active.current = null; setBusy(false); }
    }
  };

  useEffect(() => {
    if (expanded && backendOnline) void run("read");
    else { cancel(); setCodes(null); setPassword(""); setBusy(false); setStatus(null); }
    return cancel;
  }, [expanded, backendOnline]);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!busy && backendOnline && password) void run("replace");
  };
  return (
    <section className="group">
      <button type="button" className="group-title recovery-toggle" aria-expanded={expanded} onClick={() => setExpanded(value => !value)}><ChevronUpIcon size={14} />{t("Recovery codes")}</button>
      {expanded && <>
        <p className="hint-block">{t("Save recovery codes before you need them. One code can reset your password without deleting this installation's data.")}</p>
        <p className="hint-block">{t("Keep codes private. A password reset, password change or account disabling invalidates the whole set.")}</p>
        {!backendOnline && <p role="status">{t("Reconnect to manage recovery codes.")}</p>}
        {backendOnline && status && <p role="status">{t("{count} of {limit} recovery codes available.", { count: status.remaining, limit: status.limit })}</p>}
        {error && <p className="form-status auth-error" role="alert">{error}</p>}
        {backendOnline && codes && <div aria-label={t("New recovery codes")}>
          <p className="hint-block">{t("These codes are shown only now. Save them in a private password manager before closing this panel.")}</p>
          <ul className="recovery-codes">{codes.map(code => <li key={code}><code>{code}</code></li>)}</ul>
          <button className="btn btn-secondary" type="button" onClick={() => setCodes(null)}>{t("I saved the codes")}</button>
        </div>}
        <form className="account-form" onSubmit={submit}>
          <label className="field"><span className="label">{t("Current password")}</span>
            <input type="password" autoComplete="current-password" value={password} disabled={busy || !backendOnline} onChange={event => setPassword(event.target.value)} />
          </label>
          <p className="hint-block">{t("Generating a new set immediately invalidates the previous set. Existing codes cannot be displayed again.")}</p>
          <div className="audit-toolbar">
            <button className="btn btn-secondary" type="submit" disabled={busy || !backendOnline || !password || !status}>{t("Generate new recovery codes")}</button>
            <button className="btn btn-secondary" type="button" disabled={busy || !backendOnline || !password || !status?.remaining} onClick={() => void run("remove")}>{t("Remove recovery codes")}</button>
            <button className="btn btn-secondary" type="button" disabled={busy || !backendOnline} onClick={() => void run("read")}>{t("Refresh status")}</button>
          </div>
        </form>
      </>}
    </section>
  );
}
