// Device inventory and playback participation (internal/config/
// device_connections.go). The backend owns both; these helpers only read its
// snapshot, migrating the legacy output an older backend reports. Additional
// Handys keep their instance IDs: nothing here drops one the backend reports.
import type { DeviceId, DeviceKind, DeviceOutput, HandyInstanceId } from "./coyote-types";

/** The device kinds in run-clock order: the first playing device is the clock. */
export const DEVICE_ORDER: DeviceKind[] = ["handy", "coyote", "vibrator"];

/** The most Handys, the primary one included. The backend enforces it. */
export const MAX_HANDYS = 8;

/** An additional Handy's opaque instance ID. */
export function isHandyInstance(id: string): id is HandyInstanceId {
  return id.startsWith("handy-") && id.length > "handy-".length;
}

/** The primary Handy or an additional one. */
export function isHandy(id: string): boolean {
  return id === "handy" || isHandyInstance(id);
}

// The primary Handy, the additional Handys in the backend's order, the
// Coyote, then the vibrator.
function rank(id: string): number {
  if (id === "handy") return 0;
  if (isHandyInstance(id)) return 1;
  return id === "coyote" ? 2 : id === "vibrator" ? 3 : -1;
}

/** A device this client can show: a device kind or an additional Handy. */
export function isKnownDevice(id: string): id is DeviceId {
  return rank(id) >= 0;
}

function runClockOrder(devices: readonly string[]): DeviceId[] {
  return devices
    .filter((id, index): id is DeviceId => isKnownDevice(id) && devices.indexOf(id) === index)
    .sort((a, b) => rank(a) - rank(b));
}

interface DeviceSnapshot {
  playback_devices?: readonly string[];
  configured_devices?: readonly string[];
  output?: DeviceOutput | string;
}

function fromLegacyOutput(output: DeviceOutput | string | undefined): DeviceId[] {
  switch (output) {
    case "none": return [];
    case "coyote": return ["coyote"];
    case "handy_coyote": return ["handy", "coyote"];
    case "handy":
    case undefined:
    case "": return ["handy"];
    // An unknown value is never read as the Handy: nothing plays.
    default: return [];
  }
}

/** The devices in the next playback run, in run-clock order. */
export function playbackDevices(device: DeviceSnapshot | undefined): DeviceId[] {
  return runClockOrder(device?.playback_devices ?? fromLegacyOutput(device?.output));
}

/** The saved devices, whether or not they play. */
export function configuredDevices(device: DeviceSnapshot | undefined): DeviceId[] {
  return runClockOrder(device?.configured_devices ?? fromLegacyOutput(device?.output));
}
