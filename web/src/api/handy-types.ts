// Additional Handys (/api/settings/handys and /api/transport/handys). The
// primary Handy keeps its own device and motion fields under the ID "handy";
// these are the named instances beside it. Connection keys and Bluetooth
// addresses never appear here.
import type { HandyInstanceId } from "./coyote-types";
import type { HandyModel } from "./types";

/** How an additional Handy connects. Browser Bluetooth stays the primary Handy's. */
export type HandyConnectionMethod = "cloud_rest" | "native_bluetooth" | "intiface";

/** One additional Handy, as saved. */
export interface HandyConnection {
  id: HandyInstanceId;
  name: string;
  method: HandyConnectionMethod | string;
  handy_model: HandyModel | string;
  connection_key_set: boolean;
  remembered_device: boolean;
}

/** An additional Handy's link, owned by the backend. It reports the
 * connection, not the device's physical position. */
export interface HandyConnectionState {
  /** "handy" only in a reply to the primary Handy's candidate connect. */
  id: HandyInstanceId | "handy";
  connected: boolean;
  connecting: boolean;
  status: "disconnected" | "connecting" | "connected" | "lost" | string;
  message?: string;
  latency_ms?: number;
  simulated: boolean;
  native_available: boolean;
  native_supported: boolean;
  /** The Intiface device and actuator this Handy plays on, once chosen. */
  selected_device_index?: number;
  selected_actuator_index?: number;
}

/** A nearby Handy from native discovery. The ID is an opaque selection
 * token, never a Bluetooth address. */
export interface HandyCandidate {
  id: string;
  name: string;
  rssi: number;
  /** Another connection holds it; it cannot be chosen. */
  in_use: boolean;
}

/** PATCH /api/settings/handys/{id}: only the fields that changed. */
export interface HandyConnectionPatch {
  name?: string;
  method?: HandyConnectionMethod | string;
  handy_model?: HandyModel | string;
  connection_key?: string;
  clear_connection_key?: boolean;
}
