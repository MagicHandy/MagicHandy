import { useEffect, useState } from "react";
import { api } from "../api/client";
import { useAppState, useToast } from "../state/app-state";

export function VoiceQuickControls() {
  const { backendOnline, readOnly, state, refresh } = useAppState();
  const { show } = useToast();
  const provider = state?.settings?.voice?.tts_provider;
  const configured = Boolean(provider && provider !== "none");
  const authoritative = state?.settings?.voice?.speak_replies ?? false;
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
    <label className="toggle-line voice-quick-toggle">
      <span className="toggle"><input type="checkbox" checked={enabled} disabled={!backendOnline || readOnly || saving} onChange={(event) => void change(event.target.checked)} /><span className="track" aria-hidden="true" /></span>
      <span>Speak replies</span>
    </label>
  );
}
