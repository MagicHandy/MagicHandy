import {
  useEffect,
  useId,
  useMemo,
  useRef,
  type CSSProperties,
  type Dispatch,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
  type ReactNode,
  type SetStateAction,
} from "react";
import {
  ArrowLeftIcon,
  ArrowRightIcon,
  FitAllIcon,
  FitSelectionIcon,
  ZoomInIcon,
  ZoomOutIcon,
} from "../shell/icons";

const TIMELINE_W = 760;
const TIMELINE_H = 140;
const TIMELINE_Y_PAD = 6;

export interface TimelinePoint {
  at: number;
  pos: number;
}

export interface TimeWindow {
  start: number;
  end: number;
}

type TrimBound = "start" | "end";

interface Props {
  points: TimelinePoint[];
  duration: number;
  start: number;
  end: number;
  viewport: TimeWindow;
  disabled: boolean;
  onTrimChange: Dispatch<SetStateAction<TimeWindow>>;
  onViewportChange: Dispatch<SetStateAction<TimeWindow>>;
}

export function ImportTimeline({
  points,
  duration,
  start,
  end,
  viewport,
  disabled,
  onTrimChange,
  onViewportChange,
}: Props) {
  const frameRef = useRef<HTMLDivElement>(null);
  const timelineRef = useRef<SVGSVGElement>(null);
  const scrollbarRef = useRef<HTMLDivElement>(null);
  const scrollbarThumbRef = useRef<HTMLSpanElement>(null);
  const activeBound = useRef<TrimBound | null>(null);
  const dragOffset = useRef(0);
  const scrollbarDragging = useRef(false);
  const scrollbarDragOffset = useRef(0);
  const handleRefs = useRef<Partial<Record<TrimBound, HTMLDivElement | null>>>({});
  const descriptionId = useId();
  const timelineId = useId();
  const viewStart = Math.max(0, Math.min(duration - 1, viewport.start));
  const viewEnd = Math.max(viewStart + 1, Math.min(duration, viewport.end));
  const span = viewEnd - viewStart;
  const zoomLevel = Math.max(1, duration / span);
  const plotH = TIMELINE_H - TIMELINE_Y_PAD * 2;
  const toX = (at: number) => ((at - viewStart) / span) * TIMELINE_W;
  const toPercent = (at: number) => ((at - viewStart) / span) * 100;
  const path = useMemo(() => {
    const sampled = downsample(pointsAroundWindow(points, viewStart, viewEnd), 380);
    return sampled.map((point, index) => {
      const x = ((point.at - viewStart) / span) * TIMELINE_W;
      const y = TIMELINE_Y_PAD + ((100 - point.pos) / 100) * plotH;
      return `${index === 0 ? "M" : "L"}${x.toFixed(2)} ${y.toFixed(2)}`;
    }).join(" ");
  }, [points, plotH, span, viewEnd, viewStart]);
  const startIndex = nearestActionIndex(points, start);
  const endIndex = nearestActionIndex(points, end);
  const selectionOutsideView = end <= viewStart || start >= viewEnd;
  const selectedStartX = toX(Math.max(viewStart, Math.min(start, viewEnd)));
  const selectedEndX = toX(Math.max(viewStart, Math.min(end, viewEnd)));
  const fullView = viewStart === 0 && viewEnd === duration;
  const selectionView = viewStart === start && viewEnd === end;
  const maximumViewStart = Math.max(0, duration - span);
  const viewportPosition = maximumViewStart > 0 ? viewStart / maximumViewStart : 0;
  const viewportSize = duration > 0 ? span / duration : 1;

  useEffect(() => {
    const frame = frameRef.current;
    if (!frame) return;
    const handleWheel = (event: WheelEvent) => {
      const horizontal = Math.abs(event.deltaX) > Math.abs(event.deltaY);
      const bounds = frame.getBoundingClientRect();
      if (bounds.width <= 0) return;
      if (horizontal || event.shiftKey) {
        const rawDelta = horizontal ? event.deltaX : event.deltaY;
        const delta = normalizeWheelDelta(rawDelta, event.deltaMode, bounds.width);
        if (delta === 0) return;
        event.preventDefault();
        onViewportChange((current) => (
          panTimelineWindowBy(current, duration, (delta / bounds.width) * (current.end - current.start))
        ));
        return;
      }

      const delta = normalizeWheelDelta(event.deltaY, event.deltaMode, bounds.width);
      if (delta === 0 || (delta > 0 && span >= duration) || (delta < 0 && span <= 1)) return;
      const anchor = Math.max(0, Math.min(1, (event.clientX - bounds.left) / bounds.width));
      event.preventDefault();
      onViewportChange((current) => resizeTimelineWindowAt(
        current,
        duration,
        wheelZoomSpan(current.end - current.start, delta),
        anchor,
      ));
    };
    frame.addEventListener("wheel", handleWheel, { passive: false });
    return () => frame.removeEventListener("wheel", handleWheel);
  }, [duration, onViewportChange, span]);

  function timeAtClientX(clientX: number): number {
    const bounds = timelineRef.current?.getBoundingClientRect();
    if (!bounds || bounds.width <= 0) return viewStart;
    const ratio = Math.max(0, Math.min(1, (clientX - bounds.left) / bounds.width));
    return viewStart + ratio * span;
  }

  function changeTrimAt(bound: TrimBound, at: number) {
    onTrimChange((current) => snapTrimBoundToActions(points, current, at, bound));
  }

  function startTrimDrag(event: ReactPointerEvent<HTMLDivElement>) {
    if (disabled || event.button > 0) return;
    const requested = (event.target as Element).closest?.("[data-trim-bound]")?.getAttribute("data-trim-bound");
    if (requested !== "start" && requested !== "end") return;
    const at = timeAtClientX(event.clientX);
    const bound: TrimBound = Math.abs(at - start) <= Math.abs(at - end) ? "start" : "end";
    activeBound.current = bound;
    dragOffset.current = at - (bound === "start" ? start : end);
    handleRefs.current[bound]?.focus();
    event.currentTarget.setPointerCapture?.(event.pointerId);
    event.preventDefault();
  }

  function continueTrimDrag(event: ReactPointerEvent<HTMLDivElement>) {
    if (disabled) {
      finishTrimDrag(event);
      return;
    }
    if (!activeBound.current) return;
    changeTrimAt(activeBound.current, timeAtClientX(event.clientX) - dragOffset.current);
  }

  function finishTrimDrag(event: ReactPointerEvent<HTMLDivElement>) {
    activeBound.current = null;
    dragOffset.current = 0;
    if (event.currentTarget.hasPointerCapture?.(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
  }

  function moveTrimBound(event: ReactKeyboardEvent<HTMLDivElement>, bound: TrimBound) {
    if (disabled) return;
    const currentIndex = bound === "start" ? startIndex : endIndex;
    let nextIndex = currentIndex;
    switch (event.key) {
      case "ArrowLeft":
      case "ArrowDown":
        nextIndex--;
        break;
      case "ArrowRight":
      case "ArrowUp":
        nextIndex++;
        break;
      case "PageDown":
        nextIndex -= 10;
        break;
      case "PageUp":
        nextIndex += 10;
        break;
      case "Home":
        nextIndex = bound === "start" ? 0 : startIndex + 1;
        break;
      case "End":
        nextIndex = bound === "start" ? endIndex - 1 : points.length - 1;
        break;
      default:
        return;
    }
    event.preventDefault();
    event.stopPropagation();
    if (bound === "start") {
      nextIndex = Math.max(0, Math.min(endIndex - 1, nextIndex));
      const nextAt = points[nextIndex].at;
      onTrimChange({ start: nextAt, end });
      onViewportChange((current) => revealTimelineTime(current, duration, nextAt));
      return;
    }
    nextIndex = Math.max(startIndex + 1, Math.min(points.length - 1, nextIndex));
    const nextAt = points[nextIndex].at;
    onTrimChange({ start, end: nextAt });
    onViewportChange((current) => revealTimelineTime(current, duration, nextAt));
  }

  function handleTimelineKey(event: ReactKeyboardEvent<HTMLDivElement>) {
    if (event.target !== event.currentTarget) return;
    switch (event.key) {
      case "+":
      case "=":
        zoomTimeline(0.5);
        break;
      case "-":
        zoomTimeline(2);
        break;
      case "0":
        onViewportChange({ start: 0, end: duration });
        break;
      case "ArrowLeft":
        panTimeline(-1);
        break;
      case "ArrowRight":
        panTimeline(1);
        break;
      case "Home":
        onViewportChange({ start: 0, end: span });
        break;
      case "End":
        onViewportChange({ start: duration - span, end: duration });
        break;
      default:
        return;
    }
    event.preventDefault();
  }

  function zoomTimeline(factor: number) {
    onViewportChange((current) => resizeTimelineWindowAt(current, duration, (current.end - current.start) * factor, 0.5));
  }

  function panTimeline(direction: -1 | 1) {
    onViewportChange((current) => panTimelineWindowBy(current, duration, direction * (current.end - current.start) * 0.75));
  }

  function setViewportStart(requestedStart: number) {
    const nextStart = Math.max(0, Math.min(maximumViewStart, Math.round(requestedStart)));
    onViewportChange({ start: nextStart, end: nextStart + span });
  }

  function setViewportFromScrollbar(clientX: number) {
    const trackBounds = scrollbarRef.current?.getBoundingClientRect();
    const thumbBounds = scrollbarThumbRef.current?.getBoundingClientRect();
    if (!trackBounds || !thumbBounds) return;
    const available = trackBounds.width - thumbBounds.width;
    if (available <= 0 || maximumViewStart <= 0) return;
    const left = Math.max(0, Math.min(available, clientX - trackBounds.left - scrollbarDragOffset.current));
    setViewportStart((left / available) * maximumViewStart);
  }

  function startScrollbarDrag(event: ReactPointerEvent<HTMLDivElement>) {
    if (fullView || event.button > 0) return;
    const thumbBounds = scrollbarThumbRef.current?.getBoundingClientRect();
    if (!thumbBounds) return;
    const onThumb = Boolean((event.target as Element).closest?.(".import-timeline-scrollbar-thumb"));
    scrollbarDragOffset.current = onThumb ? event.clientX - thumbBounds.left : thumbBounds.width / 2;
    scrollbarDragging.current = true;
    event.currentTarget.setPointerCapture?.(event.pointerId);
    setViewportFromScrollbar(event.clientX);
    event.preventDefault();
  }

  function continueScrollbarDrag(event: ReactPointerEvent<HTMLDivElement>) {
    if (scrollbarDragging.current) setViewportFromScrollbar(event.clientX);
  }

  function finishScrollbarDrag(event: ReactPointerEvent<HTMLDivElement>) {
    scrollbarDragging.current = false;
    scrollbarDragOffset.current = 0;
    if (event.currentTarget.hasPointerCapture?.(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
  }

  function moveScrollbar(event: ReactKeyboardEvent<HTMLDivElement>) {
    if (fullView) return;
    const smallStep = Math.max(1, Math.round(span * 0.1));
    switch (event.key) {
      case "ArrowLeft":
      case "ArrowUp":
        onViewportChange((current) => panTimelineWindowBy(current, duration, -smallStep));
        break;
      case "ArrowRight":
      case "ArrowDown":
        onViewportChange((current) => panTimelineWindowBy(current, duration, smallStep));
        break;
      case "PageUp":
        panTimeline(-1);
        break;
      case "PageDown":
        panTimeline(1);
        break;
      case "Home":
        setViewportStart(0);
        break;
      case "End":
        setViewportStart(maximumViewStart);
        break;
      default:
        return;
    }
    event.preventDefault();
  }

  return (
    <>
      <div className="import-timeline-head">
        <output id={descriptionId} className="import-timeline-view" aria-label="Visible timeline range">
          Viewing {formatTimelineTime(viewStart)}-{formatTimelineTime(viewEnd)} at {formatZoom(zoomLevel)}
        </output>
        <div className="import-timeline-controls" role="group" aria-label="Timeline view">
          <TimelineButton label="Earlier" title="Move view earlier (Left arrow)" disabled={viewStart <= 0} onClick={() => panTimeline(-1)}><ArrowLeftIcon /></TimelineButton>
          <TimelineButton label="Later" title="Move view later (Right arrow)" disabled={viewEnd >= duration} onClick={() => panTimeline(1)}><ArrowRightIcon /></TimelineButton>
          <TimelineButton label="Zoom out" title="Zoom out (-)" disabled={fullView} onClick={() => zoomTimeline(2)}><ZoomOutIcon /></TimelineButton>
          <TimelineButton label="Zoom in" title="Zoom in (+)" disabled={span <= 1} onClick={() => zoomTimeline(0.5)}><ZoomInIcon /></TimelineButton>
          <TimelineButton label="Fit selection" title="Fit the selected range" disabled={selectionView} onClick={() => onViewportChange({ start, end })}><FitSelectionIcon /></TimelineButton>
          <TimelineButton label="Fit all" title="Fit the complete source (0)" disabled={fullView} onClick={() => onViewportChange({ start: 0, end: duration })}><FitAllIcon /></TimelineButton>
        </div>
      </div>
      <div
        ref={frameRef}
        id={timelineId}
        className="import-timeline-frame"
        role="group"
        tabIndex={0}
        aria-label={`Funscript timeline editor, ${formatTimelineTime(duration)} total, viewing ${formatTimelineTime(viewStart)} to ${formatTimelineTime(viewEnd)}, selection ${formatTimelineTime(start)} to ${formatTimelineTime(end)}, ${formatTimelineTime(end - start)} selected`}
        aria-describedby={descriptionId}
        title="Scroll to zoom at cursor; Shift-scroll to pan"
        onPointerDown={startTrimDrag}
        onPointerMove={continueTrimDrag}
        onPointerUp={finishTrimDrag}
        onPointerCancel={finishTrimDrag}
        onLostPointerCapture={finishTrimDrag}
        onKeyDown={handleTimelineKey}
      >
        <svg
          ref={timelineRef}
          className="import-timeline"
          viewBox={`0 0 ${TIMELINE_W} ${TIMELINE_H}`}
          preserveAspectRatio="none"
          aria-hidden="true"
          focusable="false"
        >
          {!selectionOutsideView && <rect className="import-timeline-selection" x={selectedStartX} y={0} width={Math.max(0, selectedEndX - selectedStartX)} height={TIMELINE_H} />}
          <line x1={0} y1={TIMELINE_H / 2} x2={TIMELINE_W} y2={TIMELINE_H / 2} className="pattern-grid-line" />
          {path && <path d={path} className="pattern-curve-line" />}
          {selectionOutsideView && <rect className="import-timeline-dim" x={0} y={0} width={TIMELINE_W} height={TIMELINE_H} />}
          {!selectionOutsideView && start > viewStart && <rect className="import-timeline-dim" data-trim-dim="start" x={0} y={0} width={selectedStartX} height={TIMELINE_H} />}
          {!selectionOutsideView && end < viewEnd && <rect className="import-timeline-dim" data-trim-dim="end" x={selectedEndX} y={0} width={TIMELINE_W - selectedEndX} height={TIMELINE_H} />}
        </svg>
        {(["start", "end"] as const).map((bound) => {
          const value = bound === "start" ? start : end;
          const minimum = bound === "start" ? points[0].at : points[Math.min(points.length - 1, startIndex + 1)].at;
          const maximum = bound === "start" ? points[Math.max(0, endIndex - 1)].at : points[points.length - 1].at;
          return (
            <div
              key={bound}
              ref={(node) => { handleRefs.current[bound] = node; }}
              className={`import-timeline-handle import-timeline-handle-${bound}`}
              data-trim-bound={bound}
              style={{ "--handle-position": `${toPercent(value)}%` } as CSSProperties}
              data-disabled={disabled || undefined}
              role="slider"
              tabIndex={disabled ? -1 : 0}
              aria-label={bound === "start" ? "Trim start" : "Trim end"}
              aria-orientation="horizontal"
              aria-valuemin={minimum}
              aria-valuemax={maximum}
              aria-valuenow={value}
              aria-valuetext={formatTimelineTime(value)}
              aria-disabled={disabled || undefined}
              onFocus={() => onViewportChange((current) => revealTimelineTime(current, duration, value))}
              onKeyDown={(event) => moveTrimBound(event, bound)}
            >
              <span className="import-timeline-handle-grip" aria-hidden="true" />
            </div>
          );
        })}
      </div>
      <div
        ref={scrollbarRef}
        className="import-timeline-scrollbar"
        role="scrollbar"
        tabIndex={fullView ? -1 : 0}
        aria-label="Timeline viewport"
        aria-controls={timelineId}
        aria-orientation="horizontal"
        aria-valuemin={0}
        aria-valuemax={maximumViewStart}
        aria-valuenow={viewStart}
        aria-valuetext={`${formatTimelineTime(viewStart)} to ${formatTimelineTime(viewEnd)}`}
        aria-disabled={fullView || undefined}
        title="Drag to move the visible timeline range"
        style={{
          "--viewport-position": viewportPosition,
          "--viewport-size": `${viewportSize * 100}%`,
        } as CSSProperties}
        onPointerDown={startScrollbarDrag}
        onPointerMove={continueScrollbarDrag}
        onPointerUp={finishScrollbarDrag}
        onPointerCancel={finishScrollbarDrag}
        onLostPointerCapture={finishScrollbarDrag}
        onKeyDown={moveScrollbar}
      >
        <span ref={scrollbarThumbRef} className="import-timeline-scrollbar-thumb" aria-hidden="true" />
      </div>
    </>
  );
}

function TimelineButton({
  label,
  title,
  disabled,
  onClick,
  children,
}: {
  label: string;
  title: string;
  disabled: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button type="button" className="icon-button" aria-label={label} title={title} disabled={disabled} onClick={onClick}>
      {children}
    </button>
  );
}

function snapTrimBoundToActions(points: TimelinePoint[], current: TimeWindow, at: number, bound: TrimBound): TimeWindow {
  const candidate = nearestActionIndex(points, at);
  const startIndex = nearestActionIndex(points, current.start);
  const endIndex = nearestActionIndex(points, current.end);
  if (bound === "start") {
    return { start: points[Math.max(0, Math.min(endIndex - 1, candidate))].at, end: current.end };
  }
  return { start: current.start, end: points[Math.max(startIndex + 1, Math.min(points.length - 1, candidate))].at };
}

function nearestActionIndex(points: TimelinePoint[], at: number): number {
  let low = 0;
  let high = points.length - 1;
  while (low < high) {
    const middle = Math.floor((low + high) / 2);
    if (points[middle].at < at) low = middle + 1;
    else high = middle;
  }
  if (low === 0) return 0;
  const previous = low - 1;
  return Math.abs(points[previous].at - at) <= Math.abs(points[low].at - at) ? previous : low;
}

function resizeTimelineWindowAt(current: TimeWindow, duration: number, requestedSpan: number, anchor: number): TimeWindow {
  const span = Math.max(1, Math.min(duration, Math.round(requestedSpan)));
  const ratio = Math.max(0, Math.min(1, anchor));
  const anchorTime = current.start + ratio * (current.end - current.start);
  let start = Math.round(anchorTime - ratio * span);
  start = Math.max(0, Math.min(duration - span, start));
  return { start, end: start + span };
}

function panTimelineWindowBy(current: TimeWindow, duration: number, shift: number): TimeWindow {
  const span = Math.max(1, current.end - current.start);
  const start = Math.max(0, Math.min(duration - span, Math.round(current.start + shift)));
  return { start, end: start + span };
}

function revealTimelineTime(current: TimeWindow, duration: number, at: number): TimeWindow {
  if (at >= current.start && at <= current.end) return current;
  const span = Math.max(1, current.end - current.start);
  const start = Math.max(0, Math.min(duration - span, Math.round(at - span / 2)));
  return { start, end: start + span };
}

function normalizeWheelDelta(delta: number, mode: number, pageSize: number): number {
  if (mode === 1) return delta * 16;
  if (mode === 2) return delta * pageSize;
  return delta;
}

function wheelZoomSpan(currentSpan: number, delta: number): number {
  const exponent = Math.max(-1, Math.min(1, delta / 240));
  let next = Math.round(currentSpan * (2 ** exponent));
  if (next === currentSpan) next += delta > 0 ? 1 : -1;
  return next;
}

function pointsAroundWindow(points: TimelinePoint[], start: number, end: number): TimelinePoint[] {
  let first = firstPointAtOrAfter(points, start);
  if (first > 0) first--;
  let after = firstPointAtOrAfter(points, end);
  if (after < points.length && points[after].at === end) after++;
  if (after < points.length) after++;
  return points.slice(first, after);
}

function firstPointAtOrAfter(points: TimelinePoint[], at: number): number {
  let low = 0;
  let high = points.length;
  while (low < high) {
    const middle = Math.floor((low + high) / 2);
    if (points[middle].at < at) low = middle + 1;
    else high = middle;
  }
  return low;
}

export function formatTimelineTime(milliseconds: number): string {
  const rounded = Math.max(0, Math.round(milliseconds));
  const totalSeconds = Math.floor(rounded / 1000);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  const base = hours > 0
    ? `${hours}:${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`
    : `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
  const remainder = rounded % 1000;
  return remainder > 0 ? `${base}.${String(remainder).padStart(3, "0")}` : base;
}

function formatZoom(level: number): string {
  const value = level < 10 ? Number(level.toFixed(1)) : Math.round(level);
  return `${value}x`;
}

function downsample(points: TimelinePoint[], buckets: number): TimelinePoint[] {
  if (points.length <= buckets * 2) return points;
  const result: TimelinePoint[] = [points[0]];
  const size = points.length / buckets;
  for (let bucket = 0; bucket < buckets; bucket++) {
    const from = Math.floor(bucket * size);
    const to = Math.min(points.length, Math.floor((bucket + 1) * size));
    let low = points[from];
    let high = points[from];
    for (let index = from; index < to; index++) {
      if (points[index].pos < low.pos) low = points[index];
      if (points[index].pos > high.pos) high = points[index];
    }
    const ordered = low.at <= high.at ? [low, high] : [high, low];
    for (const point of ordered) {
      if (result[result.length - 1] !== point) result.push(point);
    }
  }
  const last = points[points.length - 1];
  if (result[result.length - 1] !== last) result.push(last);
  return result;
}
