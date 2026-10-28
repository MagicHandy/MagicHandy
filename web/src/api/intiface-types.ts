// Intiface Central payloads (internal/transport/intiface*.go and
// internal/httpapi/intiface_transport.go). Split from types.ts to keep it
// within the source line budget; types.ts re-exports them.

export interface IntifaceLinearActuator {
  index: number;
  feature_descriptor?: string;
  actuator_type?: string;
  step_count?: number;
}

/** One ScalarCmd feature. index is its position in the device's full
 * ScalarCmd list, which is how the backend addresses it. */
export interface IntifaceScalarActuator {
  index: number;
  feature_descriptor?: string;
  actuator_type: string;
  step_count?: number;
}

/** Vibrator conversion settings (internal/config/vibration.go). */
export interface VibrationSettings {
  follow: "speed" | "depth" | "steady";
  depth_end: "base" | "tip";
  minimum_percent: number;
  /** The user's Max: it caps every output, including the model's level. */
  ceiling_percent: number;
  timing_offset_ms: number;
  /** Who sets the level under Max (ADR 0061); manual follows at Max. */
  level_control: "model" | "manual";
}

/** One vibration motor the vibrator drives. */
export interface IntifaceVibratorMotor {
  index: number;
  feature_descriptor?: string;
  step_count: number;
  /** Last quantized command, not physical telemetry. */
  level_percent?: number;
}

/** The vibrator's backend snapshot. Percentages are whole numbers. */
export interface IntifaceVibratorStatus {
  selected: boolean;
  device_index?: number;
  device_name?: string;
  motors: IntifaceVibratorMotor[];
  /** The user's Max, and the highest motor level at or below it. */
  ceiling_percent: number;
  effective_ceiling_percent: number;
  /** The setting is on and the chat model can set vibration. */
  model_control: boolean;
  /** The level as a percentage of Max: the model's, or The Handy's intensity. */
  model_level_percent: number;
  /** "model" once a model chose the level; "handy" while it follows The Handy. */
  level_source?: "model" | "handy";
  /** The last nonzero model level, while the level is 0. */
  previous_level_percent?: number;
  /** Stop paused the vibrator until a person resumes it. */
  suspended?: boolean;
  /** The last commanded motor level, not measured vibration. */
  level_percent: number;
  playback_state: string;
  testing: boolean;
  commanded_count: number;
  acknowledged_count: number;
  rejected_count: number;
  timeout_count: number;
  last_error?: string;
  silent_reason?: string;
  test_available?: boolean;
}

export interface IntifaceDevice {
  device_index: number;
  device_name: string;
  device_message_timing_gap_ms?: number;
  /** Absent from older backends when the device has none. */
  linear_actuators?: IntifaceLinearActuator[];
  scalar_actuators?: IntifaceScalarActuator[];
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
  /** Devices holding the shared Intiface connection. */
  leases?: string[];
  /** Whether Intiface stops its devices when this app stops answering. */
  ping_watchdog?: boolean;
  vibrator?: IntifaceVibratorStatus;
  /** A simulated Intiface server: nothing reaches a real device. */
  simulated?: boolean;
  diagnostics: Record<string, unknown>;
}
