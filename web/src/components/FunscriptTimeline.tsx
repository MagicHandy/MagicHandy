import { useEffect, useRef, type KeyboardEvent as ReactKeyboardEvent, type PointerEvent as ReactPointerEvent } from "react";
import type { MediaFunscript } from "../api/types";
import { formatTimelineTime } from "./ImportTimeline";

const TIMELINE_HEIGHT = 88;
const INTENSITY_SLOW = 50;
const INTENSITY_FAST = 200;
const INTENSITY_VERY_FAST = 400;

interface Props {
  script: MediaFunscript;
  currentTime: number;
  hidden: boolean;
  onSeek: (milliseconds: number) => void;
}

export function FunscriptTimeline({ script, currentTime, hidden, onSeek }: Props) {
  const baseRef = useRef<HTMLCanvasElement>(null);
  const playheadRef = useRef<HTMLCanvasElement>(null);
  const dragging = useRef(false);
  const duration = Math.max(1, script.duration_ms);
  const progress = Math.max(0, Math.min(1, currentTime / duration));

  useEffect(() => {
    if (hidden) return undefined;
    const canvas = baseRef.current;
    if (!canvas) return undefined;
    const redraw = () => drawFunscript(canvas, script);
    redraw();
    if (typeof ResizeObserver === "undefined") {
      window.addEventListener("resize", redraw);
      return () => window.removeEventListener("resize", redraw);
    }
    const observer = new ResizeObserver(redraw);
    observer.observe(canvas);
    return () => observer.disconnect();
  }, [hidden, script]);

  useEffect(() => {
    if (hidden) return;
    const canvas = playheadRef.current;
    if (!canvas) return;
    sizeCanvas(canvas, TIMELINE_HEIGHT);
    const context = canvas.getContext("2d");
    if (!context) return;
    const width = Math.max(1, canvas.clientWidth || 760);
    context.clearRect(0, 0, width, TIMELINE_HEIGHT);
    const x = Math.round(progress * width) + 0.5;
    context.strokeStyle = cssColor("--text", "#f4f7fa");
    context.lineWidth = 1;
    context.beginPath();
    context.moveTo(x, 0);
    context.lineTo(x, TIMELINE_HEIGHT);
    context.stroke();
  }, [currentTime, hidden, progress]);

  function seekAtClientX(clientX: number) {
    const bounds = baseRef.current?.getBoundingClientRect();
    if (!bounds || bounds.width <= 0) return;
    const ratio = Math.max(0, Math.min(1, (clientX - bounds.left) / bounds.width));
    onSeek(Math.round(ratio * duration));
  }

  function startSeek(event: ReactPointerEvent<HTMLDivElement>) {
    if (event.button > 0) return;
    dragging.current = true;
    event.currentTarget.setPointerCapture?.(event.pointerId);
    seekAtClientX(event.clientX);
    event.preventDefault();
  }

  function moveSeek(event: ReactPointerEvent<HTMLDivElement>) {
    if (dragging.current) seekAtClientX(event.clientX);
  }

  function finishSeek(event: ReactPointerEvent<HTMLDivElement>) {
    dragging.current = false;
    if (event.currentTarget.hasPointerCapture?.(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
  }

  function keyboardSeek(event: ReactKeyboardEvent<HTMLDivElement>) {
    const step = event.shiftKey ? 30_000 : 5_000;
    let next = currentTime;
    switch (event.key) {
      case "ArrowLeft":
      case "ArrowDown":
        next -= step;
        break;
      case "ArrowRight":
      case "ArrowUp":
        next += step;
        break;
      case "Home":
        next = 0;
        break;
      case "End":
        next = duration;
        break;
      default:
        return;
    }
    event.preventDefault();
    onSeek(Math.max(0, Math.min(duration, next)));
  }

  if (hidden) {
    return (
      <div className="media-timeline-collapsed" aria-label={`Funscript progress ${formatTimelineTime(currentTime)} of ${formatTimelineTime(duration)}`}>
        <span style={{ width: `${progress * 100}%` }} />
      </div>
    );
  }

  return (
    <div
      className="media-timeline-frame"
      role="slider"
      tabIndex={0}
      aria-label={`Funscript timeline, ${script.action_count.toLocaleString()} actions`}
      aria-valuemin={0}
      aria-valuemax={duration}
      aria-valuenow={Math.round(currentTime)}
      aria-valuetext={`${formatTimelineTime(currentTime)} of ${formatTimelineTime(duration)}`}
      title="Click or drag to seek; arrow keys move five seconds"
      onPointerDown={startSeek}
      onPointerMove={moveSeek}
      onPointerUp={finishSeek}
      onPointerCancel={finishSeek}
      onLostPointerCapture={finishSeek}
      onKeyDown={keyboardSeek}
    >
      <canvas ref={baseRef} className="media-timeline-canvas" aria-hidden="true" />
      <canvas ref={playheadRef} className="media-timeline-playhead" aria-hidden="true" />
    </div>
  );
}

function drawFunscript(canvas: HTMLCanvasElement, script: MediaFunscript) {
  sizeCanvas(canvas, TIMELINE_HEIGHT);
  const context = canvas.getContext("2d");
  if (!context) return;
  const width = Math.max(1, Math.floor(canvas.clientWidth || 760));
  const duration = Math.max(1, script.duration_ms);
  const actions = script.actions;
  context.clearRect(0, 0, width, TIMELINE_HEIGHT);
  context.fillStyle = cssColor("--bg-inset", "#111418");
  context.fillRect(0, 0, width, TIMELINE_HEIGHT);
  context.strokeStyle = cssColor("--line", "#343a42");
  context.lineWidth = 1;
  context.beginPath();
  context.moveTo(0, TIMELINE_HEIGHT / 2 + 0.5);
  context.lineTo(width, TIMELINE_HEIGHT / 2 + 0.5);
  context.stroke();
  if (actions.length < 2) return;

  const minimum = new Float64Array(width);
  const maximum = new Float64Array(width);
  const bucketSpeed = new Float64Array(width);
  minimum.fill(Number.POSITIVE_INFINITY);
  maximum.fill(Number.NEGATIVE_INFINITY);
  for (let index = 1; index < actions.length; index++) {
    const left = actions[index - 1];
    const right = actions[index];
    const bucket = Math.max(0, Math.min(width - 1, Math.floor((right.at / duration) * width)));
    minimum[bucket] = Math.min(minimum[bucket], left.pos, right.pos);
    maximum[bucket] = Math.max(maximum[bucket], left.pos, right.pos);
    const elapsed = Math.max(1, right.at - left.at);
    bucketSpeed[bucket] = Math.max(bucketSpeed[bucket], Math.abs(right.pos - left.pos) * 1000 / elapsed);
  }

  let actionIndex = 1;
  let previousY = yForPosition(actions[0].pos);
  for (let x = 1; x < width; x++) {
    const at = (x / Math.max(1, width - 1)) * duration;
    while (actionIndex < actions.length - 1 && actions[actionIndex].at < at) actionIndex++;
    const left = actions[Math.max(0, actionIndex - 1)];
    const right = actions[actionIndex];
    const elapsed = Math.max(1, right.at - left.at);
    const fraction = Math.max(0, Math.min(1, (at - left.at) / elapsed));
    const position = left.pos + (right.pos - left.pos) * fraction;
    const y = yForPosition(position);
    const speed = Math.abs(right.pos - left.pos) * 1000 / elapsed;
    context.strokeStyle = intensityColor(speed);
    context.lineWidth = 1.5;
    context.beginPath();
    context.moveTo(x - 1, previousY);
    context.lineTo(x, y);
    context.stroke();
    previousY = y;
  }

  for (let x = 0; x < width; x++) {
    if (!Number.isFinite(minimum[x]) || maximum[x] <= minimum[x]) continue;
    context.strokeStyle = intensityColor(bucketSpeed[x]);
    context.globalAlpha = 0.82;
    context.lineWidth = 1;
    context.beginPath();
    context.moveTo(x + 0.5, yForPosition(maximum[x]));
    context.lineTo(x + 0.5, yForPosition(minimum[x]));
    context.stroke();
  }
  context.globalAlpha = 1;
}

function sizeCanvas(canvas: HTMLCanvasElement, height: number) {
  const width = Math.max(1, Math.floor(canvas.clientWidth || 760));
  const ratio = Math.max(1, Math.min(2, window.devicePixelRatio || 1));
  const pixelWidth = Math.round(width * ratio);
  const pixelHeight = Math.round(height * ratio);
  if (canvas.width !== pixelWidth || canvas.height !== pixelHeight) {
    canvas.width = pixelWidth;
    canvas.height = pixelHeight;
  }
  canvas.getContext("2d")?.setTransform(ratio, 0, 0, ratio, 0, 0);
}

function yForPosition(position: number): number {
  const padding = 6;
  return padding + ((100 - position) / 100) * (TIMELINE_HEIGHT - padding * 2);
}

export function intensityBand(speed: number): "idle" | "moderate" | "fast" | "very-fast" {
  if (speed <= INTENSITY_SLOW) return "idle";
  if (speed <= INTENSITY_FAST) return "moderate";
  if (speed <= INTENSITY_VERY_FAST) return "fast";
  return "very-fast";
}

function intensityColor(speed: number): string {
  switch (intensityBand(speed)) {
    case "moderate": return cssColor("--accent", "#4d91c6");
    case "fast": return cssColor("--warn", "#d8a23e");
    case "very-fast": return cssColor("--text", "#f4f7fa");
    default: return cssColor("--line-strong", "#68717c");
  }
}

function cssColor(name: string, fallback: string): string {
  if (typeof document === "undefined") return fallback;
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback;
}
