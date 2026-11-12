// The tempo channel (internal/motion/tempo.go, ADR 0071): how fast a stroker
// that makes its own strokes, such as the VacuGlide, strokes. Levels are
// percentages of the person's Tempo range: 0 stops the motor, 1–100 span it.

export type TempoMode = "follow" | "steady" | "ramp" | "wave" | "pulse" | "pause" | "sequence" | "routine";

export interface TempoSection {
  mode: Exclude<TempoMode, "follow" | "sequence" | "routine">;
  level_percent?: number;
  min_percent?: number;
  max_percent?: number;
  period_seconds?: number;
  seconds?: number;
}

export interface TempoSpec {
  mode: TempoMode | string;
  level_percent?: number;
  min_percent?: number;
  max_percent?: number;
  period_seconds?: number;
  sections?: TempoSection[];
  loop?: boolean;
  routine?: string;
  seed?: number;
}

/** The engine's compact tempo readout: the level now and, for an envelope,
 * the next eight seconds. Backend-authoritative; never a measurement. */
export interface TempoReadout {
  mode: TempoMode | string;
  routine?: string;
  level_percent: number;
  ahead?: number[];
}

export interface TempoRoutine {
  id: string;
  name: string;
  description: string;
  loop: boolean;
}
