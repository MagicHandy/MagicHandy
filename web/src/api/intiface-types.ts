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
  ceiling_percent: number;
  timing_offset_ms: number;
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
  intensity_percent: number;
  ceiling_percent: number;
  effective_ceiling_percent: number;
  level_percent: number;
  playback_state: string;
  testing: boolean;
  commanded_count: number;
  acknowledged_count: number;
  rejected_count: number;
  timeout_count: number;
  last_error?: string;
  silent_reason?: string;
  effective_intensity_percent?: number;
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
