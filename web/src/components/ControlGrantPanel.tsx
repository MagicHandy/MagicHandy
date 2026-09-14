import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { ControlGrant } from "../api/types";
import { t } from "../i18n";

export function ControlGrantPanel({ accountID, disabled }: { accountID: string; disabled: boolean }) {
  const [grant, setGrant] = useState<ControlGrant | null>(null);
  const [minutes, setMinutes] = useState(60);
  const [busy, setBusy] = useState(true);
  const [error, setError] = useState("");
  useEffect(() => {
    let active = true;
    void api.controlGrant(accountID).then((result) => { if (active) setGrant(result.grant); })
      .catch((reason: unknown) => { if (active) setError(reason instanceof Error ? reason.message : t("Request failed")); })
      .finally(() => { if (active) setBusy(false); });
    return () => { active = false; };
  }, [accountID]);
  const change = async (revoke: boolean) => {
    setBusy(true); setError("");
    try {
      const result = revoke ? await api.revokeControl(accountID) : await api.grantControl(accountID, minutes);
      setGrant(result.grant);
    } catch (reason) { setError(reason instanceof Error ? reason.message : t("Request failed")); }
    finally { setBusy(false); }
  };
  return <div className="account-reset-form account-control-permission">
    <p className="hint-block">{grant ? t("Control permission expires {time}.", { time: new Date(grant.expires_at).toLocaleString() }) : t("Observer access. This account can view shared content and use Stop.")} {t("Control permission allows motion, chat and synchronized playback. Host settings and module installation remain administrator-only.")}</p>
    <label className="field"><span className="label">{t("Control permission duration")}</span><select value={minutes} disabled={busy || disabled} onChange={(event) => setMinutes(Number(event.target.value))}><option value={15}>{t("15 minutes")}</option><option value={60}>{t("1 hour")}</option><option value={240}>{t("4 hours")}</option><option value={720}>{t("12 hours")}</option></select></label>
    <button className="btn btn-secondary" type="button" disabled={busy || disabled} onClick={() => void change(false)}>{grant ? t("Replace control permission") : t("Grant control permission")}</button>
    {grant && <button className="btn btn-secondary" type="button" disabled={busy} onClick={() => void change(true)}>{t("Revoke control permission")}</button>}
    {error && <p className="form-status auth-error" role="alert">{error}</p>}
  </div>;
}
