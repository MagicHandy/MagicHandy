// Typed wrappers over the existing Go API. Every request carries a stable
// client ID so the backend controller lease can pick one active controller;
// other tabs become read-only. The frontend never builds raw transport
// payloads — only the semantic endpoints below.
import type {
  AppState,
  ChatStreamEvent,
  MemoryItem,
  MemoryState,
  MotionStyle,
  PromptSet,
  PublicSettings,
  SettingsUpdate,
} from "./types";

const CLIENT_ID_KEY = "magichandy-client-id";

export const clientId: string = (() => {
  try {
    let id = localStorage.getItem(CLIENT_ID_KEY);
    if (!id) {
      id = "ui-" + Math.random().toString(36).slice(2, 12);
      localStorage.setItem(CLIENT_ID_KEY, id);
    }
    return id;
  } catch {
    return "ui-" + Math.random().toString(36).slice(2, 12);
  }
})();

export const CLIENT_HEADER = "X-MagicHandy-Client-ID";

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { [CLIENT_HEADER]: clientId };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const res = await fetch(path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await res.text();
  const parsed = text ? (JSON.parse(text) as unknown) : null;
  if (!res.ok) {
    let message = `Request failed (${res.status})`;
    if (parsed && typeof parsed === "object" && "error" in parsed) {
      message = String((parsed as { error: unknown }).error);
    }
    throw new ApiError(message, res.status, parsed);
  }
  return parsed as T;
}

export class ApiError extends Error {
  constructor(message: string, readonly status: number, readonly body: unknown) {
    super(message);
  }
}

export const api = {
  getState: () => request<AppState>("GET", "/api/state"),

  // Motion — semantic commands only.
  stopMotion: () => request("POST", "/api/motion/stop", {}),
  // Manual test target — the strict decoder accepts only these two fields.
  startManualTest: (body: { pattern: string; speed_percent: number }) =>
    request("POST", "/api/motion/start", body),
  pauseMotion: () => request("POST", "/api/motion/pause", {}),
  resumeMotion: () => request("POST", "/api/motion/resume", {}),
  applyQuick: (patch: Partial<{
    speed_min_percent: number;
    speed_max_percent: number;
    stroke_min_percent: number;
    stroke_max_percent: number;
    reverse_direction: boolean;
    style: MotionStyle | string;
  }>) => request("POST", "/api/motion/quick", patch),

  // Modes (Freestyle / future Autopilot).
  getModes: () => request("GET", "/api/modes"),
  startMode: (mode: string, options?: Record<string, unknown>) =>
    request("POST", "/api/modes/start", { mode, ...(options ?? {}) }),
  stopMode: () => request("POST", "/api/modes/stop", {}),

  // Memory.
  getMemory: () => request<MemoryState>("GET", "/api/memory"),
  addMemory: (text: string) => request<MemoryItem>("POST", "/api/memory", { text }),
  setMemoryEnabled: (enabled: boolean) => request("POST", "/api/memory/enabled", { enabled }),
  setMemoryItemEnabled: (id: string, enabled: boolean) =>
    request("PATCH", `/api/memory/${encodeURIComponent(id)}`, { enabled }),
  removeMemory: (id: string) => request("DELETE", `/api/memory/${encodeURIComponent(id)}`),
  clearMemory: () => request("POST", "/api/memory/clear", {}),

  // Prompt sets.
  getPromptSets: () => request<{ sets: PromptSet[]; active?: string }>("GET", "/api/prompt-sets"),
  createPromptSet: (name: string, system: string) =>
    request<PromptSet>("POST", "/api/prompt-sets", { name, system }),
  updatePromptSet: (id: string, name: string, system: string) =>
    request<PromptSet>("PUT", `/api/prompt-sets/${encodeURIComponent(id)}`, { name, system }),
  deletePromptSet: (id: string) => request("DELETE", `/api/prompt-sets/${encodeURIComponent(id)}`),

  // LLM runtime.
  llmStatus: () => request("GET", "/api/llm/status"),
  llmLoad: () => request("POST", "/api/llm/load", {}),
  llmUnload: () => request("POST", "/api/llm/unload", {}),

  // Settings.
  getSettings: () => request<{ settings: PublicSettings }>("GET", "/api/settings"),
  saveSettings: (update: SettingsUpdate) => request("PUT", "/api/settings", update),
  resetSettings: () => request("POST", "/api/settings/reset", {}),

  // Non-motion connection check for the selected dispatch owner.
  connectionCheck: (owner: "cloud" | "bluetooth") =>
    request(`POST`, `/api/transport/${owner}/check`, {}),

  exportTrace: () => request("GET", "/api/traces"),
};

// Chat is a POST SSE stream; parse named events off the response body.
export async function streamChat(
  message: string,
  history: unknown[],
  onEvent: (e: ChatStreamEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  const res = await fetch("/api/chat/stream", {
    method: "POST",
    headers: { "Content-Type": "application/json", [CLIENT_HEADER]: clientId },
    body: JSON.stringify({ message, history }),
    signal,
  });
  if (!res.body) throw new ApiError("chat stream unavailable", res.status, null);
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    let sep: number;
    while ((sep = buffer.indexOf("\n\n")) !== -1) {
      const frame = buffer.slice(0, sep);
      buffer = buffer.slice(sep + 2);
      let event = "message";
      const dataLines: string[] = [];
      for (const line of frame.split("\n")) {
        if (line.startsWith("event:")) event = line.slice(6).trim();
        else if (line.startsWith("data:")) dataLines.push(line.slice(5).trim());
      }
      if (!dataLines.length) continue;
      try {
        onEvent({ event, data: JSON.parse(dataLines.join("\n")) } as ChatStreamEvent);
      } catch {
        /* ignore malformed frame */
      }
    }
  }
}
