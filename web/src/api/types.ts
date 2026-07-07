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
  memories: MemoryItem[];
}

export interface PromptSet {
  id: string;
  name: string;
  system: string;
  builtin: boolean;
}

export interface ModesStatus {
  running?: boolean;
  mode?: string;
  active_mode?: string;
  [k: string]: unknown;
}

export interface OptionHints {
  hsp_dispatch_owners?: string[];
  api_application_id_sources?: string[];
  diagnostics_verbosities?: string[];
  motion_styles?: string[];
  llm_providers?: string[];
  llama_cpp_modes?: string[];
  prompt_sets?: string[];
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
  memory?: MemoryState;
  llm?: Record<string, unknown>;
  transport?: Record<string, unknown>;
  cloud_transport?: Record<string, unknown>;
  bluetooth_transport?: Record<string, unknown>;
  bluetooth_bridge?: Record<string, unknown>;
  trace?: Record<string, unknown>;
}

export interface MotionTarget {
  pattern_identifier?: string;
  speed_percent?: number;
  label?: string;
}

export type ChatStreamEvent =
  | { event: "status"; data: { state: string; provider?: string; model?: string; prompt_set?: string } }
  | { event: "delta" | "repair_delta"; data: { phase?: string; text?: string } }
  | { event: "malformed"; data: { repaired?: boolean; recoverable?: boolean; phase?: string; error?: string } }
  | { event: "error"; data: { message?: string } }
  | { event: "done"; data: { ok?: boolean; malformed?: boolean } }
  | { event: string; data: Record<string, unknown> };
