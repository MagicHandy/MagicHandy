import { useEffect, useRef, useState, type RefObject } from "react";
import type { MotionVisualStream, MotionVisualStreamPoint } from "../api/types";

const SPARK_WINDOW_MS = 4000;

type VisualAnchor = {
  perfMs: number;
  streamMs: number;
  offsetMs: number;
  points: MotionVisualStreamPoint[];
  active: boolean;
  curveHeadMs: number;
};

function interpolateStreamPosition(
  points: MotionVisualStreamPoint[],
  elapsedMs: number,
): number {
  if (points.length === 0) return 50;
  if (elapsedMs <= points[0].t_ms) return points[0].pos_pct;
  const last = points[points.length - 1];
  if (elapsedMs >= last.t_ms) return last.pos_pct;
  for (let i = 0; i < points.length - 1; i++) {
    const a = points[i];
    const b = points[i + 1];
    if (elapsedMs < a.t_ms || elapsedMs > b.t_ms) continue;
    const span = b.t_ms - a.t_ms;
    if (span <= 0) return b.pos_pct;
    const t = (elapsedMs - a.t_ms) / span;
    return a.pos_pct + t * (b.pos_pct - a.pos_pct);
  }
  return last.pos_pct;
}

function drawSparkline(
  ctx: CanvasRenderingContext2D,
  points: MotionVisualStreamPoint[],
  relativeMs: number,
  markerPct: number,
  width: number,
  height: number,
) {
  const dpr = window.devicePixelRatio || 1;
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  const cssW = width / dpr;
  const cssH = height / dpr;
  ctx.clearRect(0, 0, cssW, cssH);

  const span = Math.max(1, SPARK_WINDOW_MS);

  if (points.length >= 2) {
    ctx.beginPath();
    let started = false;
    for (const point of points) {
      const x = (point.t_ms / span) * cssW;
      const y = cssH - (point.pos_pct / 100) * cssH;
      if (!started) {
        ctx.moveTo(x, y);
        started = true;
      } else {
        ctx.lineTo(x, y);
      }
    }
    const gradient = ctx.createLinearGradient(0, 0, cssW, 0);
    gradient.addColorStop(0, "#6366f1");
    gradient.addColorStop(1, "#a78bfa");
    ctx.strokeStyle = gradient;
    ctx.lineWidth = 1.5;
    ctx.stroke();
  }

  const markerX = Math.min(cssW - 6, (relativeMs / span) * cssW);
  const markerY = cssH - (markerPct / 100) * cssH;
  ctx.beginPath();
  ctx.arc(markerX, markerY, 4, 0, Math.PI * 2);
  ctx.fillStyle = "#c4b5fd";
  ctx.fill();
}

export type MotionVisualizerFrame = {
  positionPct: number;
  playbackActive: boolean;
};

/**
 * Zero-latency canvas visualizer driven by Go SSE schedule points.
 * Interpolates on requestAnimationFrame using performance.now().
 */
export function useMotionVisualizer(enabled = true): {
  canvasRef: RefObject<HTMLCanvasElement>;
  frame: MotionVisualizerFrame;
} {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const anchorRef = useRef<VisualAnchor>({
    perfMs: 0,
    streamMs: 0,
    offsetMs: 0,
    points: [],
    active: false,
    curveHeadMs: 0,
  });
  const [frame, setFrame] = useState<MotionVisualizerFrame>({
    positionPct: 50,
    playbackActive: false,
  });

  useEffect(() => {
    if (!enabled) return;
    let source: EventSource | null = null;
    try {
      source = new EventSource("/api/motion/visual/stream");
      source.addEventListener("visual", (ev) => {
        try {
          const payload = JSON.parse((ev as MessageEvent).data) as MotionVisualStream;
          const points = payload.points ?? [];
          const head =
            points.length > 0 ? points[points.length - 1].t_ms : 0;
          anchorRef.current = {
            perfMs: performance.now(),
            streamMs: payload.stream_elapsed_ms,
            offsetMs: payload.offset_ms,
            points,
            active: Boolean(payload.active),
            curveHeadMs: head,
          };
          setFrame((prev) => ({
            ...prev,
            playbackActive: Boolean(payload.active),
          }));
        } catch {
          /* ignore malformed payload */
        }
      });
      source.onerror = () => {
        anchorRef.current.active = false;
        setFrame((prev) => ({ ...prev, playbackActive: false }));
      };
    } catch {
      source = null;
    }
    return () => source?.close();
  }, [enabled]);

  useEffect(() => {
    if (!enabled) return;
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const resize = () => {
      const rect = canvas.getBoundingClientRect();
      const dpr = window.devicePixelRatio || 1;
      canvas.width = Math.max(1, Math.floor(rect.width * dpr));
      canvas.height = Math.max(1, Math.floor(rect.height * dpr));
    };
    resize();
    const observer = new ResizeObserver(resize);
    observer.observe(canvas);

    let raf = 0;
    const tick = () => {
      const anchor = anchorRef.current;
      if (anchor.active && anchor.points.length > 0) {
        const delta = performance.now() - anchor.perfMs;
        const relativeMs = anchor.curveHeadMs + delta;
        const pos = interpolateStreamPosition(anchor.points, relativeMs);
        drawSparkline(ctx, anchor.points, relativeMs, pos, canvas.width, canvas.height);
        setFrame({ positionPct: pos, playbackActive: true });
      }
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => {
      cancelAnimationFrame(raf);
      observer.disconnect();
    };
  }, [enabled]);

  return { canvasRef, frame };
}
