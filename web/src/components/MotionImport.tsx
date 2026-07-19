import { useMemo, useState } from "react";
import { UploadIcon } from "../shell/icons";
import { formatClock } from "../util/format";
import { RangeSlider } from "./RangeSlider";

// Import studio: funscripts get a client-side trim timeline before the file
// subset is submitted through the normal validated import endpoint; MagicHandy
// share files carry their own kind and import as-is. The backend remains the
// only parser that decides what actually lands in the library.

const PATTERN_SCHEMA = "magichandy.pattern.v1";
const PROGRAM_SCHEMA = "magichandy.program.v1";
const MAX_IMPORT_ACTIONS = 20480; // backend maxProgramPoints * 5
const TIMELINE_W = 760;
const TIMELINE_H = 140;
const TIMELINE_PAD = 6;

interface ActionPoint {
  at: number;
  pos: number;
}

type ParsedFile =
  | { type: "funscript"; file: File; stem: string; points: ActionPoint[]; duration: number; dropped: number }
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
  const [kind, setKind] = useState<"pattern" | "program">("program");
  const [name, setName] = useState("");

  const funscript = parsed?.type === "funscript" ? parsed : null;
  const selection = useMemo(() => {
    if (!funscript) return [];
    return funscript.points.filter((point) => point.at >= trim.start && point.at <= trim.end);
  }, [funscript, trim]);
  const selectionSpan = selection.length > 0 ? positionSpan(selection) : 0;
  const selectionProblem = !funscript ? "" : selectionProblemFor(selection, kind, selectionSpan);
  const trimmed = funscript !== null && (trim.start > 0 || trim.end < funscript.duration);

  async function chooseFile(file: File) {
    const next = await parseImportFile(file);
    setParsed(next);
    if (next.type === "funscript") {
      setTrim({ start: 0, end: next.duration });
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
      if (selectionProblem) return;
      const rebased = selection.map((point) => ({ at: point.at - selection[0].at, pos: point.pos }));
      const stem = name.trim() || parsed.stem || "Imported funscript";
      const payload = JSON.stringify({ actions: rebased });
      ok = await onImport(new File([payload], `${stem}.funscript`, { type: "application/json" }), kind);
    }
    if (ok) setParsed(null);
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
        <span className="hint-inline">Funscripts (.funscript) and MagicHandy share files (.json).</span>
      </div>

      {!parsed && (
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
          <ImportTimeline points={funscript.points} duration={funscript.duration} start={trim.start} end={trim.end} />
          <RangeSlider
            label="Trim"
            floor={0}
            ceil={funscript.duration}
            minGap={Math.min(200, funscript.duration)}
            minValue={trim.start}
            maxValue={trim.end}
            disabled={locked || importing}
            formatValue={(min, max) => `${formatClock(min)}–${formatClock(max)}`}
            onChange={(next) => setTrim({ start: next.min, end: next.max })}
          />
          <p className="pattern-meta import-selection-meta">
            <span>{formatClock(trim.start)}–{formatClock(trim.end)} of {formatClock(funscript.duration)}</span>
            <span>{selection.length} of {funscript.points.length} actions selected</span>
            {trimmed && <span>trimmed</span>}
            {funscript.dropped > 0 && <span>{funscript.dropped} invalid actions dropped</span>}
          </p>

          <div className="import-options">
            <div className="segmented compact-segmented" role="group" aria-label="Import as">
              <button type="button" aria-pressed={kind === "program"} data-active={kind === "program" || undefined} disabled={locked || importing} onClick={() => setKind("program")}>Program</button>
              <button type="button" aria-pressed={kind === "pattern"} data-active={kind === "pattern" || undefined} disabled={locked || importing} onClick={() => setKind("pattern")}>Loop pattern</button>
            </div>
            <label className="import-name">
              <span className="field-label">Save as</span>
              <input type="text" maxLength={120} value={name} disabled={locked || importing} onChange={(event) => setName(event.target.value)} />
            </label>
          </div>
          <p className="hint-block narrow">
            {kind === "program"
              ? "Programs keep the selection's timeline and play once through the program player."
              : "Loop patterns repeat: pauses longer than 5 seconds are collapsed and positions are stretched to the full relative span."}
          </p>

          {selectionProblem && <p className="import-problem" role="status">{selectionProblem}</p>}
          <button
            type="button"
            className="btn btn-primary"
            disabled={locked || importing || selectionProblem !== ""}
            onClick={() => void submit()}
          >
            {importing ? "Importing" : kind === "program" ? "Import as program" : "Import as loop pattern"}
          </button>
        </div>
      )}
    </section>
  );
}

function ImportTimeline({ points, duration, start, end }: { points: ActionPoint[]; duration: number; start: number; end: number }) {
  const span = Math.max(1, duration);
  const plotW = TIMELINE_W - TIMELINE_PAD * 2;
  const plotH = TIMELINE_H - TIMELINE_PAD * 2;
  const toX = (at: number) => TIMELINE_PAD + (at / span) * plotW;
  const toY = (pos: number) => TIMELINE_PAD + ((100 - pos) / 100) * plotH;
  const path = useMemo(() => {
    const sampled = downsample(points, 380);
    return sampled.map((point, index) => `${index === 0 ? "M" : "L"}${toX(point.at).toFixed(2)} ${toY(point.pos).toFixed(2)}`).join(" ");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [points, span]);
  const startX = toX(Math.min(start, duration));
  const endX = toX(Math.min(end, duration));

  return (
    <svg
      className="import-timeline"
      viewBox={`0 0 ${TIMELINE_W} ${TIMELINE_H}`}
      preserveAspectRatio="none"
      role="img"
      aria-label={`Funscript timeline, ${formatClock(duration)} total, selection ${formatClock(start)} to ${formatClock(end)}`}
    >
      <line x1={TIMELINE_PAD} y1={TIMELINE_H / 2} x2={TIMELINE_W - TIMELINE_PAD} y2={TIMELINE_H / 2} className="pattern-grid-line" />
      {path && <path d={path} className="pattern-curve-line" />}
      {startX > TIMELINE_PAD && <rect className="import-timeline-dim" x={0} y={0} width={startX} height={TIMELINE_H} />}
      {endX < TIMELINE_W - TIMELINE_PAD && <rect className="import-timeline-dim" x={endX} y={0} width={TIMELINE_W - endX} height={TIMELINE_H} />}
      <line className="import-timeline-bound" x1={startX} y1={0} x2={startX} y2={TIMELINE_H} />
      <line className="import-timeline-bound" x1={endX} y1={0} x2={endX} y2={TIMELINE_H} />
    </svg>
  );
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
  if (!Array.isArray(record.actions)) {
    return { type: "error", message: `${file.name} has no actions and no MagicHandy schema.` };
  }

  const inverted = record.inverted === true;
  const points: ActionPoint[] = [];
  let dropped = 0;
  for (const entry of record.actions) {
    const at = Number((entry as Record<string, unknown>)?.at);
    const pos = Number((entry as Record<string, unknown>)?.pos);
    if (!Number.isFinite(at) || !Number.isFinite(pos) || at < 0) {
      dropped++;
      continue;
    }
    const clamped = Math.min(100, Math.max(0, pos));
    points.push({ at: Math.round(at), pos: inverted ? 100 - clamped : clamped });
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
    dropped,
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
