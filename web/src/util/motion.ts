import type { EngineSnapshot } from "../api/types";

export function ownsActiveMotion(engine: EngineSnapshot | undefined, source: string): boolean {
  if (!engine || engine.target?.source !== source) return false;
  return Boolean(engine.running || engine.starting || engine.paused || engine.completing);
}
