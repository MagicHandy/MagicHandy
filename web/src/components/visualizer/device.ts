// Which drawing the motion visualizer shows. The drawing follows the device
// the backend dispatches to, never a guess from the motion itself: Intiface
// devices are generic linear strokers; Handy Cloud and Browser Bluetooth drive
// a Handy, whose model is the one chosen in the connection panel.

import { isHandy, playbackDevices } from "../../api/devices";
import type { PublicSettings } from "../../api/types";

export type VisualizerKind =
  | "handy-dots" // Handy 2 Standard and Pro: the device's own dot display.
  | "handy-original" // Original Handy: refined front view.
  | "scan" // Intiface: a track with a moving bar.
  | "handy-front" // Handy 2 front view. Published, not enabled.
  | "ribbon"; // Intiface ribbon timeline. Published, not enabled.

export interface VisualizerDevice {
  /** settings.device.hsp_dispatch_owner */
  owner?: string;
  /** The Handy model (engine settings first, then saved settings). */
  model?: string;
  /** No Handy plays, only another stroker such as an Autoblow device: the
   * drawing shows percent, not a Handy's millimetres. */
  generic?: boolean;
}

/** Designs that ship but are not selected for any device yet. */
export const DISABLED_VISUALIZER_KINDS: readonly VisualizerKind[] = ["handy-front", "ribbon"];

export function visualizerKind(device: VisualizerDevice | undefined): VisualizerKind {
  if (device?.owner === "intiface" || device?.generic) return "scan";
  return device?.model === "handy_original" ? "handy-original" : "handy-dots";
}

/** Full travel in millimetres, from the Handy model specifications. */
export function handyTravelMillimetres(model: string | undefined): number {
  return model === "handy_original" ? 110 : 125;
}

/** The dispatch owner and saved Handy model, which pick the visualizer drawing.
 * Playback with an Autoblow device and no Handy draws a generic track. */
export function visualizerDeviceFor(settings: PublicSettings | undefined): VisualizerDevice {
  const playing = playbackDevices(settings?.device);
  const generic = !playing.some(isHandy) && playing.some((id) => id === "autoblow" || id === "vacuglide");
  return { owner: settings?.device?.hsp_dispatch_owner, model: settings?.motion?.handy_model, generic };
}
