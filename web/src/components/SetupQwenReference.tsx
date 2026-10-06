import { useEffect, useId, useState } from "react";
import { api } from "../api/client";
import { t, translateKnown } from "../i18n";
import { HostPathField } from "./HostPathField";

export interface SetupQwenReferenceProps {
  wav: string;
  transcript: string;
  later: boolean;
  locked: boolean;
  setWAV: (path: string) => void;
  setTranscript: (text: string) => void;
  setLater: (later: boolean) => void;
}

// Reuse the host WAV picker. No microphone, upload or automatic transcription.
export function SetupQwenReference({ wav, transcript, later, locked, setWAV, setTranscript, setLater }: SetupQwenReferenceProps) {
  const transcriptID = useId();
  const [duration, setDuration] = useState(0);
  const [error, setError] = useState("");
  useEffect(() => {
    setDuration(0); setError("");
    if (later || !wav.trim()) return;
    const abort = new AbortController();
    const timer = window.setTimeout(() => {
      void api.checkSetupQwenReference(wav.trim(), abort.signal).then((result) => {
        if (!abort.signal.aborted) setDuration(result.duration_ms);
      }).catch((reason: unknown) => {
        if (!abort.signal.aborted) setError(reason instanceof Error ? translateKnown(reason.message) : t("Request failed"));
      });
    }, 300);
    return () => { window.clearTimeout(timer); abort.abort(); };
  }, [later, wav]);
  return <section className="setup-subsection" aria-label={t("Set up your Qwen voice")}>
    <h3>{t("Set up your Qwen voice")}</h3>
    <p>{t("Qwen3-TTS requires an audio sample and its exact transcript before it can speak. Choose one person speaking clearly, ideally 3 to 10 seconds, without music or background noise.")}</p>
    {later ? <>
      <p>{t("Qwen3-TTS will be installed, but spoken replies will stay off until you add the sample and transcript in Settings > Voice.")}</p>
      <button type="button" className="btn btn-quiet" disabled={locked} onClick={() => setLater(false)}>{t("Configure voice now")}</button>
    </> : <>
      <HostPathField label={t("Voice sample (WAV)")} kind="wav" value={wav} disabled={locked} onChange={setWAV} />
      {duration > 0 && <p className="hint-block">{t("Sample checked: {seconds} seconds. This checks the file and length, not the spoken words.", { seconds: (duration / 1000).toFixed(1) })}</p>}
      {error && <p className="form-status form-status-error" role="alert">{error}</p>}
      <div className="field"><label className="label" htmlFor={transcriptID}>{t("Exact words spoken in the sample")}</label><textarea id={transcriptID} aria-describedby={`${transcriptID}-hint`} rows={3} value={transcript} disabled={locked} onChange={(event) => setTranscript(event.target.value)} /><span id={`${transcriptID}-hint`} className="hint">{t("Type every word exactly as recorded. A mismatch can make the cloned voice sound wrong.")}</span></div>
      <button type="button" className="btn btn-quiet" disabled={locked} onClick={() => setLater(true)}>{t("Set up voice later")}</button>
    </>}
  </section>;
}
