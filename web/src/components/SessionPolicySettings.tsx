import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { SessionPolicy } from "../api/access-types";
import { t } from "../i18n";

const choices = [0, 30, 60, 120, 240, 480];
const validPolicy = (value: SessionPolicy) => value && choices.includes(value.idle_minutes) && value.lifetime_minutes > 0;

export function SessionPolicySettings({ backendOnline, onChanged }: { backendOnline: boolean; onChanged: () => void }) {
  const [policy, setPolicy] = useState<SessionPolicy | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const active = useRef<AbortController | null>(null);
  const alive = useRef(false);
  const changed = useRef(onChanged);
  changed.current = onChanged;

  const run = useCallback(async (minutes?: number) => {
    active.current?.abort();
    const controller = new AbortController();
    active.current = controller;
    const timeout = window.setTimeout(() => controller.abort(), 10_000);
    setBusy(true);
    setError("");
    try {
      const value = minutes === undefined ? await api.sessionPolicy(controller.signal) : await api.saveSessionPolicy(minutes, controller.signal);
      if (!alive.current || active.current !== controller) return;
      if (controller.signal.aborted || !validPolicy(value)) throw new Error("Invalid session policy");
      setPolicy(value);
      if (minutes !== undefined) changed.current();
    } catch {
      if (alive.current && active.current === controller) {
        setError(minutes === undefined ? t("Could not load idle sign-out settings.") : t("Could not confirm the change. Refresh to check the saved setting."));
      }
    } finally {
      window.clearTimeout(timeout);
      if (active.current === controller) {
        active.current = null;
        if (alive.current) setBusy(false);
      }
    }
  }, []);

  useEffect(() => {
    alive.current = true;
    if (backendOnline) void run();
    return () => { alive.current = false; active.current?.abort(); active.current = null; };
  }, [backendOnline, run]);

  return <section className="group" aria-labelledby="session-policy-title">
    <div className="access-section-header">
      <h3 id="session-policy-title" className="group-title">{t("Automatic sign-out")}</h3>
      <button type="button" className="btn btn-secondary" disabled={!backendOnline || busy} onClick={() => void run()}>{t("Refresh")}</button>
    </div>
    <label className="field"><span className="label">{t("Sign out after inactivity")}</span>
      <select value={policy?.idle_minutes ?? ""} disabled={!backendOnline || busy || !policy} onChange={(event) => void run(Number(event.target.value))}>
        {!policy && <option value="">{t("Loading…")}</option>}
        <option value={30}>{t("30 minutes")}</option>
        <option value={60}>{t("1 hour")}</option>
        <option value={120}>{t("2 hours")}</option>
        <option value={240}>{t("4 hours")}</option>
        <option value={480}>{t("8 hours (default)")}</option>
        <option value={0}>{t("Off")}</option>
      </select>
    </label>
    <p className="hint-block">{t("Applies immediately to all active logins. Viewing the page or playing motion does not count as activity.")}</p>
    {policy && <p className="hint-block">{t("Off disables inactivity sign-out. Logins still expire {hours} hours after sign-in. Losing the controller connection still stops motion.", { hours: policy.lifetime_minutes / 60 })}</p>}
    {error && <p className="form-status auth-error" role="alert">{error}</p>}
  </section>;
}
