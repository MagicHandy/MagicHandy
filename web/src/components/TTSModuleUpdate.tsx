import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { VoiceModuleUpdate } from "../api/types";
import { t, translateKnown } from "../i18n";

interface Props {
  update?: VoiceModuleUpdate;
  locked: boolean;
  refresh: () => Promise<void>;
  onComplete: () => void;
}

export function TTSModuleUpdate({ update, locked, refresh, onComplete }: Props) {
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const completed = useRef("");
  const completeCallback = useRef(onComplete);
  completeCallback.current = onComplete;
  const job = update?.job;
  const active = job?.status === "queued" || job?.status === "running";

  useEffect(() => {
    if (job?.status === "complete" && completed.current !== job.id) {
      completed.current = job.id;
      completeCallback.current();
    }
  }, [job?.id, job?.status]);

  async function run(action: () => Promise<unknown>) {
    setPending(true);
    setError("");
    try {
      await action();
    } catch (reason) {
      setError(reason instanceof Error ? translateKnown(reason.message) : t("Request failed"));
    } finally {
      try {
        await refresh();
      } finally {
        setPending(false);
      }
    }
  }

  if (!update?.available && !job) return null;
  return <div className="hint-block" role="status" aria-live="polite" aria-busy={active || pending || undefined}>
    {update?.available && <>
      <strong>{t("TTS module update available")}</strong>
      <p>{t("Update the managed speech module to use the improvements included with this MagicHandy version.")}</p>
      <p>{t("The update may download several GiB and needs space for a separate runtime. Your model, voice and reference settings are preserved. Speech may reload when the update is applied.")}</p>
    </>}
    {job && <p className="form-status">{job.status === "complete" ? t("TTS module update completed.") : translateKnown(job.message)}</p>}
    {error && <p className="form-status" role="alert">{error}</p>}
    {update?.available && !update.supported && <p>{t("TTS module updates are supported on Windows x64.")}</p>}
    <div className="row-actions">
      {active && <button type="button" className="btn btn-secondary" disabled={locked || pending} onClick={() => void run(() => api.cancelTTSModuleUpdate(job.id))}>{t("Cancel")}</button>}
      {update?.available && <button type="button" className="btn btn-secondary" disabled={locked || pending || update.busy || !update.supported || !update.id} onClick={() => void run(() => api.updateTTSModule(update.id!))}>{t("Update TTS module")}</button>}
    </div>
  </div>;
}
