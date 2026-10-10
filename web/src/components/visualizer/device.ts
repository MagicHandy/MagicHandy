// Which drawing the motion visualizer shows. The drawing follows the device
// the backend dispatches to, never a guess from the motion itself: Intiface
// devices are generic linear strokers; Handy Cloud and Browser Bluetooth drive
// a Handy, whose model is the one chosen in the connection panel.

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
}

/** Designs that ship but are not selected for any device yet. */
export const DISABLED_VISUALIZER_KINDS: readonly VisualizerKind[] = ["handy-front", "ribbon"];

export function visualizerKind(device: VisualizerDevice | undefined): VisualizerKind {
  if (device?.owner === "intiface") return "scan";
  return device?.model === "handy_original" ? "handy-original" : "handy-dots";
}

/** Full travel in millimetres, from the Handy model specifications. */
export function handyTravelMillimetres(model: string | undefined): number {
  return model === "handy_original" ? 110 : 125;
}
