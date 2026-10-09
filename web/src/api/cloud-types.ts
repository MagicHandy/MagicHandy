export interface MotionPlannerSettings {
  provider: "conversation" | "connection" | "local" | "chatgpt" | "decisions";
  model: string;
  connection_id?: string;
  context_policy?: "conversation" | "technical";
}

export interface ModelConnection {
  id: string;
  name: string;
  provider: "chatgpt" | "openai" | "openrouter" | "compatible";
  base_url: string;
  model: string;
  output_mode: "auto" | "strict" | "json" | "prompt";
  reasoning_effort?: string;
  supported_parameters?: string[];
  allow_fallbacks: boolean;
  allowed_providers?: string[];
  data_collection: "deny" | "allow";
  zero_data_retention: boolean;
  no_authentication?: boolean;
}

export interface HostedModel { id: string; name: string; supported_parameters?: string[]; reasoning_efforts?: string[]; default_reasoning_effort?: string }

export interface CloudPlanningStatus {
  connection: {
    profiles: Array<{ id: string; label: string; connected: boolean; plan_authorized: boolean; welcome_pending: boolean }>;
    active: string;
    pending: boolean;
    state: string;
    message?: string;
    decisions_key_set: boolean;
    generation: number;
  };
  models: Array<{ slug: string; display_name: string }>;
  readiness: { provider: string; model: string; ready: boolean; state: string; message?: string; checked_at?: string; connection?: ModelConnection; elapsed_ms?: number };
  motion_planner: MotionPlannerSettings;
  connection_keys?: Record<string, boolean>;
  connection_readiness?: Record<string, CloudPlanningStatus["readiness"]>;
}

// Read-only backend projection; never part of a settings update.
export interface ModelRoute {
  kind: "local" | "hosted" | "decisions" | "unavailable";
  provider: string; connection_id?: string; model: string; endpoint_host?: string;
  context_policy: "conversation" | "technical"; state: "configured" | "incomplete" | "unavailable";
}
export interface ModelRoutingSnapshot { chat: ModelRoute; autopilot: ModelRoute; local_retry?: ModelRoute }
