import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { PlayIcon, UploadIcon } from "../shell/icons";
import { ImportTimeline, formatTimelineTime, type TimelinePoint, type TimeWindow } from "./ImportTimeline";
import { MediaPreviewDialog } from "./MediaPreviewDialog";

// Import studio: funscripts get a client-side trim timeline before the file
// subset is submitted through the normal validated import endpoint; MagicHandy
// share files carry their own kind and import as-is. The backend remains the
// only parser that decides what actually lands in the library.

const PATTERN_SCHEMA = "magichandy.pattern.v1";
const PROGRAM_SCHEMA = "magichandy.program.v1";
const MAX_IMPORT_ACTIONS = 4096;
// The backend reserves one of its 256 stored pattern points for loop closure.
const MAX_PATTERN_ANCHORS = 255;
const MAX_SOURCE_ACTIONS = MAX_IMPORT_ACTIONS * 5;
const MAX_IMPORT_BYTES = 8 * 1024 * 1024;
const MAX_IMPORT_DURATION = 24 * 60 * 60 * 1000;
const MAX_NAME_CHARS = 80;
type ParsedFile =
  | { type: "funscript"; stem: string; points: TimelinePoint[]; duration: number }
  | { type: "share"; file: File; kind: "pattern" | "program"; name: string }
  | { type: "error"; message: string };

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
  const [previewOpen, setPreviewOpen] = useState(false);
  const readRequest = useRef(0);
  const closePreview = useCallback(() => setPreviewOpen(false), []);

  const funscript = parsed?.type === "funscript" ? parsed : null;
  const selection = useMemo(() => {
    if (!funscript) return [];
    return selectedActionPoints(funscript.points, trim.start, trim.end);
  }, [funscript, trim]);
  const selectionSpan = kind === "pattern" && selection.length > 0 && selection.length <= MAX_IMPORT_ACTIONS
    ? positionSpan(selection)
    : 0;
  const selectionProblem = !funscript ? "" : selectionProblemFor(selection, kind, selectionSpan);
  const contentName = name.trim() || funscript?.stem || "Imported funscript";
  const nameProblem = Array.from(contentName).length > MAX_NAME_CHARS
    ? `Name must be ${MAX_NAME_CHARS} characters or fewer.`
    : /[/\\]/.test(contentName) ? "Name cannot contain path separators (/ or \\)." : "";
  const importProblem = selectionProblem || nameProblem;
  const trimmed = funscript !== null && (trim.start > 0 || trim.end < funscript.duration);

  useEffect(() => () => {
    readRequest.current += 1;
  }, []);

  async function chooseFile(file: File) {
    const request = ++readRequest.current;
    setReading(true);
    setParsed(null);
    setPreviewOpen(false);
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
    if (ok) {
      setPreviewOpen(false);
      setParsed(null);
    }
  }

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
        {funscript && <button type="button" className="btn btn-secondary" disabled={locked || importing} onClick={() => setPreviewOpen(true)}><PlayIcon />Preview with video</button>}
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
          <ImportTimeline
            points={funscript.points}
            duration={funscript.duration}
            start={trim.start}
            end={trim.end}
            viewport={viewport}
            disabled={locked || importing}
            onTrimChange={setTrim}
            onViewportChange={setViewport}
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
              : "Loop patterns repeat. Active timing remains as selected; cycles shorter than 6.6 seconds are safety-stretched to 6.6 seconds. Qualifying stationary pauses over 5 seconds collapse, positions expand to the full relative span, and the loop closes."}
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
      {funscript && previewOpen && (
        <MediaPreviewDialog
          funscriptName={funscript.stem}
          points={funscript.points}
          duration={funscript.duration}
          trim={trim}
          viewport={viewport}
          disabled={locked || importing}
          onTrimChange={setTrim}
          onViewportChange={setViewport}
          onClose={closePreview}
        />
      )}
    </section>
  );
}

function selectedActionPoints(points: TimelinePoint[], start: number, end: number): TimelinePoint[] {
  const from = firstPointAtOrAfter(points, start);
  const after = firstPointAfter(points, end);
  return points.slice(from, after);
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

function firstPointAfter(points: TimelinePoint[], at: number): number {
  let low = 0;
  let high = points.length;
  while (low < high) {
    const middle = Math.floor((low + high) / 2);
    if (points[middle].at <= at) low = middle + 1;
    else high = middle;
  }
  return low;
}

function selectionProblemFor(selection: TimelinePoint[], kind: "pattern" | "program", span: number): string {
  if (selection.length < 2 || selection[selection.length - 1].at === selection[0].at) {
    return "Select at least two actions with distinct times.";
  }
  if (selection.length > MAX_IMPORT_ACTIONS) {
    return `Selection has ${selection.length} actions; trim it to ${MAX_IMPORT_ACTIONS} or fewer.`;
  }
  if (kind === "pattern" && span < 1) {
    return "This selection has no usable motion span for a loop pattern.";
  }
  if (kind === "pattern") {
    const anchors = reversalAnchorCount(selection);
    if (anchors > MAX_PATTERN_ANCHORS) {
      return `This loop has ${anchors} essential reversal knots; trim to a simpler section with ${MAX_PATTERN_ANCHORS} or fewer.`;
    }
  }
  return "";
}

function reversalAnchorCount(points: TimelinePoint[]): number {
  let anchors = 1;
  let lastAnchor = 0;
  let previousDirection = 0;
  for (let index = 1; index < points.length; index++) {
    const delta = points[index].pos - points[index - 1].pos;
    const direction = Math.sign(delta);
    if (direction === 0) continue;
    if (previousDirection !== 0 && direction !== previousDirection && lastAnchor !== index - 1) {
      anchors += 1;
      lastAnchor = index - 1;
    }
    previousDirection = direction;
  }
  if (lastAnchor !== points.length - 1) anchors += 1;
  return anchors;
}

function positionSpan(points: TimelinePoint[]): number {
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
  let content: string;
  try {
    content = await readFileText(file);
  } catch {
    return { type: "error", message: `${file.name} could not be read.` };
  }
  let raw: unknown;
  try {
    raw = JSON.parse(content);
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
  const points: TimelinePoint[] = [];
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
  const distinct: TimelinePoint[] = [];
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
    stem: stem || "Imported funscript",
    points: rebased,
    duration: rebased[rebased.length - 1].at,
  };
}

function readFileText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result ?? ""));
    reader.onerror = () => reject(reader.error ?? new Error("file could not be read"));
    reader.onabort = () => reject(new Error("file read was cancelled"));
    reader.readAsText(file);
  });
}
