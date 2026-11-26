import { t, translateKnown } from "../i18n";
import { useEffect, useState } from "react";
import { api } from "../api/client";
import { useAppState, useToast } from "../state/app-state";

export function VoiceQuickControls() {
  const { backendOnline, readOnly, state, refresh } = useAppState();
  const { show } = useToast();
  const voice = state?.settings?.voice;
  // The toggle earns its place only when flipping it can have an effect:
  // voice workers enabled and a speech-output provider selected.
  const configured = Boolean(voice?.enabled && voice.tts_provider && voice.tts_provider !== "none");
  const authoritative = voice?.speak_replies ?? false;
  const ttsWorker = state?.voice?.workers?.tts;
  const ttsLoading = ttsWorker?.state === "starting" || ttsWorker?.model_state === "loading";
  const ttsReady = ttsWorker?.state === "running" && ttsWorker?.model_state === "ready";
  // A failure stays on screen with its reason instead of a toast that fades.
  const ttsFailure = !ttsReady && !ttsLoading && ttsWorker?.last_error && (ttsWorker.state === "crashed" || ttsWorker.state === "running" || ttsWorker.state === "stopped")
    ? translateKnown(ttsWorker.last_error)
    : "";
  const [enabled, setEnabled] = useState(authoritative);
  const [saving, setSaving] = useState(false);

  useEffect(() => setEnabled(authoritative), [authoritative]);
  if (!configured) return null;

  async function change(next: boolean) {
    setEnabled(next);
    setSaving(true);
    try {
      await api.saveVoicePreferences(next);
      refresh();
    } catch (error) {
      setEnabled(!next);
      show(error instanceof Error ? translateKnown(error.message) : t("Voice preference failed."), "error");
    } finally {
      setSaving(false);
    }
  }

  return (
    <>
      <label className="toggle-line voice-quick-toggle">
        <span className="toggle"><input type="checkbox" checked={enabled} disabled={!backendOnline || readOnly || saving} onChange={(event) => void change(event.target.checked)} /><span className="track" aria-hidden="true" /></span>
        <span>{t("Speak replies")}</span>
      </label>
      {enabled && ttsReady && (
        <span className="status-readout voice-quick-warning" role="status">
          <span className="status-dot" data-state="ok" />
          <span className="status-text">{t("Voice ready")}</span>
        </span>
      )}
      {enabled && !ttsReady && ttsFailure && (
        <span className="status-readout voice-quick-warning voice-quick-failure" role="alert">
          <span className="status-dot" data-state="error" />
          <span className="status-text">{t("Replies aren't being spoken: {reason}", { reason: ttsFailure.length > 220 ? `${ttsFailure.slice(0, 220)}…` : ttsFailure })} <a href="#/settings/voice">{t("Fix in Settings › Voice")}</a></span>
        </span>
      )}
      {enabled && !ttsReady && !ttsFailure && (
        <span className="status-readout voice-quick-warning" role="status">
          <span className="status-dot" data-state={ttsLoading ? "active" : "warn"} />
          <span className="status-text">{ttsLoading ? t("Starting voice…") : t("Voice output is not ready — start it in Settings → Voice")}</span>
        </span>
      )}
    </>
  );
}
