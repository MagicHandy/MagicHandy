import type { LLMCatalog } from "../api/catalog-types";
import type { LLMModelManagerSnapshot, OllamaModelInfo, PublicSettings } from "../api/types";

// Recommendations come from the backend catalog. Match known model metadata,
// never a user-editable name or a guess that every 12B model is equally capable.
export function setupCapabilityDefaults(
  settings: PublicSettings, runtime: string, choice: string, catalog: LLMCatalog | null,
  models: LLMModelManagerSnapshot | null, ollama: OllamaModelInfo[],
): PublicSettings {
  if (!["managed", "ollama", "external"].includes(runtime)) return settings;
  const entries = catalog?.models ?? [];
  const metadata = runtime === "ollama" ? ollama.find(model => model.name === settings.llm.model)
    : runtime === "managed" ? models?.models.find(model => model.id === settings.llm.model) : undefined;
  const selected = runtime === "managed"
    ? entries.find(model => choice === `download:${model.id}` || (choice === "store" && model.installed_model_id === settings.llm.model))
    : undefined;
  const recommendation = selected ?? (metadata?.family && metadata.parameter_size
    ? entries.find(model => model.family === metadata.family && model.parameter_size === metadata.parameter_size)
    : undefined);
  const option = settings.options?.local_model_capabilities?.find(item => item.level === (recommendation?.recommended_capability ?? "simple"));
  if (!option) return settings;
  const context = Math.max(settings.llm.llama_cpp_context_size || 0, option.minimum_context);
  const output = Math.max(settings.llm.max_output_tokens || 0, option.minimum_output);
  if (settings.llm.local_capability_level === option.level && settings.llm.llama_cpp_context_size === context && settings.llm.max_output_tokens === output) return settings;
  return { ...settings, llm: { ...settings.llm, local_capability_level: option.level, llama_cpp_context_size: context, max_output_tokens: output } };
}
