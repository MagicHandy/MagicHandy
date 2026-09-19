import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { ControlGrant } from "../api/types";
import { t } from "../i18n";

export function ControlGrantPanel({ accountID, disabled }: { accountID: string; disabled: boolean }) {
  const [grant, setGrant] = useState<ControlGrant | null>(null);
  const [minutes, setMinutes] = useState(60);
  const [busy, setBusy] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    let active = true;
    const controller = new AbortController();
    setBusy(true); setLoaded(false); setError(""); setGrant(null);
    const timeout = window.setTimeout(() => {
      if (!active) return;
      active = false; controller.abort(); setBusy(false); setError(t("Control permission is unavailable. Reopen this panel to retry."));
    }, 10_000);
    void api.controlGrant(accountID, controller.signal).then((result) => { if (active) { setGrant(result.grant); setLoaded(true); } })
      .catch((reason: unknown) => { if (active) setError(reason instanceof Error ? reason.message : t("Request failed")); })
      .finally(() => { window.clearTimeout(timeout); if (active) setBusy(false); });
    return () => { active = false; window.clearTimeout(timeout); controller.abort(); };
  }, [accountID]);
  const change = async (revoke: boolean) => {
    if (!loaded || busy) return;
    setBusy(true); setError("");
    try {
      const result = revoke ? await api.revokeControl(accountID) : await api.grantControl(accountID, minutes);
      setGrant(result.grant);
    } catch (reason) { setError(reason instanceof Error ? reason.message : t("Request failed")); }
    finally { setBusy(false); }
  };
  return <div className="account-reset-form account-control-permission">
    <p className="hint-block">{loaded && (grant ? t("Control permission expires {time}.", { time: new Date(grant.expires_at).toLocaleString() }) : t("Observer access. This account can view shared content and use Stop."))} {t("Control permission allows motion, chat and synchronized playback. Host settings and module installation remain administrator-only.")}</p>
    {busy && !loaded && <p className="form-status" role="status">{t("Checking…")}</p>}
    <label className="field"><span className="label">{t("Control permission duration")}</span><select value={minutes} disabled={busy || disabled || !loaded} onChange={(event) => setMinutes(Number(event.target.value))}><option value={15}>{t("15 minutes")}</option><option value={60}>{t("1 hour")}</option><option value={240}>{t("4 hours")}</option><option value={720}>{t("12 hours")}</option></select></label>
    <button className="btn btn-secondary" type="button" disabled={busy || disabled || !loaded} onClick={() => void change(false)}>{grant ? t("Replace control permission") : t("Grant control permission")}</button>
    {grant && <button className="btn btn-secondary" type="button" disabled={busy || !loaded} onClick={() => void change(true)}>{t("Revoke control permission")}</button>}
    {error && <p className="form-status auth-error" role="alert">{error}</p>}
  </div>;
}
