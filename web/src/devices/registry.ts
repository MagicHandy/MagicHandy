// Every supported device, in run-clock order. A new device adds one entry
// here and its transport; the panel, the chip, Settings > Devices and the
// device pages render it from the descriptor.
import { DEVICE_ORDER } from "../api/devices";
import type { DeviceId } from "../api/types";
import { coyoteDevice } from "./coyote";
import { handyDevice } from "./handy";
import { useDeviceContext } from "./runtime";
import type { DeviceContext, DeviceDescriptor, DeviceModel } from "./types";
import { vibratorDevice } from "./vibrator";

const DESCRIPTORS: Record<DeviceId, DeviceDescriptor> = {
  handy: handyDevice,
  coyote: coyoteDevice,
  vibrator: vibratorDevice,
};

export const DEVICE_DESCRIPTORS: DeviceDescriptor[] = DEVICE_ORDER.map((id) => DESCRIPTORS[id]);

export function deviceDescriptor(id: string): DeviceDescriptor | undefined {
  return DEVICE_DESCRIPTORS.find((descriptor) => descriptor.id === id);
}

export interface DeviceEntry {
  descriptor: DeviceDescriptor;
  model: DeviceModel;
}

/** The saved devices with their live models, and the ones that can still be
 * added. Until the first backend snapshot arrives nothing is known: no
 * device is listed, and `initializing` says why. */
export function useDevices(): { ctx: DeviceContext; devices: DeviceEntry[]; addable: DeviceDescriptor[]; initializing: boolean } {
  const ctx = useDeviceContext();
  const initializing = !ctx.settings;
  const configured = initializing ? [] : ctx.configured;
  const devices = DEVICE_DESCRIPTORS
    .filter((descriptor) => configured.includes(descriptor.id))
    .map((descriptor) => ({ descriptor, model: descriptor.model(ctx) }));
  return { ctx, devices, addable: initializing ? [] : DEVICE_DESCRIPTORS.filter((descriptor) => !configured.includes(descriptor.id)), initializing };
}
