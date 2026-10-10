import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { CloudPlanningStatus } from "../api/cloud-types";
import { t } from "../i18n";

export function ChatGPTConnection({ onChange, presentation = "settings", locked = false, needsAuthorization = false }: { onChange?: (status: CloudPlanningStatus) => void; presentation?: "setup" | "settings"; locked?: boolean; needsAuthorization?: boolean }) {
  const [status, setStatus] = useState<CloudPlanningStatus | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const mounted = useRef(true);
  const busyRef = useRef(false);
  const onChangeRef = useRef(onChange);
  onChangeRef.current = onChange;
  const hostBrowser = ["127.0.0.1", "localhost", "[::1]"].includes(window.location.hostname);
  const active = status?.connection.profiles.find(profile => profile.id === status.connection.active);
  const refresh = useCallback(async () => {
    try { const next = await api.cloudPlanningStatus(); if (mounted.current) { setStatus(next); onChangeRef.current?.(next); } }
    catch (reason) { if (mounted.current) setError(reason instanceof Error ? reason.message : String(reason)); }
  }, []);
  useEffect(() => { mounted.current = true; void refresh(); return () => { mounted.current = false; }; }, [refresh]);
  useEffect(() => {
    if (!status?.connection.pending && !active) return;
    const timer = window.setInterval(() => void refresh(), status?.connection.pending ? 1500 : 15000);
    return () => window.clearInterval(timer);
  }, [refresh, status?.connection.pending, active?.id]);
  async function run(work: () => Promise<CloudPlanningStatus>) {
    if (busyRef.current || locked) return; busyRef.current = true; setBusy(true); setError("");
    try { const next = await work(); if (mounted.current) { setStatus(next); onChangeRef.current?.(next); } }
    catch (reason) { if (mounted.current) setError(reason instanceof Error ? reason.message : String(reason)); }
    finally { busyRef.current = false; if (mounted.current) setBusy(false); }
  }
  async function signIn(profile = "") {
    if (busyRef.current || locked) return;
    const popup = window.open("about:blank", "_blank");
    if (!popup) { setError(t("Allow a sign-in window, then try again.")); return; }
    if (popup) popup.opener = null;
    await run(async () => {
      try {
        const result = await api.cloudSignIn(profile);
        popup.location.href = result.authorization_url;
        return api.cloudPlanningStatus();
      } catch (reason) { popup?.close(); throw reason; }
    });
  }
  const connected = Boolean(active?.connected && active.plan_authorized && !needsAuthorization);
  const disabled = busy || locked;
  const accountPicker = status && status.connection.profiles.length > 1 && <label className="field"><span className="label">{t("ChatGPT account")}</span>
    <select value={status.connection.active} disabled={disabled} onChange={event => {
      const profile = status.connection.profiles.find(item => item.id === event.target.value);
      if (profile?.connected) void run(() => api.cloudSelect(profile.id)); else if (profile && hostBrowser) void signIn(profile.id);
    }}>
      <option value="" disabled>{t("Select an account")}</option>
      {status.connection.profiles.map(profile => <option key={profile.id} value={profile.id}>{profile.label}</option>)}
    </select>
  </label>;
  return <div className="chatgpt-connection cloud-motion-settings" data-presentation={presentation}>
    {status?.connection.pending ? <>
      <p role="status">{t("Complete sign-in in the opened window.")}</p>
      <button className="btn btn-secondary" type="button" disabled={disabled} onClick={() => void run(api.cloudCancelSignIn)}>{t("Cancel sign-in")}</button>
    </> : connected ? <>
      <p className="connected-account"><strong>{t("ChatGPT connected")}</strong><span>{active?.label}</span></p>
      <details><summary>{t("Manage account")}</summary>
        {accountPicker}
        <div className="button-row">
          <button className="btn btn-secondary" type="button" disabled={disabled || !hostBrowser} onClick={() => void signIn()}>{t("Add account")}</button>
          <button className="btn btn-secondary" type="button" disabled={disabled || !hostBrowser} onClick={() => void signIn(active?.id)}>{t("Reconnect")}</button>
          <button className="btn btn-secondary" type="button" disabled={disabled} onClick={() => void run(async () => (await api.cloudAccountDisconnect(active?.id ?? "")).status)}>{t("Disconnect")}</button>
          <a href="https://chatgpt.com/settings/usage" target="_blank" rel="noreferrer">{t("Manage usage")}</a>
        </div>
      </details>
    </> : <>
      <p className="hint">{active && !active.plan_authorized ? t("Authorize ChatGPT plan use to enable inference.") : t("Use your ChatGPT plan. Account limits and provider content rules apply.")}</p>
      {accountPicker}
      <button className="btn chatgpt-sign-in" type="button" disabled={disabled || !hostBrowser} onClick={() => void signIn(active?.id)}>{active ? t("Reconnect") : t("Continue with ChatGPT")}</button>
    </>}
    {!hostBrowser && <p className="hint">{t("Connect ChatGPT from MagicHandy on the host computer. Sign-in returns to that computer.")}</p>}
    {status?.connection.message && !status.connection.pending && <p role="status">{status.connection.message}</p>}
    {connected && active?.welcome_pending && <div className="chatgpt-welcome" role="note" aria-label={t("You're using your ChatGPT plan")}>
      <h4>{t("You're using your ChatGPT plan")}</h4>
      <p>{t("Eligible requests use your ChatGPT plan or authorized credits. You control usage in ChatGPT Settings.")}</p>
      <button className="btn btn-secondary" type="button" disabled={disabled} onClick={() => void run(api.cloudWelcome)}>{t("Got it")}</button>
    </div>}
    {error && <p className="error-text" role="alert">{error}</p>}
  </div>;
}
