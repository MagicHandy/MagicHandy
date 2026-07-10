// Types mirror the Go JSON payloads (internal/httpapi, internal/config). They
// are intentionally partial and defensive: the backend snapshot is
// authoritative, so unknown fields are ignored and read sites use optional
// chaining. See docs/decisions/0009-react-frontend.md (State Model Rules).

export type MotionStyle = "gentle" | "balanced" | "intense";

export interface MotionSettings {
  speed_min_percent: number;
  speed_max_percent: number;
  stroke_min_percent: number;
  stroke_max_percent: number;
  reverse_direction: boolean;
  style: string;
}

export interface MotionSample {
  position_percent: number;
  time_millis: number;
}

export interface EngineSnapshot {
  running: boolean;
  paused: boolean;
  running_ms?: number;
  phase?: string;
  target?: { label?: string; speed_percent?: number; pattern_identifier?: string };
  last_sample?: MotionSample;
  settings?: MotionSettings;
  last_error?: string;
}

export interface MotionInfo {
  available: boolean;
  error?: string;
  engine?: EngineSnapshot;
}

export interface ControllerSnapshot {
  client_id?: string;
  active: boolean;
  read_only: boolean;
  reason?: string;
  active_client_id?: string;
}

export interface MemoryItem {
  id: string;
  text: string;
  enabled: boolean;
  created_at: string;
}
export interface MemoryState {
  enabled: boolean;
  memories?: MemoryItem[];
}

export interface PromptSet {
  id: string;
  name: string;
  system: string;
  builtin: boolean;
}

export interface PromptSetsPayload {
  selected?: string;
  default?: string;
  sets?: PromptSet[];
  set?: PromptSet;
}

export interface BluetoothBridgeSnapshot {
  connected?: boolean;
  supported?: boolean;
  ready?: boolean;
  status?: string;
  message?: string;
  device_name?: string;
  pending?: number;
  inflight?: number;
  last_ack?: { ok?: boolean; status?: string; error?: string };
}

export interface BluetoothStatusResponse {
  status: string;
  dispatch_owner: string;
  bluetooth: BluetoothBridgeSnapshot;
  diagnostics?: Record<string, unknown>;
}

export interface BluetoothCommand {
  id: string;
  path: string;
  body?: Record<string, unknown>;
}

export interface BluetoothCommandsResponse {
  status: string;
  commands?: BluetoothCommand[];
  bluetooth: BluetoothBridgeSnapshot;
}

export interface BluetoothClientStatus {
  client_id: string;
  connected: boolean;
  supported: boolean;
  device_name?: string;
  protocol?: string;
  status?: string;
  message?: string;
  error?: string;
}

export interface BluetoothAckPayload {
  id: string;
  ok: boolean;
  status?: string;
  elapsed_ms?: number;
  error?: string;
  response?: Record<string, unknown>;
}

export interface ChatHistoryMessage {
  role: "user" | "assistant";
  content: string;
}

// One row of the server-side shared chat log (the canonical history; each
// client reads via its own cursor and reads are never destructive).
export interface ChatLogMessage {
  seq: number;
  role: "user" | "assistant";
  content: string;
  client_id?: string;
  created_at: string;
}

export interface ChatMessagesResponse {
  messages: ChatLogMessage[];
  latest_seq: number;
  cursor: number;
}

export interface ModesStatus {
  running?: boolean;
  mode?: string;
  active_mode?: string;
  [k: string]: unknown;
}

export interface VoiceSettings {
  enabled: boolean;
  tts_provider: string;
  asr_provider: string;
  tts_worker_path?: string;
  tts_worker_args?: string[];
  asr_worker_path?: string;
  asr_worker_args?: string[];
  speak_replies?: boolean;
  elevenlabs_voice_id?: string;
  elevenlabs_model_id?: string;
  parakeet_server_path?: string;
  parakeet_model_path?: string;
  parakeet_port?: number;
  asr_base_url?: string;
  asr_model?: string;
  neutts_runner_path?: string;
  neutts_reference_wav?: string;
  neutts_reference_codes?: string;
  neutts_reference_text?: string;
  neutts_backbone?: string;
  // Read side only: the stored ElevenLabs key is never returned, just a flag.
  elevenlabs_key_set?: boolean;
}

// Write payload for voice settings: the key is write-only (omit to keep the
// stored secret; clear_elevenlabs_key removes it). Exact shape — the strict
// backend decoder rejects unknown fields like elevenlabs_key_set.
export interface VoiceSettingsUpdate {
  enabled: boolean;
  tts_provider: string;
  asr_provider: string;
  tts_worker_path: string;
  tts_worker_args: string[];
  asr_worker_path: string;
  asr_worker_args: string[];
  speak_replies: boolean;
  elevenlabs_voice_id: string;
  elevenlabs_model_id: string;
  parakeet_server_path: string;
  parakeet_model_path: string;
  parakeet_port: number;
  asr_base_url: string;
  asr_model: string;
  neutts_runner_path: string;
  neutts_reference_wav: string;
  neutts_reference_codes: string;
  neutts_reference_text: string;
  neutts_backbone: string;
  elevenlabs_api_key?: string;
  clear_elevenlabs_key: boolean;
}

export type VoiceWorkerState =
  | "disabled"
  | "not_configured"
  | "stopped"
  | "starting"
  | "running"
  | "crashed"
  | string;

export interface VoiceWorkerStatus {
  role: "tts" | "asr" | string;
  state: VoiceWorkerState;
  configured: boolean;
  command?: string;
  provider?: string;
  provider_version?: string;
  protocol_version?: number;
  capabilities?: string[];
  model_state?: string;
  worker_queue_depth: number;
  queue_depth: number;
  active_request_id?: string;
  started_at?: string;
  last_error?: string;
  stderr_tail?: string;
}

export interface VoiceState {
  enabled: boolean;
  protocol_version: number;
  workers?: Record<string, VoiceWorkerStatus>;
}

export interface VoiceRequestSnapshot {
  id: string;
  role: string;
  type: string;
  state: string;
  created_at: string;
  audio_chunks?: number;
  audio_bytes?: number;
  audio_truncated?: boolean;
  transcript?: { text: string; confidence: number }[];
  rejected?: string;
  error?: { code: string; message: string; retryable?: boolean };
}

export interface OptionHints {
  hsp_dispatch_owners?: string[];
  api_application_id_sources?: string[];
  diagnostics_verbosities?: string[];
  motion_styles?: string[];
  llm_providers?: string[];
  llama_cpp_modes?: string[];
  prompt_sets?: string[];
  tts_providers?: string[];
  asr_providers?: string[];
}

export interface PublicSettings {
  version: number;
  server: { port: number };
  device: {
    hsp_dispatch_owner: string;
    firmware_api_requirement: string;
    api_application_id_source: string;
    api_application_id_override?: string;
    connection_key_set: boolean;
  };
  motion: MotionSettings;
  llm: {
    provider: string;
    llama_cpp_mode: string;
    llama_cpp_base_url: string;
    llama_cpp_runner_path?: string;
    llama_cpp_model_path?: string;
    ollama_base_url: string;
    model: string;
    prompt_set: string;
    request_timeout_ms: number;
  };
  voice: VoiceSettings;
  diagnostics: { verbosity: string };
  options: OptionHints;
}

// One-way update body for PUT /api/settings. handy_connection_key is omitted to
// keep the stored secret; clear_connection_key removes it.
export interface SettingsUpdate {
  server: { port: number };
  device: {
    hsp_dispatch_owner: string;
    firmware_api_requirement: string;
    api_application_id_source: string;
    api_application_id_override: string;
    handy_connection_key?: string;
  };
  motion: MotionSettings;
  llm: PublicSettings["llm"];
  voice: VoiceSettingsUpdate;
  diagnostics: { verbosity: string };
  clear_connection_key: boolean;
}

export interface AppState {
  version?: string;
  commit?: string;
  uptime_seconds?: number;
  data_dir?: string;
  datastore_path?: string;
  settings?: PublicSettings;
  settings_status?: Record<string, unknown>;
  controller?: ControllerSnapshot;
  motion?: MotionInfo;
  modes?: ModesStatus;
  memory?: MemoryState | Record<string, unknown>;
  llm?: Record<string, unknown>;
  voice?: VoiceState;
  chat?: { latest_seq?: number };
  transport?: Record<string, unknown>;
  cloud_transport?: Record<string, unknown>;
  bluetooth_transport?: Record<string, unknown>;
  bluetooth_bridge?: BluetoothBridgeSnapshot;
  trace?: Record<string, unknown>;
}

export interface MotionTarget {
  pattern_identifier?: string;
  speed_percent?: number;
  label?: string;
}

export type ChatStreamEvent =
  | { event: "status"; data: { state: string; provider?: string; model?: string; prompt_set?: string; user_seq?: number } }
  | { event: "delta" | "repair_delta"; data: { phase?: string; text?: string } }
  | { event: "message"; data: { reply?: string; motion?: Record<string, unknown>; initial_malformed?: boolean; seq?: number } }
  | { event: "speech"; data: { request_id?: string } }
  | { event: "motion"; data: { applied?: boolean; action?: string; error?: string } }
  | { event: "malformed"; data: { repaired?: boolean; recoverable?: boolean; phase?: string; error?: string } }
  | { event: "error"; data: { message?: string } }
  | { event: "done"; data: { ok?: boolean; malformed?: boolean } }
  | { event: string; data: Record<string, unknown> };
