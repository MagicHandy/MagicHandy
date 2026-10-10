import type { LLMCatalog } from "../api/catalog-types";
import type { SetupAssessment, SetupRequirement } from "../api/setup-assessment-types";
import type { PublicSettings, SetupStatus } from "../api/types";
import { t, translateKnown, type MessageKey } from "../i18n";
import { formatBytes } from "../util/format";
import { setupRequiredBytes } from "../util/setup-space";
import { SetupChoice } from "./SetupChoice";
import { FieldRow } from "./SetupSection";
import type { VoiceChoice } from "./SetupSteps";
import { SetupVoiceMemoryNotice } from "./SetupVoiceMemoryNotice";
import { SetupQwenReference, type SetupQwenReferenceProps } from "./SetupQwenReference";
import type { ReactNode } from "react";

const MIB = 1024 * 1024;

const EXPLICITNESS: Array<{ value: string; label: MessageKey; detail: MessageKey }> = [
  { value: "utility", label: "Utility (neutral assistant)", detail: "No sexual content. Replies stay practical." },
  { value: "warm", label: "Warm (flirtatious, never explicit)", detail: "Flirty and suggestive, never graphic." },
  { value: "intimate", label: "Intimate (sensual partner)", detail: "A sensual partner voice with indirect language." },
  { value: "explicit", label: "Explicit (direct sexual language)", detail: "Direct, graphic sexual language." },
];

function RequirementLine({ requirement, title, detail }: { requirement: SetupRequirement; title: string; detail: string }) {
  const tone = requirement.status === "met" ? "ok" : requirement.status === "partial" ? "warn" : "error";
  const state = requirement.status === "met" ? t("Requirements met") : requirement.status === "partial" ? t("Requirements partly met") : t("Requirements not met");
  return (
    <div className="easy-requirement form-row" data-status={requirement.status}>
      <span className="status-dot" data-state={tone} aria-hidden="true" />
      <span>
        <strong>{title}</strong>
        <small><span className="easy-requirement-state">{state}</span><span>{detail}</span></small>
      </span>
    </div>
  );
}

// chatRequirementText words the backend's verdict for local chat.
function chatRequirementText(assessment: SetupAssessment, setup: SetupStatus, catalog: LLMCatalog | null): { title: string; detail: string } {
  const model = catalog?.models.find((item) => item.id === assessment.model_id);
  const gpu = setup.hardware.gpu_name || t("NVIDIA GPU");
  const total = setup.hardware.vram_mib ? formatBytes(Number(setup.hardware.vram_mib) * MIB) : "";
  switch (assessment.chat.reason) {
    case "existing_server":
      return { title: t("Chat: {model}", { model: assessment.model_name ?? "" }), detail: t("Your saved local server and model are kept. No managed model download is needed; connection settings remain available in Custom setup.") };
    case "gpu_fits":
      return {
        title: t("Chat: {model}", { model: assessment.model_name ?? model?.display_name ?? assessment.model_id ?? "" }),
        detail: assessment.model_installed_id
          ? t("Already in your model store. Uses about {memory} of the {total} on your {gpu}.", { memory: formatBytes((assessment.chat.vram_mib ?? 0) * MIB), total, gpu })
          : t("Picked for your {gpu}: uses about {memory} of its {total}. Downloads {size}.", { gpu, memory: formatBytes((assessment.chat.vram_mib ?? 0) * MIB), total, size: formatBytes(model?.size_bytes ?? 0) }),
      };
    case "vram_unknown":
      return {
        title: t("Chat: {model}", { model: assessment.model_name ?? model?.display_name ?? assessment.model_id ?? "" }),
        detail: t("MagicHandy could not read how much memory your {gpu} has. This model runs well with about {memory} or more.", { gpu, memory: formatBytes((assessment.chat.min_vram_mib ?? 0) * MIB) }),
      };
    case "model_unknown":
      return { title: t("Chat: {model}", { model: assessment.model_name ?? "" }), detail: t("Your selected model is kept. Its graphics memory use is not measured for this model or context size, so the voice fit cannot be confirmed.") };
    case "selected_vram_below":
      return { title: t("Chat: {model}", { model: assessment.model_name ?? "" }), detail: t("Your selected model is kept, but its graphics memory estimate exceeds this GPU. A smaller chat model can leave more room for voice.") };
    case "vram_below":
      return {
        title: t("Local chat needs more graphics memory"),
        detail: t("The smallest tested model needs about {memory}; your {gpu} has {total}. Chat and Autopilot stay off, and Custom setup can connect a chat server on another computer.", { memory: formatBytes((assessment.chat.min_vram_mib ?? 0) * MIB), gpu, total }),
      };
    case "platform":
      return { title: t("Local chat is not available here"), detail: t("Managed local chat runs on Windows x64. Custom setup can connect a chat server on another computer.") };
    default:
      return { title: t("Local chat needs an NVIDIA graphics card"), detail: t("On the CPU alone, replies are too slow to use. Chat and Autopilot stay off, and Custom setup can connect a chat server on another computer.") };
  }
}

function voiceOutputText(requirement: SetupRequirement): string {
  if (requirement.reason.startsWith("qwen_")) return t("Qwen3-TTS streams speech on your NVIDIA GPU. Configure its audio sample and exact transcript below, or do it later in Settings > Voice.");
  if (requirement.reason === "gpu") return t("Chatterbox Turbo runs on your graphics card and speaks right after it installs.");
  if (requirement.reason === "cpu") return t("Chatterbox Turbo runs on the CPU to leave graphics memory for chat; speech is slower.");
  return t("The voice installer is not included in this installation.");
}

function voiceInputText(requirement: SetupRequirement): string {
  if (requirement.reason === "installed") return t("Parakeet is already installed.");
  if (requirement.reason === "cpu") return t("Parakeet runs on the CPU; any recent processor keeps up.");
  return t("The speech recognition installer is not included in this installation.");
}

export function EasySetupStep({ setup, settings, catalog, voiceChoice, voiceInput, connectionKey, locked, qwenReference, aiSetup, useLocalAI, chatEnabled, setChatVoice, setVoiceOutput, setVoiceChoice, setVoiceInput, setConnectionKey }: {
  setup: SetupStatus;
  settings: PublicSettings;
  catalog: LLMCatalog | null;
  voiceChoice: VoiceChoice;
  voiceInput: boolean;
  connectionKey: string;
  locked: boolean;
  qwenReference: SetupQwenReferenceProps;
  aiSetup: ReactNode;
  useLocalAI: boolean;
  chatEnabled: boolean;
  setChatVoice: (voice: string) => void;
  setVoiceOutput: (enabled: boolean) => void;
  setVoiceChoice: (choice: VoiceChoice) => void;
  setVoiceInput: (enabled: boolean) => void;
  setConnectionKey: (key: string) => void;
}) {
  const assessment = setup.assessment;
  if (!assessment) return <div className="setup-copy"><p>{t("Checking your computer...")}</p></div>;
  const chat = chatRequirementText(assessment, setup, catalog);
  const voiceOutput = voiceChoice !== "none";
  const selectedVoice = assessment.voice_options?.find((option) => option.module === (voiceOutput ? voiceChoice : assessment.voice_module));
  const voiceRequirement = selectedVoice?.requirement ?? assessment.voice_output;
  const chatAvailable = chatEnabled;
  const voiceOutputAvailable = voiceRequirement.status !== "unmet";
  const voiceInputAvailable = assessment.voice_input.status !== "unmet";
  const needed = setupRequiredBytes(assessment, useLocalAI && settings.llm.provider === "llama_cpp" && settings.llm.llama_cpp_mode === "managed", voiceChoice, voiceInput);
  const shortOfSpace = assessment.free_disk_bytes > 0 && needed > assessment.free_disk_bytes;
  const hardware = setup.hardware.nvidia
    ? t("Detected {gpu} with {memory} of graphics memory.", { gpu: setup.hardware.gpu_name || t("NVIDIA GPU"), memory: setup.hardware.vram_mib ? formatBytes(Number(setup.hardware.vram_mib) * MIB) : t("unknown") })
    : t("No NVIDIA graphics card was found.");
  const chatVoice = settings.llm.chat_voice ?? "utility";
  const spaceMessage = needed <= 0
    ? t("Nothing needs to be downloaded.")
    : shortOfSpace
      ? t("Installing these needs about {needed} of free space, but only {free} is free. Free up space before continuing.", { needed: formatBytes(needed), free: formatBytes(assessment.free_disk_bytes) })
      : t("Installing these needs about {needed} of free space.", { needed: formatBytes(needed) });

  return (
    <div className="setup-copy easy-setup">
      <p>{t("Choose how AI should run, then add voice or a device if you want. MagicHandy installs only what you select.")}</p>
      {aiSetup}
      {useLocalAI && <section className="setup-section" aria-labelledby="easy-system-title">
        <h2 id="easy-system-title">{t("Your computer")}</h2>
        <p className="hint-block">{hardware}</p>
        {assessment.free_disk_bytes > 0 && <p className="hint-block">{t("{free} of free disk space.", { free: formatBytes(assessment.free_disk_bytes) })}</p>}
        <div className="form-rows"><RequirementLine requirement={assessment.chat} title={chat.title} detail={chat.detail} /></div>
      </section>}

      {chatAvailable && (
        <section className="setup-section" aria-labelledby="easy-voice-title">
          <h2 id="easy-voice-title">{t("How explicit should chat be?")}</h2>
          <div className="setup-choices" role="radiogroup" aria-labelledby="easy-voice-title">
            {EXPLICITNESS.map((level) => (
              <SetupChoice key={level.value} selected={chatVoice === level.value} title={translateKnown(level.label)} detail={translateKnown(level.detail)} disabled={locked} onSelect={() => setChatVoice(level.value)} />
            ))}
          </div>
          <p className="hint-block">{t("This changes wording only. Motion limits and Stop work the same at every level, and you can change it any time in Settings or per persona.")}</p>
          {!useLocalAI && <p className="hint-block">{t("Your cloud provider's content rules still apply, including at the Explicit setting.")}</p>}
        </section>
      )}

      <section className="setup-section" aria-labelledby="easy-speech-title">
        <h2 id="easy-speech-title">{t("Voice")}</h2>
        <div className="form-rows">
        <label className="toggle-line form-row" data-disabled={!voiceOutputAvailable || undefined}>
          <span className="toggle"><input type="checkbox" checked={voiceOutput && voiceOutputAvailable} disabled={locked || !voiceOutputAvailable} onChange={(event) => setVoiceOutput(event.target.checked)} /><span className="track" aria-hidden="true" /></span>
          <span>{t("Speak replies aloud")}<small>{t("Voice output")}</small></span>
        </label>
        {voiceOutput && assessment.voice_options && <div className="form-row form-row-plain"><div className="setup-choices" role="radiogroup" aria-label={t("Voice module")}>
          {assessment.voice_options.map((option) => <SetupChoice
            key={option.module}
            selected={voiceChoice === option.module}
            title={translateKnown(option.module === "faster-qwen3-tts" ? "Faster Qwen3-TTS" : "Chatterbox Turbo")}
            detail={option.module === "faster-qwen3-tts"
              ? t("Quick streaming speech and voice cloning on NVIDIA GPUs. Needs more graphics memory, plus a reference WAV and its exact transcript.")
              : t("A smaller GPU budget and an included English voice. Speaks sentence by sentence on GPU, or more slowly on CPU.")}
            badge={option.module === assessment.voice_module ? (option.memory.status === "unknown" ? t("Fit unconfirmed") : t("Recommended")) : undefined}
            disabled={locked || option.requirement.status === "unmet"}
            onSelect={() => setVoiceChoice(option.module)}
          />)}
        </div></div>}
        {(voiceOutput || voiceRequirement.status !== "met") && <RequirementLine requirement={voiceRequirement} title={t("Voice output")} detail={voiceOutputText(voiceRequirement)} />}
        {voiceOutput && selectedVoice && <div className="form-row form-row-plain"><SetupVoiceMemoryNotice memory={selectedVoice.memory} device={selectedVoice.device} /></div>}
        {voiceChoice === "faster-qwen3-tts" && <div className="form-row form-row-plain"><SetupQwenReference {...qwenReference} /></div>}
        <label className="toggle-line form-row" data-disabled={!voiceInputAvailable || undefined}>
          <span className="toggle"><input type="checkbox" checked={voiceInput && voiceInputAvailable} disabled={locked || !voiceInputAvailable} onChange={(event) => setVoiceInput(event.target.checked)} /><span className="track" aria-hidden="true" /></span>
          <span>{t("Talk instead of typing")}<small>{t("Voice input")}</small></span>
        </label>
        {(voiceInput || assessment.voice_input.status !== "met") && <RequirementLine requirement={assessment.voice_input} title={t("Voice input")} detail={voiceInputText(assessment.voice_input)} />}
        </div>
      </section>

      <section className="setup-section" aria-labelledby="easy-device-title">
        <h2 id="easy-device-title">{t("Your Handy")}<span className="hint-inline">{t("optional")}</span></h2>
        <div className="form-rows">
          <FieldRow id="easy-device-key" label={t("Handy connection key")} hint={t("Connect your Handy in the official Handy Onboarding app; the key appears in the middle of its screen, under the picture of the device.")} stack>
            <input id="easy-device-key" aria-describedby="easy-device-key-hint" type="password" autoComplete="off" value={connectionKey} disabled={locked} placeholder={settings.device.connection_key_set ? t("Saved key will be kept") : t("Enter connection key")} onChange={(event) => setConnectionKey(event.target.value)} />
          </FieldRow>
          <p className="hint form-row-note">{t("Leave it empty to connect later from the top bar. Remote access stays off; Custom setup and Settings can turn it on.")}</p>
        </div>
      </section>

      <p className={shortOfSpace ? "form-status form-status-error" : "hint-block"} role={shortOfSpace ? "alert" : undefined}>{spaceMessage}</p>
    </div>
  );
}

// SetupModeChoice offers a guided path beside the detailed Custom setup.
export function SetupModeChoice({ mode, setMode }: { mode: "easy" | "custom"; setMode: (mode: "easy" | "custom") => void }) {
  return (
    <section className="setup-section" aria-labelledby="setup-mode-title">
      <h2 id="setup-mode-title">{t("How would you like to set up?")}</h2>
      <div className="setup-choices" role="radiogroup" aria-labelledby="setup-mode-title">
        <SetupChoice selected={mode === "easy"} title={t("Easy setup")} detail={t("Choose local AI, ChatGPT, or an API provider. MagicHandy guides the connection and offers optional voice.")} badge={t("Recommended")} onSelect={() => setMode("easy")} />
        <SetupChoice selected={mode === "custom"} title={t("Custom setup")} detail={t("Choose each part yourself: who can open MagicHandy, the device connection, the chat engine and model, and voice modules.")} onSelect={() => setMode("custom")} />
      </div>
    </section>
  );
}
