import type { ModelConnection } from "../api/cloud-types";
import type { LLMSettings } from "../api/llm-settings-types";

export const modelProviderNames = { chatgpt: "ChatGPT", openrouter: "OpenRouter", openai: "OpenAI API", compatible: "Other compatible provider" } as const;

// Provider identity is never inferred from a user-editable connection name.
export function modelConnectionLabel(connection: ModelConnection): string {
  const provider = modelProviderNames[connection.provider];
  const name = connection.name.trim();
  return [provider, name && name !== provider && name !== "Compatible API" ? name : "", connection.model.trim()].filter(Boolean).join(" · ");
}

// Compare the public metadata of the backend's exact tested connection.
export function connectionSignature(connection: ModelConnection): string {
  return JSON.stringify([
    connection.id, connection.name.trim(), connection.provider,
    connection.provider === "openrouter" ? "https://openrouter.ai/api/v1" : connection.provider === "chatgpt" || connection.provider === "openai" ? "https://api.openai.com/v1" : connection.base_url.trim().replace(/\/+$/, ""),
    connection.model.trim(), connection.output_mode || "auto", connection.supported_parameters ?? [],
    Boolean(connection.allow_fallbacks), connection.allowed_providers ?? [], connection.data_collection || "deny",
    Boolean(connection.zero_data_retention), Boolean(connection.no_authentication),
    connection.reasoning_effort ?? "",
  ]);
}

export function newModelConnection(provider: ModelConnection["provider"], connections: ModelConnection[]): ModelConnection {
  let number = 1;
  while (connections.some(connection => connection.id === `${provider}-${number}`)) number++;
  const names = { chatgpt: "ChatGPT", openrouter: "OpenRouter", openai: "OpenAI API", compatible: "Compatible API" };
  return {
    id: `${provider}-${number}`, name: names[provider], provider,
    base_url: provider === "openrouter" ? "https://openrouter.ai/api/v1" : provider === "compatible" ? "http://127.0.0.1:8000/v1" : "https://api.openai.com/v1",
    model: "", output_mode: "auto", allow_fallbacks: false, data_collection: "deny", zero_data_retention: false,
  };
}

// Restore only this module's fields; other settings pages may have their own drafts.
export function modelSettingsPatch(settings: LLMSettings): Partial<LLMSettings> {
  const { connections, conversation_connection_id, retry_refusal_locally, motion_planner, provider, model,
    llama_cpp_mode, llama_cpp_base_url, llama_cpp_context_size, ollama_base_url, ollama_models_path,
    request_timeout_ms, max_output_tokens, managed_load_policy, reasoning_mode, reply_length,
    motion_generation_mode, motion_capabilities } = settings;
  return { connections, conversation_connection_id, retry_refusal_locally, motion_planner, provider, model,
    llama_cpp_mode, llama_cpp_base_url, llama_cpp_context_size, ollama_base_url, ollama_models_path,
    request_timeout_ms, max_output_tokens, managed_load_policy, reasoning_mode, reply_length,
    motion_generation_mode, motion_capabilities };
}

// Compare semantic fields in a fixed order, including omitted backend defaults.
export function modelSettingsSignature(settings: LLMSettings): string {
  const planner = settings.motion_planner;
  const capabilities = settings.motion_capabilities ?? { motion: true, patterns: true, area_focus: true, experimental_patterns: false };
  return JSON.stringify([
    settings.conversation_connection_id || "local", Boolean(settings.retry_refusal_locally), planner?.provider || "conversation",
    planner?.provider === "connection" ? planner.connection_id || "" : "", planner?.model || "",
    planner?.context_policy || "conversation", settings.provider, settings.model,
    settings.llama_cpp_mode, settings.llama_cpp_base_url, settings.ollama_base_url,
    settings.llama_cpp_context_size, settings.request_timeout_ms, settings.max_output_tokens,
    settings.managed_load_policy || "startup", settings.reasoning_mode, settings.reply_length,
    settings.ollama_models_path || "", settings.motion_generation_mode,
    capabilities.motion, capabilities.patterns, capabilities.area_focus, capabilities.experimental_patterns,
    [...(settings.connections ?? [])].sort((a, b) => a.id.localeCompare(b.id)).map(connectionSignature),
  ]);
}

export type ModelRole = "chat" | "autopilot";

// The roles that would stop working if this connection went away. Autopilot
// set to "Same as conversation" follows chat.
export function connectionRoles(settings: LLMSettings, id: string): ModelRole[] {
  const chat = (settings.conversation_connection_id || "local") === id;
  const planner = settings.motion_planner;
  const autopilot = planner?.provider === "connection" ? planner.connection_id === id : (planner?.provider ?? "conversation") === "conversation" && chat;
  return [...(chat ? ["chat" as const] : []), ...(autopilot ? ["autopilot" as const] : [])];
}

// Removing a connection hands its roles to the local model, so the saved
// settings never point at a connection that no longer exists. Its API key is
// deleted by the backend when this draft is saved.
export function removeConnection(settings: LLMSettings, id: string): Partial<LLMSettings> {
  const planner = settings.motion_planner;
  const change: Partial<LLMSettings> = { connections: (settings.connections ?? []).filter(connection => connection.id !== id) };
  if ((settings.conversation_connection_id || "local") === id) change.conversation_connection_id = "local";
  if (planner?.provider === "connection" && planner.connection_id === id) change.motion_planner = { ...planner, provider: "local", connection_id: "", context_policy: "conversation" };
  return change;
}
