import { useId } from "react";
import type { PublicSettings } from "../api/types";
import { t, translateKnown, type MessageKey } from "../i18n";
import { RefreshIcon } from "../shell/icons";
import { useAppState } from "../state/app-state";
import { HostPathField } from "./HostPathField";
import { TTSModuleUpdate } from "./TTSModuleUpdate";
import { useParakeetRepair } from "./useParakeetRepair";
import { useVoiceRuntimeStatus, type VoiceRuntimeStatus } from "./useVoiceRuntimeStatus";
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
  onRuntimeChanged?: () => void;
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
  onRuntimeChanged = () => undefined,
}: Props) {
  const baseID = useId();
  const enableHintID = `${baseID}-enable-hint`;
  const toneID = `${baseID}-tone`;
  const toneHintID = `${baseID}-tone-hint`;
  const tonePromptID = `${baseID}-tone-prompt`;
  const tonePromptHintID = `${baseID}-tone-prompt-hint`;
  const seedHintID = `${baseID}-seed-hint`;
  const voice = s.voice;
  const runtime = useVoiceRuntimeStatus();
  // Worker status describes the saved settings. The backend snapshot names the
  // saved providers, so an unsaved provider choice can be told apart from them.
  const savedVoice = useAppState().state?.settings?.voice;
  const savedASR = savedVoice?.asr_provider ?? voice.asr_provider;
  const savedTTS = savedVoice?.tts_provider ?? voice.tts_provider;
  const providerLabel = (provider: string) => translateKnown(PROVIDER_LABELS[provider] ?? provider);
  const parakeetSource = voice.parakeet_source || "app_managed";
  const managedTTS = MANAGED_TTS.has(voice.tts_provider);
  const responseFormats = ["wav", "mp3", "opus", "aac", "flac"];
  const tonePresets = s.options.tts_tone_presets?.length
    ? s.options.tts_tone_presets
    : FALLBACK_TONE_PRESETS;
  const speechPolicies = s.options.chat_speech_policies?.length
    ? s.options.chat_speech_policies
    : ["interrupt", "finish_current"];
  const tonePreset = voice.tts_tone_preset ?? "natural";
  const seedMode = voice.tts_seed_mode ?? "fixed";
  const parakeetRepair = useParakeetRepair(
    voice.asr_provider === "parakeet_managed",
    () => {
      void runtime.refresh();
      onRuntimeChanged();
    },
  );
  const runtimeProps = {
    locked,
    dirty,
    enabled: voice.enabled,
    workers: runtime.workers,
    requests: runtime.requests,
    modules: runtime.modules,
    refresh: runtime.refresh,
    loaded: runtime.loaded,
    snapshotRequestedAt: runtime.requestedAt,
    statusError: runtime.error,
  };
  // While a draft names another provider, the card keeps showing the saved
  // one's worker (it is still what runs) and says so.
  const roleCard = (draft: string, saved: string) => {
    const changed = dirty && saved !== draft;
    return {
      providerSelected: draft !== "none" || (changed && saved !== "none"),
      savedProviderLabel: changed && saved !== "none" ? providerLabel(saved) : undefined,
    };
  };

  const providerSelect = (
    accessibleLabel: string,
    value: string,
    options: string[] | undefined,
    onChange: (value: string) => void,
  ) => (
    <select aria-label={accessibleLabel} value={value} disabled={locked} onChange={(event) => onChange(event.target.value)}>
      {(options?.length ? options : [value]).map((option) => (
        <option key={option} value={option}>{providerLabel(option)}</option>
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
        tts_auto_launch: true,
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
        tts_auto_launch: true,
      });
      return;
    }
    patch({ tts_provider });
  };

  return (
    <>
      <h2 className="section-title">{t("Voice")}</h2>
      <VoiceStatusNotice runtime={runtime} />

      <div className="group">
        <h3 className="group-title">{t("Voice features")}</h3>
        <label className="toggle-line">
          <span className="toggle"><input type="checkbox" aria-describedby={enableHintID} checked={voice.enabled} disabled={locked} onChange={(event) => patch({ enabled: event.target.checked })} /><span className="track" aria-hidden="true" /></span>
          <span>{t("Enable voice")}</span>
        </label>
        <p id={enableHintID} className="hint-block">{t("Speech input and output run only while voice is on. Changes apply when you save.")}</p>
      </div>

      <div className="group">
        <h3 className="group-title">{t("Speech input")}</h3>
        <VoiceWorkers role="asr" {...roleCard(voice.asr_provider, savedASR)} showParakeetModule={voice.asr_provider === "parakeet_managed" && parakeetSource === "app_managed"} parakeetRepair={parakeetRepair} {...runtimeProps} />
        <label className="field"><span className="label">{t("Provider")}</span>{providerSelect(t("Speech input provider"), voice.asr_provider, s.options.asr_providers, (asr_provider) => patch(asr_provider === "parakeet_managed" ? { asr_provider, parakeet_source: "app_managed" } : { asr_provider }))}</label>
        {voice.asr_provider === "parakeet_managed" && <details className="advanced-fields"><summary>{t("Advanced")}</summary>
          <label className="toggle-line"><span className="toggle"><input type="checkbox" checked={parakeetSource === "custom_local"} disabled={locked} onChange={(event) => patch({ parakeet_source: event.target.checked ? "custom_local" : "app_managed" })} /><span className="track" aria-hidden="true" /></span><span>{t("Custom local server")}</span></label>
          {parakeetSource === "custom_local" && <>
            <HostPathField label={t("Custom parakeet-server path")} kind="executable" value={voice.parakeet_server_path ?? ""} disabled={locked} onChange={(parakeet_server_path) => patch({ parakeet_server_path })} />
            <HostPathField label={t("Custom GGUF model path")} kind="gguf" value={voice.parakeet_model_path ?? ""} disabled={locked} onChange={(parakeet_model_path) => patch({ parakeet_model_path })} />
            <label className="field"><span className="label">{t("Server port")}</span><input type="number" min={1} max={65535} value={voice.parakeet_port ?? 8990} disabled={locked} onChange={(event) => patch({ parakeet_port: Number(event.target.value) })} /></label>
          </>}
        </details>}
        {voice.asr_provider === "openai_compatible" && <>
          <label className="field"><span className="label">{t("Base URL")}</span><input type="url" value={voice.asr_base_url ?? ""} disabled={locked} onChange={(event) => patch({ asr_base_url: event.target.value })} /></label>
          <label className="field"><span className="label">{t("Model name")}</span><input type="text" value={voice.asr_model ?? ""} disabled={locked} onChange={(event) => patch({ asr_model: event.target.value })} /></label>
        </>}
        {voice.asr_provider === "custom" && <>
          <HostPathField label={t("ASR worker path")} kind="file" value={voice.asr_worker_path ?? ""} disabled={locked} onChange={(asr_worker_path) => patch({ asr_worker_path })} />
          <label className="field"><span className="label">{t("Worker arguments")}</span><textarea rows={4} value={joinArgs(voice.asr_worker_args)} disabled={locked} onChange={(event) => patch({ asr_worker_args: splitArgs(event.target.value) })} /></label>
        </>}
      </div>

      <div className="group">
        <h3 className="group-title">{t("Speech output")}</h3>
        {/* The backend inspects the saved TTS module, so its readout only belongs under that provider. */}
        <VoiceWorkers role="tts" {...roleCard(voice.tts_provider, savedTTS)} showTTSModule={managedTTS && savedTTS === voice.tts_provider} {...runtimeProps} />
        {managedTTS && <TTSModuleUpdate update={runtime.modules.tts?.update} locked={locked} dirty={dirty} refresh={runtime.refresh} onComplete={onRuntimeChanged} />}
        <label className="field"><span className="label">{t("Provider")}</span>{providerSelect(t("Speech output provider"), voice.tts_provider, s.options.tts_providers, selectTTSProvider)}</label>

        {voice.tts_provider === "elevenlabs" && <>
          <label className="field"><span className="label">{t("API key")}{voice.elevenlabs_key_set && <span className="badge">{t("set")}</span>}</span><input type="password" autoComplete="off" placeholder={voice.elevenlabs_key_set ? t("set (leave blank to keep)") : t("Paste API key")} value={newKey} disabled={locked} onChange={(event) => setNewKey(event.target.value)} /></label>
          <label className="toggle-line"><span className="toggle"><input type="checkbox" checked={clearKey} disabled={locked || Boolean(newKey.trim())} onChange={(event) => setClearKey(event.target.checked)} /><span className="track" aria-hidden="true" /></span><span>{t("Clear API key on save")}</span></label>
          <label className="field"><span className="label">{t("Voice ID")}</span><input type="text" value={voice.elevenlabs_voice_id ?? ""} disabled={locked} onChange={(event) => patch({ elevenlabs_voice_id: event.target.value })} /></label>
          <label className="field"><span className="label">{t("Model ID")}</span><input type="text" value={voice.elevenlabs_model_id ?? ""} disabled={locked} onChange={(event) => patch({ elevenlabs_model_id: event.target.value })} /></label>
        </>}

        {voice.tts_provider === "faster_qwen3_tts" && <>
          <p className="hint-block">{t("Faster Qwen3-TTS requires an NVIDIA GPU with CUDA.")}</p>
          <HostPathField label={t("Reference WAV")} kind="wav" value={voice.tts_reference_wav ?? ""} disabled={locked} onChange={(tts_reference_wav) => patch({ tts_reference_wav })} />
          <label className="field"><span className="label">{t("Exact reference transcript")}</span><textarea rows={3} value={voice.tts_reference_text ?? ""} disabled={locked} onChange={(event) => patch({ tts_reference_text: event.target.value })} /></label>
          <p className="hint-block">{t("Use clean single-speaker audio, ideally 3 to 10 seconds, with an exact transcript.")}</p>
          <label className="field"><span className="label">{t("Language")}</span><input type="text" value={voice.tts_language ?? "Auto"} disabled={locked} onChange={(event) => patch({ tts_language: event.target.value })} /></label>
          <div className="field"><label className="label" htmlFor={toneID}>{t("Voice tone")}</label><select id={toneID} aria-describedby={toneHintID} value={tonePreset} disabled={locked} onChange={(event) => patch({ tts_tone_preset: event.target.value })}>{tonePresets.map((preset) => <option key={preset} value={preset}>{translateKnown(TONE_LABELS[preset] ?? preset)}</option>)}</select><span id={toneHintID} className="hint">{t("Tone prompting is experimental for cloned Base voices; results vary with the reference audio and seed.")}</span></div>
          {tonePreset === "custom" && <div className="field"><label className="label" htmlFor={tonePromptID}>{t("Custom tone prompt")}</label><textarea id={tonePromptID} aria-describedby={tonePromptHintID} rows={3} maxLength={2048} required value={voice.tts_tone_prompt ?? ""} disabled={locked} onChange={(event) => patch({ tts_tone_prompt: event.target.value })} /><span id={tonePromptHintID} className="hint">{t("Describe the delivery you want, such as pace, emotion, pitch, or emphasis.")}</span></div>}
        </>}
        {voice.tts_provider === "chatterbox_tts" && <>
          <HostPathField label={t("Reference WAV")} kind="wav" value={voice.tts_reference_wav ?? ""} disabled={locked} onChange={(tts_reference_wav) => patch({ tts_reference_wav })} />
          <label className="field"><span className="label">{t("Voice name")}</span><input type="text" value={voice.tts_voice ?? ""} disabled={locked} onChange={(event) => patch({ tts_voice: event.target.value })} /></label>
        </>}

        {voice.tts_provider === "openai_compatible" && <>
          <label className="field"><span className="label">{t("Base URL")}</span><input type="url" value={voice.tts_base_url ?? ""} disabled={locked} onChange={(event) => patch({ tts_base_url: event.target.value })} /></label>
          <label className="field"><span className="label">{t("Model")}</span><input type="text" value={voice.tts_model ?? ""} disabled={locked} onChange={(event) => patch({ tts_model: event.target.value })} /></label>
          <label className="field"><span className="label">{t("Voice name")}</span><input type="text" value={voice.tts_voice ?? ""} disabled={locked} onChange={(event) => patch({ tts_voice: event.target.value })} /></label>
          <label className="field"><span className="label">{t("API key")}{voice.openai_tts_key_set && <span className="badge">{t("set")}</span>}</span><input type="password" autoComplete="off" placeholder={voice.openai_tts_key_set ? t("set (leave blank to keep)") : t("Optional bearer key")} value={newOpenAIKey} disabled={locked} onChange={(event) => setNewOpenAIKey(event.target.value)} /></label>
          <label className="toggle-line"><span className="toggle"><input type="checkbox" checked={clearOpenAIKey} disabled={locked || Boolean(newOpenAIKey.trim())} onChange={(event) => setClearOpenAIKey(event.target.checked)} /><span className="track" aria-hidden="true" /></span><span>{t("Clear API key on save")}</span></label>
        </>}

        {voice.tts_provider === "custom" && <>
          <HostPathField label={t("TTS worker path")} kind="file" value={voice.tts_worker_path ?? ""} disabled={locked} onChange={(tts_worker_path) => patch({ tts_worker_path })} />
          <label className="field"><span className="label">{t("Worker arguments")}</span><textarea rows={4} value={joinArgs(voice.tts_worker_args)} disabled={locked} onChange={(event) => patch({ tts_worker_args: splitArgs(event.target.value) })} /></label>
        </>}

        {(voice.tts_provider === "faster_qwen3_tts" || voice.tts_provider === "openai_compatible") && <details className="advanced-fields"><summary>{t("Advanced")}</summary>
          {voice.tts_provider === "faster_qwen3_tts" && <>
            <label className="toggle-line"><span className="toggle"><input type="checkbox" aria-describedby={seedHintID} checked={seedMode === "fixed"} disabled={locked} onChange={(event) => patch({ tts_seed_mode: event.target.checked ? "fixed" : "varied" })} /><span className="track" aria-hidden="true" /></span><span>{t("Repeatable voice generation")}</span></label>
            <p id={seedHintID} className="hint-block">{seedMode === "fixed" ? t("Fixed mode reuses one seed for more consistent output.") : t("Varied mode uses a new seed for every request and can produce unusually long or degraded speech.")}</p>
            {seedMode === "fixed" && <div className="field"><span className="label">{t("Generation seed")}</span><div className="field-action-row"><input aria-label={t("Generation seed")} type="number" min={0} max={4294967295} step={1} value={voice.tts_seed ?? 1337} disabled={locked} onChange={(event) => { const seed = Number(event.target.value); if (Number.isSafeInteger(seed) && seed >= 0 && seed <= 4294967295) patch({ tts_seed: seed }); }} /><button type="button" className="btn btn-secondary" disabled={locked} onClick={() => { const values = new Uint32Array(1); globalThis.crypto.getRandomValues(values); patch({ tts_seed: values[0], tts_seed_mode: "fixed" }); }}><RefreshIcon size={16} />{t("New seed")}</button></div></div>}
          </>}
          {voice.tts_provider === "openai_compatible" && <>
            <label className="field"><span className="label">{t("Response format")}</span><select value={voice.tts_response_format ?? "wav"} disabled={locked} onChange={(event) => patch({ tts_response_format: event.target.value })}>{responseFormats.map((format) => <option key={format} value={format}>{format.toUpperCase()}</option>)}</select></label>
            <label className="field"><span className="label">{t("Health path")}</span><input type="text" value={voice.tts_health_path ?? "/health"} disabled={locked} onChange={(event) => patch({ tts_health_path: event.target.value })} /></label>
          </>}
        </details>}
        {voice.tts_provider !== "none" && <label className="toggle-line"><span className="toggle"><input type="checkbox" checked={voice.speak_replies ?? false} disabled={locked} onChange={(event) => patch({ speak_replies: event.target.checked })} /><span className="track" aria-hidden="true" /></span><span>{t("Speak chat replies")}</span></label>}
        {voice.tts_provider !== "none" && (voice.speak_replies ?? false) && <div className="field"><label className="label" htmlFor="chat-speech-policy">{t("When a new message is sent")}</label><select id="chat-speech-policy" value={voice.chat_speech_policy || "interrupt"} disabled={locked} onChange={(event) => patch({ chat_speech_policy: event.target.value })}>{speechPolicies.map((policy) => <option key={policy} value={policy}>{translateKnown(SPEECH_POLICY_LABELS[policy] ?? policy)}</option>)}</select><span className="hint">{voice.chat_speech_policy === "finish_current" ? t("Finishing speech preserves playback, but a shared local GPU can delay the next model response.") : t("Interrupting speech frees a shared local GPU sooner for the next model response.")}</span></div>}
      </div>

      <VoiceRequestQueue locked={locked} requests={runtime.requests} refresh={runtime.refresh} />
    </>
  );
}

// Poll failures are page-level: they make every card's status uncertain.
// Before any snapshot the cards say Status unavailable; afterwards they keep
// the last snapshot, labeled with its time, and runtime actions wait.
function VoiceStatusNotice({ runtime }: { runtime: VoiceRuntimeStatus }) {
  if (!runtime.error) return null;
  const time = runtime.receivedAt ? new Date(runtime.receivedAt).toLocaleTimeString() : "";
  return (
    <div className="voice-status-notice" role="alert">
      <div className="voice-status-notice-text">
        <span className="status-dot" data-state={runtime.loaded ? "pending" : "error"} aria-hidden="true" />
        <strong>{runtime.loaded ? t("Voice status is not updating") : t("Voice status is unavailable")}</strong>
        {runtime.loaded && time && <span>{t("Showing the last status, received at {time}. Starting and testing wait until it updates.", { time })}</span>}
        <span>{runtime.error}</span>
      </div>
      <button type="button" className="btn btn-secondary btn-sm" disabled={runtime.retrying} onClick={() => void runtime.retry()}>{runtime.retrying ? t("Retrying…") : t("Retry")}</button>
    </div>
  );
}
