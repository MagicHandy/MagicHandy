import { useEffect, useRef, useState } from "react";
import type { MotionVisual } from "../api/types";
import {
  interpolateWaveformPosition,
  type FunscriptAction,
} from "./funscriptHeatmap";

const SPARK_WINDOW_MS = 4000;
const SPARK_STEP_MS = 24;
const CURVE_RESYNC_DRIFT_MS = 80;

export type FluidMotionFrame = {
  pos: number;
  pathD: string;
};

function buildSparkPathFromCurve(
  actions: FunscriptAction[],
  endMs: number,
  windowMs: number,
): string {
  if (actions.length < 1 || endMs < 0) return "";
  const t0 = Math.max(0, endMs - windowMs);
  const span = Math.max(1, endMs - t0);
  const parts: string[] = [];
  for (let t = t0; t <= endMs; t += SPARK_STEP_MS) {
    const x = ((t - t0) / span) * 100;
    const y = 100 - interpolateWaveformPosition(actions, t);
    parts.push(`${parts.length === 0 ? "M" : "L"} ${x} ${y}`);
  }
  return parts.join(" ");
}

function buildSparkPathFromRecent(
  recent: { t_ms: number; pos_pct: number }[],
): string {
  if (recent.length < 2) return "";
  const endT = recent[recent.length - 1].t_ms;
  return recent
    .map((p, i) => {
      const x = (p.t_ms / Math.max(endT, 1)) * 100;
      const y = 100 - p.pos_pct;
      return `${i === 0 ? "M" : "L"} ${x} ${y}`;
    })
    .join(" ");
}

function extrapolateRecent(
  recent: { t_ms: number; pos_pct: number }[],
  extraMs: number,
): number {
  if (recent.length === 0) return 50;
  if (recent.length === 1) return recent[0].pos_pct;
  const a = recent[recent.length - 2];
  const b = recent[recent.length - 1];
  const dt = Math.max(1, b.t_ms - a.t_ms);
  const velocity = (b.pos_pct - a.pos_pct) / dt;
  return Math.max(0, Math.min(100, b.pos_pct + velocity * extraMs));
}

function sameCurveScript(
  a: FunscriptAction[],
  b: FunscriptAction[],
): boolean {
  if (a.length !== b.length || a.length === 0) return false;
  return a[0].at === b[0].at && a[a.length - 1].at === b[a.length - 1].at;
}

type CurveAnchor = {
  actions: FunscriptAction[];
  elapsedMs: number;
  durationMs: number;
  at: number;
};

/**
 * Um único loop rAF para marcador vertical + sparkline — mesma posição que a bolinha do reprodutor.
 */
export function useFluidMotionVisual(
  visual: MotionVisual | null,
  fallbackPct: number,
): FluidMotionFrame {
  const [frame, setFrame] = useState<FluidMotionFrame>({
    pos: fallbackPct,
    pathD: "",
  });
  const visualRef = useRef(visual);
  const fallbackRef = useRef(fallbackPct);
  const curveRef = useRef<CurveAnchor | null>(null);
  const recentRef = useRef<{ t_ms: number; pos_pct: number }[]>([]);
  const pollAtRef = useRef(performance.now());
  const playbackActiveRef = useRef(false);
  const livePosRef = useRef<number | null>(null);

  useEffect(() => {
    visualRef.current = visual;
    fallbackRef.current = fallbackPct;
    playbackActiveRef.current = Boolean(visual?.playback_active);
    livePosRef.current =
      visual?.live_position_pct ?? visual?.position_pct ?? null;

    const actions = visual?.curve_actions;
    if (visual?.schedule_active && actions?.length && visual.curve_elapsed_ms != null) {
      const prev = curveRef.current;
      const serverElapsed = visual.curve_elapsed_ms;
      const durationMs = visual.curve_duration_ms ?? prev?.durationMs ?? 0;

      if (!prev || !sameCurveScript(prev.actions, actions)) {
        curveRef.current = {
          actions,
          elapsedMs: serverElapsed,
          durationMs,
          at: performance.now(),
        };
      } else {
        const localElapsed = prev.elapsedMs + (performance.now() - prev.at);
        const drift = Math.abs(serverElapsed - localElapsed);
        if (drift > CURVE_RESYNC_DRIFT_MS) {
          curveRef.current = {
            actions,
            elapsedMs: serverElapsed,
            durationMs,
            at: performance.now(),
          };
        } else if (durationMs !== prev.durationMs) {
          curveRef.current = { ...prev, durationMs };
        }
      }
    } else if (!playbackActiveRef.current) {
      curveRef.current = null;
    }

    if (visual?.recent?.length) {
      recentRef.current = visual.recent;
    }
    pollAtRef.current = performance.now();

    if (!curveRef.current) {
      const pos = livePosRef.current ?? fallbackPct;
      setFrame((prev) => ({
        ...prev,
        pos,
        pathD: buildSparkPathFromRecent(recentRef.current),
      }));
    }
  }, [visual, fallbackPct]);

  useEffect(() => {
    let raf = 0;
    const tick = () => {
      const curve = curveRef.current;
      if (curve?.actions.length) {
        const elapsed = performance.now() - curve.at;
        const durationCap =
          curve.durationMs > 0 ? curve.durationMs : Number.POSITIVE_INFINITY;
        const ms = Math.min(curve.elapsedMs + elapsed, durationCap);
        const pos = interpolateWaveformPosition(curve.actions, ms);
        setFrame({
          pos,
          pathD: buildSparkPathFromCurve(
            curve.actions,
            ms,
            SPARK_WINDOW_MS,
          ),
        });
      } else {
        const extraMs = performance.now() - pollAtRef.current;
        const live = livePosRef.current;
        const pos =
          live != null
            ? live
            : extrapolateRecent(recentRef.current, extraMs);
        const blended =
          live != null
            ? pos
            : Math.abs(pos - fallbackRef.current) > 0.5
              ? pos * 0.65 + fallbackRef.current * 0.35
              : fallbackRef.current;
        setFrame({
          pos: blended,
          pathD: buildSparkPathFromRecent(recentRef.current),
        });
      }
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, []);

  return frame;
}
