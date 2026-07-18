import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { PublicSettings, VoiceModuleStatus, VoiceRequestSnapshot, VoiceWorkerStatus } from "../api/types";
import { HostPathField } from "./HostPathField";
import { NeuTTSReferenceDialog } from "./NeuTTSReferenceDialog";
import { VoiceRequestQueue } from "./VoiceRequestQueue";
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

const NEUTTS_SAMPLING_LABELS: Record<string, string> = {
  fixed: "Consistent",
  random: "Varied",
};
const DEFAULT_NEUTTS_SEED = 3;
const MAX_NEUTTS_SEED = 0xffffffff;
const message = (error: unknown) => error instanceof Error ? error.message : "Voice runtime request failed.";

function newNeuTTSSeed(current: number) {
  const values = new Uint32Array(1);
  window.crypto.getRandomValues(values);
  return values[0] === current ? (current + 1) >>> 0 : values[0];
}

interface Props {
  settings: PublicSettings;
  locked: boolean;
  // Unsaved voice edits: worker controls act on the saved config, so they
  // lock until the form is saved.
  dirty: boolean;
  patch: (next: Partial<PublicSettings["voice"]>) => void;
  newKey: string;
  setNewKey: (value: string) => void;
  clearKey: boolean;
  setClearKey: (value: boolean) => void;
}

export function VoiceSettingsPanel({ settings: s, locked, dirty, patch, newKey, setNewKey, clearKey, setClearKey }: Props) {
  const voice = s.voice;
  const voiceRuntime = useVoiceRuntimeStatus();
  const referenceEncoderInstalled = voiceRuntime.modules.neutts?.reference_encoder_installed ?? false;
  const parakeetSource = voice.parakeet_source || "app_managed";
  const neuTTSSamplingMode = voice.neutts_sampling_mode || "fixed";
  const neuTTSSamplerSeed = voice.neutts_sampler_seed ?? DEFAULT_NEUTTS_SEED;
  const neuTTSSamplingModes = s.options.neutts_sampling_modes?.length ? s.options.neutts_sampling_modes : ["fixed", "random"];
  const [referenceOpen, setReferenceOpen] = useState(false);
  const referenceTrigger = useRef<HTMLButtonElement>(null);
  const closeReference = useCallback(() => {
    setReferenceOpen(false);
    window.requestAnimationFrame(() => referenceTrigger.current?.focus());
  }, []);
  const providerSelect = (accessibleLabel: string, value: string, options: string[] | undefined, onChange: (value: string) => void) => (
    <select aria-label={accessibleLabel} value={value} disabled={locked} onChange={(event) => onChange(event.target.value)}>
      {(options?.length ? options : [value]).map((option) => <option key={option} value={option}>{PROVIDER_LABELS[option] ?? option}</option>)}
    </select>
  );

  return (
    <>
      <h2 className="section-title">Voice</h2>
      {voiceRuntime.error && <p className="form-status form-status-error" role="alert">Voice runtime unavailable: {voiceRuntime.error}</p>}
      {voiceRuntime.loading && !voiceRuntime.error && <p className="form-status" role="status">Checking voice runtime...</p>}
      <label className="toggle-line hint-block"><span className="toggle"><input type="checkbox" checked={voice.enabled} disabled={locked} onChange={(event) => patch({ enabled: event.target.checked })} /><span className="track" aria-hidden="true" /></span><span>Enable voice workers</span></label>

      <div className="divider" />
      <h3 className="group-title">Speech input (ASR)</h3>
      <label className="field"><span className="label">Provider</span>{providerSelect("Speech input provider", voice.asr_provider, s.options.asr_providers, (asr_provider) => patch({ asr_provider }))}</label>
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
      <VoiceWorkers
        locked={locked}
        role="asr"
        dirty={dirty}
        enabled={voice.enabled}
        providerSelected={voice.asr_provider !== "none"}
        showParakeetModule={voice.asr_provider === "parakeet_managed" && parakeetSource === "app_managed"}
        {...voiceRuntime}
      />

      <div className="divider" />
      <h3 className="group-title">Speech output (TTS)</h3>
      <label className="field"><span className="label">Provider</span>{providerSelect("Speech output provider", voice.tts_provider, s.options.tts_providers, (tts_provider) => patch({ tts_provider }))}</label>
      {voice.tts_provider === "elevenlabs" && <>
        <label className="field"><span className="label">API key {voice.elevenlabs_key_set && <span className="badge">set</span>}</span><input type="password" autoComplete="off" placeholder={voice.elevenlabs_key_set ? "set (leave blank to keep)" : "Paste API key"} value={newKey} disabled={locked} onChange={(event) => setNewKey(event.target.value)} /></label>
        <label className="toggle-line hint-block"><span className="toggle"><input type="checkbox" checked={clearKey} disabled={locked || Boolean(newKey.trim())} onChange={(event) => setClearKey(event.target.checked)} /><span className="track" aria-hidden="true" /></span><span>Clear API key on save</span></label>
        <label className="field"><span className="label">Voice ID</span><input type="text" value={voice.elevenlabs_voice_id ?? ""} disabled={locked} onChange={(event) => patch({ elevenlabs_voice_id: event.target.value })} /></label>
        <label className="field"><span className="label">Model ID</span><input type="text" value={voice.elevenlabs_model_id ?? ""} disabled={locked} onChange={(event) => patch({ elevenlabs_model_id: event.target.value })} /></label>
      </>}
      {voice.tts_provider === "neutts_air" && <>
        <div className="reference-voice-control">
          <div>
            <strong>Reference voice</strong>
            <span>{voice.neutts_reference_codes && voice.neutts_reference_text ? "Configured" : "Not configured"}</span>
          </div>
          <button
            ref={referenceTrigger}
            type="button"
            className="btn btn-secondary"
            disabled={locked || !referenceEncoderInstalled}
            title={referenceEncoderInstalled ? "" : "Install or update the managed NeuTTS reference encoder first"}
            onClick={() => setReferenceOpen(true)}
          >Generate reference voice</button>
        </div>
        <details className="advanced-fields"><summary>Advanced</summary>
          <div className="field">
            <span className="label">Speech variation <span className="hint-inline">{neuTTSSamplingMode === "random" ? "repeat cache off" : "repeat cache available"}</span></span>
            <div className="segmented neutts-sampling-control" role="group" aria-label="NeuTTS speech variation">
              {neuTTSSamplingModes.map((mode) => <button
                key={mode}
                type="button"
                aria-pressed={neuTTSSamplingMode === mode}
                disabled={locked}
                title={mode === "random" ? "Uses a new seed per request; quality and pacing may vary." : "Uses one seed for repeatable output and exact-text caching."}
                onClick={() => patch({ neutts_sampling_mode: mode })}
              >{NEUTTS_SAMPLING_LABELS[mode] ?? mode}</button>)}
            </div>
          </div>
          {neuTTSSamplingMode === "fixed" && <div className="field">
            <span className="label">Fixed seed <span className="hint-inline">recommended: {DEFAULT_NEUTTS_SEED}</span></span>
            <div className="field-action-row">
              <input
                aria-label="Fixed seed"
                type="number"
                min={0}
                max={MAX_NEUTTS_SEED}
                step={1}
                value={neuTTSSamplerSeed}
                disabled={locked}
                onChange={(event) => {
                  const seed = Number(event.target.value);
                  if (Number.isInteger(seed) && seed >= 0 && seed <= MAX_NEUTTS_SEED) patch({ neutts_sampler_seed: seed });
                }}
              />
              <button type="button" className="btn btn-secondary" disabled={locked} title="Choose a different fixed seed." onClick={() => patch({ neutts_sampler_seed: newNeuTTSSeed(neuTTSSamplerSeed) })}>New seed</button>
            </div>
          </div>}
          <HostPathField label="stream_pcm runner override" kind="executable" value={voice.neutts_runner_path ?? ""} disabled={locked} onChange={(neutts_runner_path) => patch({ neutts_runner_path })} />
          <HostPathField label="Reference WAV" kind="wav" value={voice.neutts_reference_wav ?? ""} disabled={locked} onChange={(neutts_reference_wav) => patch({ neutts_reference_wav })} />
          <HostPathField label="Pre-encoded reference codes (.npy)" kind="npy" value={voice.neutts_reference_codes ?? ""} disabled={locked} onChange={(neutts_reference_codes) => patch({ neutts_reference_codes })} />
          <label className="field"><span className="label">Reference transcript</span><textarea rows={3} value={voice.neutts_reference_text ?? ""} disabled={locked} onChange={(event) => patch({ neutts_reference_text: event.target.value })} /></label>
          <HostPathField label="TTS worker binary override" kind="file" value={voice.tts_worker_path ?? ""} disabled={locked} onChange={(tts_worker_path) => patch({ tts_worker_path })} />
        </details>
        <p className="form-status">Leave the runner override blank to use the runtime installed with managed llama.cpp. The managed module includes local WAV-to-reference encoding without Python. Pre-encoded <code>.npy</code> files remain available under Advanced.</p>
      </>}
      {voice.tts_provider === "custom" && <>
        <HostPathField label="TTS worker path" kind="file" value={voice.tts_worker_path ?? ""} disabled={locked} onChange={(tts_worker_path) => patch({ tts_worker_path })} />
        <label className="field"><span className="label">Worker arguments</span><textarea rows={4} value={joinArgs(voice.tts_worker_args)} disabled={locked} onChange={(event) => patch({ tts_worker_args: splitArgs(event.target.value) })} /></label>
      </>}
      {voice.tts_provider !== "none" && voice.tts_provider !== "custom" && voice.tts_provider !== "neutts_air" && <details className="advanced-fields"><summary>Advanced</summary><HostPathField label="TTS worker binary override" kind="file" value={voice.tts_worker_path ?? ""} disabled={locked} onChange={(tts_worker_path) => patch({ tts_worker_path })} /></details>}
      {voice.tts_provider !== "none" && <label className="toggle-line hint-block"><span className="toggle"><input type="checkbox" checked={voice.speak_replies ?? false} disabled={locked} onChange={(event) => patch({ speak_replies: event.target.checked })} /><span className="track" aria-hidden="true" /></span><span>Speak chat replies</span></label>}
      <VoiceWorkers
        locked={locked}
        role="tts"
        dirty={dirty}
        enabled={voice.enabled}
        providerSelected={voice.tts_provider !== "none"}
        showNeuTTSModule={voice.tts_provider === "neutts_air"}
        {...voiceRuntime}
      />
      <div className="divider" />
      <VoiceRequestQueue locked={locked} requests={voiceRuntime.requests} refresh={voiceRuntime.refresh} />
      {referenceOpen && <NeuTTSReferenceDialog
        initialWAV={voice.neutts_reference_wav ?? ""}
        initialTranscript={voice.neutts_reference_text ?? ""}
        onApply={(reference) => patch({
          neutts_reference_codes: reference.codes,
          neutts_reference_wav: reference.wav,
          neutts_reference_text: reference.transcript,
        })}
        onClose={closeReference}
      />}
    </>
  );
}

function useVoiceRuntimeStatus() {
  const [workers, setWorkers] = useState<Record<string, VoiceWorkerStatus>>({});
  const [requests, setRequests] = useState<VoiceRequestSnapshot[]>([]);
  const [modules, setModules] = useState<Record<string, VoiceModuleStatus>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const mounted = useRef(false);
  const inFlight = useRef<Promise<void> | null>(null);

  const refresh = useCallback(async () => {
    if (inFlight.current) return inFlight.current;
    if (mounted.current) setLoading(true);
    const request = (async () => {
      try {
        const response = await api.voiceStatus();
        if (!mounted.current) return;
        setWorkers(response.voice.workers ?? {});
        setModules(response.voice.modules ?? {});
        setRequests(response.requests ?? []);
        setError("");
      } catch (requestError) {
        if (mounted.current) setError(message(requestError));
      } finally {
        if (mounted.current) setLoading(false);
      }
    })();
    inFlight.current = request;
    try {
      await request;
    } finally {
      if (inFlight.current === request) inFlight.current = null;
    }
  }, []);

  useEffect(() => {
    mounted.current = true;
    let canceled = false;
    let timer: number | undefined;
    const poll = async () => {
      await refresh();
      if (!canceled) timer = window.setTimeout(() => void poll(), 3000);
    };
    void poll();
    return () => {
      canceled = true;
      mounted.current = false;
      window.clearTimeout(timer);
    };
  }, [refresh]);

  return { workers, requests, modules, loading, error, refresh };
}
