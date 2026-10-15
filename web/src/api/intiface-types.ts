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
  diagnostics: Record<string, unknown>;
}
