import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { CloudPlanningStatus } from "../api/cloud-types";
import { t } from "../i18n";
import { FieldRow } from "./SetupSection";

// The Decisions API key, edited as rows inside its External providers entry.
// It uses its own OpenAI key and billing; ChatGPT sign-in never authorizes it.
export function DecisionsConnection({ locked, onChange }: { locked: boolean; onChange: (status: CloudPlanningStatus) => void }) {
  const [decisionsKey, setDecisionsKey] = useState("");
  const [status, setStatus] = useState<CloudPlanningStatus | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const mounted = useRef(true);
  const busyRef = useRef(false);
  const onChangeRef = useRef(onChange); onChangeRef.current = onChange;
  const disabled = locked || busy;
  useEffect(() => {
    mounted.current = true;
    void api.cloudPlanningStatus().then(next => { if (mounted.current) setStatus(next); }).catch(() => {});
    return () => { mounted.current = false; };
  }, []);
  async function run(work: () => Promise<CloudPlanningStatus>) {
    if (locked || busyRef.current) return;
    busyRef.current = true; setBusy(true); setError(""); setMessage("");
    try { const next = await work(); if (mounted.current) { setStatus(next); onChangeRef.current(next); } return next; }
    catch (reason) { if (mounted.current) setError(reason instanceof Error ? reason.message : String(reason)); }
    finally { busyRef.current = false; if (mounted.current) setBusy(false); }
  }
  const keySet = Boolean(status?.connection.decisions_key_set);
  return <div className="model-connection-editor form-rows">
    <FieldRow id="decisions-key" label={t("Decisions API key")} stack hint={t("Decisions uses its own OpenAI API key and separate billing to choose complete enabled library patterns. ChatGPT sign-in does not authorize this billing path. Select Library motion and explicitly assign the Decisions role to use it.")}>
      <div className="form-input-action">
        <input id="decisions-key" aria-describedby="decisions-key-hint" type="password" autoComplete="off" value={decisionsKey} disabled={disabled} placeholder={keySet ? t("Saved key will be kept") : undefined} onChange={event => setDecisionsKey(event.target.value)} />
        <button className="btn btn-secondary" type="button" disabled={disabled || !decisionsKey.trim()} onClick={() => void run(async () => { const next = await api.cloudDecisionsKey(decisionsKey); if (mounted.current) setDecisionsKey(""); return next; })}>{t("Save API key")}</button>
      </div>
    </FieldRow>
    {keySet && <div className="form-row">
      <span className="form-row-label"><small role="status">{message || t("Check model uses a text-only request. Nothing moves.")}</small></span>
      <button className="btn btn-secondary" type="button" disabled={disabled} onClick={() => void run(async () => {
        const next = await api.cloudTest("decisions", "");
        if (mounted.current) { if (next.readiness.ready) setMessage(t("Ready")); else setError(next.readiness.message ?? t("Request failed")); }
        return next;
      })}>{t("Test Decisions connection")}</button>
    </div>}
    {error && <p className="form-status form-status-error" role="alert">{error}</p>}
  </div>;
}
