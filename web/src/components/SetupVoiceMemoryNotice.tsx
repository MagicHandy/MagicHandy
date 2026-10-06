import type { SetupVoiceMemory } from "../api/setup-assessment-types";
import { t } from "../i18n";
import { formatBytes } from "../util/format";

// The backend owns the estimate and fit verdict. This component only words it.
export function SetupVoiceMemoryNotice({ memory, device }: { memory: SetupVoiceMemory; device: "cpu" | "cuda" }) {
  if (memory.status === "cpu") return null;
  const mib = (value: number) => formatBytes(value * 1024 * 1024);
  const warning = memory.status === "insufficient" || memory.status === "unknown";
  return <div className={warning ? "setup-notice" : "hint-block"} role={warning ? "alert" : undefined}>
    {memory.status === "insufficient" && <p>{t("The chat and voice memory allowance needs at least {needed}, but this GPU has {available}. You can still install this voice; running both may fail or slow down. Choose Chatterbox or a smaller chat model to leave more room.", { needed: mib(memory.required_mib), available: mib(memory.available_mib) })}</p>}
    {(memory.status === "unknown" || !memory.llm_known) && <p>{t("The GPU memory or selected chat model's memory use is unknown. You can still install this voice, but there may not be enough graphics memory to run chat and voice together.")}</p>}
    {memory.status === "fits" && <p>{t("Estimated combined graphics memory: {needed} of {available}.", { needed: mib(memory.required_mib), available: mib(memory.available_mib) })}</p>}
    {memory.llm_known && <p>{t("Graphics memory estimate: {chat} for chat, {voice} for voice, plus {reserve} for the desktop. This is a planning estimate; larger contexts and other GPU apps can need more.", { chat: mib(memory.llm_vram_mib), voice: mib(memory.voice_vram_mib), reserve: mib(memory.reserve_mib) })}</p>}
    {device === "cpu" && <p>{t("Voice will run on the CPU, keeping graphics memory available for chat.")}</p>}
  </div>;
}
