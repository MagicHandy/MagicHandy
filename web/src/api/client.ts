// Typed wrappers over the existing Go API. Every request carries a stable
// client ID so the backend controller lease can pick one active controller;
// other tabs become read-only. The frontend never builds raw transport
// payloads — only the semantic endpoints below.
import type {
  AppState,
  ChatStreamEvent,
  MemoryState,
  MotionStyle,
  NeuTTSReference,
  BluetoothAckPayload,
  BluetoothClientStatus,
  BluetoothCommandsResponse,
  BluetoothStatusResponse,
  IntifaceTransportSnapshot,
  ChatHistoryMessage,
  ChatMessagesResponse,
  ChatSessionsResponse,
  ConnectionCheckResult,
  CloudDisconnectResponse,
  PromptSetsPayload,
  PatternInput,
  PatternLibrary,
  PatternPreview,
  MotionImportResult,
  LibraryPattern,
  LLMModelImport,
  LLMModelManagerSnapshot,
  LLMProviderStatus,
  ManagedLlamaRuntimeBuild,
  MediaScanState,
  MediaFunscript,
  MediaSyncEvent,
  MediaSyncStatus,
  MediaVideo,
  OllamaModelInfo,
  OllamaModelScan,
  PatternFeedback,
  PublicSettings,
  SettingsUpdate,
  VoiceRequestSnapshot,
  VoiceState,
  VoiceWorkerStatus,
} from "./types";

const CLIENT_ID_KEY = "magichandy-client-id";
const STOP_SEQUENCE_HEADER = "X-MagicHandy-Stop-Sequence";

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

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  signal?: AbortSignal,
  extraHeaders?: Record<string, string>,
  keepalive = false,
): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json", [CLIENT_HEADER]: clientId, ...extraHeaders };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const res = await fetch(path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    signal,
    keepalive,
  });
  const text = await res.text();
  let parsed: unknown = null;
  if (text) {
    try {
      parsed = JSON.parse(text) as unknown;
    } catch {
      parsed = { error: text };
    }
  }
  if (!res.ok) {
    let message = `Request failed (${res.status})`;
    if (parsed && typeof parsed === "object" && "error" in parsed) {
      message = String((parsed as { error: unknown }).error);
    }
    throw new ApiError(message, res.status, parsed);
  }
  return parsed as T;
}

async function uploadVoiceTranscription(audio: Blob, format: string, stopSequence?: number, signal?: AbortSignal): Promise<{ request: VoiceRequestSnapshot }> {
  const headers: Record<string, string> = {
    Accept: "application/json",
    "Content-Type": `audio/${format}`,
    [CLIENT_HEADER]: clientId,
  };
  if (stopSequence !== undefined) headers["X-MagicHandy-Stop-Sequence"] = String(stopSequence);
  const res = await fetch("/api/voice/transcriptions", {
    method: "POST",
    headers,
    body: audio,
    signal,
  });
  const text = await res.text();
  let parsed: unknown = null;
  if (text) {
    try {
      parsed = JSON.parse(text) as unknown;
    } catch {
      parsed = { error: text };
    }
  }
  if (!res.ok) {
    const message = parsed && typeof parsed === "object" && "error" in parsed
      ? String((parsed as { error: unknown }).error)
      : `Transcription upload failed (${res.status})`;
    throw new ApiError(message, res.status, parsed);
  }
  return parsed as { request: VoiceRequestSnapshot };
}

export class ApiError extends Error {
  constructor(message: string, readonly status: number, readonly body: unknown) {
    super(message);
  }
}

export const api = {
  getState: (signal?: AbortSignal) => request<AppState>("GET", "/api/state", undefined, signal),

  // Motion — semantic commands only.
  stopMotion: () => request<{ error?: string }>("POST", "/api/motion/stop", {}),
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

  // Shared-engine modes. Freestyle is surfaced in Preset Modes; Autopilot is
  // surfaced with its conversation in Chat.
  getModes: () => request("GET", "/api/modes"),
  startMode: (mode: string, options?: Record<string, unknown>) =>
    request("POST", "/api/modes/start", { mode, ...(options ?? {}) }),
  stopMode: () => request("POST", "/api/modes/stop", {}),

  // Pattern library. Point arrays and previews are loaded only in this route;
  // the regular state poll carries counts, not content documents.
  getLibrary: (signal?: AbortSignal) => request<{ library: PatternLibrary }>("GET", "/api/library", undefined, signal),
  previewPattern: (body: PatternInput, signal?: AbortSignal) => request<{ preview: PatternPreview }>("POST", "/api/library/preview", body, signal),
  createPattern: (body: PatternInput) => request<{ pattern: LibraryPattern }>("POST", "/api/library/patterns", body),
  patchPattern: (id: string, patch: Partial<LibraryPattern>) =>
    request<{ pattern: LibraryPattern }>("PATCH", `/api/library/patterns/${encodeURIComponent(id)}`, patch),
  deletePattern: (id: string) => request("DELETE", `/api/library/patterns/${encodeURIComponent(id)}`),
  playPattern: (id: string, intensity: number, feel = "original") =>
    request("POST", `/api/library/patterns/${encodeURIComponent(id)}/play`, { intensity, feel }),
  deleteProgram: (id: string) => request("DELETE", `/api/library/programs/${encodeURIComponent(id)}`),
  playProgram: (id: string, intensity: number) =>
    request("POST", `/api/library/programs/${encodeURIComponent(id)}/play`, { intensity }),
  patternFeedback: (pattern_id: string, rating: -1 | 1) =>
    request<{ feedback: PatternFeedback; pattern: LibraryPattern }>("POST", "/api/library/feedback", { pattern_id, rating }),
  undoPatternFeedback: (id: number) =>
    request<{ feedback: PatternFeedback; pattern: LibraryPattern }>("POST", `/api/library/feedback/${id}/undo`),
  setPatternAutoDisable: (enabled: boolean) =>
    request<{ auto_disable: boolean }>("PUT", "/api/library/auto-disable", { enabled }),
  importMotionContent: (file: File, asKind: "pattern" | "program") => importMotionContent(file, asKind),
  exportPattern: (id: string) => download(`/api/library/patterns/${encodeURIComponent(id)}/export`),
  exportProgram: (id: string) => download(`/api/library/programs/${encodeURIComponent(id)}/export`),

  // Local video catalog. Streaming takes opaque catalog IDs and stays
  // read-only; scans and metadata backfill require the active controller.
  mediaVideos: (signal?: AbortSignal) => request<{ videos: MediaVideo[] }>("GET", "/api/media/videos", undefined, signal),
  mediaScan: (signal?: AbortSignal) => request<{ scan: MediaScanState }>("GET", "/api/media/scan", undefined, signal),
  startMediaScan: () => request<{ scan: MediaScanState }>("POST", "/api/media/scan", {}),
  cancelMediaScan: () => request<{ scan: MediaScanState }>("DELETE", "/api/media/scan"),
  saveMediaDuration: (id: string, duration_ms: number) =>
    request<{ status: string }>("POST", "/api/media/duration", { id, duration_ms }),
  mediaStreamURL: (id: string) => `/api/media/videos/${encodeURIComponent(id)}/stream`,
  mediaFunscript: (id: string, signal?: AbortSignal) =>
    request<{ funscript: MediaFunscript }>("GET", `/api/media/videos/${encodeURIComponent(id)}/funscript`, undefined, signal),
  mediaSync: (event: MediaSyncEvent, stopSequence?: number, signal?: AbortSignal, keepalive = false) =>
    request<{ sync: MediaSyncStatus }>(
      "POST",
      "/api/media/sync",
      event,
      signal,
      stopSequence === undefined ? undefined : { [STOP_SEQUENCE_HEADER]: String(stopSequence) },
      keepalive,
    ),

  // Memory.
  getMemory: () => request<MemoryState>("GET", "/api/memory"),
  addMemory: (text: string) => request<MemoryState>("POST", "/api/memory", { text }),
  setMemoryEnabled: (enabled: boolean) => request("POST", "/api/memory/enabled", { enabled }),
  setMemoryItemEnabled: (id: string, enabled: boolean) =>
    request("PATCH", `/api/memory/${encodeURIComponent(id)}`, { enabled }),
  removeMemory: (id: string) => request("DELETE", `/api/memory/${encodeURIComponent(id)}`),
  clearMemory: () => request("POST", "/api/memory/clear", {}),

  // Prompt sets.
  getPromptSets: () => request<PromptSetsPayload>("GET", "/api/prompt-sets"),
  createPromptSet: (name: string, system: string) =>
    request<PromptSetsPayload>("POST", "/api/prompt-sets", { name, system }),
  updatePromptSet: (id: string, name: string, system: string) =>
    request<PromptSetsPayload>("PUT", `/api/prompt-sets/${encodeURIComponent(id)}`, { name, system }),
  deletePromptSet: (id: string) => request<PromptSetsPayload>("DELETE", `/api/prompt-sets/${encodeURIComponent(id)}`),

  // LLM runtime.
  llmStatus: () => request<LLMProviderStatus>("GET", "/api/llm/status"),
  llmLoad: () => request<LLMProviderStatus>("POST", "/api/llm/load", {}),
  llmUnload: () => request<LLMProviderStatus>("POST", "/api/llm/unload", {}),
  llmModels: () => request<LLMModelManagerSnapshot>("GET", "/api/llm/models"),
  buildManagedLlamaRuntime: (backend: "auto" | "cpu" | "cuda") =>
    request<{ build: ManagedLlamaRuntimeBuild }>("POST", "/api/llm/runtime/build", { backend }),
  cancelManagedLlamaRuntimeBuild: () =>
    request<{ build: ManagedLlamaRuntimeBuild }>("DELETE", "/api/llm/runtime/build"),
  ollamaModels: () => request<{ available: boolean; models: OllamaModelInfo[]; message?: string }>("GET", "/api/llm/ollama/models"),
  scanOllamaModels: (path: string) => request<OllamaModelScan>("POST", "/api/llm/ollama/scan", { path }),
  importOllamaModel: (path: string, candidate_id: string) =>
    request<{ import: LLMModelImport }>("POST", "/api/llm/imports/ollama", { path, candidate_id }),
  importGGUFModel: (path: string, display_name: string) =>
    request<{ import: LLMModelImport }>("POST", "/api/llm/imports/gguf", { path, display_name }),
  llmImport: (id: string) => request<{ import: LLMModelImport }>("GET", `/api/llm/imports/${encodeURIComponent(id)}`),
  cancelLLMImport: (id: string) => request<{ import: LLMModelImport }>("DELETE", `/api/llm/imports/${encodeURIComponent(id)}`),
  deleteLLMModel: (id: string) => request<null>("DELETE", `/api/llm/models/${encodeURIComponent(id)}`),

  // Settings.
  getSettings: () => request<{ settings: PublicSettings }>("GET", "/api/settings"),
  saveSettings: (update: SettingsUpdate) => request("PUT", "/api/settings", update),
  pickHostPath: (kind: "executable" | "gguf" | "wav" | "npy" | "neutts_codes" | "file" | "directory", current: string) =>
    request<{ path: string; canceled: boolean }>("POST", "/api/host/path-picker", { kind, current }),
  saveConnectionKey: (connection_key: string) =>
    request<{ settings: PublicSettings }>("PUT", "/api/settings/device/connection-key", { connection_key }),
  resetSettings: () => request<{ settings: PublicSettings }>("POST", "/api/settings/reset", {}),

  // Provider checks are diagnostic-only. Cloud Connect/Disconnect own the
  // controller-gated command lifecycle.
  connectionCheck: (owner: "cloud" | "bluetooth") =>
    request<ConnectionCheckResult>(`POST`, `/api/transport/${owner}/check`, {}),
  cloudConnect: () => request<ConnectionCheckResult>("POST", "/api/transport/cloud/connect", {}),
  cloudDisconnect: () => request<CloudDisconnectResponse>("POST", "/api/transport/cloud/disconnect", {}),

  // Intiface Central session. Device indices are scoped to this discovery
  // session, so selection deliberately remains runtime state rather than a setting.
  intifaceStatus: () => request<IntifaceTransportSnapshot>("GET", "/api/transport/intiface/status"),
  intifaceConnect: () => request<IntifaceTransportSnapshot>("POST", "/api/transport/intiface/connect", {}),
  intifaceDisconnect: () => request<IntifaceTransportSnapshot>("POST", "/api/transport/intiface/disconnect", {}),
  intifaceStartScan: () => request<IntifaceTransportSnapshot>("POST", "/api/transport/intiface/scan", {}),
  intifaceStopScan: () => request<IntifaceTransportSnapshot>("DELETE", "/api/transport/intiface/scan"),
  intifaceSelect: (device_index: number, actuator_index: number) =>
    request<IntifaceTransportSnapshot>("POST", "/api/transport/intiface/select", { device_index, actuator_index }),

  // Browser Bluetooth bridge. React owns only the browser/device session; all
  // motion commands still come from backend bridge commands.
  bluetoothStatus: () => request<BluetoothStatusResponse>("GET", "/api/transport/bluetooth/status"),
  postBluetoothStatus: (status: BluetoothClientStatus) =>
    request<BluetoothStatusResponse>("POST", "/api/transport/bluetooth/status", status),
  bluetoothConnect: (status: BluetoothClientStatus) =>
    request<BluetoothStatusResponse>("POST", "/api/transport/bluetooth/connect", status),
  bluetoothDisconnect: (client_id: string, message?: string) =>
    request<BluetoothStatusResponse>("POST", "/api/transport/bluetooth/disconnect", { client_id, message }),
  bluetoothCommands: (bridgeClientId: string, waitSeconds: number, signal?: AbortSignal) =>
    requestWithSignal<BluetoothCommandsResponse>(
      "GET",
      `/api/transport/bluetooth/commands?client_id=${encodeURIComponent(bridgeClientId)}&wait=${waitSeconds}`,
      signal,
    ),
  bluetoothAck: (bridgeClientId: string, payload: BluetoothAckPayload) =>
    request<{ status: string; bluetooth: BluetoothStatusResponse["bluetooth"] }>("POST", "/api/transport/bluetooth/ack", {
      client_id: bridgeClientId,
      ...payload,
    }),

  // Backend-owned chat sessions and their non-destructive per-client cursors.
  getChatSessions: () => request<ChatSessionsResponse>("GET", "/api/chat/sessions"),
  createChatSession: (discardCurrentUnsaved = false) =>
    request<ChatSessionsResponse>("POST", "/api/chat/sessions", { discard_current_unsaved: discardCurrentUnsaved }),
  activateChatSession: (sessionId: string, discardCurrentUnsaved = false) => {
    const query = discardCurrentUnsaved ? "?discard_current_unsaved=true" : "";
    return request<ChatSessionsResponse>("PUT", `/api/chat/sessions/${encodeURIComponent(sessionId)}/active${query}`, {});
  },
  saveChatSession: (sessionId: string) =>
    request<ChatSessionsResponse>("PUT", `/api/chat/sessions/${encodeURIComponent(sessionId)}/save`, {}),
  deleteChatSession: (sessionId: string) =>
    request<ChatSessionsResponse>("DELETE", `/api/chat/sessions/${encodeURIComponent(sessionId)}`),
  getChatMessages: (sessionId: string, after = 0) => {
    const query = new URLSearchParams({ session_id: sessionId });
    if (after > 0) query.set("after", String(after));
    return request<ChatMessagesResponse>("GET", `/api/chat/messages?${query.toString()}`);
  },
  advanceChatCursor: (sessionId: string, seq: number) =>
    request<{ cursor: number; session_id: string }>("POST", "/api/chat/cursor", { session_id: sessionId, seq }),

  // Voice workers (optional; the app runs fully without them).
  voiceStatus: () =>
    request<{ voice: VoiceState; requests?: VoiceRequestSnapshot[] }>("GET", "/api/voice/status"),
  voiceWorkerStart: (role: "tts" | "asr") =>
    request<{ worker: VoiceWorkerStatus }>("POST", `/api/voice/workers/${role}/start`),
  voiceWorkerStop: (role: "tts" | "asr") =>
    request<{ worker: VoiceWorkerStatus }>("POST", `/api/voice/workers/${role}/stop`),
  voiceWorkerRestart: (role: "tts" | "asr") =>
    request<{ worker: VoiceWorkerStatus }>("POST", `/api/voice/workers/${role}/restart`),
  voiceWorkerModel: (role: "tts" | "asr", loaded: boolean) =>
    request<{ model_state?: string; worker: VoiceWorkerStatus }>("POST", `/api/voice/workers/${role}/model`, { loaded }),
  voiceWorkerTest: (role: "tts" | "asr", body: { text: string; delay_ms: number }) =>
    request<{ request: VoiceRequestSnapshot }>("POST", `/api/voice/workers/${role}/test`, body),
  voiceRequest: (id: string, signal?: AbortSignal) =>
    request<{ request: VoiceRequestSnapshot }>("GET", `/api/voice/requests/${encodeURIComponent(id)}`, undefined, signal),
  voiceRequestCancel: (id: string) =>
    request<{ request: VoiceRequestSnapshot }>("POST", `/api/voice/requests/${encodeURIComponent(id)}/cancel`),
  voiceTranscribe: (audio: Blob, format: string, stopSequence?: number, signal?: AbortSignal) => uploadVoiceTranscription(audio, format, stopSequence, signal),
  saveVoicePreferences: (speak_replies: boolean) =>
    request<{ speak_replies: boolean }>("PUT", "/api/voice/preferences", { speak_replies }),
  saveVoiceInputPreferences: (patch: Partial<{
    input_mode: "hands_free" | "hold";
    input_sensitivity: number;
    input_silence_ms: number;
    input_noise_suppression: boolean;
  }>) => request<{
    input_mode: "hands_free" | "hold";
    input_sensitivity: number;
    input_silence_ms: number;
    input_noise_suppression: boolean;
  }>("PUT", "/api/voice/input-preferences", patch),
  generateNeuTTSReference: (reference_wav: string, transcript: string, signal?: AbortSignal) =>
    request<{ reference: NeuTTSReference; preview_url: string }>(
      "POST",
      "/api/voice/neutts/references",
      { reference_wav, transcript },
      signal,
    ),
  // Lease-gated audio: only the active controller may fetch a clip.
  voiceRequestAudio: async (id: string, signal?: AbortSignal): Promise<Blob> => {
    const res = await fetch(`/api/voice/requests/${encodeURIComponent(id)}/audio`, {
      headers: { [CLIENT_HEADER]: clientId },
      signal,
    });
    if (!res.ok) throw new ApiError(`Audio fetch failed (${res.status})`, res.status, null);
    return res.blob();
  },

  exportTrace: () => request("GET", "/api/traces"),
};

async function importMotionContent(file: File, asKind: "pattern" | "program"): Promise<{ import: MotionImportResult }> {
  const path = `/api/library/import?filename=${encodeURIComponent(file.name)}&as=${asKind}`;
  const res = await fetch(path, {
    method: "POST",
    headers: { Accept: "application/json", "Content-Type": "application/json", [CLIENT_HEADER]: clientId },
    body: file,
  });
  const text = await res.text();
  let parsed: unknown = null;
  if (text) {
    try {
      parsed = JSON.parse(text) as unknown;
    } catch {
      parsed = { error: text };
    }
  }
  if (!res.ok) {
    const message = parsed && typeof parsed === "object" && "error" in parsed ? String((parsed as { error: unknown }).error) : `Import failed (${res.status})`;
    throw new ApiError(message, res.status, parsed);
  }
  return parsed as { import: MotionImportResult };
}

async function download(path: string): Promise<{ blob: Blob; filename: string }> {
  const res = await fetch(path, { headers: { [CLIENT_HEADER]: clientId } });
  if (!res.ok) throw new ApiError(`Export failed (${res.status})`, res.status, null);
  const disposition = res.headers.get("Content-Disposition") ?? "";
  const match = disposition.match(/filename="?([^";]+)"?/i);
  return { blob: await res.blob(), filename: match?.[1] ?? "motion-content.json" };
}

async function requestWithSignal<T>(method: string, path: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(path, { method, headers: { Accept: "application/json", [CLIENT_HEADER]: clientId }, signal });
  const text = await res.text();
  let parsed: unknown = null;
  if (text) {
    try {
      parsed = JSON.parse(text) as unknown;
    } catch {
      parsed = { error: text };
    }
  }
  if (!res.ok) {
    const message = parsed && typeof parsed === "object" && "error" in parsed ? String((parsed as { error: unknown }).error) : `Request failed (${res.status})`;
    throw new ApiError(message, res.status, parsed);
  }
  return parsed as T;
}

// Chat is a POST SSE stream; parse named events off the response body.
export async function streamChat(
  sessionId: string,
  message: string,
  history: ChatHistoryMessage[],
  onEvent: (e: ChatStreamEvent) => void,
  signal?: AbortSignal,
  stopSequence?: number,
): Promise<void> {
  const headers: Record<string, string> = { "Content-Type": "application/json", [CLIENT_HEADER]: clientId };
  if (stopSequence !== undefined) headers["X-MagicHandy-Stop-Sequence"] = String(stopSequence);
  const res = await fetch("/api/chat/stream", {
    method: "POST",
    headers,
    body: JSON.stringify({ session_id: sessionId, message, history }),
    signal,
  });
  if (!res.ok) {
    let message = `Chat request failed (${res.status})`;
    try {
      const body = await res.json();
      if (body && typeof body === "object" && "error" in body) message = String((body as { error: unknown }).error);
    } catch {
      // Keep the status-based fallback.
    }
    throw new ApiError(message, res.status, null);
  }
  if (!res.body) throw new ApiError("chat stream unavailable", res.status, null);
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let completed = false;
  const dispatch = (frame: string) => {
    const parsed = parseSSEFrame(frame, res.status);
    if (!parsed) return;
    onEvent(parsed);
    if (parsed.event === "done") completed = true;
  };
  try {
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      for (;;) {
        const boundary = nextSSEBoundary(buffer);
        if (!boundary) break;
        dispatch(buffer.slice(0, boundary.index));
        buffer = buffer.slice(boundary.index + boundary.length);
      }
    }
    buffer += decoder.decode();
    if (buffer.trim()) dispatch(buffer);
  } finally {
    reader.releaseLock();
  }
  if (!completed) throw new ApiError("Chat stream ended before completion.", res.status, null);
}

function nextSSEBoundary(buffer: string): { index: number; length: number } | null {
  const match = /\r\n\r\n|\n\n|\r\r/.exec(buffer);
  return match ? { index: match.index, length: match[0].length } : null;
}

function parseSSEFrame(frame: string, status: number): ChatStreamEvent | null {
  let event = "message";
  const dataLines: string[] = [];
  for (const line of frame.split(/\r\n|\r|\n/)) {
    if (!line || line.startsWith(":")) continue;
    const colon = line.indexOf(":");
    const field = colon === -1 ? line : line.slice(0, colon);
    let value = colon === -1 ? "" : line.slice(colon + 1);
    if (value.startsWith(" ")) value = value.slice(1);
    if (field === "event") event = value;
    if (field === "data") dataLines.push(value);
  }
  if (!dataLines.length) return null;
  try {
    return { event, data: JSON.parse(dataLines.join("\n")) } as ChatStreamEvent;
  } catch {
    throw new ApiError("Chat stream contained malformed JSON.", status, null);
  }
}
