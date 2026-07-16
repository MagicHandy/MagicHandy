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
  time_ms: number;
}

export interface EngineSnapshot {
  running: boolean;
  starting?: boolean;
  completing?: boolean;
  paused: boolean;
  running_ms?: number;
  phase?: number;
  recent_command_latency_ms?: number;
  target?: { label?: string; speed_percent?: number; pattern_id?: string; program_id?: string };
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

export interface CurvePoint {
  time_ms: number;
  position_percent: number;
}

export interface LibraryPattern {
  id: string;
  name: string;
  description?: string;
  origin: "builtin" | "user" | "generated" | string;
  kind: "routine" | "burst" | string;
  enabled: boolean;
  weight: number;
  cycle_ms: number;
  points: CurvePoint[];
  preview_samples: CurvePoint[];
  tags: string[];
  created_at: string;
  updated_at: string;
}

export interface LibraryProgram {
  id: string;
  name: string;
  origin: string;
  duration_ms: number;
  points: CurvePoint[];
  preview_samples: CurvePoint[];
  created_at: string;
  updated_at: string;
}

export interface PatternFeedback {
  id: number;
  pattern_id: string;
  rating: -1 | 1;
  weight_before: number;
  weight_after: number;
  enabled_before: boolean;
  enabled_after: boolean;
  reverted: boolean;
  created_at: string;
  reverted_at?: string;
}

export interface PatternLibrary {
  patterns: LibraryPattern[];
  programs: LibraryProgram[];
  feedback: PatternFeedback[];
  auto_disable: boolean;
}

export interface PatternInput {
  name: string;
  description?: string;
  kind: "routine" | "burst";
  cycle_ms: number;
  points: CurvePoint[];
  tags?: string[];
  simplify_error?: number;
}

export interface PatternPreview {
  points: CurvePoint[];
  samples: CurvePoint[];
  cycle_ms: number;
  original_count: number;
  simplified_count: number;
}

export interface LibrarySummary {
  pattern_count: number;
  enabled_pattern_count: number;
  program_count: number;
  auto_disable: boolean;
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
  parakeet_source: string;
  asr_base_url?: string;
  asr_model?: string;
  input_mode: "hands_free" | "hold" | string;
  input_sensitivity: number;
  input_silence_ms: number;
  input_noise_suppression: boolean;
  neutts_runner_path?: string;
  neutts_reference_wav?: string;
  neutts_reference_codes?: string;
  neutts_reference_text?: string;
  neutts_backbone?: string;
  neutts_sampling_mode?: "fixed" | "random" | string;
  neutts_sampler_seed?: number;
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
  parakeet_source: string;
  asr_base_url: string;
  asr_model: string;
  input_mode: "hands_free" | "hold" | string;
  input_sensitivity: number;
  input_silence_ms: number;
  input_noise_suppression: boolean;
  neutts_runner_path: string;
  neutts_reference_wav: string;
  neutts_reference_codes: string;
  neutts_reference_text: string;
  neutts_backbone: string;
  neutts_sampling_mode: "fixed" | "random" | string;
  neutts_sampler_seed: number;
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
  modules?: Record<string, VoiceModuleStatus>;
}

export interface VoiceModuleStatus {
  state: "ready" | "incomplete" | "missing" | string;
  installed: boolean;
  worker_installed: boolean;
  runtime_installed: boolean;
  runtime_backend?: "cpu" | "cuda" | "custom" | string;
  reference_encoder_installed?: boolean;
  message: string;
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

export interface NeuTTSReference {
  id: string;
  codes_path: string;
  audio_path?: string;
  transcript?: string;
  token_count: number;
  source_format: "torch_int32" | "npy_int32" | string;
  reused: boolean;
}

export interface OptionHints {
  hsp_dispatch_owners?: string[];
  api_application_id_sources?: string[];
  diagnostics_verbosities?: string[];
  motion_styles?: string[];
  llm_providers?: string[];
  llama_cpp_modes?: string[];
  llm_reasoning_modes?: string[];
  llm_max_output_tokens?: number[];
  prompt_sets?: string[];
  tts_providers?: string[];
  asr_providers?: string[];
  parakeet_sources?: string[];
  neutts_sampling_modes?: string[];
}

export interface PublicSettings {
  version: number;
  server: { port: number };
  device: {
    hsp_dispatch_owner: string;
    intiface_server_address: string;
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
    ollama_base_url: string;
    ollama_models_path?: string;
    model: string;
    prompt_set: string;
    request_timeout_ms: number;
    max_output_tokens: number;
    reasoning_mode: string;
  };
  voice: VoiceSettings;
  diagnostics: { verbosity: string };
  options: OptionHints;
}

export interface LLMProviderStatus {
  provider: string;
  base_url: string;
  model: string;
  available: boolean;
  model_available?: boolean;
  managed?: boolean;
  loaded?: boolean;
  models?: string[];
  message?: string;
}

export interface ManagedLLMModel {
  id: string;
  display_name: string;
  provider: "llama_cpp";
  source: "gguf" | "ollama";
  source_name?: string;
  format: string;
  family?: string;
  parameter_size?: string;
  quantization?: string;
  size_bytes: number;
  sha256: string;
  model_path: string;
  license?: string;
  imported_at: string;
  updated_at: string;
  state: "ready" | "missing" | "changed";
  message?: string;
}

export interface LLMModelImport {
  id: string;
  source: "gguf" | "ollama";
  display_name: string;
  status: "queued" | "copying" | "complete" | "failed" | "cancelled";
  bytes_copied: number;
  total_bytes: number;
  model_id?: string;
  error?: string;
  started_at: string;
  updated_at: string;
}

export interface LLMModelManagerSnapshot {
  models: ManagedLLMModel[];
  imports: LLMModelImport[];
  store_path: string;
  suggested_ollama_path: string;
  runtime: ManagedLlamaRuntimeStatus;
  runtime_build?: ManagedLlamaRuntimeBuild;
}

export interface ManagedLlamaRuntimeStatus {
  state: "missing" | "ready" | "outdated" | "invalid";
  installed: boolean;
  current: boolean;
  build_supported: boolean;
  supported_backends: Array<"auto" | "cpu" | "cuda">;
  expected_version: string;
  version?: string;
  commit?: string;
  backend?: "cpu" | "cuda";
  source?: "built_from_source";
  built_at?: string;
  message: string;
}

export interface ManagedLlamaRuntimeBuild {
  id: string;
  backend: "auto" | "cpu" | "cuda";
  status: "queued" | "building" | "complete" | "failed" | "cancelled";
  message: string;
  output?: string;
  started_at: string;
  updated_at: string;
}

export interface OllamaModelInfo {
  name: string;
  model?: string;
  modified_at?: string;
  size_bytes: number;
  digest?: string;
  format?: string;
  family?: string;
  parameter_size?: string;
  quantization?: string;
}

export interface OllamaModelCandidate {
  id: string;
  name: string;
  format?: string;
  family?: string;
  parameter_size?: string;
  quantization?: string;
  size_bytes: number;
  digest?: string;
  license?: string;
  importable: boolean;
  reason?: string;
  imported_model_id?: string;
}

export interface OllamaModelScan {
  path: string;
  candidates: OllamaModelCandidate[];
}

// One-way update body for PUT /api/settings. handy_connection_key is omitted to
// keep the stored secret; clear_connection_key removes it.
export interface SettingsUpdate {
  server: { port: number };
  device: {
    hsp_dispatch_owner: string;
    intiface_server_address: string;
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

export interface ConnectionCheckResult {
  ok: boolean;
  status: string;
  hsp_available: boolean;
  playback_state?: string;
  latency_ms: number;
}

export interface TransportDiagnostics {
  name?: string;
  connected?: boolean;
  playback_state?: string;
  command_count?: number;
  last_latency_ms?: number;
  last_error?: string;
}

export interface IntifaceLinearActuator {
  index: number;
  feature_descriptor?: string;
  actuator_type?: string;
  step_count?: number;
}

export interface IntifaceDevice {
  device_index: number;
  device_name: string;
  device_message_timing_gap_ms?: number;
  linear_actuators: IntifaceLinearActuator[];
}

export interface IntifaceDispatchStatus {
  device_index: number;
  actuator_index: number;
  startup_anchor?: boolean;
  relative_scheduled_time_ms: number;
  actual_send_time: string;
  lateness_ms: number;
  effective_duration_ms: number;
  ack_latency_ms?: number;
  status: string;
}

export interface IntifaceTransportSnapshot {
  dispatch_owner: string;
  address: string;
  status: {
    connected: boolean;
    scanning: boolean;
    playback_state: string;
    max_ping_time_ms: number;
    queue_depth: number;
    queue_coverage_ms?: number;
    pending_acks?: number;
    linear_sent_count?: number;
    linear_acked_count?: number;
    linear_rejected_count?: number;
    linear_timeout_count?: number;
    last_ack_latency_ms?: number;
    max_ack_latency_ms?: number;
    last_send_lateness_ms?: number;
    max_send_lateness_ms?: number;
    coalesced_segments?: number;
    recent_dispatches_dropped?: number;
    last_wire_duration_ms?: number;
    selected_resolution_percent?: number;
    last_pacer_failure?: string;
    recent_dispatches?: IntifaceDispatchStatus[];
    selected_device_index?: number;
    selected_actuator_index?: number;
    devices: IntifaceDevice[];
  };
  diagnostics: Record<string, unknown>;
}

export interface AppState {
  version?: string;
  commit?: string;
  uptime_seconds?: number;
  stop_sequence?: number;
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
  library?: LibrarySummary;
  transport?: Record<string, unknown>;
  cloud_transport?: TransportDiagnostics;
  bluetooth_transport?: TransportDiagnostics;
  bluetooth_bridge?: BluetoothBridgeSnapshot;
  intiface_transport?: IntifaceTransportSnapshot;
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
