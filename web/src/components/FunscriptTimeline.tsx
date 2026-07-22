import { useEffect, useRef, type KeyboardEvent as ReactKeyboardEvent, type PointerEvent as ReactPointerEvent } from "react";
import type { MediaFunscript, MediaFunscriptAction } from "../api/types";
import { formatTimelineTime } from "./ImportTimeline";

const TIMELINE_HEIGHT = 60;
const PLOT_TOP = 5;
const ACTIVITY_HEIGHT = 3;
const ACTIVITY_BOTTOM = 3;
const ACTIVITY_TOP = TIMELINE_HEIGHT - ACTIVITY_HEIGHT - ACTIVITY_BOTTOM;
const PLOT_BOTTOM = ACTIVITY_TOP - 5;

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
    const x = Math.round(progress * Math.max(0, width - 1)) + 0.5;
    context.strokeStyle = cssColor("--bg-inset", "#101215");
    context.lineWidth = 3;
    context.beginPath();
    context.moveTo(x, 0);
    context.lineTo(x, TIMELINE_HEIGHT);
    context.stroke();
    context.strokeStyle = cssColor("--text", "#f4f7fa");
    context.lineWidth = 1;
    context.beginPath();
    context.moveTo(x, 0);
    context.lineTo(x, TIMELINE_HEIGHT);
    context.stroke();
    context.fillStyle = cssColor("--text", "#f4f7fa");
    context.fillRect(x - 1.5, 0, 3, 3);
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

  drawPositionGuides(context, width);
  context.fillStyle = cssColor("--line-strong", "#3d434c");
  context.fillRect(0, ACTIVITY_TOP, width, ACTIVITY_HEIGHT);
  if (actions.length < 2) return;

  const samples = buildTimelineSamples(actions, duration, width);
  const accent = cssColor("--accent", "#5b9dd9");

  // Only dense authored actions get an extrema envelope. Long, sparse
  // segments remain diagonals instead of acquiring false vertical bars.
  context.fillStyle = accent;
  context.globalAlpha = 0.2;
  for (let x = 0; x < width; x++) {
    if (samples.count[x] < 2 || samples.maximum[x] <= samples.minimum[x]) continue;
    const top = yForPosition(samples.maximum[x]);
    const bottom = yForPosition(samples.minimum[x]);
    context.fillRect(x, top, 1, Math.max(1, bottom - top));
  }
  context.globalAlpha = 1;

  context.strokeStyle = accent;
  context.lineWidth = 1.75;
  context.lineJoin = "round";
  context.lineCap = "round";
  context.beginPath();
  for (let x = 0; x < width; x++) {
    const y = yForPosition(samples.position[x]);
    if (x === 0) context.moveTo(0, y);
    else context.lineTo(x, y);
  }
  context.stroke();

  drawActivityRail(context, samples.speed, accent);
  context.globalAlpha = 1;
}

function drawPositionGuides(context: CanvasRenderingContext2D, width: number) {
  context.strokeStyle = cssColor("--line", "#343a42");
  context.lineWidth = 1;
  context.globalAlpha = 0.58;
  context.beginPath();
  for (const position of [25, 50, 75]) {
    const y = Math.round(yForPosition(position)) + 0.5;
    context.moveTo(0, y);
    context.lineTo(width, y);
  }
  context.stroke();
  context.globalAlpha = 1;
}

function drawActivityRail(context: CanvasRenderingContext2D, speeds: Float64Array, accent: string) {
  const radius = 2;
  let total = 0;
  for (let index = 0; index <= radius && index < speeds.length; index++) total += speeds[index];
  context.fillStyle = accent;
  for (let x = 0; x < speeds.length; x++) {
    if (x > 0) {
      const incoming = x + radius;
      const outgoing = x - radius - 1;
      if (incoming < speeds.length) total += speeds[incoming];
      if (outgoing >= 0) total -= speeds[outgoing];
    }
    const sampleCount = Math.min(speeds.length - 1, x + radius) - Math.max(0, x - radius) + 1;
    context.globalAlpha = activityOpacity(total / sampleCount);
    context.fillRect(x, ACTIVITY_TOP, 1, ACTIVITY_HEIGHT);
  }
}

interface TimelineSamples {
  position: Float64Array;
  speed: Float64Array;
  minimum: Float64Array;
  maximum: Float64Array;
  count: Uint32Array;
}

export function buildTimelineSamples(actions: MediaFunscriptAction[], duration: number, requestedWidth: number): TimelineSamples {
  const width = Math.max(1, Math.floor(requestedWidth));
  const boundedDuration = Math.max(1, duration);
  const position = new Float64Array(width);
  const speed = new Float64Array(width);
  const minimum = new Float64Array(width);
  const maximum = new Float64Array(width);
  const count = new Uint32Array(width);
  minimum.fill(Number.POSITIVE_INFINITY);
  maximum.fill(Number.NEGATIVE_INFINITY);

  for (const action of actions) {
    const bucket = Math.max(0, Math.min(width - 1, Math.round((action.at / boundedDuration) * (width - 1))));
    minimum[bucket] = Math.min(minimum[bucket], action.pos);
    maximum[bucket] = Math.max(maximum[bucket], action.pos);
    count[bucket]++;
  }

  if (actions.length === 0) return { position, speed, minimum, maximum, count };
  if (actions.length === 1) {
    position.fill(actions[0].pos);
    return { position, speed, minimum, maximum, count };
  }

  let actionIndex = 1;
  for (let x = 0; x < width; x++) {
    const at = (x / Math.max(1, width - 1)) * boundedDuration;
    while (actionIndex < actions.length - 1 && actions[actionIndex].at < at) actionIndex++;
    const left = actions[Math.max(0, actionIndex - 1)];
    const right = actions[actionIndex];
    const elapsed = Math.max(1, right.at - left.at);
    const fraction = Math.max(0, Math.min(1, (at - left.at) / elapsed));
    position[x] = left.pos + (right.pos - left.pos) * fraction;
    speed[x] = Math.abs(right.pos - left.pos) * 1000 / elapsed;
  }
  return { position, speed, minimum, maximum, count };
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
  return PLOT_TOP + ((100 - position) / 100) * (PLOT_BOTTOM - PLOT_TOP);
}

export function activityOpacity(speed: number): number {
  const normalized = Math.log1p(Math.max(0, speed) / 40) / Math.log1p(500 / 40);
  return 0.18 + Math.min(1, normalized) * 0.82;
}

function cssColor(name: string, fallback: string): string {
  if (typeof document === "undefined") return fallback;
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback;
}
