import type { LLMSettings, ModelCapabilityLevel, ModelCapabilityOption } from "../api/llm-settings-types";
import { t, translateKnown } from "../i18n";
import { capabilityNames, contextLabel } from "../util/model-connections";
import { FieldRow } from "./SetupSection";

function levelHint(level: ModelCapabilityLevel): string {
  return level === "simple" ? t("Compact prompts for smaller models. In Layered they also set the vibration level.")
    : level === "intermediate" ? t("Fuller guidance and grouped motion edits for capable models. They also set the vibration level.")
      : t("Counted phrases, vibration patterns of their own, Auto timing and response repair. Choose it only for a model that handles complex instructions reliably.");
}

// One capability and context pair for every model: the local model and each
// connection. Levels and their floors come from the backend; a higher level
// raises a smaller budget, and a larger saved budget is kept. These are drafts
// for Save; live controls follow the saved model-routing features instead.
export function CapabilityRows({ id, level, options, context, contextSizes = [], contextHint, locked, onChange }: {
  id: string; level: ModelCapabilityLevel; options: ModelCapabilityOption[];
  context?: number; contextSizes?: readonly number[]; contextHint?: string; locked: boolean;
  onChange: (change: { level?: ModelCapabilityOption; context?: number }) => void;
}) {
  const selected = options.find(option => option.level === level);
  if (!selected) return null;
  const value = context === undefined ? undefined : Math.max(context, selected.minimum_context);
  const sizes = value === undefined ? [] : [...new Set([value, ...contextSizes])].filter(size => size >= selected.minimum_context).sort((a, b) => a - b);
  return <>
    <FieldRow id={`${id}-capability`} label={t("Capability")} hint={levelHint(level)}>
      <select id={`${id}-capability`} aria-describedby={`${id}-capability-hint`} value={level} disabled={locked} onChange={event => {
        const option = options.find(item => item.level === event.target.value);
        if (option) onChange({ level: option, context: value === undefined ? undefined : Math.max(value, option.minimum_context) });
      }}>
        {options.map(option => <option key={option.level} value={option.level}>{translateKnown(capabilityNames[option.level])}</option>)}
      </select>
    </FieldRow>
    {value !== undefined && <FieldRow id={`${id}-context`} label={t("Context")} hint={contextHint}>
      <select id={`${id}-context`} aria-describedby={contextHint ? `${id}-context-hint` : undefined} value={value} disabled={locked} onChange={event => onChange({ context: Number(event.target.value) })}>
        {sizes.map(size => <option key={size} value={size}>{contextLabel(size)}</option>)}
      </select>
    </FieldRow>}
  </>;
}

// The local model's pair. Ollama keeps its own allocation at Simple, so its
// context appears only once a level needs more; managed llama.cpp shows it in
// Settings, because MagicHandy sets that allocation. Setup keeps Simple short.
export function ModelCapabilityField({ settings, options, contextSizes, locked, patch, contextAtSimple = true }: {
  settings: LLMSettings; options: ModelCapabilityOption[]; contextSizes: number[]; locked: boolean;
  patch: (change: Partial<LLMSettings>) => void; contextAtSimple?: boolean;
}) {
  const level = settings.local_capability_level || "simple";
  const managed = settings.provider === "llama_cpp" && settings.llama_cpp_mode === "managed";
  const server = settings.provider === "llama_cpp" && settings.llama_cpp_mode === "external";
  const floor = options.find(option => option.level === level)?.minimum_context ?? 0;
  const showContext = level !== "simple" || (managed && contextAtSimple);
  const contextHint = server
    ? t("Set your server's context length to at least {context}. MagicHandy budgets each request to fit.", { context: contextLabel(floor) })
    : t("{level} needs at least {context}. Larger contexts use more RAM and VRAM. Applied after Save.", { level: translateKnown(capabilityNames[level]), context: contextLabel(floor) });
  return <div className="form-rows model-capability-settings">
    <CapabilityRows id="local-model" level={level} options={options} context={showContext ? settings.llama_cpp_context_size : undefined}
      contextSizes={contextSizes} contextHint={contextHint} locked={locked}
      onChange={({ level: option, context }) => patch({
        ...(option ? { local_capability_level: option.level, max_output_tokens: Math.max(settings.max_output_tokens, option.minimum_output) } : {}),
        llama_cpp_context_size: context ?? Math.max(settings.llama_cpp_context_size, option?.minimum_context ?? 0),
      })} />
  </div>;
}
