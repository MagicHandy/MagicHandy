import type {
  AppSettings,
  BluetoothAckPayload,
  BluetoothBridgeSnapshot,
  BluetoothClientStatus,
  BluetoothCommandsResponse,
  BluetoothStatusResponse,
  ChatHistoryMessage,
  ChatMessage,
  ChatStreamEvent,
  ControllerSnapshot,
  DeviceTransport,
  FunscriptFileEntry,
  ImportResult,
  ManualQueueDraft,
  ManualQueuePreview,
  MemoryState,
  MotionBlock,
  MotionVisual,
  OperationMode,
  Persona,
  PromptSetsPayload,
  SavedQueueSummary,
  SessionRow,
  SignalPreset,
  StatusSnapshot,
  UIPreferences,
  VoiceStatus,
} from "./types";
import { controllerHeaders } from "../lib/controllerClient";

const API_BASE = "/api";

export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public detail?: unknown,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

function extractErrorMessage(data: unknown, status: number): string {
  if (typeof data === "string" && data.trim()) return data;
  if (typeof data === "object" && data !== null) {
    const record = data as Record<string, unknown>;
    if (typeof record.error === "string" && record.error.trim()) {
      return record.error;
    }
    if (typeof record.detail === "string" && record.detail.trim()) {
      return record.detail;
    }
    if (typeof record.message === "string" && record.message.trim()) {
      return record.message;
    }
  }
  return `HTTP ${status}`;
}

async function request<T>(
  path: string,
  init?: RequestInit,
): Promise<T> {
  const url = path.startsWith("/") ? `${API_BASE}${path}` : `${API_BASE}/${path}`;
  const res = await fetch(url, {
    ...init,
    headers: {
      Accept: "application/json",
      ...controllerHeaders(),
      ...(init?.body instanceof FormData
        ? {}
        : { "Content-Type": "application/json" }),
      ...init?.headers,
    },
  });

  const text = await res.text();
  let data: unknown = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = text;
    }
  }

  if (!res.ok) {
    throw new ApiError(extractErrorMessage(data, res.status), res.status, data);
  }

  return data as T;
}

async function bluetoothRequest<T>(path: string, signal?: AbortSignal): Promise<T> {
  const url = path.startsWith("/") ? `${API_BASE}${path}` : `${API_BASE}/${path}`;
  const res = await fetch(url, {
    method: "GET",
    headers: { Accept: "application/json", ...controllerHeaders() },
    signal,
  });
  const text = await res.text();
  let data: unknown = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = text;
    }
  }
  if (!res.ok) {
    throw new ApiError(extractErrorMessage(data, res.status), res.status, data);
  }
  return data as T;
}

/** POST SSE chat stream from /api/chat/stream */
export async function streamChat(
  message: string,
  history: ChatHistoryMessage[],
  onEvent: (e: ChatStreamEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  const res = await fetch(`${API_BASE}/chat/stream`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "text/event-stream",
      ...controllerHeaders(),
    },
    body: JSON.stringify({ message, history }),
    signal,
  });
  if (!res.ok) {
    let errMsg = `Chat request failed (${res.status})`;
    try {
      const body = await res.json();
      if (body && typeof body === "object" && "error" in body) {
        errMsg = String((body as { error: unknown }).error);
      }
    } catch {
      /* keep fallback */
    }
    throw new ApiError(errMsg, res.status);
  }
  if (!res.body) throw new ApiError("chat stream unavailable", res.status);
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

export const api = {
  getStatus: () => request<StatusSnapshot>("/status"),

  emergencyStop: (reason = "ui_stop") =>
    request<{ ok: boolean }>("/emergency-stop", {
      method: "POST",
      body: JSON.stringify({ reason }),
    }),

  clearEmergencyStop: () =>
    request<{ ok: boolean }>("/emergency-stop/clear", { method: "POST" }),

  getChatMessages: () =>
    request<{ messages: ChatMessage[]; last_persona_message: string }>(
      "/chat/messages",
    ),

  sendChat: (text: string) =>
    request<{
      ok: boolean;
      reply?: string;
      pending?: boolean;
      response?: unknown;
      stopped?: boolean;
      error?: string;
    }>("/chat/send", { method: "POST", body: JSON.stringify({ text }) }),

  setOperationMode: async (mode: OperationMode) => {
    const settings = await request<AppSettings>("/settings");
    const app = (settings.app ?? {}) as Record<string, unknown>;
    return request<{ ok: boolean }>("/settings", {
      method: "PUT",
      body: JSON.stringify({
        updates: { app: { ...app, operation_mode: mode } },
      }),
    });
  },

  startAuto: () => request<{ ok: boolean }>("/auto/start", { method: "POST" }),
  stopAuto: () => request<{ ok: boolean }>("/auto/stop", { method: "POST" }),
  toggleAutospeak: (enabled: boolean) =>
    request<{
      ok: boolean;
      autospeak_enabled: boolean;
      autospeak_scheduled?: boolean;
    }>("/autospeak/toggle", {
      method: "POST",
      body: JSON.stringify({ enabled }),
    }),

  startHandsFree: () =>
    request<{ ok: boolean; intensity?: number }>("/hands-free/start", {
      method: "POST",
    }),
  stopHandsFree: () =>
    request<{ ok: boolean }>("/hands-free/stop", { method: "POST" }),

  getModes: () =>
    request<{
      active?: boolean;
      running?: boolean;
      mode?: string;
      active_mode?: string;
      style?: string;
    }>("/modes"),
  startMode: (mode: string) =>
    request("/modes/start", {
      method: "POST",
      body: JSON.stringify({ mode }),
    }),
  stopMode: () =>
    request("/modes/stop", {
      method: "POST",
      body: JSON.stringify({}),
    }),
  applyQuick: (patch: { style?: string }) =>
    request("/motion/quick", {
      method: "POST",
      body: JSON.stringify(patch),
    }),
  handsFreeSignal: (signal: string) =>
    request<{
      ok: boolean;
      signal?: string;
      intensity?: number;
      enqueued?: number;
      phase?: string;
    }>("/hands-free/signal", {
      method: "POST",
      body: JSON.stringify({ signal }),
    }),
  startPlayback: () =>
    request<{ ok: boolean; buffer_sec?: number }>("/playback/start", {
      method: "POST",
    }),
  stopPlayback: () =>
    request<{ ok: boolean }>("/playback/stop", { method: "POST" }),
  refillPlayback: () =>
    request<{ ok: boolean; buffer_sec?: number; queue_blocks?: number }>(
      "/playback/refill",
      { method: "POST" },
    ),

  getQueue: () =>
    request<{
      blocks: {
        block_id: string;
        duration_ms: number;
        intensity: number;
        bpm?: number;
        loop_pattern?: boolean;
        source?: string;
        enqueue_seq?: number;
      }[];
      buffer_sec: number;
      buffer_remaining_sec?: number;
      count: number;
      playback_active?: boolean;
    }>("/queue"),

  clearQueue: () =>
    request<{ ok: boolean; removed: number }>("/queue/clear", { method: "POST" }),

  clearQueueAndRefill: () =>
    request<{
      ok: boolean;
      removed: number;
      buffer_sec: number;
      queue_blocks: number;
    }>("/queue/clear-and-refill", { method: "POST" }),

  applyMotionPreset: (preset: "slow" | "medium" | "fast") =>
    request<{ ok: boolean; preset: string }>("/settings/motion-preset", {
      method: "POST",
      body: JSON.stringify({ preset }),
    }),

  deletePatternsBatch: (block_ids: string[]) =>
    request<{ ok: boolean; removed: number }>("/patterns/delete-batch", {
      method: "POST",
      body: JSON.stringify({ block_ids }),
    }),

  deletePatternsBySource: (source_file_id: string) =>
    request<{ ok: boolean; removed: number; source_file_id: string }>(
      `/patterns/by-source/${encodeURIComponent(source_file_id)}`,
      { method: "DELETE" },
    ),

  listPersonas: () =>
    request<{ personas: Persona[]; active_persona_id: string }>("/personas"),

  getPersona: (id: string) => request<Persona>(`/personas/${id}`),

  createPersona: (body: Partial<Persona>) =>
    request<Persona>("/personas", {
      method: "POST",
      body: JSON.stringify(body),
    }),

  updatePersona: (id: string, body: Partial<Persona>) =>
    request<Persona>(`/personas/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),

  deletePersona: (id: string) =>
    request<{ ok: boolean }>(`/personas/${id}`, { method: "DELETE" }),

  activatePersona: (id: string) =>
    request<{ ok: boolean }>(`/personas/${id}/activate`, { method: "POST" }),

  listPatterns: (params: Record<string, string | number | boolean>) => {
    const q = new URLSearchParams();
    Object.entries(params).forEach(([k, v]) => {
      if (v !== "" && v !== undefined && v !== null) q.set(k, String(v));
    });
    return request<{ blocks: MotionBlock[]; total?: number }>(`/patterns?${q}`);
  },

  getPatternMeta: () =>
    request<{
      categories: { id: string; label: string }[];
      zones: { id: string; label: string }[];
      speeds: { id: string; label: string }[];
      rhythms: { id: string; label: string }[];
      stroke_lengths: { id: string; label: string }[];
    }>("/patterns/meta"),

  countPatterns: (params: Record<string, string | number | boolean>) => {
    const q = new URLSearchParams();
    Object.entries(params).forEach(([k, v]) => {
      if (v !== "" && v !== undefined && v !== null) q.set(k, String(v));
    });
    return request<{ total: number; count?: number }>(`/patterns/count?${q}`);
  },

  listPatternIds: (params: Record<string, string | number | boolean>) => {
    const q = new URLSearchParams();
    Object.entries(params).forEach(([k, v]) => {
      if (v !== "" && v !== undefined && v !== null) q.set(k, String(v));
    });
    return request<{ ids: string[]; total: number; returned: number }>(
      `/patterns/ids?${q}`,
    );
  },

  patchPattern: (
    id: string,
    body: {
      favorite?: number;
      blocked?: number;
      user_rating?: number;
      tags_json?: string;
    },
  ) =>
    request<MotionBlock>(`/patterns/${id}`, {
      method: "PATCH",
      body: JSON.stringify(body),
    }),

  deletePattern: (id: string) =>
    request<{ ok: boolean; deleted: string }>(`/patterns/${id}`, {
      method: "DELETE",
    }),

  patternFeedback: (id: string, feedback: string, note?: string) =>
    request<{ ok: boolean; success_score?: number }>(`/patterns/${id}/feedback`, {
      method: "POST",
      body: JSON.stringify({ feedback, note }),
    }),

  testPatternMock: (id: string) =>
    request<{ ok: boolean; actions_played?: number }>(
      `/patterns/${id}/test-mock`,
      { method: "POST" },
    ),

  testPatternDevice: (id: string) =>
    request<{ ok: boolean; actions_played?: number; device?: string }>(
      `/patterns/${id}/test-device`,
      { method: "POST" },
    ),

  bootstrapDevice: () =>
    request<{ ok: boolean; connected?: boolean; device_label?: string; error?: string }>(
      "/device/bootstrap",
      { method: "POST" },
    ),

  exportPattern: (id: string, format: string) =>
    request<{ filename: string; content: string }>(
      `/patterns/${id}/export?format=${encodeURIComponent(format)}`,
    ),

  getSettings: () => request<AppSettings>("/settings"),

  saveSettings: (updates: AppSettings) =>
    request<{ ok: boolean }>("/settings", {
      method: "PUT",
      body: JSON.stringify({ updates }),
    }),

  setMockDevice: (enabled: boolean) =>
    request<{
      ok: boolean;
      use_mock?: boolean;
      connected?: boolean;
      device_label?: string;
      warning?: string;
    }>("/device/mock", {
      method: "POST",
      body: JSON.stringify({ enabled }),
    }),

  connectDevice: () =>
    request<{ ok: boolean }>("/device/connect", { method: "POST" }),

  scanDevices: () =>
    request<{ devices: { device_id: string; name: string; has_linear: boolean }[] }>(
      "/device/scan",
      { method: "POST" },
    ),

  selectDevice: (device_id: string) =>
    request<{ ok: boolean }>("/device/select", {
      method: "POST",
      body: JSON.stringify({ device_id }),
    }),

  setDeviceTransport: (transport: DeviceTransport, connection_key?: string) =>
    request<{
      ok: boolean;
      transport?: DeviceTransport;
      handy_key_configured?: boolean;
    }>("/device/transport", {
      method: "POST",
      body: JSON.stringify({ transport, connection_key }),
    }),

  listSessions: () => request<{ sessions: SessionRow[] }>("/sessions"),

  getSession: (id: string) =>
    request<Record<string, unknown>>(`/sessions/${encodeURIComponent(id)}`),

  exportSession: async (id: string) => {
    const url = `/api/sessions/${encodeURIComponent(id)}/export`;
    const res = await fetch(url);
    if (!res.ok) {
      const text = await res.text();
      throw new ApiError(text || `HTTP ${res.status}`, res.status);
    }
    const blob = await res.blob();
    const disp = res.headers.get("Content-Disposition") ?? "";
    const match = /filename="([^"]+)"/.exec(disp);
    const filename = match?.[1] ?? `lso_session_${id.slice(0, 8)}.zip`;
    return { filename, blob };
  },

  sessionFeedback: (id: string, rating: number, notes?: string) =>
    request<{ ok: boolean }>(`/sessions/${id}/feedback`, {
      method: "POST",
      body: JSON.stringify({ rating, notes }),
    }),

  getVoiceStatus: () => request<VoiceStatus>("/voice/status"),

  speakVoice: async (text: string) => {
    const res = await fetch(`${API_BASE}/voice/speak`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "audio/mpeg" },
      body: JSON.stringify({ text }),
    });
    if (!res.ok) {
      const errText = await res.text();
      throw new ApiError(errText || `HTTP ${res.status}`, res.status);
    }
    return res.blob();
  },

  transcribeVoice: async (blob: Blob, filename = "recording.webm") => {
    const form = new FormData();
    form.append("audio", blob, filename);
    const res = await fetch(`${API_BASE}/voice/transcribe`, {
      method: "POST",
      body: form,
    });
    const text = await res.text();
    let data: unknown = null;
    if (text) {
      try {
        data = JSON.parse(text);
      } catch {
        data = text;
      }
    }
    if (!res.ok) {
      const detail =
        typeof data === "object" && data && "detail" in data
          ? String((data as { detail: unknown }).detail)
          : text;
      throw new ApiError(detail || `HTTP ${res.status}`, res.status);
    }
    return data as { ok: boolean; text: string };
  },

  reclassifyPatterns: () =>
    request<{
      ok: boolean;
      updated: number;
      skipped: number;
      tag_distribution?: Record<string, number>;
    }>("/patterns/reclassify", { method: "POST" }),

  recalculateBlockTimings: () =>
    request<{
      ok: boolean;
      updated: number;
      unchanged: number;
      skipped: number;
      trimmed_actions: number;
    }>("/patterns/recalculate-timings", { method: "POST" }),

  saveEditedBlock: (
    blockId: string,
    body: { actions: { at: number; pos: number }[]; mode: "replace" | "new" },
  ) =>
    request<{
      ok: boolean;
      mode: string;
      source_block_id: string;
      block_id: string;
      action_count: number;
      duration_ms: number;
      block?: MotionBlock;
    }>(`/patterns/${encodeURIComponent(blockId)}/edit`, {
      method: "POST",
      body: JSON.stringify(body),
    }),

  importFile: async (file: File) => {
    const fd = new FormData();
    fd.append("file", file);
    return request<ImportResult>("/import", { method: "POST", body: fd });
  },

  exportImport: (fileId: string, format: string) =>
    request<{ filename: string; content: string }>(
      `/import/${fileId}/export?format=${encodeURIComponent(format)}`,
    ),

  listImports: (limit = 50) =>
    request<{ files: FunscriptFileEntry[] }>(`/import?limit=${limit}`),

  enqueueFullScript: (fileId: string) =>
    request<{ ok: boolean; file_id: string; enqueued: number }>(
      `/import/${encodeURIComponent(fileId)}/enqueue`,
      { method: "POST" },
    ),

  playFullScript: (fileId: string) =>
    request<{
      ok: boolean;
      file_id: string;
      filename: string;
      actions_played: number;
      duration_ms: number;
    }>(`/import/${encodeURIComponent(fileId)}/play`, { method: "POST" }),

  planRosterSession: (body: {
    message?: string;
    duration_min?: number;
    intensity_preset?: "leve" | "medio" | "rapido" | "intenso";
    include_buildup?: boolean;
  } = {}) =>
    request<{
      ok: boolean;
      enqueued?: number;
      roster?: Record<string, unknown>;
      response?: unknown;
    }>("/sessions/roster/plan", {
      method: "POST",
      body: JSON.stringify({
        message: body.message ?? "",
        duration_min: body.duration_min ?? 15,
        intensity_preset: body.intensity_preset ?? "medio",
        include_buildup: body.include_buildup ?? true,
      }),
    }),

  getDiagnostics: () => request<Record<string, unknown>>("/diagnostics"),

  getHandyLog: (limit = 100) =>
    request<{
      path: string;
      count: number;
      entries: Record<string, unknown>[];
    }>(`/diagnostics/handy-log?limit=${limit}`),

  pingOllama: () =>
    request<{
      ok: boolean;
      error?: string;
      body_preview?: string;
      provider?: string;
      llm_provider?: string;
      llm_connected?: boolean;
      llm_error?: string | null;
      ollama_connected?: boolean;
      ollama_error?: string | null;
      loaded?: boolean;
      managed?: boolean;
    }>("/diagnostics/ping-ollama", { method: "POST" }),

  getMotionVisual: () => request<MotionVisual>("/motion/visual"),

  setSyncOffset: (offset_ms: number) =>
    request<{ ok: boolean; offset_ms: number }>("/motion/sync-offset", {
      method: "PUT",
      body: JSON.stringify({ offset_ms }),
    }),

  autoSync: () =>
    request<{
      ok: boolean;
      offset_ms: number;
      measured_rtt_ms: number;
      device_latency_ms: number;
      client_latency_ms: number;
    }>("/motion/auto-sync", { method: "POST" }),

  getDirectControlStatus: () =>
    request<{
      ok: boolean;
      active: boolean;
      min_pct: number;
      max_pct: number;
      transport?: string;
      recording?: boolean;
      recording_action_count?: number;
      recording_duration_ms?: number;
      limits_enabled?: boolean;
      max_duration_ms?: number;
      min_send_interval_ms?: number;
    }>("/motion/direct/status"),

  startDirectControl: () =>
    request<{
      ok: boolean;
      min_pct: number;
      max_pct: number;
      transport: string;
      limits_enabled?: boolean;
    }>("/motion/direct/start", { method: "POST" }),

  stopDirectControl: () =>
    request<{
      ok: boolean;
      saved_recording?: {
        block_id: string;
        file_id: string;
        display_title: string;
        duration_ms: number;
        action_count: number;
      };
    }>("/motion/direct/stop", { method: "POST" }),

  startDirectRecording: () =>
    request<{ ok: boolean; recording: boolean; action_count: number }>(
      "/motion/direct/recording/start",
      { method: "POST" },
    ),

  stopDirectRecording: (title?: string) =>
    request<{
      ok: boolean;
      recording: boolean;
      block_id: string;
      file_id: string;
      display_title: string;
      duration_ms: number;
      action_count: number;
      favorite: boolean;
    }>("/motion/direct/recording/stop", {
      method: "POST",
      body: JSON.stringify({ title: title ?? null }),
    }),

  sendDirectControlMove: (normalized: number, duration_ms = 66) =>
    request<{
      ok: boolean;
      skipped?: boolean;
      position_pct?: number;
      duration_ms?: number;
    }>("/motion/direct", {
      method: "POST",
      body: JSON.stringify({ normalized, duration_ms }),
    }),

  uploadPersonaAvatar: (personaId: string, file: File) => {
    const form = new FormData();
    form.append("image", file);
    return request<{ ok: boolean; path: string; avatar_url: string | null }>(
      `/personas/${encodeURIComponent(personaId)}/avatar`,
      { method: "POST", body: form },
    );
  },

  getManualQueueDraft: () =>
    request<ManualQueueDraft>("/manual-queue"),

  getManualQueuePreview: () =>
    request<ManualQueuePreview>("/manual-queue/preview"),

  getManualQueueSignalPresets: () =>
    request<{ presets: Record<string, SignalPreset> }>(
      "/manual-queue/signal-presets",
    ),

  setManualQueueDraft: (items: { block_id: string; duration_sec: number; loop: boolean }[]) =>
    request<ManualQueueDraft>("/manual-queue", {
      method: "PUT",
      body: JSON.stringify({ items }),
    }),

  addManualQueueItem: (body: {
    block_id: string;
    duration_sec?: number;
    loop?: boolean;
  }) =>
    request<ManualQueueDraft>("/manual-queue/items", {
      method: "POST",
      body: JSON.stringify(body),
    }),

  patchManualQueueItem: (
    index: number,
    body: { duration_sec?: number; loop?: boolean },
  ) =>
    request<ManualQueueDraft>(`/manual-queue/items/${index}`, {
      method: "PATCH",
      body: JSON.stringify(body),
    }),

  removeManualQueueItem: (index: number) =>
    request<ManualQueueDraft>(`/manual-queue/items/${index}`, {
      method: "DELETE",
    }),

  reorderManualQueue: (from_index: number, to_index: number) =>
    request<ManualQueueDraft>("/manual-queue/reorder", {
      method: "POST",
      body: JSON.stringify({ from_index, to_index }),
    }),

  clearManualQueue: () =>
    request<ManualQueueDraft>("/manual-queue/clear", { method: "POST" }),

  playManualQueue: () =>
    request<{
      ok: boolean;
      started?: boolean;
      actions_played?: number;
      duration_ms?: number;
      items?: number;
      autoloop?: boolean;
      sync?: { offset_ms?: number; measured_rtt_ms?: number };
    }>("/manual-queue/play", { method: "POST" }),

  pauseManualQueuePlayer: () =>
    request<{ ok: boolean; paused: boolean }>("/manual-queue/player/pause", {
      method: "POST",
    }),

  resumeManualQueuePlayer: () =>
    request<{ ok: boolean; paused: boolean }>("/manual-queue/player/resume", {
      method: "POST",
    }),

  stopManualQueuePlayer: () =>
    request<{ ok: boolean; stopped: boolean }>("/manual-queue/player/stop", {
      method: "POST",
    }),

  skipManualQueueSegment: () =>
    request<{ ok: boolean; skip_to_ms: number }>("/manual-queue/player/skip", {
      method: "POST",
    }),

  setManualQueueAutoloop: (autoloop: boolean) =>
    request<{ ok: boolean; autoloop: boolean }>("/manual-queue/player/options", {
      method: "PATCH",
      body: JSON.stringify({ autoloop }),
    }),

  manualQueueSignal: (body: {
    signal: string;
    block_id?: string;
    duration_sec?: number;
  }) =>
    request<{
      ok: boolean;
      signal?: string;
      block_id?: string;
      duration_sec?: number;
      queued?: boolean;
      started?: boolean;
    }>("/manual-queue/player/signal", {
      method: "POST",
      body: JSON.stringify(body),
    }),

  setBlockSessionRole: (
    blockId: string,
    role: string,
    enabled: boolean,
  ) =>
    request<{ ok: boolean; block_id: string; session_roles: string[] }>(
      `/patterns/${encodeURIComponent(blockId)}/session-role`,
      {
        method: "POST",
        body: JSON.stringify({ role, enabled }),
      },
    ),

  listSavedQueues: () =>
    request<{ queues: SavedQueueSummary[] }>("/manual-queue/saved"),

  saveManualQueue: (name: string) =>
    request<{
      ok: boolean;
      id: string;
      name: string;
      duration_ms: number;
      item_count: number;
    }>("/manual-queue/saved", {
      method: "POST",
      body: JSON.stringify({ name }),
    }),

  loadSavedQueue: (id: string) =>
    request<ManualQueueDraft>(`/manual-queue/saved/${id}/load`, {
      method: "POST",
    }),

  deleteSavedQueue: (id: string) =>
    request<{ ok: boolean; deleted: string }>(`/manual-queue/saved/${id}`, {
      method: "DELETE",
    }),

  playSavedQueue: (id: string) =>
    request<{
      ok: boolean;
      actions_played?: number;
      duration_ms?: number;
      name?: string;
    }>(`/manual-queue/saved/${id}/play`, { method: "POST" }),

  exportManualQueueDraft: (format: string) =>
    request<{ filename: string; content: string }>(
      `/manual-queue/export?format=${encodeURIComponent(format)}`,
    ),

  exportSavedQueue: (id: string, format: string) =>
    request<{ filename: string; content: string }>(
      `/manual-queue/saved/${encodeURIComponent(id)}/export?format=${encodeURIComponent(format)}`,
    ),

  getUIPreferences: () => request<UIPreferences>("/ui/preferences"),

  saveUIPreferences: (body: { locale?: string; locale_prompt_dismissed?: boolean }) =>
    request<UIPreferences>("/ui/preferences", {
      method: "PUT",
      body: JSON.stringify(body),
    }),

  getController: () => request<ControllerSnapshot>("/controller"),

  stopMotion: () =>
    request<{ ok: boolean }>("/motion/stop", { method: "POST", body: JSON.stringify({}) }),

  getMemory: () => request<MemoryState>("/memory"),
  addMemory: (text: string) =>
    request<MemoryState>("/memory", {
      method: "POST",
      body: JSON.stringify({ text }),
    }),
  setMemoryEnabled: (enabled: boolean) =>
    request("/memory/enabled", { method: "POST", body: JSON.stringify({ enabled }) }),
  setMemoryItemEnabled: (id: string, enabled: boolean) =>
    request(`/memory/${encodeURIComponent(id)}`, {
      method: "PATCH",
      body: JSON.stringify({ enabled }),
    }),
  removeMemory: (id: string) =>
    request(`/memory/${encodeURIComponent(id)}`, { method: "DELETE" }),
  clearMemory: () => request("/memory/clear", { method: "POST", body: JSON.stringify({}) }),

  getPromptSets: () => request<PromptSetsPayload>("/prompt-sets"),
  createPromptSet: (name: string, system: string) =>
    request<PromptSetsPayload>("/prompt-sets", {
      method: "POST",
      body: JSON.stringify({ name, system }),
    }),
  updatePromptSet: (id: string, name: string, system: string) =>
    request<PromptSetsPayload>(`/prompt-sets/${encodeURIComponent(id)}`, {
      method: "PUT",
      body: JSON.stringify({ name, system }),
    }),
  deletePromptSet: (id: string) =>
    request<PromptSetsPayload>(`/prompt-sets/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),

  bluetoothStatus: () => request<BluetoothStatusResponse>("/transport/bluetooth/status"),
  postBluetoothStatus: (status: BluetoothClientStatus) =>
    request<BluetoothStatusResponse>("/transport/bluetooth/status", {
      method: "POST",
      body: JSON.stringify(status),
    }),
  bluetoothConnect: (status: BluetoothClientStatus) =>
    request<BluetoothStatusResponse>("/transport/bluetooth/connect", {
      method: "POST",
      body: JSON.stringify(status),
    }),
  bluetoothDisconnect: (client_id: string, message?: string) =>
    request<BluetoothStatusResponse>("/transport/bluetooth/disconnect", {
      method: "POST",
      body: JSON.stringify({ client_id, message }),
    }),
  bluetoothCommands: (bridgeClientId: string, waitSeconds: number, signal?: AbortSignal) =>
    bluetoothRequest<BluetoothCommandsResponse>(
      `/transport/bluetooth/commands?client_id=${encodeURIComponent(bridgeClientId)}&wait=${waitSeconds}`,
      signal,
    ),
  bluetoothAck: (bridgeClientId: string, payload: BluetoothAckPayload) =>
    request<{ status: string; bluetooth: BluetoothBridgeSnapshot }>(
      "/transport/bluetooth/ack",
      {
        method: "POST",
        body: JSON.stringify({ client_id: bridgeClientId, ...payload }),
      },
    ),

};

export function downloadBlob(filename: string, blob: Blob) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

export function downloadText(filename: string, content: string) {
  const blob = new Blob([content], { type: "text/plain;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}
