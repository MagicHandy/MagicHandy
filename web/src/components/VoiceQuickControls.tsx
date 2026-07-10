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
  const ttsReady = ttsWorker?.state === "running" && ttsWorker?.model_state === "ready";
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
      show(error instanceof Error ? error.message : "Voice preference failed.", "error");
    } finally {
      setSaving(false);
    }
  }

  return (
    <>
      <label className="toggle-line voice-quick-toggle">
        <span className="toggle"><input type="checkbox" checked={enabled} disabled={!backendOnline || readOnly || saving} onChange={(event) => void change(event.target.checked)} /><span className="track" aria-hidden="true" /></span>
        <span>Speak replies</span>
      </label>
      {enabled && !ttsReady && (
        <span className="status-readout voice-quick-warning">
          <span className="status-dot" data-state="warn" />
          <span className="status-text">Voice output is not ready — start it in Settings → Voice</span>
        </span>
      )}
    </>
  );
}
