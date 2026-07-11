import { useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from "react";
import type { CurvePoint, PatternInput, PatternPreview } from "../api/types";
import { ClearIcon, PlayIcon } from "../shell/icons";

interface Props {
  locked: boolean;
  saving: boolean;
  onPreview: (input: PatternInput) => Promise<PatternPreview>;
  onSave: (input: PatternInput) => Promise<void>;
}

const initialPoints: CurvePoint[] = [
  { time_ms: 0, position_percent: 10 },
  { time_ms: 3300, position_percent: 90 },
  { time_ms: 6600, position_percent: 10 },
];

export function PatternAuthoring({ locked, saving, onPreview, onSave }: Props) {
  const [name, setName] = useState("Custom pattern");
  const [description, setDescription] = useState("");
  const [kind, setKind] = useState<"routine" | "burst">("routine");
  const [cycle, setCycle] = useState(6600);
  const [tolerance, setTolerance] = useState(1.5);
  const [tags, setTags] = useState("");
  const [mode, setMode] = useState<"draw" | "edit">("draw");
  const [points, setPoints] = useState<CurvePoint[]>(initialPoints);
  const [preview, setPreview] = useState<PatternPreview | null>(null);
  const [previewing, setPreviewing] = useState(false);

  const input = (): PatternInput => ({
    name: name.trim() || "Untitled pattern",
    description: description.trim(),
    kind,
    cycle_ms: cycle,
    points,
    tags: tags.split(",").map((tag) => tag.trim()).filter(Boolean),
    simplify_error: tolerance,
  });

  async function refreshPreview(nextPoints = points) {
    if (nextPoints.length < 2) return;
    setPreviewing(true);
    try {
      const result = await onPreview({ ...input(), points: nextPoints });
      setPreview(result);
      setPoints(result.points);
      setCycle(result.cycle_ms);
      setMode("edit");
    } catch {
      setPreview(null);
    } finally {
      setPreviewing(false);
    }
  }

  async function save() {
    const source = preview?.points ?? points;
    await onSave({ ...input(), points: source });
  }

  function resetCanvas() {
    setPoints([]);
    setPreview(null);
    setMode("draw");
  }

  return (
    <section className="authoring-layout" aria-label="Pattern authoring">
      <div className="authoring-controls">
        <label className="field"><span className="label">Name</span><input value={name} maxLength={80} disabled={locked} onChange={(event) => setName(event.target.value)} /></label>
        <label className="field"><span className="label">Description</span><input value={description} maxLength={400} disabled={locked} onChange={(event) => setDescription(event.target.value)} /></label>
        <div className="field"><span className="label">Pattern type</span><div className="segmented" role="group" aria-label="Pattern type"><button type="button" aria-pressed={kind === "routine"} data-active={kind === "routine" || undefined} disabled={locked} onClick={() => { setKind("routine"); setCycle(Math.max(6600, cycle)); setPreview(null); }}>Routine</button><button type="button" aria-pressed={kind === "burst"} data-active={kind === "burst" || undefined} disabled={locked} onClick={() => { setKind("burst"); setPreview(null); }}>Burst</button></div></div>
        <label className="field"><span className="label">Cycle length (seconds)</span><input type="number" min={kind === "routine" ? 6.6 : 0.5} max={120} step={0.1} value={cycle / 1000} disabled={locked} onChange={(event) => { setCycle(Math.round(Number(event.target.value) * 1000)); setPreview(null); }} /></label>
        <label className="field"><span className="label">Simplification <strong>{tolerance.toFixed(1)}%</strong></span><input type="range" min={0.2} max={5} step={0.1} value={tolerance} disabled={locked} onChange={(event) => { setTolerance(Number(event.target.value)); setPreview(null); }} /></label>
        <label className="field"><span className="label">Tags</span><input value={tags} placeholder="steady, progressive" disabled={locked} onChange={(event) => setTags(event.target.value)} /></label>
        <dl className="authoring-readout"><div><dt>Source points</dt><dd>{preview?.original_count ?? points.length}</dd></div><div><dt>Saved knots</dt><dd>{preview?.simplified_count ?? points.length}</dd></div><div><dt>Preview points</dt><dd>{preview?.samples.length ?? 0}</dd></div></dl>
      </div>

      <div className="authoring-stage">
        <div className="authoring-toolbar">
          <div className="segmented compact-segmented" role="group" aria-label="Canvas mode"><button type="button" aria-pressed={mode === "draw"} data-active={mode === "draw" || undefined} disabled={locked} onClick={() => setMode("draw")}>Draw</button><button type="button" aria-pressed={mode === "edit"} data-active={mode === "edit" || undefined} disabled={locked || points.length < 2} onClick={() => setMode("edit")}>Edit knots</button></div>
          <span className="sampler-label">Backend preview</span>
          <button type="button" className="icon-button" title="Clear canvas" aria-label="Clear canvas" disabled={locked || points.length === 0} onClick={resetCanvas}><ClearIcon /></button>
        </div>
        <PatternCanvas mode={mode} cycle={cycle} points={points} samples={preview?.samples ?? []} disabled={locked} onChange={(next) => { setPoints(next); setPreview(null); }} onCommit={(next) => void refreshPreview(next)} />
        <div className="authoring-actions">
          <button type="button" className="btn btn-secondary" disabled={locked || previewing || points.length < 2} onClick={() => void refreshPreview()}><PlayIcon /> {previewing ? "Sampling" : "Preview"}</button>
          <button type="button" className="btn btn-primary" disabled={locked || saving || points.length < 2 || !name.trim()} onClick={() => void save()}>{saving ? "Saving" : "Save pattern"}</button>
        </div>
        <KnotEditor points={points} cycle={cycle} disabled={locked} onChange={(next) => { setPoints(next); setPreview(null); }} onCommit={(next) => void refreshPreview(next)} />
      </div>
    </section>
  );
}

function PatternCanvas({ mode, cycle, points, samples, disabled, onChange, onCommit }: {
  mode: "draw" | "edit";
  cycle: number;
  points: CurvePoint[];
  samples: CurvePoint[];
  disabled: boolean;
  onChange: (points: CurvePoint[]) => void;
  onCommit: (points: CurvePoint[]) => void;
}) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const active = useRef<{ drawing: boolean; index: number }>({ drawing: false, index: -1 });
  const draft = useRef(points);

  useEffect(() => { draft.current = points; }, [points]);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const render = () => drawCanvas(canvas, cycle, points, samples);
    render();
    const observer = new ResizeObserver(render);
    observer.observe(canvas);
    return () => observer.disconnect();
  }, [cycle, points, samples]);

  function pointerPoint(event: ReactPointerEvent<HTMLCanvasElement>): CurvePoint {
    const rect = event.currentTarget.getBoundingClientRect();
    const x = Math.max(0, Math.min(rect.width, event.clientX - rect.left));
    const y = Math.max(0, Math.min(rect.height, event.clientY - rect.top));
    return { time_ms: Math.round((x / Math.max(1, rect.width)) * cycle), position_percent: Math.round((1 - y / Math.max(1, rect.height)) * 1000) / 10 };
  }

  function down(event: ReactPointerEvent<HTMLCanvasElement>) {
    if (disabled) return;
    event.currentTarget.setPointerCapture(event.pointerId);
    const point = pointerPoint(event);
    if (mode === "draw") {
      active.current = { drawing: true, index: -1 };
      draft.current = [point];
      onChange(draft.current);
      return;
    }
    const index = nearestPoint(points, point, cycle);
    active.current = { drawing: false, index };
  }

  function move(event: ReactPointerEvent<HTMLCanvasElement>) {
    if (disabled || (!active.current.drawing && active.current.index < 0)) return;
    const point = pointerPoint(event);
    if (active.current.drawing) {
      const last = draft.current[draft.current.length - 1];
      if (!last || Math.abs(point.time_ms - last.time_ms) >= 12) {
        draft.current = [...draft.current, point];
        onChange(draft.current);
      }
      return;
    }
    draft.current = draft.current.map((existing, index) => index === active.current.index ? point : existing);
    onChange(draft.current);
  }

  function up(event: ReactPointerEvent<HTMLCanvasElement>) {
    if (disabled) return;
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
    const wasActive = active.current.drawing || active.current.index >= 0;
    active.current = { drawing: false, index: -1 };
    if (wasActive && draft.current.length >= 2) onCommit(draft.current);
  }

  return <canvas ref={canvasRef} className="pattern-canvas" aria-label="Pattern drawing canvas" onPointerDown={down} onPointerMove={move} onPointerUp={up} onPointerCancel={up} />;
}

function drawCanvas(canvas: HTMLCanvasElement, cycle: number, points: CurvePoint[], samples: CurvePoint[]) {
  const rect = canvas.getBoundingClientRect();
  const ratio = window.devicePixelRatio || 1;
  canvas.width = Math.max(1, Math.round(rect.width * ratio));
  canvas.height = Math.max(1, Math.round(rect.height * ratio));
  const context = canvas.getContext("2d");
  if (!context) return;
  context.scale(ratio, ratio);
  const style = getComputedStyle(canvas);
  context.fillStyle = style.getPropertyValue("--surface").trim() || "#111";
  context.fillRect(0, 0, rect.width, rect.height);
  context.strokeStyle = style.getPropertyValue("--line").trim() || "#444";
  context.lineWidth = 1;
  for (let index = 1; index < 4; index += 1) {
    const y = (rect.height * index) / 4;
    context.beginPath(); context.moveTo(0, y); context.lineTo(rect.width, y); context.stroke();
  }
  drawLine(context, points, cycle, rect.width, rect.height, style.getPropertyValue("--muted").trim() || "#888", 1.5);
  drawLine(context, samples, cycle, rect.width, rect.height, style.getPropertyValue("--accent").trim() || "#3b82f6", 2.25);
  context.fillStyle = style.getPropertyValue("--accent").trim() || "#3b82f6";
  for (const point of points) {
    const { x, y } = canvasPoint(point, cycle, rect.width, rect.height);
    context.beginPath(); context.arc(x, y, 3.5, 0, Math.PI * 2); context.fill();
  }
}

function drawLine(context: CanvasRenderingContext2D, points: CurvePoint[], cycle: number, width: number, height: number, color: string, lineWidth: number) {
  if (points.length < 2) return;
  context.strokeStyle = color;
  context.lineWidth = lineWidth;
  context.beginPath();
  points.forEach((point, index) => {
    const projected = canvasPoint(point, cycle, width, height);
    if (index === 0) context.moveTo(projected.x, projected.y); else context.lineTo(projected.x, projected.y);
  });
  context.stroke();
}

function canvasPoint(point: CurvePoint, cycle: number, width: number, height: number) {
  return { x: (point.time_ms / Math.max(1, cycle)) * width, y: (1 - point.position_percent / 100) * height };
}

function nearestPoint(points: CurvePoint[], target: CurvePoint, cycle: number) {
  let best = -1;
  let distance = 14;
  points.forEach((point, index) => {
    const dx = ((point.time_ms - target.time_ms) / Math.max(1, cycle)) * 100;
    const dy = point.position_percent - target.position_percent;
    const next = Math.hypot(dx, dy);
    if (next < distance) { best = index; distance = next; }
  });
  return best;
}

function KnotEditor({ points, cycle, disabled, onChange, onCommit }: { points: CurvePoint[]; cycle: number; disabled: boolean; onChange: (points: CurvePoint[]) => void; onCommit: (points: CurvePoint[]) => void }) {
  return (
    <details className="advanced-fields knot-editor">
      <summary>Edit sparse knots</summary>
      <div className="knot-list">
        {points.map((point, index) => (
          <div className="knot-row" key={`${index}-${point.time_ms}`}>
            <span>{index + 1}</span>
            <label>Time <input type="number" min={0} max={cycle} value={point.time_ms} disabled={disabled} onChange={(event) => onChange(points.map((item, itemIndex) => itemIndex === index ? { ...item, time_ms: Number(event.target.value) } : item))} onBlur={() => onCommit(points)} /></label>
            <label>Position <input type="number" min={0} max={100} step={0.1} value={point.position_percent} disabled={disabled} onChange={(event) => onChange(points.map((item, itemIndex) => itemIndex === index ? { ...item, position_percent: Number(event.target.value) } : item))} onBlur={() => onCommit(points)} /></label>
          </div>
        ))}
      </div>
    </details>
  );
}
