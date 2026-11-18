import type { EngineSnapshot } from "../api/types";

export function motionPhase(engine: EngineSnapshot | undefined, available: boolean | undefined) {
  if (available === false) return "unavailable";
  if (engine?.paused) return "paused";
  if (engine?.completing) return "stopping";
  if (engine?.starting) return "starting";
  if (engine?.running) return "running";
  return available || engine ? "idle" : "unavailable";
}
