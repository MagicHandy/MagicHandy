import type { ModelConnection } from "../api/cloud-types";

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
