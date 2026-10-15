// Device inventory and playback participation (internal/config/
// device_connections.go). The backend owns both; these helpers only read its
// snapshot, migrating the legacy output an older backend reports.
import type { DeviceId, DeviceOutput } from "./coyote-types";

/** Saved devices in run-clock order: the first playing device is the clock. */
export const DEVICE_ORDER: DeviceId[] = ["handy", "coyote"];

interface DeviceSnapshot {
  playback_devices?: DeviceId[];
  configured_devices?: DeviceId[];
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
  const devices = device?.playback_devices ?? fromLegacyOutput(device?.output);
  return DEVICE_ORDER.filter((id) => devices.includes(id));
}

/** The saved devices, whether or not they play. */
export function configuredDevices(device: DeviceSnapshot | undefined): DeviceId[] {
  const devices = device?.configured_devices ?? fromLegacyOutput(device?.output);
  return DEVICE_ORDER.filter((id) => devices.includes(id));
}
