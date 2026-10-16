// Coyote and device-output payloads (internal/config/estim.go and
// internal/transport/coyote.go). Split from types.ts to keep it within the
// source line budget; types.ts re-exports them.

/** Legacy participation: older backends report only the Handy and a Coyote. */
export type DeviceOutput = "none" | "handy" | "coyote" | "handy_coyote";

/** A saved device, in run-clock order: the first playing device sets the clock. */
export type DeviceId = "handy" | "coyote" | "vibrator";

/** Coyote settings. Ceilings are written to the device on every connection. */
export interface EstimSettings {
  safety_acknowledged: boolean;
  max_strength_a: number;
  max_strength_b: number;
  follow: "stroke" | "speed" | "position" | string;
  frequency_min_hz: number;
  frequency_max_hz: number;
  balance: number;
  swap_channels: boolean;
  timing_offset_ms: number;
  frequency_balance_a: number;
  frequency_balance_b: number;
  intensity_balance_a: number;
  intensity_balance_b: number;
}

/** The strength source distinguishes a device report from accepted zero delivery. */
export interface CoyoteStatus {
  connected: boolean;
  connecting: boolean;
  simulated: boolean;
  device_name?: string;
  address?: string;
  battery_percent?: number;
  strength_a: number;
  strength_b: number;
  strength_known: boolean;
  strength_source: "unknown" | "reported" | "zero_command";
  strength_pending: boolean;
  limits_applied: boolean;
  ceiling_a: number;
  ceiling_b: number;
  playback_state: string;
  reference_pulse: boolean;
  commanded_a: number;
  commanded_b: number;
  packets_sent: number;
  write_failures: number;
  starved_packets: number;
  max_send_lateness_ms: number;
  last_error?: string;
}

export interface CoyoteTransportSnapshot {
  output: DeviceOutput | string;
  available: boolean;
  status: CoyoteStatus;
}


export interface PublicDeviceSettings {
    /** The playback set; absent from older backends, which report only output. */
    playback_devices?: DeviceId[];
    output?: DeviceOutput;
    configured_devices?: DeviceId[];
    linked_failure_policy?: "stop_all" | "continue_healthy";
    hsp_dispatch_owner: string;
    intiface_server_address: string;
    firmware_api_requirement: string;
    api_application_id_source: string;
    api_application_id_override?: string;
    connection_key_set: boolean;
  }

export interface DeviceSettingsUpdate {
    playback_devices?: DeviceId[];
    output?: DeviceOutput;
    configured_devices?: DeviceId[];
    linked_failure_policy?: "stop_all" | "continue_healthy";
    hsp_dispatch_owner: string;
    intiface_server_address: string;
    firmware_api_requirement: string;
    api_application_id_source: string;
    api_application_id_override: string;
    handy_connection_key?: string;
  }

/** One device option change from Settings > Devices (PATCH /api/settings/device).
 * Absent fields keep their value; the connection key keeps its own Save. */
export interface DeviceOptionsPatch {
  linked_failure_policy?: "stop_all" | "continue_healthy";
  intiface_server_address?: string;
  api_application_id_source?: string;
  api_application_id_override?: string;
  /** Forgets the saved Handy connection key. */
  clear_connection_key?: boolean;
  estim?: Partial<Omit<EstimSettings, "safety_acknowledged">> & { forget_device?: boolean };
  vibration?: Partial<import("./intiface-types").VibrationSettings>;
}
