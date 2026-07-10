import type { Dispatch, SetStateAction } from "react";
import type { PublicSettings } from "../api/types";
import { VoiceWorkers } from "./VoiceWorkers";

const joinArgs = (args?: string[]) => (args ?? []).join("\n");
const splitArgs = (value: string) => value.split(/\r?\n/).map((arg) => arg.trim()).filter(Boolean);

const PROVIDER_LABELS: Record<string, string> = {
  none: "None",
  parakeet_managed: "Parakeet (managed, local)",
  openai_compatible: "OpenAI-compatible server",
  elevenlabs: "ElevenLabs (cloud)",
  neutts_air: "NeuTTS Air (local)",
  custom: "Custom worker",
};

interface Props {
  settings: PublicSettings;
  locked: boolean;
  // Unsaved voice edits: worker controls act on the saved config, so they
  // lock until the form is saved.
  dirty: boolean;
  patch: (next: Partial<PublicSettings["voice"]>) => void;
  newKey: string;
  setNewKey: Dispatch<SetStateAction<string>>;
  clearKey: boolean;
  setClearKey: Dispatch<SetStateAction<boolean>>;
}

export function VoiceSettingsPanel({ settings: s, locked, dirty, patch, newKey, setNewKey, clearKey, setClearKey }: Props) {
  const voice = s.voice;
  const providerSelect = (value: string, options: string[] | undefined, onChange: (value: string) => void) => (
    <select value={value} disabled={locked} onChange={(event) => onChange(event.target.value)}>
      {(options?.length ? options : [value]).map((option) => <option key={option} value={option}>{PROVIDER_LABELS[option] ?? option}</option>)}
    </select>
  );

  return (
    <>
      <h2 className="section-title">Voice</h2>
      <label className="toggle-line hint-block"><span className="toggle"><input type="checkbox" checked={voice.enabled} disabled={locked} onChange={(event) => patch({ enabled: event.target.checked })} /><span className="track" aria-hidden="true" /></span><span>Enable voice workers</span></label>

      <div className="divider" />
      <h3 className="group-title">Speech input (ASR)</h3>
      <label className="field"><span className="label">Provider</span>{providerSelect(voice.asr_provider, s.options.asr_providers, (asr_provider) => patch({ asr_provider }))}</label>
      {voice.asr_provider === "parakeet_managed" && <>
        <label className="field"><span className="label">parakeet-server path</span><input type="text" value={voice.parakeet_server_path ?? ""} disabled={locked} onChange={(event) => patch({ parakeet_server_path: event.target.value })} /></label>
        <label className="field"><span className="label">GGUF model path</span><input type="text" value={voice.parakeet_model_path ?? ""} disabled={locked} onChange={(event) => patch({ parakeet_model_path: event.target.value })} /></label>
        <label className="field"><span className="label">Server port</span><input type="number" min={1} max={65535} value={voice.parakeet_port ?? 8990} disabled={locked} onChange={(event) => patch({ parakeet_port: Number(event.target.value) })} /></label>
      </>}
      {voice.asr_provider === "openai_compatible" && <>
        <label className="field"><span className="label">Base URL</span><input type="url" value={voice.asr_base_url ?? ""} disabled={locked} onChange={(event) => patch({ asr_base_url: event.target.value })} /></label>
        <label className="field"><span className="label">Model name</span><input type="text" value={voice.asr_model ?? ""} disabled={locked} onChange={(event) => patch({ asr_model: event.target.value })} /></label>
      </>}
      {voice.asr_provider === "custom" && <>
        <label className="field"><span className="label">Worker path</span><input type="text" value={voice.asr_worker_path ?? ""} disabled={locked} onChange={(event) => patch({ asr_worker_path: event.target.value })} /></label>
        <label className="field"><span className="label">Worker arguments</span><textarea rows={4} value={joinArgs(voice.asr_worker_args)} disabled={locked} onChange={(event) => patch({ asr_worker_args: splitArgs(event.target.value) })} /></label>
      </>}
      {voice.asr_provider !== "none" && voice.asr_provider !== "custom" && <details className="advanced-fields"><summary>Advanced</summary><label className="field"><span className="label">Worker binary override</span><input type="text" value={voice.asr_worker_path ?? ""} disabled={locked} onChange={(event) => patch({ asr_worker_path: event.target.value })} /></label></details>}
      <VoiceWorkers locked={locked} role="asr" dirty={dirty} />

      <div className="divider" />
      <h3 className="group-title">Speech output (TTS)</h3>
      <label className="field"><span className="label">Provider</span>{providerSelect(voice.tts_provider, s.options.tts_providers, (tts_provider) => patch({ tts_provider }))}</label>
      {voice.tts_provider === "elevenlabs" && <>
        <label className="field"><span className="label">API key {voice.elevenlabs_key_set && <span className="badge">set</span>}</span><input type="password" autoComplete="off" placeholder={voice.elevenlabs_key_set ? "set (leave blank to keep)" : "Paste API key"} value={newKey} disabled={locked} onChange={(event) => setNewKey(event.target.value)} /></label>
        <label className="toggle-line hint-block"><span className="toggle"><input type="checkbox" checked={clearKey} disabled={locked} onChange={(event) => setClearKey(event.target.checked)} /><span className="track" aria-hidden="true" /></span><span>Clear API key on save</span></label>
        <label className="field"><span className="label">Voice ID</span><input type="text" value={voice.elevenlabs_voice_id ?? ""} disabled={locked} onChange={(event) => patch({ elevenlabs_voice_id: event.target.value })} /></label>
        <label className="field"><span className="label">Model ID</span><input type="text" value={voice.elevenlabs_model_id ?? ""} disabled={locked} onChange={(event) => patch({ elevenlabs_model_id: event.target.value })} /></label>
      </>}
      {voice.tts_provider === "neutts_air" && <>
        <label className="field"><span className="label">stream_pcm runner path</span><input type="text" value={voice.neutts_runner_path ?? ""} disabled={locked} onChange={(event) => patch({ neutts_runner_path: event.target.value })} /></label>
        <label className="field"><span className="label">Reference WAV</span><input type="text" value={voice.neutts_reference_wav ?? ""} disabled={locked} onChange={(event) => patch({ neutts_reference_wav: event.target.value })} /></label>
        <label className="field"><span className="label">Pre-encoded reference codes (.npy)</span><input type="text" value={voice.neutts_reference_codes ?? ""} disabled={locked} onChange={(event) => patch({ neutts_reference_codes: event.target.value })} /></label>
        <label className="field"><span className="label">Reference transcript</span><textarea rows={3} value={voice.neutts_reference_text ?? ""} disabled={locked} onChange={(event) => patch({ neutts_reference_text: event.target.value })} /></label>
        <p className="form-status">The current non-Python runner requires pre-encoded voice codes. The WAV is retained as provenance and is not encoded by MagicHandy.</p>
      </>}
      {voice.tts_provider === "custom" && <>
        <label className="field"><span className="label">Worker path</span><input type="text" value={voice.tts_worker_path ?? ""} disabled={locked} onChange={(event) => patch({ tts_worker_path: event.target.value })} /></label>
        <label className="field"><span className="label">Worker arguments</span><textarea rows={4} value={joinArgs(voice.tts_worker_args)} disabled={locked} onChange={(event) => patch({ tts_worker_args: splitArgs(event.target.value) })} /></label>
      </>}
      {voice.tts_provider !== "none" && voice.tts_provider !== "custom" && <details className="advanced-fields"><summary>Advanced</summary><label className="field"><span className="label">Worker binary override</span><input type="text" value={voice.tts_worker_path ?? ""} disabled={locked} onChange={(event) => patch({ tts_worker_path: event.target.value })} /></label></details>}
      {voice.tts_provider !== "none" && <label className="toggle-line hint-block"><span className="toggle"><input type="checkbox" checked={voice.speak_replies ?? false} disabled={locked} onChange={(event) => patch({ speak_replies: event.target.checked })} /><span className="track" aria-hidden="true" /></span><span>Speak chat replies</span></label>}
      <VoiceWorkers locked={locked} role="tts" dirty={dirty} />
    </>
  );
}
