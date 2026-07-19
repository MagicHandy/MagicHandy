import { useMemo, useRef, useState } from "react";
import { UploadIcon } from "../shell/icons";
import { RangeSlider } from "./RangeSlider";

// Import studio: funscripts get a client-side trim timeline before the file
// subset is submitted through the normal validated import endpoint; MagicHandy
// share files carry their own kind and import as-is. The backend remains the
// only parser that decides what actually lands in the library.

const PATTERN_SCHEMA = "magichandy.pattern.v1";
const PROGRAM_SCHEMA = "magichandy.program.v1";
const MAX_IMPORT_ACTIONS = 4096;
const MAX_SOURCE_ACTIONS = MAX_IMPORT_ACTIONS * 5;
const MAX_IMPORT_BYTES = 8 * 1024 * 1024;
const MAX_IMPORT_DURATION = 24 * 60 * 60 * 1000;
const MAX_NAME_CHARS = 80;
const TIMELINE_W = 760;
const TIMELINE_H = 140;
const TIMELINE_PAD = 6;

interface ActionPoint {
  at: number;
  pos: number;
}

type ParsedFile =
  | { type: "funscript"; file: File; stem: string; points: ActionPoint[]; duration: number }
  | { type: "share"; file: File; kind: "pattern" | "program"; name: string }
  | { type: "error"; message: string };

interface TimeWindow {
  start: number;
  end: number;
}

interface Props {
  locked: boolean;
  importing: boolean;
  onImport: (file: File, asKind: "pattern" | "program") => Promise<boolean>;
}

export function MotionImport({ locked, importing, onImport }: Props) {
  const [parsed, setParsed] = useState<ParsedFile | null>(null);
  const [trim, setTrim] = useState({ start: 0, end: 0 });
  const [viewport, setViewport] = useState<TimeWindow>({ start: 0, end: 0 });
  const [kind, setKind] = useState<"pattern" | "program">("program");
  const [name, setName] = useState("");
  const [reading, setReading] = useState(false);
  const readRequest = useRef(0);

  const funscript = parsed?.type === "funscript" ? parsed : null;
  const selection = useMemo(() => {
    if (!funscript) return [];
    return funscript.points.filter((point) => point.at >= trim.start && point.at <= trim.end);
  }, [funscript, trim]);
  const selectionSpan = selection.length > 0 ? positionSpan(selection) : 0;
  const selectionProblem = !funscript ? "" : selectionProblemFor(selection, kind, selectionSpan);
  const contentName = name.trim() || funscript?.stem || "Imported funscript";
  const nameProblem = Array.from(contentName).length > MAX_NAME_CHARS
    ? `Name must be ${MAX_NAME_CHARS} characters or fewer.`
    : /[/\\]/.test(contentName) ? "Name cannot contain path separators (/ or \\)." : "";
  const importProblem = selectionProblem || nameProblem;
  const trimmed = funscript !== null && (trim.start > 0 || trim.end < funscript.duration);
  const trimStartIndex = funscript ? nearestActionIndex(funscript.points, trim.start) : 0;
  const trimEndIndex = funscript ? nearestActionIndex(funscript.points, trim.end) : 0;

  async function chooseFile(file: File) {
    const request = ++readRequest.current;
    setReading(true);
    setParsed(null);
    const next = await parseImportFile(file);
    if (request !== readRequest.current) return;
    setReading(false);
    setParsed(next);
    if (next.type === "funscript") {
      setTrim({ start: 0, end: next.duration });
      setViewport({ start: 0, end: next.duration });
      setName(next.stem);
      setKind("program");
    }
  }

  async function submit() {
    if (!parsed || parsed.type === "error" || locked || importing) return;
    let ok: boolean;
    if (parsed.type === "share") {
      ok = await onImport(parsed.file, parsed.kind);
    } else {
      if (importProblem) return;
      const rebased = selection.map((point) => ({ at: point.at - selection[0].at, pos: point.pos }));
      const payload = JSON.stringify({ actions: rebased });
      ok = await onImport(new File([payload], `${contentName}.funscript`, { type: "application/json" }), kind);
    }
    if (ok) setParsed(null);
  }

  function zoomTimeline(factor: number) {
    if (!funscript) return;
    setViewport((current) => resizeTimelineWindow(current, funscript.duration, (current.end - current.start) * factor));
  }

  function panTimeline(direction: -1 | 1) {
    if (!funscript) return;
    setViewport((current) => panTimelineWindow(current, funscript.duration, direction));
  }

  const viewportSpan = funscript ? Math.max(1, viewport.end - viewport.start) : 1;
  const zoomLevel = funscript ? Math.max(1, funscript.duration / viewportSpan) : 1;

  return (
    <section className="library-view import-studio" aria-label="Import motion content">
      <div className="program-toolbar">
        <label className="btn btn-secondary file-button" aria-disabled={locked || importing}>
          <UploadIcon /> Choose file
          <input
            type="file"
            accept=".funscript,.json"
            disabled={locked || importing}
            onChange={(event) => {
              const file = event.target.files?.[0];
              event.currentTarget.value = "";
              if (file) void chooseFile(file);
            }}
          />
        </label>
        <span className="hint-inline">Funscripts (.funscript) and MagicHandy share files (.json).</span>
      </div>

      {reading && (
        <div className="empty-state compact-empty" role="status">
          <h2>Reading file</h2>
          <p>Checking the selected motion file.</p>
        </div>
      )}

      {!reading && !parsed && (
        <div className="empty-state compact-empty">
          <h2>No file selected</h2>
          <p>Pick a funscript to trim it into a pattern or program, or a MagicHandy share file to import it as-is.</p>
        </div>
      )}

      {parsed?.type === "error" && (
        <div className="empty-state compact-empty" role="alert">
          <h2>File not usable</h2>
          <p>{parsed.message}</p>
        </div>
      )}

      {parsed?.type === "share" && (
        <div className="import-card">
          <h2>MagicHandy share file</h2>
          <p className="pattern-meta">
            <span>{parsed.name || parsed.file.name}</span>
            <span>Imports as {parsed.kind}</span>
          </p>
          <p className="hint-block narrow">Share files carry their own kind and content; they import without trimming.</p>
          <button type="button" className="btn btn-primary" disabled={locked || importing} onClick={() => void submit()}>
            {importing ? "Importing" : `Import ${parsed.kind}`}
          </button>
        </div>
      )}

      {funscript && (
        <div className="import-card">
          <h2 className="visually-hidden">Trim and import funscript</h2>
          <div className="import-timeline-toolbar">
            <div className="import-timeline-controls" role="group" aria-label="Timeline view">
              <button type="button" className="btn btn-secondary" disabled={viewport.start <= 0} onClick={() => panTimeline(-1)}>Earlier</button>
              <button type="button" className="btn btn-secondary" disabled={viewport.end >= funscript.duration} onClick={() => panTimeline(1)}>Later</button>
              <button type="button" className="btn btn-secondary" disabled={viewportSpan <= 1} onClick={() => zoomTimeline(0.5)}>Zoom in</button>
              <button type="button" className="btn btn-secondary" disabled={viewportSpan >= funscript.duration} onClick={() => zoomTimeline(2)}>Zoom out</button>
              <button type="button" className="btn btn-secondary" disabled={viewport.start === trim.start && viewport.end === trim.end} onClick={() => setViewport({ start: trim.start, end: trim.end })}>Fit selection</button>
              <button type="button" className="btn btn-secondary" disabled={viewport.start === 0 && viewport.end === funscript.duration} onClick={() => setViewport({ start: 0, end: funscript.duration })}>Fit all</button>
            </div>
            <output className="import-timeline-view" aria-label="Visible timeline range">
              Viewing {formatTimelineTime(viewport.start)}-{formatTimelineTime(viewport.end)} at {formatZoom(zoomLevel)}
            </output>
          </div>
          <ImportTimeline
            points={funscript.points}
            duration={funscript.duration}
            start={trim.start}
            end={trim.end}
            viewport={viewport}
          />
          <RangeSlider
            label="Trim"
            floor={0}
            ceil={funscript.duration}
            minGap={Math.min(1, funscript.duration)}
            minAriaMax={funscript.points[Math.max(0, trimEndIndex - 1)].at}
            maxAriaMin={funscript.points[Math.min(funscript.points.length - 1, trimStartIndex + 1)].at}
            minValue={trim.start}
            maxValue={trim.end}
            disabled={locked || importing}
            formatValue={(min, max) => `${formatTimelineTime(min)}-${formatTimelineTime(max)}`}
            formatBoundValue={(value) => formatTimelineTime(value)}
            onChange={(next, changed, source) => setTrim(snapTrimToActions(funscript.points, trim, next, changed, source))}
          />
          <p className="pattern-meta import-selection-meta">
            <output className="import-selection-length" aria-label="Current trim selection length">
              Selection length {formatTimelineTime(trim.end - trim.start)}
            </output>
            <span>{formatTimelineTime(trim.start)}-{formatTimelineTime(trim.end)} of {formatTimelineTime(funscript.duration)}</span>
            <span>{selection.length} of {funscript.points.length} actions selected</span>
            {trimmed && <span>trimmed</span>}
          </p>

          <div className="import-options">
            <div className="segmented compact-segmented" role="group" aria-label="Import as">
              <button type="button" aria-pressed={kind === "program"} data-active={kind === "program" || undefined} disabled={locked || importing} onClick={() => setKind("program")}>Program</button>
              <button type="button" aria-pressed={kind === "pattern"} data-active={kind === "pattern" || undefined} disabled={locked || importing} onClick={() => setKind("pattern")}>Loop pattern</button>
            </div>
            <label className="import-name">
              <span className="field-label">Save as</span>
              <input type="text" maxLength={MAX_NAME_CHARS} value={name} disabled={locked || importing} onChange={(event) => setName(event.target.value)} />
            </label>
          </div>
          <p className="hint-block narrow">
            {kind === "program"
              ? "Programs preserve the selected knots and duration, play once, and use a 500 ms minimum playback period."
              : "Loop patterns repeat: qualifying pauses over 5 seconds collapse, positions stretch to the full relative span, and the cycle closes and safety-stretches to at least 6.6 seconds."}
          </p>

          {importProblem && <p className="import-problem" role="status">{importProblem}</p>}
          <button
            type="button"
            className="btn btn-primary"
            disabled={locked || importing || importProblem !== ""}
            onClick={() => void submit()}
          >
            {importing ? "Importing" : kind === "program" ? "Import as program" : "Import as loop pattern"}
          </button>
        </div>
      )}
    </section>
  );
}

function ImportTimeline({
  points,
  duration,
  start,
  end,
  viewport,
}: {
  points: ActionPoint[];
  duration: number;
  start: number;
  end: number;
  viewport: TimeWindow;
}) {
  const viewStart = Math.max(0, Math.min(duration, viewport.start));
  const viewEnd = Math.max(viewStart + 1, Math.min(duration, viewport.end));
  const span = Math.max(1, viewEnd - viewStart);
  const plotW = TIMELINE_W - TIMELINE_PAD * 2;
  const plotH = TIMELINE_H - TIMELINE_PAD * 2;
  const toX = (at: number) => TIMELINE_PAD + ((at - viewStart) / span) * plotW;
  const path = useMemo(() => {
    const sampled = downsample(pointsAroundWindow(points, viewStart, viewEnd), 380);
    return sampled.map((point, index) => {
      const x = TIMELINE_PAD + ((point.at - viewStart) / span) * plotW;
      const y = TIMELINE_PAD + ((100 - point.pos) / 100) * plotH;
      return `${index === 0 ? "M" : "L"}${x.toFixed(2)} ${y.toFixed(2)}`;
    }).join(" ");
  }, [points, plotH, plotW, span, viewEnd, viewStart]);
  const selectionOutsideView = end <= viewStart || start >= viewEnd;
  const startVisible = start >= viewStart && start <= viewEnd;
  const endVisible = end >= viewStart && end <= viewEnd;
  const startX = toX(Math.max(viewStart, Math.min(start, viewEnd)));
  const endX = toX(Math.max(viewStart, Math.min(end, viewEnd)));

  return (
    <svg
      className="import-timeline"
      viewBox={`0 0 ${TIMELINE_W} ${TIMELINE_H}`}
      preserveAspectRatio="none"
      role="img"
      aria-label={`Funscript timeline source view, ${formatTimelineTime(duration)} total, viewing ${formatTimelineTime(viewStart)} to ${formatTimelineTime(viewEnd)}, selection ${formatTimelineTime(start)} to ${formatTimelineTime(end)}, ${formatTimelineTime(end - start)} selected`}
    >
      <line x1={TIMELINE_PAD} y1={TIMELINE_H / 2} x2={TIMELINE_W - TIMELINE_PAD} y2={TIMELINE_H / 2} className="pattern-grid-line" />
      {path && <path d={path} className="pattern-curve-line" />}
      {selectionOutsideView && <rect className="import-timeline-dim" x={TIMELINE_PAD} y={0} width={plotW} height={TIMELINE_H} />}
      {!selectionOutsideView && start > viewStart && <rect className="import-timeline-dim" x={TIMELINE_PAD} y={0} width={startX - TIMELINE_PAD} height={TIMELINE_H} />}
      {!selectionOutsideView && end < viewEnd && <rect className="import-timeline-dim" x={endX} y={0} width={TIMELINE_W - TIMELINE_PAD - endX} height={TIMELINE_H} />}
      {startVisible && <line className="import-timeline-bound" x1={startX} y1={0} x2={startX} y2={TIMELINE_H} />}
      {endVisible && <line className="import-timeline-bound" x1={endX} y1={0} x2={endX} y2={TIMELINE_H} />}
    </svg>
  );
}

function snapTrimToActions(
  points: ActionPoint[],
  current: TimeWindow,
  next: { min: number; max: number },
  changed: "min" | "max",
  source: "keyboard" | "pointer",
): TimeWindow {
  let startIndex = nearestActionIndex(points, next.min);
  let endIndex = nearestActionIndex(points, next.max);
  if (changed === "min") {
    const currentIndex = nearestActionIndex(points, current.start);
    if (source === "keyboard" && next.min > current.start && startIndex <= currentIndex) startIndex = currentIndex + 1;
    if (source === "keyboard" && next.min < current.start && startIndex >= currentIndex) startIndex = currentIndex - 1;
    startIndex = Math.min(startIndex, endIndex - 1);
  } else {
    const currentIndex = nearestActionIndex(points, current.end);
    if (source === "keyboard" && next.max > current.end && endIndex <= currentIndex) endIndex = currentIndex + 1;
    if (source === "keyboard" && next.max < current.end && endIndex >= currentIndex) endIndex = currentIndex - 1;
    endIndex = Math.max(endIndex, startIndex + 1);
  }
  startIndex = Math.max(0, startIndex);
  endIndex = Math.min(points.length - 1, endIndex);
  return { start: points[startIndex].at, end: points[endIndex].at };
}

function nearestActionIndex(points: ActionPoint[], at: number): number {
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

function resizeTimelineWindow(current: TimeWindow, duration: number, requestedSpan: number): TimeWindow {
  const span = Math.max(1, Math.min(duration, Math.round(requestedSpan)));
  const center = (current.start + current.end) / 2;
  let start = Math.round(center - span / 2);
  start = Math.max(0, Math.min(duration - span, start));
  return { start, end: start + span };
}

function panTimelineWindow(current: TimeWindow, duration: number, direction: -1 | 1): TimeWindow {
  const span = Math.max(1, current.end - current.start);
  const shift = Math.max(1, Math.round(span * 0.75)) * direction;
  const start = Math.max(0, Math.min(duration - span, current.start + shift));
  return { start, end: start + span };
}

function pointsAroundWindow(points: ActionPoint[], start: number, end: number): ActionPoint[] {
  let first = firstPointAtOrAfter(points, start);
  if (first > 0) first--;
  let after = firstPointAtOrAfter(points, end);
  if (after < points.length && points[after].at === end) after++;
  if (after < points.length) after++;
  return points.slice(first, after);
}

function firstPointAtOrAfter(points: ActionPoint[], at: number): number {
  let low = 0;
  let high = points.length;
  while (low < high) {
    const middle = Math.floor((low + high) / 2);
    if (points[middle].at < at) low = middle + 1;
    else high = middle;
  }
  return low;
}

function formatTimelineTime(milliseconds: number): string {
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

function selectionProblemFor(selection: ActionPoint[], kind: "pattern" | "program", span: number): string {
  if (selection.length < 2 || selection[selection.length - 1].at === selection[0].at) {
    return "Select at least two actions with distinct times.";
  }
  if (selection.length > MAX_IMPORT_ACTIONS) {
    return `Selection has ${selection.length} actions; trim it to ${MAX_IMPORT_ACTIONS} or fewer.`;
  }
  if (kind === "pattern" && span < 1) {
    return "This selection has no usable motion span for a loop pattern.";
  }
  return "";
}

function positionSpan(points: ActionPoint[]): number {
  let minimum = points[0].pos;
  let maximum = points[0].pos;
  for (const point of points) {
    minimum = Math.min(minimum, point.pos);
    maximum = Math.max(maximum, point.pos);
  }
  return maximum - minimum;
}

async function parseImportFile(file: File): Promise<ParsedFile> {
  if (file.size > MAX_IMPORT_BYTES) {
    return { type: "error", message: `${file.name} exceeds the 8 MiB import limit.` };
  }
  let raw: unknown;
  try {
    raw = JSON.parse(await readFileText(file));
  } catch {
    return { type: "error", message: `${file.name} is not valid JSON.` };
  }
  if (raw === null || typeof raw !== "object") {
    return { type: "error", message: `${file.name} does not contain motion content.` };
  }
  const record = raw as Record<string, unknown>;
  if (record.schema === PATTERN_SCHEMA || record.schema === PROGRAM_SCHEMA) {
    return {
      type: "share",
      file,
      kind: record.schema === PATTERN_SCHEMA ? "pattern" : "program",
      name: typeof record.name === "string" ? record.name : "",
    };
  }
  if (record.schema !== undefined) {
    return { type: "error", message: `${file.name} uses an unknown motion content schema.` };
  }
  if (!Array.isArray(record.actions)) {
    return { type: "error", message: `${file.name} has no actions and no MagicHandy schema.` };
  }
  if (record.actions.length < 2 || record.actions.length > MAX_SOURCE_ACTIONS) {
    return { type: "error", message: `${file.name} must contain 2 to ${MAX_SOURCE_ACTIONS} source actions.` };
  }

  if (record.version !== undefined && typeof record.version !== "string") {
    return { type: "error", message: `${file.name} has an invalid funscript version.` };
  }
  if (record.inverted !== undefined && typeof record.inverted !== "boolean") {
    return { type: "error", message: `${file.name} has an invalid inverted flag.` };
  }
  const inverted = record.inverted === true;
  const points: ActionPoint[] = [];
  for (let index = 0; index < record.actions.length; index++) {
    const entry = record.actions[index];
    if (entry === null || typeof entry !== "object" || Array.isArray(entry)) {
      return { type: "error", message: `${file.name} action ${index + 1} is not usable.` };
    }
    const action = entry as Record<string, unknown>;
    const at = action.at;
    const pos = action.pos;
    if (typeof at !== "number" || !Number.isFinite(at) || at < 0 || at > MAX_IMPORT_DURATION) {
      return { type: "error", message: `${file.name} action ${index + 1} has an invalid time.` };
    }
    if (typeof pos !== "number" || !Number.isFinite(pos) || pos < 0 || pos > 100) {
      return { type: "error", message: `${file.name} action ${index + 1} position must be between 0 and 100.` };
    }
    points.push({ at: Math.round(at), pos: inverted ? 100 - pos : pos });
  }
  points.sort((left, right) => left.at - right.at);
  const distinct: ActionPoint[] = [];
  for (const point of points) {
    if (distinct.length > 0 && distinct[distinct.length - 1].at === point.at) distinct[distinct.length - 1] = point;
    else distinct.push(point);
  }
  if (distinct.length < 2 || distinct[distinct.length - 1].at === distinct[0].at) {
    return { type: "error", message: `${file.name} needs at least two actions with distinct times.` };
  }
  const startAt = distinct[0].at;
  const rebased = distinct.map((point) => ({ at: point.at - startAt, pos: point.pos }));
  const stem = file.name.replace(/\.[^.]*$/, "").trim();
  return {
    type: "funscript",
    file,
    stem: stem || "Imported funscript",
    points: rebased,
    duration: rebased[rebased.length - 1].at,
  };
}

function downsample(points: ActionPoint[], buckets: number): ActionPoint[] {
  if (points.length <= buckets * 2) return points;
  const result: ActionPoint[] = [points[0]];
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

function readFileText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result ?? ""));
    reader.onerror = () => reject(reader.error ?? new Error("file could not be read"));
    reader.readAsText(file);
  });
}
