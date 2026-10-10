import { DismissibleNotice } from "./DismissibleNotice";
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { CloudPlanningStatus } from "../api/cloud-types";
import { t } from "../i18n";

export function DecisionsConnection({ locked, onReadyChange }: { locked: boolean; onReadyChange: (ready: boolean) => void }) {
  const [decisionsKey, setDecisionsKey] = useState("");
  const [status, setStatus] = useState<CloudPlanningStatus | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const mounted = useRef(true);
  const busyRef = useRef(false);
  const controlsLocked = locked || busy;
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  async function run(work: () => Promise<void>) {
    if (locked || busyRef.current) return;
    busyRef.current = true; setBusy(true); setError("");
    try { await work(); } catch (reason) { if (mounted.current) setError(reason instanceof Error ? reason.message : String(reason)); }
    finally { busyRef.current = false; if (mounted.current) setBusy(false); }
  }
  useEffect(() => { if (status) onReadyChange(status.connection.decisions_key_set); }, [status, onReadyChange]);
  return <>
    <details className="group cloud-motion-settings" onToggle={event => { if (event.currentTarget.open) void run(async () => { const next = await api.cloudPlanningStatus(); if (mounted.current) setStatus(next); }); }}><summary>{t("Optional Decisions API")}</summary>
      <DismissibleNotice id="model-decisions" className="model-help"><p className="hint">{t("Decisions uses its own OpenAI API key and separate billing to choose complete enabled library patterns. ChatGPT sign-in does not authorize this billing path. Select Library motion and explicitly assign the Decisions role to use it.")}</p></DismissibleNotice>
      <label className="field"><span className="label">{t("Decisions API key")}</span><input type="password" autoComplete="off" value={decisionsKey} disabled={controlsLocked} onChange={event => setDecisionsKey(event.target.value)} /></label>
      <div className="button-row">
        <button className="btn btn-secondary" type="button" disabled={controlsLocked || !decisionsKey.trim()} onClick={() => void run(async () => { const next = await api.cloudDecisionsKey(decisionsKey); if (mounted.current) { setStatus(next); setDecisionsKey(""); } })}>{t("Save API key")}</button>
        {status?.connection.decisions_key_set && <><button className="btn btn-secondary" type="button" disabled={controlsLocked} onClick={() => void run(async () => { const next = await api.cloudDecisionsKey(""); if (mounted.current) setStatus(next); })}>{t("Remove API key")}</button>
          <button className="btn btn-secondary" type="button" disabled={controlsLocked} onClick={() => void run(async () => { const next = await api.cloudTest("decisions", ""); if (mounted.current) { setStatus(next); if (!next.readiness.ready) setError(next.readiness.message ?? t("Request failed")); } })}>{t("Test Decisions connection")}</button></>}
      </div>
    </details>
    {error && <p className="error-text" role="alert">{error}</p>}
  </>;
}
