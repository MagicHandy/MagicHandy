import type { LLMSettings, ModelCapabilityLevel, ModelCapabilityOption } from "../api/llm-settings-types";
import { t, translateKnown } from "../i18n";

const names: Record<ModelCapabilityLevel, string> = { simple: "Simple", intermediate: "Intermediate", advanced: "Advanced" };

// Options and allocation floors come from the backend. These are drafts for
// Save; live controls use the saved model-routing features instead.
export function ModelCapabilityField({ settings, options, contextSizes, locked, patch }: {
  settings: LLMSettings; options: ModelCapabilityOption[]; contextSizes: number[]; locked: boolean;
  patch: (change: Partial<LLMSettings>) => void;
}) {
  const level = settings.local_capability_level || "simple";
  const selected = options.find(option => option.level === level);
  if (!selected) return null;
  const context = Math.max(settings.llama_cpp_context_size, selected.minimum_context);
  const sizes = [...new Set([context, ...contextSizes])].filter(size => size >= selected.minimum_context).sort((a, b) => a - b);
  return <div className="model-capability-settings">
    <div className="model-runtime-grid">
      <label className="field">
        <span className="label">{t("Model capability")}</span>
        <select value={level} disabled={locked} aria-describedby="model-capability-hint" onChange={event => {
          const choice = options.find(option => option.level === event.target.value);
          if (choice) patch({ local_capability_level: choice.level,
            llama_cpp_context_size: Math.max(settings.llama_cpp_context_size, choice.minimum_context),
            max_output_tokens: Math.max(settings.max_output_tokens, choice.minimum_output) });
        }}>
          {options.map(option => <option key={option.level} value={option.level}>{translateKnown(names[option.level])}</option>)}
        </select>
      </label>
      {level !== "simple" && <label className="field">
        <span className="label">{t("Context size")}</span>
        <select value={context} disabled={locked} onChange={event => patch({ llama_cpp_context_size: Number(event.target.value) })}>
          {sizes.map(size => <option key={size} value={size}>{t("{count} tokens", { count: size })}</option>)}
        </select>
      </label>}
    </div>
    <p className="form-status" id="model-capability-hint">
      {level === "simple" ? t("Compact prompts for smaller models. Vibration follows movement.")
        : level === "intermediate" ? t("Fuller guidance and grouped motion edits for capable local models. Vibration still follows movement.")
        : t("The same motion tools as external models: counted phrases, independent vibration, Auto timing, and response repair. Requires a model that handles complex instructions reliably.")}
    </p>
    {level !== "simple" && <p className="form-status">
      {t("At least {context} context tokens and {output} output tokens. Larger contexts use more RAM and VRAM.", { context: selected.minimum_context, output: selected.minimum_output })}
      {" "}{settings.provider === "llama_cpp" && settings.llama_cpp_mode === "external"
        ? t("Configure your server to provide this context size; MagicHandy uses it as the request budget.")
        : t("Applied to the local runtime after Save.")}
    </p>}
  </div>;
}
