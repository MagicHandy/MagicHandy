import type { EngineSnapshot, MotionPaceSummary, MotionSettings } from "./types";

export type MotionLabMethod = "creative" | "anchored" | "directional" | "combined";
export interface MotionLabRequest {
  speed_percent: number;
  center_percent: number;
  span_percent: number;
  span_min_percent: number;
  span_profile: "steady" | "breathe" | "wander" | "contrast";
  variation_percent: number;
  range_anchor_percent: number;
  outbound_time_percent: number;
  seed: number;
}
export interface MotionLabCandidate {
  method: MotionLabMethod;
  target: NonNullable<EngineSnapshot["target"]>;
  period_ms: number;
  outbound_time_percent: number;
  maximum_acceleration: number;
  maximum_jerk: number;
  perceptual: {
    position_min_percent: number;
    position_max_percent: number;
    mean_stroke_percent: number;
    minimum_local_stroke_cv: number;
    minimum_local_stroke_range_percent: number;
    commanded_peak_velocity_percent_per_second: number;
    pace: MotionPaceSummary;
  };
  samples: Array<{ time_ms: number; position_percent: number; velocity_percent_per_second: number }>;
}
export interface MotionLabPreview {
  version: number;
  request: MotionLabRequest;
  settings: MotionSettings;
  settings_key: string;
  preview_ms: number;
  candidates: MotionLabCandidate[];
}

export interface MotionLabProposalResult {
  proposal: { method: MotionLabMethod; range_anchor_percent: number; outbound_time_percent: number; explanation: string };
  preview: MotionLabPreview;
  model: string;
  elapsed_ms: number;
  prompt: string;
}
