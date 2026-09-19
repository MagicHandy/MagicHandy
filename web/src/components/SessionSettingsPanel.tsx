import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";
import { api } from "../api/client";
import type { ManagedSession, ManagedSessionsResponse } from "../api/types";
import { t, translateKnown } from "../i18n";

const REQUEST_TIMEOUT_MS = 10_000;
const visible = () => document.visibilityState !== "hidden";
const failure = (reason: unknown) => reason instanceof Error ? translateKnown(reason.message) : t("Session management is unavailable.");

export function SessionSettingsPanel({ backendOnline, onSignedOut }: {
  backendOnline: boolean;
  onSignedOut: () => Promise<unknown>;
}) {
  const [sessions, setSessions] = useState<ManagedSession[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [editing, setEditing] = useState("");
  const [name, setName] = useState("");
  const alive = useRef(false);
  const read = useRef<{ controller: AbortController; timer: number } | null>(null);
  const action = useRef<{ controller: AbortController; timer: number } | null>(null);

  const cancelRead = useCallback(() => {
    read.current?.controller.abort();
    window.clearTimeout(read.current?.timer);
    read.current = null;
  }, []);

  const load = useCallback(async (preserveError = false) => {
    if (!backendOnline || !visible() || read.current || action.current) return;
    const controller = new AbortController();
    const entry = { controller, timer: window.setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS) };
    read.current = entry;
    setLoading(true);
    try {
      const result = await api.authSessions(controller.signal);
      if (!alive.current || read.current !== entry) return;
      if (controller.signal.aborted) throw new Error("Session request timed out.");
      validateSessions(result);
      setSessions(result.sessions);
      if (!preserveError) setError("");
    } catch (reason) {
      if (alive.current && read.current === entry) setError(controller.signal.aborted ? t("Session request timed out.") : failure(reason));
    } finally {
      window.clearTimeout(entry.timer);
      if (read.current === entry) {
        read.current = null;
        if (alive.current) setLoading(false);
      }
    }
  }, [backendOnline]);

  useEffect(() => {
    alive.current = true;
    setBusy(false);
    setLoading(false);
    void load();
    const changed = () => {
      cancelRead();
      setLoading(false);
      if (visible()) void load();
    };
    document.addEventListener("visibilitychange", changed);
    return () => {
      alive.current = false;
      cancelRead();
      action.current?.controller.abort();
      window.clearTimeout(action.current?.timer);
      action.current = null;
      document.removeEventListener("visibilitychange", changed);
    };
  }, [cancelRead, load]);

  const mutate = async (operation: (signal: AbortSignal) => Promise<unknown>, signedOut = false, renamed = false) => {
    if (!backendOnline || action.current) return;
    cancelRead();
    setLoading(false);
    const controller = new AbortController();
    const entry = { controller, timer: window.setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS) };
    action.current = entry;
    setBusy(true);
    setError("");
    setNotice("");
    let failed = false;
    try {
      await operation(controller.signal);
      if (!alive.current || action.current !== entry) return;
      if (controller.signal.aborted) throw new Error("Session request timed out.");
      setEditing("");
      setNotice(renamed ? t("Session name updated.") : t("Selected sessions signed out."));
    } catch {
      failed = true;
      if (alive.current && action.current === entry) setError(t("Could not confirm the session change. Refresh to check its status."));
    } finally {
      window.clearTimeout(entry.timer);
      if (action.current === entry) {
        action.current = null;
        if (alive.current) {
          setBusy(false);
          // Query the result after a lost response; never replay the mutation.
          if (signedOut) await onSignedOut();
          else await load(failed);
        }
      }
    }
  };

  const revoke = (session: ManagedSession) => {
    const prompt = session.current
      ? t("Sign out this browser? All tabs sharing this login will need to sign in again.")
      : t("Sign out {name}? Any control or Bluetooth connection from that login will end.", { name: sessionLabel(session) });
    if (window.confirm(prompt)) void mutate((signal) => api.revokeSession(session.id, signal), session.current);
  };

  const revokeOthers = () => {
    if (window.confirm(t("Sign out all other logins for this account? Their control and Bluetooth connections will end, and Stop will be requested when needed."))) {
      void mutate((signal) => api.revokeOtherSessions(signal));
    }
  };

  const rename = (event: FormEvent) => {
    event.preventDefault();
    if ([...name.trim()].length > 80) { setError(t("Session names can contain at most 80 characters.")); return; }
    void mutate((signal) => api.renameSession(editing, name.trim(), signal), false, true);
  };

  return <section className="group session-management" aria-labelledby="session-management-title" aria-busy={loading || busy || undefined}>
    <div className="access-section-header">
      <h3 id="session-management-title" className="group-title">{t("Your signed-in browsers")}</h3>
      <button type="button" className="btn btn-secondary" aria-label={t("Refresh sessions")} disabled={!backendOnline || loading || busy} onClick={() => void load()}>{t("Refresh")}</button>
    </div>
    <p className="hint-block">{t("A login may be shared by several tabs. Device names and browser hints help identify it; they are not proof of identity.")}</p>
    <p className="hint-block">{t("Signing out a controller or Bluetooth browser requests Stop.")}</p>
    {error && <p className="form-status auth-error" role="alert">{error}</p>}
    {notice && <p className="form-status" role="status">{notice}</p>}
    {loading && sessions.length === 0 && <p className="form-status" role="status">{t("Loading sessions…")}</p>}
    <ul className="session-list">
      {sessions.map((session) => <li key={session.id} className="session-row">
        <div className="session-summary">
          <div className="session-heading"><strong>{sessionLabel(session)}</strong>
          <span className="session-flags">
            {session.current && <span>{t("This browser")}</span>}
            {session.controller && <span>{t("Active controller")}</span>}
            {session.device_gateway && <span>{t("Bluetooth browser")}</span>}
          </span>
          </div>
          {session.name && <small>{clientLabel(session)}</small>}
          <div className="session-metadata">
          <small>{t("Last activity: {time}", { time: formatSessionTime(session.last_active_at) })}</small>
          <details className="session-details"><summary>{t("Session details")}</summary>
            <small>{t("Signed in: {time}", { time: formatSessionTime(session.created_at) })}</small>
            <small>{t("Idle sign-out: {time}", { time: formatSessionTime(session.idle_expires_at) })}</small>
            <small>{t("Sign-in expires: {time}", { time: formatSessionTime(session.expires_at) })}</small>
          </details>
          </div>
        </div>
        <div className="session-actions">
          <button type="button" className="btn btn-secondary" aria-label={t("Name this browser")} title={t("Name this browser")} disabled={!backendOnline || busy} onClick={() => { setEditing(session.id); setName(session.name); }}>{t("Name")}</button>
          <button type="button" className="btn btn-secondary" aria-label={session.current ? t("Sign out this browser") : t("Sign out session")} disabled={!backendOnline || busy} onClick={() => revoke(session)}>{t("Sign out")}</button>
        </div>
        {editing === session.id && <form className="session-name-form" onSubmit={rename}>
          <label className="field"><span className="label">{t("Device name")}</span><input type="text" maxLength={160} autoComplete="off" value={name} disabled={!backendOnline || busy} onChange={(event) => setName(event.target.value)} placeholder={clientLabel(session)} /></label>
          <button type="submit" className="btn btn-secondary" disabled={!backendOnline || busy}>{t("Save name")}</button>
          <button type="button" className="btn btn-secondary" disabled={busy} onClick={() => setEditing("")}>{t("Cancel")}</button>
        </form>}
      </li>)}
    </ul>
    <button type="button" className="btn btn-secondary" disabled={!backendOnline || busy || sessions.every((session) => session.current)} onClick={revokeOthers}>{t("Sign out other sessions")}</button>
  </section>;
}

function validateSessions(result: ManagedSessionsResponse) {
  if (!result || !Array.isArray(result.sessions) || result.sessions.length > 20 || !result.current_session_id ||
      result.sessions.filter((session) => session.current).length !== 1 ||
      new Set(result.sessions.map((session) => session.id)).size !== result.sessions.length ||
      result.sessions.some((session) => !/^[A-Za-z0-9_-]{22}$/.test(session.id) || typeof session.name !== "string" || !session.client ||
        typeof session.client.browser !== "string" || typeof session.client.platform !== "string" ||
        session.current !== (session.id === result.current_session_id))) throw new Error("Session management is unavailable.");
}

function sessionLabel(session: ManagedSession) { return session.name || clientLabel(session); }

function clientLabel(session: ManagedSession) {
  const browsers: Record<string, string> = { chrome: t("Chrome"), edge: t("Microsoft Edge"), firefox: t("Firefox"), safari: t("Safari") };
  const platforms: Record<string, string> = { windows: t("Windows"), macos: t("macOS"), linux: t("Linux"), android: t("Android"), ios: t("iOS") };
  return t("{browser} on {platform}", { browser: browsers[session.client.browser] || t("Other client"), platform: platforms[session.client.platform] || t("Unknown system") });
}

function formatSessionTime(value: string) {
  const date = new Date(value);
  return Number.isFinite(date.getTime()) ? date.toLocaleString() : t("Unknown");
}
