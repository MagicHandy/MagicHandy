// Every supported device, in run-clock order. A new device kind adds one
// entry here and its transport; the panel, the chip, Settings > Devices and
// the device pages render it from the descriptor. Additional Handys are
// described one per saved instance, from the settings.
import { DEVICE_ORDER, MAX_HANDYS, isHandy, isHandyInstance } from "../api/devices";
import type { DeviceId, DeviceKind } from "../api/types";
import { additionalHandyDevice } from "./additional-handy";
import { coyoteDevice } from "./coyote";
import { handyDevice } from "./handy";
import { useDeviceContext } from "./runtime";
import type { DeviceContext, DeviceDescriptor, DeviceModel } from "./types";
import { vibratorDevice } from "./vibrator";

const DESCRIPTORS: Record<DeviceKind, DeviceDescriptor> = {
  handy: handyDevice,
  coyote: coyoteDevice,
  vibrator: vibratorDevice,
};

export const DEVICE_DESCRIPTORS: DeviceDescriptor[] = DEVICE_ORDER.map((id) => DESCRIPTORS[id]);

/** A device kind's descriptor; additional Handys come from useDevices. */
export function deviceDescriptor(id: string): DeviceDescriptor | undefined {
  return DEVICE_DESCRIPTORS.find((descriptor) => descriptor.id === id);
}

export interface DeviceEntry {
  descriptor: DeviceDescriptor;
  model: DeviceModel;
}

function savedDescriptor(id: DeviceId, ctx: DeviceContext): DeviceDescriptor | undefined {
  return isHandyInstance(id) ? additionalHandyDevice(id, ctx.settings) : deviceDescriptor(id);
}

/** The saved devices with their live models, in the backend's run-clock
 * order, and the kinds that can still be added: the Handy until there are
 * eight. Until the first backend snapshot arrives nothing is known: no
 * device is listed, and `initializing` says why. */
export function useDevices(): { ctx: DeviceContext; devices: DeviceEntry[]; addable: DeviceDescriptor[]; initializing: boolean } {
  const ctx = useDeviceContext();
  const initializing = !ctx.settings;
  const configured = initializing ? [] : ctx.configured;
  const devices = configured.flatMap((id) => {
    const descriptor = savedDescriptor(id, ctx);
    return descriptor ? [{ descriptor, model: descriptor.model(ctx) }] : [];
  });
  const handys = configured.filter(isHandy).length;
  const addable = initializing ? [] : DEVICE_DESCRIPTORS.filter((descriptor) => (descriptor.id === "handy" ? handys < MAX_HANDYS : !configured.includes(descriptor.id)));
  return { ctx, devices, addable, initializing };
}
