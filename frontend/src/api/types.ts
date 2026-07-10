export type OperationMode = "manual" | "auto" | "hybrid";

export interface VoiceTtsStatus {
  enabled: boolean;
  provider: string;
  available: boolean;
  voice_id: string;
  auto_speak_after_chat: boolean;
  install_hint?: string | null;
}

export interface VoiceSttStatus {
  enabled: boolean;
  provider?: string;
  available: boolean;
  model: string;
  language: string;
  auto_send: boolean;
  install_hint?: string | null;
}

export interface VoiceStatus extends VoiceTtsStatus {
  tts?: VoiceTtsStatus;
  stt?: VoiceSttStatus;
}

export type DeviceTransport = "intiface" | "handy_cloud";

export interface StatusSnapshot {
  server_url?: string;
  device_transport?: DeviceTransport | "cloud_rest";
  intiface_connected: boolean;
  intiface_url: string;
  intiface_error: string | null;
  handy_connected?: boolean;
  handy_error?: string | null;
  handy_base_url?: string;
  handy_key_configured?: boolean;
  ollama_connected?: boolean;
  ollama_error?: string | null;
  llm_provider?: string;
  llm_connected?: boolean;
  llm_error?: string | null;
  llm_model?: string;
  llm_base_url?: string;
  llm_cpp_mode?: string;
  llm?: {
    provider?: string;
    connected?: boolean;
    error?: string | null;
    model?: string;
    base_url?: string;
    llama_cpp_mode?: string;
    managed?: boolean;
    loaded?: boolean;
  };
  device_label: string;
  device_connected: boolean;
  use_mock: boolean;
  persona_id: string;
  persona_name: string;
  persona_avatar_url?: string | null;
  ollama_model: string;
  ollama_url: string;
  sync_offset_ms?: number;
  measured_rtt_ms?: number | null;
  intiface_reconnecting?: boolean;
  motion_position_pct?: number;
  operation_mode: OperationMode;
  intensity: number;
  min_position: number;
  max_position: number;
  buffer_sec: number;
  buffer_remaining_sec?: number;
  buffer_queued_sec?: number;
  queue_preview: {
    block_id: string;
    duration_ms: number;
    intensity: number;
    bpm?: number | null;
    loop_pattern?: boolean;
  }[];
  phase: string;
  phase_elapsed_sec?: number;
  phase_max_sec?: number | null;
  phase_min_sec?: number | null;
  phase_planned_duration_sec?: number | null;
  phase_remaining_sec?: number | null;
  phase_progress_pct?: number | null;
  phase_clock_running?: boolean;
  phase_locked?: boolean;
  phase_ready_to_advance?: boolean;
  ai_controls_pacing?: boolean;
  user_session_engaged?: boolean;
  active_pose?: string;
  pose_detail?: string;
  pose_label?: string;
  auto_running: boolean;
  autospeak_enabled?: boolean;
  autospeak_scheduled?: boolean;
  autospeak_min_seconds?: number;
  autospeak_max_seconds?: number;
  autospeak_motion_autonomy?: string;
  hands_free_active?: boolean;
  hands_free_last_signal?: string | null;
  hands_free_favorites_only?: boolean;
  emergency_stop: boolean;
  safety_limits_enabled?: boolean;
  footer_status?: string;
  playback_active?: boolean;
  direct_control_active?: boolean;
  planner_refill_busy?: boolean;
  planner_busy?: boolean;
  planner_busy_source?: string | null;
  chat_pending?: boolean;
  roster_chat_active?: boolean;
  roster_session?: {
    duration_min?: number;
    intensity_preset?: string;
    include_buildup?: boolean;
    enqueued?: number;
  };
  queue_blocks?: number;
  manual_queue_count?: number;
  manual_queue_playing?: boolean;
  manual_queue_progress_pct?: number;
  manual_queue_elapsed_sec?: number;
  manual_queue_duration_sec?: number;
  manual_queue_name?: string | null;
  manual_queue_paused?: boolean;
  manual_queue_playhead_ms?: number;
  manual_queue_current_segment_index?: number;
  manual_queue_segment_count?: number;
  manual_queue_autoloop?: boolean;
  manual_queue_signal_active?: boolean;
  manual_queue_signal_label?: string | null;
  manual_queue_playback_mode?: "script" | "pointwise" | null;
  playback_current?: {
    block_id: string;
    display_name?: string;
    semantic_summary?: string;
    zone?: string | null;
    speed?: string | null;
    rhythm?: string | null;
    success_score?: number;
    duration_ms?: number;
  } | null;
  chat_auto?: ChatAutoState;
}

export interface ChatAutoMotion {
  action?: string;
  velocidade?: number;
  intensidade?: number;
  regiao?: string;
  tipo_batida?: string;
  atraso_ms?: number;
}

export interface ChatAutoState {
  active?: boolean;
  stamina?: number;
  humor?: string;
  mood_progress?: number;
  posicao?: string;
  motion?: ChatAutoMotion;
  last_reply?: string;
  reply_partial?: string;
  segment_ends_in_ms?: number;
  llm_busy?: boolean;
  error?: string;
}

export interface ChatMessage {
  id: string;
  role: string;
  content: string;
  created_at: string;
}

export interface Persona {
  id: string;
  name: string;
  description: string | null;
  system_prompt: string;
  tone?: unknown;
  mood?: unknown;
  boundaries?: unknown;
  motion_bias?: unknown;
  tone_json?: string | null;
  mood_json?: string | null;
  boundaries_json?: string | null;
  motion_bias_json?: string | null;
  created_at?: string;
  updated_at?: string;
}

export interface FunscriptActionPoint {
  at: number;
  pos: number;
}

export interface HeatmapStatsApi {
  action_count: number;
  duration_ms: number;
  max_speed: number;
  avg_speed: number;
}

export interface CurvePoint {
  t_ms: number;
  pos: number;
}

export interface MotionBlock {
  id: string;
  zone: string | null;
  speed: string | null;
  stroke_length: string | null;
  rhythm: string | null;
  duration_ms: number;
  intensity: number | null;
  success_score: number | null;
  user_rating: number | null;
  favorite: number;
  blocked: number;
  times_used?: number;
  action_count?: number;
  playback_action_count?: number;
  preview?: CurvePoint[];
  actions?: FunscriptActionPoint[];
  source_filename?: string | null;
  source_display_name?: string | null;
  display_name?: string | null;
  is_full_script?: boolean;
  is_user_recorded?: boolean;
  source_file_id?: string | null;
  script_duration_ms?: number | null;
  heatmap_stats?: HeatmapStatsApi | null;
  min_pos?: number | null;
  max_pos?: number | null;
  amplitude?: number | null;
  bpm?: number;
  stroke_legs?: number;
  stroke_reversals?: number;
  pace?: string;
  pace_label?: string;
  tags?: string[] | string | null;
  session_roles?: string[];
  script_number?: number | null;
  source_start_ms?: number | null;
  source_end_ms?: number | null;
  source_time_range?: string | null;
  motion_time_range?: string | null;
  semantic_summary?: string | null;
  dislike_count?: number;
  source_range_slug?: string | null;
}

export interface SignalPreset {
  block_id?: string | null;
  duration_sec?: number | null;
  display_name?: string | null;
  configured?: boolean;
}

export interface ManualQueueItem {
  block_id: string;
  duration_sec: number;
  script_duration_sec?: number;
  loop: boolean;
  display_name?: string;
}

export interface ManualQueueDraft {
  items: ManualQueueItem[];
  count: number;
  total_duration_sec: number;
}

export interface ManualQueueSegment {
  index: number;
  start_ms: number;
  duration_ms: number;
  display_name?: string;
  block_id: string;
}

export interface ManualQueuePreview {
  ok: boolean;
  error?: string;
  preview: { t_ms: number; pos: number }[];
  actions?: FunscriptActionPoint[];
  segments: ManualQueueSegment[];
  duration_ms: number;
  total_duration_sec: number;
  action_count: number;
  script_duration_ms?: number | null;
  heatmap_stats?: HeatmapStatsApi | null;
}

export interface SavedQueueSummary {
  id: string;
  name: string;
  item_count: number;
  duration_ms: number | null;
  created_at: string;
  updated_at: string;
  funscript_file_id: string | null;
}

export interface SessionRow {
  id: string;
  persona_id: string | null;
  mode: string | null;
  started_at: string;
  ended_at: string | null;
  rating: number | null;
  notes: string | null;
}

export interface UIPreferences {
  locale: string;
  locale_prompt_dismissed: boolean;
  supported_locales: string[];
}

export interface AppSettings {
  motion?: Record<string, unknown>;
  safety?: Record<string, unknown>;
  queue?: Record<string, unknown>;
  ollama?: Record<string, unknown>;
  intiface?: Record<string, unknown>;
  app?: Record<string, unknown>;
  diagnostics?: Record<string, unknown>;
  planner?: Record<string, unknown>;
  [key: string]: unknown;
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
  settings?: {
    stroke_min_percent?: number;
    stroke_max_percent?: number;
    [key: string]: unknown;
  };
  last_error?: string;
}

export interface MotionInfo {
  available: boolean;
  error?: string;
  engine?: EngineSnapshot;
}

export interface MotionVisual {
  position_pct: number;
  target_pct: number;
  offset_ms: number;
  stroke_min_pct: number;
  stroke_max_pct: number;
  playback_active?: boolean;
  measured_rtt_ms?: number | null;
  device_latency_ms?: number | null;
  client_latency_ms?: number | null;
  recent: { t_ms: number; pos_pct: number }[];
  curve_actions?: { at: number; pos: number }[];
  curve_elapsed_ms?: number;
  curve_duration_ms?: number;
  live_position_pct?: number;
  live_sample_mono?: number;
  schedule_active?: boolean;
  stream_elapsed_ms?: number;
}

export interface ImportBlockSummary {
  id: string;
  display_name: string;
  zone?: string | null;
  speed?: string | null;
  stroke_length?: string | null;
  rhythm?: string | null;
  duration_ms: number;
  intensity?: number | null;
  action_count: number;
  preview?: CurvePoint[];
  actions?: FunscriptActionPoint[];
  inserted?: boolean;
  is_full_script?: boolean;
  bpm?: number;
  pace_label?: string;
  script_duration_ms?: number | null;
  heatmap_stats?: HeatmapStatsApi | null;
}

export interface ImportFullScriptSummary {
  file_id: string;
  block_id?: string | null;
  filename: string;
  action_count: number;
  duration_ms: number;
  preview?: CurvePoint[];
  actions?: FunscriptActionPoint[];
  bpm?: number;
  pace_label?: string;
  script_duration_ms?: number | null;
  heatmap_stats?: HeatmapStatsApi | null;
}

export interface ImportResult {
  ok?: boolean;
  persisted?: {
    file_id: string;
    blocks_inserted: number;
    blocks_skipped_content_hash?: number;
    blocks_skipped_duplicate?: number;
    inserted_block_ids?: string[];
    source_format?: string;
  };
  summary?: {
    action_count?: number;
    duration_ms?: number;
    block_count?: number;
  };
  source?: {
    filename?: string;
    source_format?: string;
  };
  imported_blocks?: ImportBlockSummary[];
  imported_full_block?: ImportBlockSummary;
  full_script?: ImportFullScriptSummary;
  error?: string;
}

export interface FunscriptFileEntry {
  file_id: string;
  filename: string;
  display_filename?: string;
  duration_sec: number;
  action_count: number;
  block_count: number;
  source_format?: string | null;
}

export interface ControllerSnapshot {
  client_id?: string;
  active: boolean;
  read_only: boolean;
  reason?: string;
  active_client_id?: string;
  active_client_age_ms?: number;
  lease_expires_in_ms?: number;
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
  builtin?: boolean;
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

export interface BluetoothStatusResponse {
  status: string;
  dispatch_owner?: string;
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

export interface BluetoothAckPayload {
  id: string;
  ok: boolean;
  status?: string;
  elapsed_ms?: number;
  error?: string;
  response?: Record<string, unknown>;
}

export interface ChatStreamEvent {
  event: string;
  data: Record<string, unknown>;
}

export interface ChatHistoryMessage {
  role: string;
  content: string;
}
