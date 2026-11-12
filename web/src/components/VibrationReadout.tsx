import type { MotionInfo, PublicSettings } from "../api/types";
import { playsPositions, playbackDevices } from "../api/devices";

/** The active run's channel, or the next run's saved device selection. */
export function showsVibration(settings: PublicSettings | undefined, motion: MotionInfo | null): boolean {
  const engine = motion?.engine;
  if (engine?.running || engine?.paused || engine?.starting || engine?.completing) return Boolean(engine.target?.vibration_only);
  const outputs = playbackDevices(settings?.device);
  return outputs.includes("vibrator") && !outputs.includes("vacuglide") && !outputs.some(playsPositions);
}
