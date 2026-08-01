import { useCallback, useEffect, useId, useRef, useState } from "react";
import { api } from "../api/client";
import type { PublicSettings, VoiceModuleStatus, VoiceRequestSnapshot, VoiceWorkerStatus } from "../api/types";
import { t, translateKnown, type MessageKey } from "../i18n";
import { RefreshIcon } from "../shell/icons";
import { HostPathField } from "./HostPathField";
import { VoiceRequestQueue } from "./VoiceRequestQueue";
import { VoiceWorkers } from "./VoiceWorkers";

const joinArgs = (args?: string[]) => (args ?? []).join("\n");
const splitArgs = (value: string) => value.split(/\r?\n/).map((arg) => arg.trim()).filter(Boolean);

const PROVIDER_LABELS: Partial<Record<string, MessageKey>> = {
  none: "None",
  parakeet_managed: "Parakeet (managed, local)",
  openai_compatible: "OpenAI-compatible server",
  elevenlabs: "ElevenLabs (cloud)",
  faster_qwen3_tts: "Faster Qwen3-TTS (managed)",
  chatterbox_tts: "Chatterbox Turbo (managed)",
  custom: "Custom worker",
};

const PARAKEET_SOURCE_LABELS: Partial<Record<string, MessageKey>> = {
  app_managed: "MagicHandy module",
  custom_local: "Custom local server",
};

const DEVICE_LABELS: Partial<Record<string, MessageKey>> = {
  auto: "Automatic / provider default",
  cuda: "NVIDIA CUDA",
  cpu: "CPU",
};

const FALLBACK_TONE_PRESETS = ["natural", "warm", "playful", "tender", "commanding", "excited", "custom"];
const TONE_LABELS: Partial<Record<string, MessageKey>> = {
  natural: "Natural (default)",
  warm: "Warm and intimate",
  playful: "Playful and teasing",
  tender: "Soft and reassuring",
  commanding: "Confident and commanding",
  excited: "Excited and energetic",
  custom: "Custom",
};

const SPEECH_POLICY_LABELS: Partial<Record<string, MessageKey>> = {
  interrupt: "Interrupt current speech",
  finish_current: "Finish current speech",
};

const MANAGED_TTS = new Set(["faster_qwen3_tts", "chatterbox_tts"]);
const message = (error: unknown) => error instanceof Error ? translateKnown(error.message) : t("Voice runtime request failed.");

interface Props {
  settings: PublicSettings;
  locked: boolean;
  dirty: boolean;
  patch: (next: Partial<PublicSettings["voice"]>) => void;
  newKey: string;
  setNewKey: (value: string) => void;
  clearKey: boolean;
  setClearKey: (value: boolean) => void;
  newOpenAIKey?: string;
  setNewOpenAIKey?: (value: string) => void;
  clearOpenAIKey?: boolean;
  setClearOpenAIKey?: (value: boolean) => void;
}

export function VoiceSettingsPanel({
  settings: s,
  locked,
  dirty,
  patch,
  newKey,
  setNewKey,
  clearKey,
  setClearKey,
  newOpenAIKey = "",
  setNewOpenAIKey = () => undefined,
  clearOpenAIKey = false,
  setClearOpenAIKey = () => undefined,
}: Props) {
  const tonePromptID = useId();
  const tonePromptHintID = `${tonePromptID}-hint`;
  const voice = s.voice;
  const voiceRuntime = useVoiceRuntimeStatus();
  const parakeetSource = voice.parakeet_source || "app_managed";
  const managedTTS = MANAGED_TTS.has(voice.tts_provider);
  const ttsModuleName = voice.tts_provider === "faster_qwen3_tts" ? t("Faster Qwen3-TTS") : t("Chatterbox Turbo");
  const availableDevices = s.options.tts_devices?.length ? s.options.tts_devices : ["auto", "cuda", "cpu"];
  const devices = voice.tts_provider === "faster_qwen3_tts"
    ? availableDevices.filter((device) => device === "cuda")
    : availableDevices;
  const requestedDevice = voice.tts_device ?? "auto";
  const selectedDevice = devices.includes(requestedDevice) ? requestedDevice : (devices[0] ?? requestedDevice);
  const responseFormats = voice.tts_provider === "faster_qwen3_tts"
    ? ["wav"]
    : voice.tts_provider === "chatterbox_tts"
      ? ["wav", "mp3", "opus"]
      : ["wav", "mp3", "opus", "aac", "flac"];
  const tonePresets = s.options.tts_tone_presets?.length
    ? s.options.tts_tone_presets
    : FALLBACK_TONE_PRESETS;
  const speechPolicies = s.options.chat_speech_policies?.length
    ? s.options.chat_speech_policies
    : ["interrupt", "finish_current"];
  const tonePreset = voice.tts_tone_preset ?? "natural";

  const providerSelect = (
    accessibleLabel: string,
    value: string,
    options: string[] | undefined,
    onChange: (value: string) => void,
  ) => (
    <select aria-label={accessibleLabel} value={value} disabled={locked} onChange={(event) => onChange(event.target.value)}>
      {(options?.length ? options : [value]).map((option) => (
        <option key={option} value={option}>{translateKnown(PROVIDER_LABELS[option] ?? option)}</option>
      ))}
    </select>
  );

  const selectTTSProvider = (tts_provider: string) => {
    if (tts_provider === "faster_qwen3_tts") {
      patch({
        tts_provider,
        tts_model: "Qwen/Qwen3-TTS-12Hz-0.6B-Base",
        tts_voice: "default",
        tts_base_url: "http://127.0.0.1:8991",
        tts_server_port: 8991,
        tts_health_path: "/health",
        tts_response_format: "wav",
        tts_device: "cuda",
        tts_seed: 1337,
        tts_seed_mode: "fixed",
      });
      return;
    }
    if (tts_provider === "chatterbox_tts") {
      patch({
        tts_provider,
        tts_model: "chatterbox-turbo",
        tts_voice: "Emily.wav",
        tts_base_url: "http://127.0.0.1:8992",
        tts_server_port: 8992,
        tts_health_path: "/api/model-info",
        tts_response_format: "wav",
        tts_device: "auto",
      });
      return;
    }
    patch({ tts_provider });
  };

  return (
    <>
      <h2 className="section-title">{t("Voice")}</h2>
      {voiceRuntime.error && <p className="form-status form-status-error" role="alert">{t("Voice runtime unavailable: {message}", { message: voiceRuntime.error })}</p>}
      {voiceRuntime.loading && !voiceRuntime.error && <p className="form-status" role="status">{t("Checking voice runtime...")}</p>}

      <div className="group">
        <h3 className="group-title">{t("Workers")}</h3>
        <label className="toggle-line">
          <span className="toggle"><input type="checkbox" checked={voice.enabled} disabled={locked} onChange={(event) => patch({ enabled: event.target.checked })} /><span className="track" aria-hidden="true" /></span>
          <span>{t("Enable voice workers")}</span>
        </label>
      </div>

      <div className="group">
        <h3 className="group-title">{t("Speech input (ASR)")}</h3>
        <label className="field"><span className="label">{t("Provider")}</span>{providerSelect(t("Speech input provider"), voice.asr_provider, s.options.asr_providers, (asr_provider) => patch({ asr_provider }))}</label>
        {voice.asr_provider === "parakeet_managed" && <>
          <label className="field"><span className="label">{t("Runtime source")}</span><select value={parakeetSource} disabled={locked} onChange={(event) => patch({ parakeet_source: event.target.value })}>{(s.options.parakeet_sources?.length ? s.options.parakeet_sources : [parakeetSource]).map((source) => <option key={source} value={source}>{translateKnown(PARAKEET_SOURCE_LABELS[source] ?? source)}</option>)}</select></label>
          {parakeetSource === "app_managed" && <p className="form-status">{t("Uses the worker, runner, and model installed by MagicHandy. No custom paths are required.")}</p>}
          {parakeetSource === "custom_local" && <>
            <HostPathField label={t("Custom parakeet-server path")} kind="executable" value={voice.parakeet_server_path ?? ""} disabled={locked} onChange={(parakeet_server_path) => patch({ parakeet_server_path })} />
            <HostPathField label={t("Custom GGUF model path")} kind="gguf" value={voice.parakeet_model_path ?? ""} disabled={locked} onChange={(parakeet_model_path) => patch({ parakeet_model_path })} />
            <label className="field"><span className="label">{t("Server port")}</span><input type="number" min={1} max={65535} value={voice.parakeet_port ?? 8990} disabled={locked} onChange={(event) => patch({ parakeet_port: Number(event.target.value) })} /></label>
          </>}
        </>}
        {voice.asr_provider === "openai_compatible" && <>
          <label className="field"><span className="label">{t("Base URL")}</span><input type="url" value={voice.asr_base_url ?? ""} disabled={locked} onChange={(event) => patch({ asr_base_url: event.target.value })} /></label>
          <label className="field"><span className="label">{t("Model name")}</span><input type="text" value={voice.asr_model ?? ""} disabled={locked} onChange={(event) => patch({ asr_model: event.target.value })} /></label>
        </>}
        {voice.asr_provider === "custom" && <>
          <HostPathField label={t("ASR worker path")} kind="file" value={voice.asr_worker_path ?? ""} disabled={locked} onChange={(asr_worker_path) => patch({ asr_worker_path })} />
          <label className="field"><span className="label">{t("Worker arguments")}</span><textarea rows={4} value={joinArgs(voice.asr_worker_args)} disabled={locked} onChange={(event) => patch({ asr_worker_args: splitArgs(event.target.value) })} /></label>
        </>}
        {voice.asr_provider !== "none" && voice.asr_provider !== "custom" && <details className="advanced-fields"><summary>{t("Advanced")}</summary>{voice.asr_provider === "parakeet_managed" && parakeetSource === "app_managed" && <label className="field"><span className="label">{t("Server port")}</span><input type="number" min={1} max={65535} value={voice.parakeet_port ?? 8990} disabled={locked} onChange={(event) => patch({ parakeet_port: Number(event.target.value) })} /></label>}<HostPathField label={t("ASR worker binary override")} kind="file" value={voice.asr_worker_path ?? ""} disabled={locked} onChange={(asr_worker_path) => patch({ asr_worker_path })} /></details>}
        <VoiceWorkers locked={locked} role="asr" dirty={dirty} enabled={voice.enabled} providerSelected={voice.asr_provider !== "none"} showParakeetModule={voice.asr_provider === "parakeet_managed" && parakeetSource === "app_managed"} {...voiceRuntime} />
      </div>

      <div className="group">
        <h3 className="group-title">{t("Speech output (TTS)")}</h3>
        <label className="field"><span className="label">{t("Provider")}</span>{providerSelect(t("Speech output provider"), voice.tts_provider, s.options.tts_providers, selectTTSProvider)}</label>

        {voice.tts_provider === "elevenlabs" && <>
          <label className="field"><span className="label">{t("API key")}{voice.elevenlabs_key_set && <span className="badge">{t("set")}</span>}</span><input type="password" autoComplete="off" placeholder={voice.elevenlabs_key_set ? t("set (leave blank to keep)") : t("Paste API key")} value={newKey} disabled={locked} onChange={(event) => setNewKey(event.target.value)} /></label>
          <label className="toggle-line hint-block"><span className="toggle"><input type="checkbox" checked={clearKey} disabled={locked || Boolean(newKey.trim())} onChange={(event) => setClearKey(event.target.checked)} /><span className="track" aria-hidden="true" /></span><span>{t("Clear API key on save")}</span></label>
          <label className="field"><span className="label">{t("Voice ID")}</span><input type="text" value={voice.elevenlabs_voice_id ?? ""} disabled={locked} onChange={(event) => patch({ elevenlabs_voice_id: event.target.value })} /></label>
          <label className="field"><span className="label">{t("Model ID")}</span><input type="text" value={voice.elevenlabs_model_id ?? ""} disabled={locked} onChange={(event) => patch({ elevenlabs_model_id: event.target.value })} /></label>
        </>}

        {managedTTS && <>
          <HostPathField label={t("Module folder")} kind="directory" value={voice.tts_module_root ?? ""} disabled={locked} onChange={(tts_module_root) => patch({ tts_module_root })} />
          <label className="field"><span className="label">{t("Model")}</span><input type="text" value={voice.tts_model ?? ""} disabled={locked} onChange={(event) => patch({ tts_model: event.target.value })} /></label>
          {voice.tts_provider === "faster_qwen3_tts" && <>
            <p className="form-status">{t("Faster Qwen3-TTS requires an NVIDIA GPU with CUDA.")}</p>
            <HostPathField label={t("Reference WAV")} kind="wav" value={voice.tts_reference_wav ?? ""} disabled={locked} onChange={(tts_reference_wav) => patch({ tts_reference_wav })} />
            <label className="field"><span className="label">{t("Exact reference transcript")}</span><textarea rows={3} value={voice.tts_reference_text ?? ""} disabled={locked} onChange={(event) => patch({ tts_reference_text: event.target.value })} /></label>
            <p className="form-status">{t("Use clean single-speaker audio, ideally 3 to 10 seconds, with an exact transcript.")}</p>
            <label className="field"><span className="label">{t("Language")}</span><input type="text" value={voice.tts_language ?? "Auto"} disabled={locked} onChange={(event) => patch({ tts_language: event.target.value })} /></label>
            <label className="field"><span className="label">{t("Voice tone")}</span><select value={tonePreset} disabled={locked} onChange={(event) => patch({ tts_tone_preset: event.target.value })}>{tonePresets.map((preset) => <option key={preset} value={preset}>{translateKnown(TONE_LABELS[preset] ?? preset)}</option>)}</select></label>
            {tonePreset === "custom" && <div className="field"><label className="label" htmlFor={tonePromptID}>{t("Custom tone prompt")}</label><textarea id={tonePromptID} aria-describedby={tonePromptHintID} rows={3} maxLength={2048} required value={voice.tts_tone_prompt ?? ""} disabled={locked} onChange={(event) => patch({ tts_tone_prompt: event.target.value })} /><span id={tonePromptHintID} className="hint">{t("Describe the delivery you want, such as pace, emotion, pitch, or emphasis.")}</span></div>}
            <p className="form-status">{t("Tone prompting is experimental for cloned Base voices; results vary with the reference audio and seed.")}</p>
          </>}
          {voice.tts_provider === "chatterbox_tts" && <>
            <HostPathField label={t("Reference WAV")} kind="wav" value={voice.tts_reference_wav ?? ""} disabled={locked} onChange={(tts_reference_wav) => patch({ tts_reference_wav })} />
            <label className="field"><span className="label">{t("Voice name")}</span><input type="text" value={voice.tts_voice ?? ""} disabled={locked} onChange={(event) => patch({ tts_voice: event.target.value })} /></label>
          </>}
          <div className="settings-field-grid">
            <label className="field"><span className="label">{t("Device")}</span><select value={selectedDevice} disabled={locked} onChange={(event) => patch({ tts_device: event.target.value })}>{devices.map((device) => <option key={device} value={device}>{translateKnown(DEVICE_LABELS[device] ?? device)}</option>)}</select></label>
            <label className="field"><span className="label">{t("Server port")}</span><input type="number" min={1} max={65535} value={voice.tts_server_port ?? 8991} disabled={locked} onChange={(event) => patch({ tts_server_port: Number(event.target.value), tts_base_url: `http://127.0.0.1:${event.target.value}` })} /></label>
          </div>
          <label className="toggle-line hint-block"><span className="toggle"><input type="checkbox" checked={voice.tts_auto_launch ?? false} disabled={locked} onChange={(event) => patch({ tts_auto_launch: event.target.checked })} /><span className="track" aria-hidden="true" /></span><span>{t("Launch this module with MagicHandy")}</span></label>
        </>}

        {voice.tts_provider === "openai_compatible" && <>
          <label className="field"><span className="label">{t("Base URL")}</span><input type="url" value={voice.tts_base_url ?? ""} disabled={locked} onChange={(event) => patch({ tts_base_url: event.target.value })} /></label>
          <label className="field"><span className="label">{t("Model")}</span><input type="text" value={voice.tts_model ?? ""} disabled={locked} onChange={(event) => patch({ tts_model: event.target.value })} /></label>
          <label className="field"><span className="label">{t("Voice name")}</span><input type="text" value={voice.tts_voice ?? ""} disabled={locked} onChange={(event) => patch({ tts_voice: event.target.value })} /></label>
          <label className="field"><span className="label">{t("API key")}{voice.openai_tts_key_set && <span className="badge">{t("set")}</span>}</span><input type="password" autoComplete="off" placeholder={voice.openai_tts_key_set ? t("set (leave blank to keep)") : t("Optional bearer key")} value={newOpenAIKey} disabled={locked} onChange={(event) => setNewOpenAIKey(event.target.value)} /></label>
          <label className="toggle-line hint-block"><span className="toggle"><input type="checkbox" checked={clearOpenAIKey} disabled={locked || Boolean(newOpenAIKey.trim())} onChange={(event) => setClearOpenAIKey(event.target.checked)} /><span className="track" aria-hidden="true" /></span><span>{t("Clear API key on save")}</span></label>
        </>}

        {voice.tts_provider === "custom" && <>
          <HostPathField label={t("TTS worker path")} kind="file" value={voice.tts_worker_path ?? ""} disabled={locked} onChange={(tts_worker_path) => patch({ tts_worker_path })} />
          <label className="field"><span className="label">{t("Worker arguments")}</span><textarea rows={4} value={joinArgs(voice.tts_worker_args)} disabled={locked} onChange={(event) => patch({ tts_worker_args: splitArgs(event.target.value) })} /></label>
        </>}

        {voice.tts_provider !== "none" && voice.tts_provider !== "custom" && <details className="advanced-fields"><summary>{t("Advanced")}</summary>
          {voice.tts_provider === "faster_qwen3_tts" && <>
            <label className="toggle-line hint-block"><span className="toggle"><input type="checkbox" checked={(voice.tts_seed_mode ?? "fixed") === "fixed"} disabled={locked} onChange={(event) => patch({ tts_seed_mode: event.target.checked ? "fixed" : "varied" })} /><span className="track" aria-hidden="true" /></span><span>{t("Repeatable voice generation")}</span></label>
            <p className="form-status">{(voice.tts_seed_mode ?? "fixed") === "fixed" ? t("Fixed mode reuses one seed for more consistent output.") : t("Varied mode uses a new seed for every request and can produce unusually long or degraded speech.")}</p>
            {(voice.tts_seed_mode ?? "fixed") === "fixed" && <div className="field"><span className="label">{t("Generation seed")}</span><div className="field-action-row"><input aria-label={t("Generation seed")} type="number" min={0} max={4294967295} step={1} value={voice.tts_seed ?? 1337} disabled={locked} onChange={(event) => { const seed = Number(event.target.value); if (Number.isSafeInteger(seed) && seed >= 0 && seed <= 4294967295) patch({ tts_seed: seed }); }} /><button type="button" className="btn btn-secondary" disabled={locked} onClick={() => { const values = new Uint32Array(1); globalThis.crypto.getRandomValues(values); patch({ tts_seed: values[0], tts_seed_mode: "fixed" }); }}><RefreshIcon size={16} />{t("New seed")}</button></div></div>}
          </>}
          {(managedTTS || voice.tts_provider === "openai_compatible") && <>
            <label className="field"><span className="label">{t("Response format")}</span><select value={voice.tts_response_format ?? "wav"} disabled={locked} onChange={(event) => patch({ tts_response_format: event.target.value })}>{responseFormats.map((format) => <option key={format} value={format}>{format.toUpperCase()}</option>)}</select></label>
            <label className="field"><span className="label">{t("Health path")}</span><input type="text" value={voice.tts_health_path ?? "/health"} disabled={locked} onChange={(event) => patch({ tts_health_path: event.target.value })} /></label>
          </>}
          <HostPathField label={t("TTS worker binary override")} kind="file" value={voice.tts_worker_path ?? ""} disabled={locked} onChange={(tts_worker_path) => patch({ tts_worker_path })} />
        </details>}
        {voice.tts_provider !== "none" && <label className="toggle-line hint-block"><span className="toggle"><input type="checkbox" checked={voice.speak_replies ?? false} disabled={locked} onChange={(event) => patch({ speak_replies: event.target.checked })} /><span className="track" aria-hidden="true" /></span><span>{t("Speak chat replies")}</span></label>}
        {voice.tts_provider !== "none" && (voice.speak_replies ?? false) && <div className="field"><label className="label" htmlFor="chat-speech-policy">{t("When a new message is sent")}</label><select id="chat-speech-policy" value={voice.chat_speech_policy || "interrupt"} disabled={locked} onChange={(event) => patch({ chat_speech_policy: event.target.value })}>{speechPolicies.map((policy) => <option key={policy} value={policy}>{translateKnown(SPEECH_POLICY_LABELS[policy] ?? policy)}</option>)}</select><span className="hint">{voice.chat_speech_policy === "finish_current" ? t("Finishing speech preserves playback, but a shared local GPU can delay the next model response.") : t("Interrupting speech frees a shared local GPU sooner for the next model response.")}</span></div>}
        <VoiceWorkers locked={locked} role="tts" dirty={dirty} enabled={voice.enabled} providerSelected={voice.tts_provider !== "none"} showTTSModule={managedTTS} ttsModuleName={ttsModuleName} {...voiceRuntime} />
      </div>

      <VoiceRequestQueue locked={locked} requests={voiceRuntime.requests} refresh={voiceRuntime.refresh} />
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
