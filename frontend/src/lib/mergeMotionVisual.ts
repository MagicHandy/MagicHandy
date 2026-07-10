import type { MotionInfo, MotionVisual, StatusSnapshot } from "../api/types";

export type MergeMotionVisualInput = {
  cached: MotionVisual | null;
  motion: MotionInfo | null;
  snap: StatusSnapshot | null;
};

function playbackFromSnap(snap: StatusSnapshot | null): boolean {
  return Boolean(
    snap?.manual_queue_playing ||
      snap?.playback_active ||
      snap?.direct_control_active,
  );
}

function playbackFromMotion(motion: MotionInfo | null): boolean {
  const engine = motion?.engine;
  return Boolean(engine?.running || engine?.paused);
}

function positionFromMotion(motion: MotionInfo | null): number | null {
  const pct = motion?.engine?.last_sample?.position_percent;
  return pct != null ? pct : null;
}

function defaultVisual(snap: StatusSnapshot | null): MotionVisual {
  return {
    position_pct: snap?.motion_position_pct ?? 50,
    target_pct: snap?.motion_position_pct ?? 50,
    offset_ms: snap?.sync_offset_ms ?? -160,
    stroke_min_pct: snap?.min_position ?? 10,
    stroke_max_pct: snap?.max_position ?? 90,
    recent: [],
    playback_active: playbackFromSnap(snap),
    measured_rtt_ms: snap?.measured_rtt_ms ?? null,
    live_position_pct: snap?.motion_position_pct ?? 50,
  };
}

/**
 * Merges a one-shot /api/motion/visual snapshot with live SSE motion and status
 * flags. Curve and schedule fields stay on `cached` until refreshed on playback
 * start; position and playback flags follow SSE + status.
 */
export function mergeMotionVisual({
  cached,
  motion,
  snap,
}: MergeMotionVisualInput): MotionVisual | null {
  if (!cached && !motion && !snap) return null;

  const base = cached ?? defaultVisual(snap);
  const livePos = positionFromMotion(motion);
  const positionPct =
    livePos ?? base.live_position_pct ?? base.position_pct ?? snap?.motion_position_pct ?? 50;
  const playbackActive =
    playbackFromMotion(motion) || playbackFromSnap(snap) || Boolean(base.playback_active);

  return {
    ...base,
    position_pct: positionPct,
    target_pct: positionPct,
    live_position_pct: positionPct,
    playback_active: playbackActive,
    stroke_min_pct: base.stroke_min_pct ?? snap?.min_position ?? 10,
    stroke_max_pct: base.stroke_max_pct ?? snap?.max_position ?? 90,
    offset_ms: base.offset_ms ?? snap?.sync_offset_ms ?? -160,
    measured_rtt_ms: base.measured_rtt_ms ?? snap?.measured_rtt_ms ?? null,
  };
}
