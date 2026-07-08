export function llmProviderFromSnap(snap: {
  llm_provider?: string;
  llm?: { provider?: string };
}): string {
  return snap.llm?.provider ?? snap.llm_provider ?? "llama_cpp";
}

export function llmConnectedFromSnap(snap: {
  llm_connected?: boolean;
  ollama_connected?: boolean;
}): boolean {
  return Boolean(snap.llm_connected ?? snap.ollama_connected);
}

export function llmErrorFromSnap(snap: {
  llm_error?: string | null;
  ollama_error?: string | null;
}): string | null | undefined {
  const value = snap.llm_error ?? snap.ollama_error;
  if (value == null || value === "") {
    return null;
  }
  return String(value);
}

export function llmModelFromSnap(snap: {
  llm_model?: string;
  ollama_model?: string;
}): string {
  return snap.llm_model ?? snap.ollama_model ?? "";
}

export function llmBaseURLFromSnap(snap: {
  llm_base_url?: string;
  ollama_url?: string;
}): string {
  return snap.llm_base_url ?? snap.ollama_url ?? "";
}

export function isOllamaProvider(provider: string): boolean {
  return provider === "ollama";
}

export function llmIdleFromSnap(snap: {
  llm?: { managed?: boolean; loaded?: boolean };
  llm_cpp_mode?: string;
}): boolean {
  const managed =
    snap.llm?.managed === true || snap.llm_cpp_mode === "managed";
  const loaded = snap.llm?.loaded;
  return managed && loaded === false;
}
