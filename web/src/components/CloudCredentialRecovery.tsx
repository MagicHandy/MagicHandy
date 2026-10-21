import { useRef, useState } from "react";
import { api } from "../api/client";
import type { CloudPlanningStatus } from "../api/cloud-types";
import { t } from "../i18n";

// Rendered only when the backend marks the store unreadable. Recovery applies
// to every cloud credential on this host, so its scope needs explicit consent.
export function CloudCredentialRecovery({ locked, onReset }: {
  locked: boolean; onReset: (status: CloudPlanningStatus) => void;
}) {
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const busyRef = useRef(false);
  async function reset() {
    if (locked || busyRef.current || !confirm) return;
    busyRef.current = true; setBusy(true); setError("");
    try { onReset(await api.cloudResetCredentials()); }
    catch (reason) { setError(reason instanceof Error ? reason.message : String(reason)); }
    finally { busyRef.current = false; setBusy(false); }
  }
  return <div className="notice" role="group" aria-label={t("Cloud credential recovery")}>
    <strong>{t("Saved cloud credentials cannot be opened")}</strong>
    <p>{t("This can happen after moving app data to another Windows account or computer. Reset saved credentials, then sign in again or re-enter your API keys.")}</p>
    {confirm && <p role="status">{t("This removes all saved ChatGPT sign-ins and cloud API keys on this app host. Provider and model settings stay. Remote access is not revoked; manage it with each provider if needed.")}</p>}
    <div className="button-row">
      {confirm ? <>
        <button type="button" className="btn btn-secondary" disabled={locked || busy} onClick={() => void reset()}>{t("Reset all saved cloud credentials")}</button>
        <button type="button" className="btn btn-quiet" disabled={busy} onClick={() => setConfirm(false)}>{t("Cancel")}</button>
      </> : <button type="button" className="btn btn-secondary" disabled={locked} onClick={() => setConfirm(true)}>{t("Reset saved credentials…")}</button>}
    </div>
    {error && <p className="form-status form-status-error" role="alert">{error}</p>}
  </div>;
}
