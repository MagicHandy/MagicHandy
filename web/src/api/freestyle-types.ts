// FreestyleSettings mirror config.FreestyleSettings. A named feel is
// authoritative: the backend replaces its control values with the preset.
export interface FreestyleSettings {
  feel: "gentle" | "balanced" | "intense" | "custom" | string;
  pace_percent: number;
  length_percent: number;
  focus_percent: number;
  roaming_percent: number;
  variety_percent: number;
  accent: "even" | "tip" | "base" | string;
  shape: "steady" | "build" | "waves" | "edge" | "cooldown" | string;
  shape_minutes: number;
}

// FreestyleStatus is the running stream's visible shape.
export interface FreestyleStatus {
  feel: string;
  shape: string;
  shape_phase?: string;
  shape_progress_percent?: number;
  energy_percent: number;
  ending?: boolean;
}
