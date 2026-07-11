import { useEffect, useMemo, useState } from "react";
import type { LibraryPattern, PatternFeedback } from "../api/types";
import { PlayIcon, ThumbDownIcon, ThumbUpIcon, UndoIcon } from "../shell/icons";
import { PatternCurve } from "./PatternCurve";

interface Props {
  patterns: LibraryPattern[];
  feedback: PatternFeedback[];
  autoDisable: boolean;
  locked: boolean;
  busyId: string;
  maxIntensity: number;
  onPlay: (id: string, intensity: number, feel: string) => Promise<void>;
  onFeedback: (id: string, rating: -1 | 1) => Promise<void>;
  onUndo: (id: number) => Promise<void>;
  onAutoDisable: (enabled: boolean) => Promise<void>;
}

export function PatternTraining({ patterns, feedback, autoDisable, locked, busyId, maxIntensity, onPlay, onFeedback, onUndo, onAutoDisable }: Props) {
  const enabled = useMemo(() => patterns.filter((pattern) => pattern.enabled), [patterns]);
  const [index, setIndex] = useState(0);
  const [intensity, setIntensity] = useState(Math.min(30, maxIntensity));
  const [feel, setFeel] = useState("original");
  useEffect(() => { if (index >= enabled.length) setIndex(0); }, [enabled.length, index]);
  useEffect(() => setIntensity((value) => Math.min(value, maxIntensity)), [maxIntensity]);
  const pattern = enabled[index];
  const latest = pattern ? feedback.find((item) => item.pattern_id === pattern.id && !item.reverted) : undefined;

  if (!pattern) {
    return <section className="library-view"><div className="empty-state"><h2>No enabled patterns</h2><p>Deterministic motion remains active for chat.</p></div></section>;
  }

  return (
    <section className="training-layout" aria-label="Pattern training">
      <div className="training-stage">
        <div className="training-heading">
          <div><span className="eyebrow">Pattern {index + 1} of {enabled.length}</span><h2>{pattern.name}</h2></div>
          <button type="button" className="btn btn-secondary" disabled={enabled.length < 2} onClick={() => setIndex((index + 1) % enabled.length)}>Next pattern</button>
        </div>
        <PatternCurve points={pattern.preview_samples} label={`${pattern.name} backend-sampled training curve`} className="training-curve" />
        <div className="training-stats"><span>Weight <strong>{pattern.weight.toFixed(2)}</strong></span><span>{(pattern.cycle_ms / 1000).toFixed(1)} s cycle</span><span>{pattern.kind}</span></div>
        <div className="training-controls">
          <label className="inline-slider"><span>Intensity <strong>{intensity}%</strong></span><input type="range" min={1} max={maxIntensity} value={intensity} disabled={locked} onChange={(event) => setIntensity(Number(event.target.value))} /></label>
          <div className="segmented compact-segmented" role="group" aria-label="Audition feel"><button type="button" aria-pressed={feel === "original"} data-active={feel === "original" || undefined} onClick={() => setFeel("original")}>Original</button><button type="button" aria-pressed={feel === "smooth"} data-active={feel === "smooth" || undefined} onClick={() => setFeel("smooth")}>Smooth</button><button type="button" aria-pressed={feel === "crisp"} data-active={feel === "crisp" || undefined} onClick={() => setFeel("crisp")}>Crisp</button></div>
          <button type="button" className="btn btn-primary" disabled={locked || busyId === pattern.id} onClick={() => void onPlay(pattern.id, intensity, feel)}><PlayIcon /> Audition</button>
        </div>
        <div className="rating-controls" role="group" aria-label={`Rate ${pattern.name}`}>
          <button type="button" className="btn btn-secondary" disabled={locked || busyId === pattern.id} onClick={() => void onFeedback(pattern.id, 1)}><ThumbUpIcon /> More like this</button>
          <button type="button" className="btn btn-secondary" disabled={locked || busyId === pattern.id} onClick={() => void onFeedback(pattern.id, -1)}><ThumbDownIcon /> Less like this</button>
          {latest && <button type="button" className="btn btn-secondary" disabled={locked || busyId === pattern.id || busyId === `feedback-${latest.id}`} onClick={() => void onUndo(latest.id)}><UndoIcon /> Undo rating</button>}
        </div>
      </div>
      <aside className="training-preferences">
        <h2 className="section-title">Preference controls</h2>
        <label className="toggle-line"><span className="toggle"><input type="checkbox" checked={autoDisable} disabled={locked || busyId === "auto-disable"} onChange={(event) => void onAutoDisable(event.target.checked)} /><span className="track" aria-hidden="true" /></span><span>Auto-disable at low weight</span></label>
        <div className="feedback-ledger">
          <h3>Recent ratings</h3>
          {feedback.slice(0, 8).map((item) => {
            const itemPattern = patterns.find((candidate) => candidate.id === item.pattern_id);
            return <div className="feedback-row" key={item.id} data-reverted={item.reverted || undefined}><span>{itemPattern?.name ?? item.pattern_id}</span><strong>{item.rating > 0 ? "+" : "-"}{Math.abs(item.weight_after - item.weight_before).toFixed(2)}</strong><span>{item.reverted ? "Undone" : item.enabled_after ? item.weight_after.toFixed(2) : "Disabled"}</span></div>;
          })}
          {feedback.length === 0 && <p className="form-status">No ratings yet.</p>}
        </div>
      </aside>
    </section>
  );
}
