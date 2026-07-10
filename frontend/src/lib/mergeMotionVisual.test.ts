import { describe, expect, it } from "vitest";
import { mergeMotionVisual } from "./mergeMotionVisual";
import type { MotionVisual, StatusSnapshot } from "../api/types";

const baseVisual: MotionVisual = {
  position_pct: 40,
  target_pct: 40,
  offset_ms: -160,
  stroke_min_pct: 10,
  stroke_max_pct: 90,
  recent: [{ t_ms: 0, pos_pct: 40 }],
  curve_actions: [{ at: 0, pos: 40 }],
  schedule_active: true,
  playback_active: false,
  measured_rtt_ms: 200,
};

const baseSnap = {
  manual_queue_playing: false,
  playback_active: false,
  direct_control_active: false,
  motion_position_pct: 55,
} as StatusSnapshot;

describe("mergeMotionVisual", () => {
  it("returns null when all inputs are empty", () => {
    expect(mergeMotionVisual({ cached: null, motion: null, snap: null })).toBeNull();
  });

  it("keeps curve fields from cached while updating position from SSE", () => {
    const merged = mergeMotionVisual({
      cached: baseVisual,
      motion: {
        available: true,
        engine: {
          running: true,
          paused: false,
          last_sample: { position_percent: 72, time_millis: 100 },
        },
      },
      snap: baseSnap,
    });
    expect(merged?.live_position_pct).toBe(72);
    expect(merged?.curve_actions).toEqual(baseVisual.curve_actions);
    expect(merged?.schedule_active).toBe(true);
    expect(merged?.playback_active).toBe(true);
  });

  it("uses status flags for playback when SSE engine is idle", () => {
    const merged = mergeMotionVisual({
      cached: baseVisual,
      motion: { available: true },
      snap: { ...baseSnap, direct_control_active: true },
    });
    expect(merged?.playback_active).toBe(true);
  });

  it("builds defaults from status when cache is missing", () => {
    const merged = mergeMotionVisual({
      cached: null,
      motion: null,
      snap: {
        ...baseSnap,
        motion_position_pct: 33,
        sync_offset_ms: -200,
        min_position: 5,
        max_position: 95,
      },
    });
    expect(merged?.position_pct).toBe(33);
    expect(merged?.offset_ms).toBe(-200);
    expect(merged?.stroke_min_pct).toBe(5);
    expect(merged?.stroke_max_pct).toBe(95);
  });
});
