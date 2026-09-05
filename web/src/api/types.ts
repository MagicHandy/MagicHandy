// Types mirror the Go JSON payloads (internal/httpapi, internal/config). They
// are intentionally partial and defensive: the backend snapshot is
// authoritative, so unknown fields are ignored and read sites use optional
// chaining. See docs/decisions/0009-react-frontend.md (State Model Rules).

export type MotionStyle = "gentle" | "balanced" | "intense";
export type HandyModel = "handy_original" | "handy_2_standard" | "handy_2_pro";
export type NotificationCategory = "app" | "system" | "library" | "voice" | "updates";

export interface MotionSettings {
  speed_min_percent: number;
  speed_max_percent: number;
  stroke_min_percent: number;
  stroke_max_percent: number;
  reverse_direction: boolean;
  apply_video_speed_limit: boolean;
  style: string;
  handy_model: HandyModel | string;
}

export interface AutopilotSettings {
  speech_cadence: "off" | "quiet" | "natural" | "talkative" | "custom" | string;
  speech_min_seconds: number;
  speech_max_seconds: number;
  motion_cadence: "scaled" | "steady" | "natural" | "dynamic" | "custom" | string;
  motion_change_level: number;
  motion_min_seconds: number;
  motion_max_seconds: number;
  adaptive_speech_timing: boolean;
  adaptive_motion_timing: boolean;
  speech_motion_authority: "chat_only" | "style_only" | "full_motion" | string;
  // Inert read-only context: elapsed time, speed history, and backend-measured
  // phrase sameness. Off omits the facts from the prompt rather than sending
  // zeros.
  session_tracking: boolean;
  // The visible progression bar. Separate from tracking because knowing how long
  // a session has run and being encouraged to build through it are different
  // things. Requires session_tracking.
  session_arc: boolean;
  session_arc_minutes: number;
}

// SessionArc is the visible progression bar. The backend derives the value from
// active elapsed time; model output cannot alter it.
export interface SessionArc {
  enabled: boolean;
  percent: number;
  minutes: number;
}

export interface MotionSample {
  position_percent: number;
  time_ms: number;
}

export interface MotionPaceSummary {
  requested_percent: number;
  effective_percent: number;
  requested_mean_travel_percent_per_second: number;
  commanded_mean_travel_percent_per_second: number;
  commanded_peak_velocity_percent_per_second: number;
  device_peak_velocity_percent_per_second: number;
  limited: boolean;
  limiters?: Array<"device_velocity" | "acceleration" | "jerk" | "reversal_spacing" | "curve_geometry" | string>;
}

export interface EngineSnapshot {
  running: boolean;
  starting?: boolean;
  completing?: boolean;
  paused: boolean;
  running_ms?: number;
  phase?: number;
  recent_command_latency_ms?: number;
  target?: {
    label?: string;
    source?: string;
    speed_percent?: number;
    pattern_id?: string;
    pattern_name?: string;
    program_id?: string;
    media_id?: string;
    dynamic?: {
      center_percent: number;
      span_percent: number;
      span_min_percent?: number;
      span_profile?: "steady" | "breathe" | "wander" | "contrast" | string;
      phrase_seed?: number;
      anchors?: Array<{ name: string; position_percent: number }>;
      variation_percent: number;
      segment_seconds: number;
      sections?: Array<{
        center_percent: number;
        span_percent: number;
        span_min_percent?: number;
        span_profile?: "steady" | "breathe" | "wander" | "contrast" | string;
        anchors?: Array<{ name: string; position_percent: number }>;
        variation_percent: number;
        cycles: number;
      }>;
    };
  };
  current_sample?: MotionSample;
  last_sample?: MotionSample;
  pace?: MotionPaceSummary;
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
  active_client_age_ms?: number;
  lease_expires_in_ms?: number;
  takeover_in_progress?: boolean;
}

export interface ControllerTakeoverResponse {
  controller: ControllerSnapshot;
  changed: boolean;
  stop_confirmed: boolean;
  stop_sequence: number;
  warning?: string;
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
  diagnostics?: ChatMessageDiagnostics;
  speech_request_id?: string;
}

export interface ChatMessageDiagnostics {
  source?: string;
  provider?: string;
  model?: string;
  prompt_set?: string;
  persona_id?: string;
  persona_name?: string;
  request_ms?: number;
  preparation_ms?: number;
  scheduler_wait_ms?: number;
  first_token_ms?: number;
  generation_ms?: number;
  repair_ms?: number;
  provider_calls?: number;
  repaired?: boolean;
  semantic_fallback?: boolean;
  initial_malformed?: boolean;
  motion_action?: string;
  mood?: AssistantMood;
  mood_changed?: boolean;
}

export type AssistantMood =
  | "Curious"
  | "Teasing"
  | "Playful"
  | "Loving"
  | "Excited"
  | "Passionate"
  | "Seductive"
  | "Anticipatory"
  | "Breathless"
  | "Dominant"
  | "Submissive"
  | "Vulnerable"
  | "Confident"
  | "Intimate"
  | "Needy"
  | "Overwhelmed"
  | "Afterglow";

export type LLMUserAnatomy = "penis" | "vagina" | "custom";

export interface ChatSession {
  id: string;
  title: string;
  saved: boolean;
  // Which persona this conversation is held with, or absent for the global axis
  // values from Settings. It can name a persona that has since been deleted.
  persona_id?: string;
  // Effective backend-resolved display name. Dangling and empty persona IDs
  // deliberately resolve to the Settings-backed MagicHandy default.
  persona_name?: string;
  active: boolean;
  message_count: number;
  latest_seq: number;
  created_at: string;
  updated_at: string;
}

// Persona is a named preset over the personalization axes plus a portrait. It
// never carries capability gates or limits — see docs/persona-page.md §3.
export interface Persona {
  id: string;
  name: string;
  description: string;
  chat_voice: string;
  reaction_style: string;
  prompt_set_id: string;
  default_focus_area: string;
  lore_mode: string;
  lore_count: number;
  has_portrait: boolean;
  // Doubles as the portrait cache-buster: replacing a picture in place moves
  // this, which is what makes the tile refetch it.
  portrait_updated_at?: string;
  last_used_at?: string;
  created_at: string;
  updated_at: string;
}

export interface PersonaDraft {
  name?: string;
  description?: string;
  chat_voice?: string;
  reaction_style?: string;
  prompt_set_id?: string;
  default_focus_area?: string;
  lore_mode?: string;
}

// DefaultPersona is the global Settings-backed fallback presented as a
// first-class library choice. It is deliberately not a stored persona row:
// selecting it keeps the session's persona_id empty.
export interface DefaultPersona {
  name: string;
  description: string;
  chat_voice: string;
  prompt_set_id: string;
}

export interface PersonaLoreEntry {
  id: string;
  persona_id: string;
  text: string;
  keywords: string[];
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface PersonaLoreDraft {
  text?: string;
  keywords?: string[];
  enabled?: boolean;
}

export interface PersonaLorePayload {
  persona: Persona;
  entries: PersonaLoreEntry[];
  entry?: PersonaLoreEntry;
  options: {
    max_entries: number;
    max_text: number;
    max_total: number;
    max_keywords: number;
    max_keyword: number;
  };
}

export interface PersonasPayload {
  personas: Persona[];
  default_persona: DefaultPersona;
  active_persona_id: string;
  active_session_id: string;
  prompt_sets: PromptSet[];
  persona?: Persona;
  options: {
    chat_voices: string[];
    reaction_styles: string[];
    focus_areas: string[];
    lore_modes: string[];
    max_name: number;
    max_description: number;
    max_portrait_edge: number;
    max_lore_entries: number;
    max_lore_text: number;
    max_lore_total: number;
    max_lore_keywords: number;
    max_lore_keyword: number;
  };
}

export interface PromptSection {
  id: string;
  title: string;
  text: string;
  characters: number;
  bytes: number;
}

export interface PromptCompositionPayload {
  session_id: string;
  provider: string;
  model: string;
  prompt_set: string;
  persona_id?: string;
  persona_name?: string;
  lore_mode?: string;
  lore: {
    entry_ids?: string[];
    characters?: number;
  };
  composition: {
    prompt: string;
    sections: PromptSection[];
    characters: number;
    bytes: number;
  };
}

export interface ChatSessionsResponse {
  active_session_id: string;
  sessions: ChatSession[];
}

export interface ChatMessagesResponse {
  messages: ChatLogMessage[];
  latest_seq: number;
  cursor: number;
  session_id: string;
}

// LLMMotionCapabilities is the user-selected checkbox list of control methods
// the model may use; enforcement is server-side. Absent means "never saved"
// and resolves to the defaults (everything but experimental patterns).
export interface LLMMotionCapabilities {
  motion: boolean;
  patterns: boolean;
  area_focus: boolean;
  experimental_patterns: boolean;
}

export interface ModesStatus {
  running?: boolean;
  mode?: string;
  active_mode?: string;
  status_at?: string;
  segment_index?: number;
  segment_ends_in_ms?: number;
  segment_due_at?: string;
  decision_source?: string;
  last_say?: string;
  motion_change_in_ms?: number;
  motion_change_due_at?: string;
  speech_in_ms?: number;
  speech_due_at?: string;
  motion_planned?: boolean;
  speech_waiting_playback?: boolean;
  // Absent while the user has the bar switched off, so the UI renders nothing
  // rather than an empty bar.
  session_arc?: SessionArc;
  [k: string]: unknown;
}

export interface CurvePoint {
  time_ms: number;
  position_percent: number;
}

export interface LibraryPattern {
	deprecated?: boolean;
	continuous?: boolean;
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

// MediaCompatibility mirrors internal/media.Compatibility. Only "playable" is
// ever a positive claim, and only a real playback attempt produces it: a
// supported container says nothing about whether its codec decodes here.
export type MediaCompatibility =
  | "unknown"
  | "playable"
  | "unsupported_container"
  | "unsupported_codec";

export interface MediaVideo {
  id: string;
  location_path: string;
  display_name: string;
  size_bytes: number;
  modified_at: string;
  duration_ms: number | null;
  has_funscript: boolean;
  missing: boolean;
  scanned_at: string;
  script_offset_ms?: number;
  thumbnail_generated_at?: string | null;
  compatibility?: MediaCompatibility;
  video_codec?: string | null;
  audio_codec?: string | null;
  superseded?: boolean;
  container_type?: string;
}

// canPlayContainer asks this browser's own engine whether it opens a container,
// cached because the answer cannot change within a page load.
const containerSupport = new Map<string, boolean>();

function canPlayContainer(containerType: string): boolean {
  const cached = containerSupport.get(containerType);
  if (cached !== undefined) return cached;
  let supported = false;
  try {
    supported = document.createElement("video").canPlayType(containerType) !== "";
  } catch {
    supported = false;
  }
  containerSupport.set(containerType, supported);
  return supported;
}

/**
 * needsConversion decides whether to offer a repair.
 *
 * The two incompatibility states are not equally certain, and treating them
 * alike would put a wrong badge on a lot of working files. An observed codec
 * refusal is fact: this browser tried and failed. An unsupported *container* is
 * only the server's guess from a file extension, and that guess is browser-
 * specific — Chrome opens MKV, Firefox does not. So the guess is checked
 * against the engine that would actually do the playing before it is shown.
 */
export function needsConversion(video: MediaVideo): boolean {
  if (video.missing) return false;
  if (video.compatibility === "unsupported_codec") return true;
  if (video.compatibility !== "unsupported_container") return false;
  return !video.container_type || !canPlayContainer(video.container_type);
}

export interface MediaToolStatus {
  configured: boolean;
  available: boolean;
  ffmpeg_path?: string;
  version?: string;
  error?: string;
}

export interface MediaJobIssue {
  name: string;
  message: string;
}

export interface MediaJobState {
  kind?: "thumbnails" | "conversion";
  running: boolean;
  cancellable: boolean;
  cancelled: boolean;
  started_at?: string;
  completed_at?: string;
  current_name?: string;
  total: number;
  processed: number;
  succeeded: number;
  failed: number;
  item_percent: number;
  issues: MediaJobIssue[] | null;
  error?: string;
}

export interface MediaFunscriptAction {
  at: number;
  pos: number;
}

export interface MediaFunscript {
  video_id: string;
  name: string;
  duration_ms: number;
  action_count: number;
  actions: MediaFunscriptAction[];
}

export interface MediaSyncEvent {
  video_id: string;
  session_id: string;
  event_sequence: number;
  state: "playing" | "paused" | "seeking" | "ended" | "closed";
  event: "play" | "pause" | "seeking" | "seeked" | "ratechange" | "resync" | "heartbeat" | "waiting" | "stalled" | "error" | "ended" | "closed";
  media_time_ms: number;
  client_time_ms: number;
  playback_rate: number;
}

export interface MediaSyncStatus {
  active: boolean;
  video_id?: string;
  state: "idle" | "following" | "seeking" | "paused" | "ended" | "completed" | "drifted" | "interrupted" | "timed_out" | "stopped" | "error";
  last_event?: string;
  media_time_ms?: number;
  drift_ms?: number;
  playback_rate?: number;
  motion_speed_limit_percent?: number;
  requires_reanchor?: boolean;
  expected_media_time_ms?: number;
  script_offset_ms?: number;
  filter_effect?: { actions_removed?: number; peak_reduction_percent?: number };
  message?: string;
  updated_at?: string;
}

export interface MediaScanIssue {
  location: string;
  message: string;
}

export interface MediaSettingsPayload {
  library_paths: string[];
  auto_scan_on_startup?: boolean;
  remove_missing_on_scan?: boolean;
  script_offset_ms?: number;
  script_smoothing_percent?: number;
  peak_rounding_ms?: number;
  ffmpeg_path?: string;
  convert_h265_for_compatibility?: boolean;
  reencode_codec?: "h264" | "h265";
  reencode_crf_h264?: number;
  reencode_crf_h265?: number;
  reencode_preset?: string;
  reencode_audio_kbps?: number;
  generate_thumbnails_on_scan?: boolean;
  convert_incompatible_on_scan?: boolean;
  show_superseded_originals?: boolean;
}

export interface MediaScanState {
  running: boolean;
  trigger?: "manual" | "startup";
  cancellable: boolean;
  cancelled: boolean;
  started_at?: string;
  completed_at?: string;
  current_location?: string;
  files_visited: number;
  videos_found: number;
  summary: {
    locations: number;
    added: number;
    updated: number;
    missing: number;
    removed: number;
    skipped: number;
    issues: MediaScanIssue[] | null;
  };
  error?: string;
}

export interface MediaSummary {
  available: boolean;
  error?: string;
  video_count?: number;
  available_count?: number;
  paired_count?: number;
  scan?: MediaScanState;
  sync?: MediaSyncStatus;
  job?: MediaJobState;
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

export interface MotionImportResult {
  kind: "pattern" | "program" | string;
  pattern?: LibraryPattern;
  program?: LibraryProgram;
  gaps_stripped: number;
}

export interface LibrarySummary {
  available: boolean;
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
  chat_speech_policy?: "interrupt" | "finish_current" | string;
  elevenlabs_voice_id?: string;
  elevenlabs_model_id?: string;
  tts_auto_launch: boolean;
  tts_base_url?: string;
  tts_model?: string;
  tts_voice?: string;
  tts_response_format?: string;
  tts_health_path?: string;
  tts_module_root?: string;
  tts_server_port?: number;
  tts_reference_wav?: string;
  tts_reference_text?: string;
  tts_language?: string;
  tts_device?: "auto" | "cuda" | "cpu" | string;
  tts_seed: number;
  tts_seed_mode: "fixed" | "varied" | string;
  tts_tone_preset: "natural" | "warm" | "playful" | "tender" | "commanding" | "excited" | "custom" | string;
  tts_tone_prompt?: string;
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
  // Read side only: the stored ElevenLabs key is never returned, just a flag.
  elevenlabs_key_set?: boolean;
  // Read side only: compatible endpoint keys are never returned.
  openai_tts_key_set?: boolean;
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
  chat_speech_policy: "interrupt" | "finish_current" | string;
  elevenlabs_voice_id: string;
  elevenlabs_model_id: string;
  tts_auto_launch: boolean;
  tts_base_url: string;
  tts_model: string;
  tts_voice: string;
  tts_response_format: string;
  tts_health_path: string;
  tts_module_root: string;
  tts_server_port: number;
  tts_reference_wav: string;
  tts_reference_text: string;
  tts_language: string;
  tts_device: "auto" | "cuda" | "cpu" | string;
  tts_seed: number;
  tts_seed_mode: "fixed" | "varied" | string;
  tts_tone_preset: "natural" | "warm" | "playful" | "tender" | "commanding" | "excited" | "custom" | string;
  tts_tone_prompt: string;
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
  elevenlabs_api_key?: string;
  clear_elevenlabs_key: boolean;
  openai_tts_api_key?: string;
  clear_openai_tts_key: boolean;
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
  runner_installed?: boolean;
  model_installed?: boolean;
  resumable_partial?: boolean;
  partial_bytes?: number;
  runtime_backend?: "cpu" | "cuda" | "custom" | string;
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

export interface OptionHints {
  hsp_dispatch_owners?: string[];
  api_application_id_sources?: string[];
  diagnostics_verbosities?: string[];
  motion_styles?: string[];
  handy_models?: HandyModel[];
  autopilot_speech_cadences?: string[];
  autopilot_motion_cadences?: string[];
  autopilot_speech_motion_authorities?: string[];
  llm_providers?: string[];
  llama_cpp_modes?: string[];
  llm_managed_load_policies?: string[];
  llama_cpp_context_sizes?: number[];
  llm_reasoning_modes?: string[];
  llm_max_output_tokens?: number[];
	llm_motion_modes?: Array<"dynamic" | "pattern" | "off" | string>;
  llm_chat_voices?: string[];
  llm_user_anatomies?: LLMUserAnatomy[];
  prompt_sets?: string[];
  tts_providers?: string[];
  asr_providers?: string[];
  parakeet_sources?: string[];
  tts_devices?: string[];
  tts_tone_presets?: string[];
  chat_speech_policies?: string[];
  chat_startup_behaviors?: string[];
  locales?: string[];
  themes?: string[];
}

export interface PublicSettings {
  version: number;
  labs?: {enabled:boolean};
  server: { port: number };
  ui?: {
    locale: string;
    theme?: string;
    setup_completed?: boolean;
    update_check_mode?: "automatic" | "manual" | string;
    notification_categories?: NotificationCategory[];
  };
  media?: MediaSettingsPayload;
  device: {
    hsp_dispatch_owner: string;
    intiface_server_address: string;
    firmware_api_requirement: string;
    api_application_id_source: string;
    api_application_id_override?: string;
    connection_key_set: boolean;
  };
  motion: MotionSettings;
  autopilot: AutopilotSettings;
  llm: {
    provider: string;
    llama_cpp_mode: string;
    managed_load_policy?: "startup" | "on_demand" | string;
    llama_cpp_base_url: string;
    llama_cpp_context_size: number;
    ollama_base_url: string;
    ollama_models_path?: string;
    model: string;
    prompt_set: string;
    request_timeout_ms: number;
    max_output_tokens: number;
    reasoning_mode: string;
    chat_voice?: string;
    user_anatomy?: LLMUserAnatomy;
    custom_anatomy?: string;
    persona_description?: string;
    motion_capabilities?: LLMMotionCapabilities;
		motion_generation_mode: "dynamic" | "pattern" | "layered" | "creative_v2" | "off" | string;
  };
  voice: VoiceSettings;
  chat?: {
    startup_behavior: "previous" | "new" | string;
    keep_unsaved_on_exit: boolean;
  };
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
  loading?: boolean;
  models?: string[];
  message?: string;
}

export interface ManagedLLMDuplicateProcess {
  pid: number;
  executable: string;
}

export interface ManagedLLMDuplicateSnapshot {
  managed: boolean;
  runner_name?: string;
  processes: ManagedLLMDuplicateProcess[];
}

export interface SetupJob {
  id: string;
  kind: "llama_runtime" | "parakeet" | "voice_module" | string;
  module: string;
  device: string;
  status: "queued" | "running" | "complete" | "failed" | "cancelled" | string;
  message: string;
  output?: string;
  steps?: Array<{
    id: string;
    label: string;
    status: "queued" | "running" | "complete" | "failed" | "cancelled" | string;
    message?: string;
  }>;
  completed_steps?: number;
  total_steps?: number;
  bytes_completed?: number;
  bytes_total?: number;
  started_at: string;
  updated_at: string;
}

export interface SetupInstallPlan {
  llama?: { backend: "auto" | "cpu" | "cuda" };
  voice?: { module: string; device: "cpu" | "cuda"; auto_launch: boolean };
  parakeet: boolean;
}

export interface SetupVoiceModule {
  id: "faster-qwen3-tts" | "chatterbox" | string;
  name: string;
  provider: string;
  summary: string;
  license: string;
  model: string;
  model_license: string;
  python_version: string;
  disk_estimate: string;
  supported_devices: string[];
  recommended_for_nvidia: boolean;
  reference_requirement: string;
  source_url: string;
  source_revision: string;
  port: number;
}

export interface SetupStatus {
  required: boolean;
  data_dir: string;
  hardware: {
    platform: string;
    nvidia: boolean;
    cuda: boolean;
    gpu_name?: string;
    vram_mib?: string;
  };
  voice_modules: SetupVoiceModule[];
  llama_runtime: {
    name: string;
    summary: string;
    license: string;
    source_version: string;
    disk_estimate: string;
    build_dependencies: string[];
    backends: Array<"auto" | "cpu" | "cuda">;
  };
  parakeet: {
    name: string;
    summary: string;
    runner_license: string;
    model_license: string;
    download_size: string;
    runner_version: string;
    model: string;
    preselected: boolean;
  };
  installation?: SetupJob;
  scripts_present: boolean;
  helpers: { llama: boolean; parakeet: boolean; voice: boolean };
}

export type AccountRole = "admin" | "operator";

export interface UserAccount {
  id: string;
  username: string;
  role: AccountRole;
  disabled: boolean;
  has_profile_image: boolean;
  profile_updated_at?: string;
  last_login_at?: string;
  created_at: string;
  updated_at: string;
}

export interface ControlIdentity {
  account: UserAccount;
  relationship: "self" | "linked";
  label: string;
  selected: boolean;
}

export interface AuthenticationStatus {
  initialized: boolean;
  authentication_required: boolean;
  authenticated: boolean;
  bootstrap_available: boolean;
  ui_locale: string;
  account: UserAccount | null;
  control_identities: ControlIdentity[] | null;
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
  state: "ready" | "missing" | "changed" | "unsupported";
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
  source?: "built_from_source" | "verified_upstream_release";
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
  ui?: {
    locale: string;
    theme: string;
    setup_completed: boolean;
    update_check_mode: "automatic" | "manual" | string;
    notification_categories: NotificationCategory[];
  };
  media: MediaSettingsPayload;
  device: {
    hsp_dispatch_owner: string;
    intiface_server_address: string;
    firmware_api_requirement: string;
    api_application_id_source: string;
    api_application_id_override: string;
    handy_connection_key?: string;
  };
  motion: MotionSettings;
  autopilot?: AutopilotSettings;
  llm: PublicSettings["llm"];
  voice: VoiceSettingsUpdate;
  chat?: NonNullable<PublicSettings["chat"]>;
  diagnostics: { verbosity: string };
  clear_connection_key: boolean;
}

export interface UpdateRelease {
  version: string;
  tag: string;
  name?: string;
  url: string;
  published_at?: string;
}

export interface UpdateStatus {
  state: "available" | "current" | "development" | "no_release" | "error" | string;
  current_version: string;
  latest?: UpdateRelease;
  checked_at?: string;
  stale?: boolean;
  message?: string;
}

export interface ConnectionCheckResult {
  ok: boolean;
  status: string;
  hsp_available: boolean;
  playback_state?: string;
  latency_ms: number;
  message?: string;
}

export interface CloudDisconnectResponse {
  released: boolean;
  stopped: boolean;
  warning?: string;
  diagnostics: TransportDiagnostics;
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
  motion_simulated?: boolean;
  labs_enabled?: boolean;
  modes?: ModesStatus;
  memory?: MemoryState | Record<string, unknown>;
  llm?: Record<string, unknown>;
  voice?: VoiceState;
  chat?: { available?: boolean; latest_seq?: number; active_session_id?: string; current_mood?: AssistantMood | "" };
  library?: LibrarySummary;
  media?: MediaSummary;
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
  | { event: "status"; data: { state: string; provider?: string; model?: string; prompt_set?: string; persona_id?: string; persona_name?: string; session_id?: string; user_seq?: number; current_mood?: AssistantMood | ""; stop_sequence?: number } }
  | { event: "delta" | "repair_delta"; data: { phase?: string; text?: string } }
  | { event: "message"; data: { reply?: string; motion?: Record<string, unknown>; new_mood?: AssistantMood | null; current_mood?: AssistantMood | ""; initial_malformed?: boolean; diagnostics?: ChatMessageDiagnostics; seq?: number } }
  | { event: "speech"; data: { request_id?: string } }
  | { event: "motion"; data: { applied?: boolean; action?: string; error?: string } }
  | { event: "malformed"; data: { repaired?: boolean; recoverable?: boolean; phase?: string; error?: string } }
  | { event: "error"; data: { message?: string } }
  | { event: "done"; data: { ok?: boolean; malformed?: boolean } }
  | { event: string; data: Record<string, unknown> };
