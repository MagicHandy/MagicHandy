import type { Dispatch, SetStateAction } from "react";
import type { PublicSettings } from "../api/types";
import { HostPathField } from "./HostPathField";
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

const PARAKEET_SOURCE_LABELS: Record<string, string> = {
  app_managed: "MagicHandy module",
  custom_local: "Custom local server",
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
  const parakeetSource = voice.parakeet_source || "app_managed";
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
        <label className="field"><span className="label">Runtime source</span><select value={parakeetSource} disabled={locked} onChange={(event) => patch({ parakeet_source: event.target.value })}>{(s.options.parakeet_sources?.length ? s.options.parakeet_sources : [parakeetSource]).map((source) => <option key={source} value={source}>{PARAKEET_SOURCE_LABELS[source] ?? source}</option>)}</select></label>
        {parakeetSource === "app_managed" && <p className="form-status">Uses the worker, runner, and model installed by MagicHandy. No custom paths are required.</p>}
        {parakeetSource === "custom_local" && <>
          <HostPathField label="Custom parakeet-server path" kind="executable" value={voice.parakeet_server_path ?? ""} disabled={locked} onChange={(parakeet_server_path) => patch({ parakeet_server_path })} />
          <HostPathField label="Custom GGUF model path" kind="gguf" value={voice.parakeet_model_path ?? ""} disabled={locked} onChange={(parakeet_model_path) => patch({ parakeet_model_path })} />
          <label className="field"><span className="label">Server port</span><input type="number" min={1} max={65535} value={voice.parakeet_port ?? 8990} disabled={locked} onChange={(event) => patch({ parakeet_port: Number(event.target.value) })} /></label>
        </>}
      </>}
      {voice.asr_provider === "openai_compatible" && <>
        <label className="field"><span className="label">Base URL</span><input type="url" value={voice.asr_base_url ?? ""} disabled={locked} onChange={(event) => patch({ asr_base_url: event.target.value })} /></label>
        <label className="field"><span className="label">Model name</span><input type="text" value={voice.asr_model ?? ""} disabled={locked} onChange={(event) => patch({ asr_model: event.target.value })} /></label>
      </>}
      {voice.asr_provider === "custom" && <>
        <HostPathField label="ASR worker path" kind="file" value={voice.asr_worker_path ?? ""} disabled={locked} onChange={(asr_worker_path) => patch({ asr_worker_path })} />
        <label className="field"><span className="label">Worker arguments</span><textarea rows={4} value={joinArgs(voice.asr_worker_args)} disabled={locked} onChange={(event) => patch({ asr_worker_args: splitArgs(event.target.value) })} /></label>
      </>}
      {voice.asr_provider !== "none" && voice.asr_provider !== "custom" && <details className="advanced-fields"><summary>Advanced</summary>{voice.asr_provider === "parakeet_managed" && parakeetSource === "app_managed" && <label className="field"><span className="label">Server port</span><input type="number" min={1} max={65535} value={voice.parakeet_port ?? 8990} disabled={locked} onChange={(event) => patch({ parakeet_port: Number(event.target.value) })} /></label>}<HostPathField label="ASR worker binary override" kind="file" value={voice.asr_worker_path ?? ""} disabled={locked} onChange={(asr_worker_path) => patch({ asr_worker_path })} /></details>}
      <VoiceWorkers locked={locked} role="asr" dirty={dirty} enabled={voice.enabled} providerSelected={voice.asr_provider !== "none"} showParakeetModule={voice.asr_provider === "parakeet_managed" && parakeetSource === "app_managed"} />

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
        <HostPathField label="stream_pcm runner path" kind="executable" value={voice.neutts_runner_path ?? ""} disabled={locked} onChange={(neutts_runner_path) => patch({ neutts_runner_path })} />
        <HostPathField label="Reference WAV" kind="wav" value={voice.neutts_reference_wav ?? ""} disabled={locked} onChange={(neutts_reference_wav) => patch({ neutts_reference_wav })} />
        <HostPathField label="Pre-encoded reference codes (.npy)" kind="npy" value={voice.neutts_reference_codes ?? ""} disabled={locked} onChange={(neutts_reference_codes) => patch({ neutts_reference_codes })} />
        <label className="field"><span className="label">Reference transcript</span><textarea rows={3} value={voice.neutts_reference_text ?? ""} disabled={locked} onChange={(event) => patch({ neutts_reference_text: event.target.value })} /></label>
        <p className="form-status">Manual runtime setup is required: the source installer builds the MagicHandy adapter, but not the external stream_pcm runner or model assets. The WAV is retained as provenance and is not encoded by MagicHandy.</p>
      </>}
      {voice.tts_provider === "custom" && <>
        <HostPathField label="TTS worker path" kind="file" value={voice.tts_worker_path ?? ""} disabled={locked} onChange={(tts_worker_path) => patch({ tts_worker_path })} />
        <label className="field"><span className="label">Worker arguments</span><textarea rows={4} value={joinArgs(voice.tts_worker_args)} disabled={locked} onChange={(event) => patch({ tts_worker_args: splitArgs(event.target.value) })} /></label>
      </>}
      {voice.tts_provider !== "none" && voice.tts_provider !== "custom" && <details className="advanced-fields"><summary>Advanced</summary><HostPathField label="TTS worker binary override" kind="file" value={voice.tts_worker_path ?? ""} disabled={locked} onChange={(tts_worker_path) => patch({ tts_worker_path })} /></details>}
      {voice.tts_provider !== "none" && <label className="toggle-line hint-block"><span className="toggle"><input type="checkbox" checked={voice.speak_replies ?? false} disabled={locked} onChange={(event) => patch({ speak_replies: event.target.checked })} /><span className="track" aria-hidden="true" /></span><span>Speak chat replies</span></label>}
      <VoiceWorkers locked={locked} role="tts" dirty={dirty} enabled={voice.enabled} providerSelected={voice.tts_provider !== "none"} showNeuTTSModule={voice.tts_provider === "neutts_air"} />
    </>
  );
}
