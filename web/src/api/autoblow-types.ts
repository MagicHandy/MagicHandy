// Autoblow's cloud devices: the Autoblow Ultra and the VacuGlide
// (internal/config/autoblow.go and internal/transport/autoblow.go, ADRs 0069
// and 0071).

/** The Ultra's saved settings. The token itself never reaches a client. */
export interface AutoblowSettings {
  token_set: boolean;
  timing_offset_ms: number;
}

/** The VacuGlide's saved settings: its Tempo range, in percent of motor
 * speed, and whether chat and Autopilot may step its stroke valves. */
export interface VacuGlideSettings {
  token_set: boolean;
  timing_offset_ms: number;
  tempo_min_percent: number;
  tempo_max_percent: number;
  motion_air: boolean;
}

/** A device's live state. Commanded values are what MagicHandy last sent:
 * the Ultra's position or the VacuGlide's motor speed, never a measurement. */
export interface AutoblowStatus {
  device: "autoblow" | "vacuglide" | string;
  connected: boolean;
  connecting: boolean;
  simulated: boolean;
  token_set: boolean;
  cluster?: string;
  firmware_version?: number;
  firmware_status?: "UP_TO_DATE" | "UPDATE_AVAILABLE" | "UPDATE_REQUIRED" | string;
  operational_mode?: string;
  playback_state: string;
  round_trip_ms: number;
  commanded_percent?: number;
  testing: boolean;
  button_events: boolean;
  last_button?: string;
  requests_sent: number;
  request_failures: number;
  rate_limited: number;
  rate_limit_remaining?: number;
  late_segments?: number;
  /** The VacuGlide's net stroke valve steps in this connection. */
  stroke_steps: number;
  motion_air?: boolean;
  /** The valve steps chat and Autopilot took since playback began. */
  motion_air_steps?: number;
  valve_open?: string;
  last_error?: string;
}

export interface AutoblowTransportSnapshot {
  autoblow: AutoblowStatus;
  vacuglide: AutoblowStatus;
}

export type AutoblowDevice = "autoblow" | "vacuglide";

/** Settings > Devices changes for the two devices; the token has its own Save. */
export interface AutoblowOptionsPatch {
  autoblow?: { timing_offset_ms?: number; clear_token?: boolean };
  vacuglide?: { timing_offset_ms?: number; tempo_min_percent?: number; tempo_max_percent?: number; motion_air?: boolean; clear_token?: boolean };
}
