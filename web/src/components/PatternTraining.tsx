import { t } from "../i18n";
import { useEffect, useMemo, useState } from "react";
import type { LibraryPattern, PatternFeedback } from "../api/types";
import { PlayIcon, ThumbDownIcon, ThumbUpIcon, UndoIcon } from "../shell/icons";
import { libraryActionKey, type LibraryBusyKeys } from "./library-actions";
import { PatternCurve } from "./PatternCurve";

interface Props {
  patterns: LibraryPattern[];
  feedback: PatternFeedback[];
  autoDisable: boolean;
  locked: boolean;
  busyKeys: LibraryBusyKeys;
  maxSpeed: number;
  onPlay: (id: string, speedPercent: number, feel: string) => Promise<void>;
  onFeedback: (id: string, rating: -1 | 1) => Promise<void>;
  onUndo: (id: number) => Promise<void>;
  onAutoDisable: (enabled: boolean) => Promise<void>;
}

export function PatternTraining({ patterns, feedback, autoDisable, locked, busyKeys, maxSpeed, onPlay, onFeedback, onUndo, onAutoDisable }: Props) {
  const enabled = useMemo(() => patterns.filter((pattern) => pattern.enabled), [patterns]);
  const patternNames = useMemo(() => new Map(patterns.map((pattern) => [pattern.id, pattern.name])), [patterns]);
  const speedCap = Math.max(1, Math.min(100, Number.isFinite(maxSpeed) ? Math.round(maxSpeed) : 100));
  const [index, setIndex] = useState(0);
  const [speed, setSpeed] = useState(Math.min(30, speedCap));
  const [feel, setFeel] = useState("original");
  useEffect(() => { if (index >= enabled.length) setIndex(0); }, [enabled.length, index]);
  useEffect(() => setSpeed((value) => Math.min(value, speedCap)), [speedCap]);
  const pattern = enabled[index];
  const latest = pattern ? feedback.find((item) => item.pattern_id === pattern.id && !item.reverted) : undefined;
  const patternBusy = pattern ? busyKeys.has(libraryActionKey.pattern(pattern.id)) : false;

  if (!pattern) {
    return <section className="library-view"><div className="empty-state"><h2>{t("No enabled patterns")}</h2><p>{t("Deterministic motion remains active for chat.")}</p></div></section>;
  }

  return (
    <section className="training-layout" aria-label={t("Pattern training")}>
      <div className="training-stage">
        <div className="training-heading">
          <div><span className="eyebrow">{t("Pattern {current} of {total}", { current: index + 1, total: enabled.length })}</span><h2>{pattern.name}</h2></div>
          <button type="button" className="btn btn-secondary" disabled={enabled.length < 2} onClick={() => setIndex((index + 1) % enabled.length)}>{t("Next pattern")}</button>
        </div>
        <PatternCurve points={pattern.preview_samples} knots={pattern.points} label={t("Backend-sampled training curve for {name}", { name: pattern.name })} className="training-curve" />
        <div className="training-stats"><span>{t("Weight")}<strong>{pattern.weight.toFixed(2)}</strong></span><span>{t("{seconds} s cycle", { seconds: (pattern.cycle_ms / 1000).toFixed(1) })}</span><span>{pattern.kind}</span></div>
        <div className="training-controls">
          <label className="inline-slider"><span>{t("Speed")}<strong>{speed}%</strong></span><input type="range" min={1} max={speedCap} value={speed} disabled={locked} onChange={(event) => setSpeed(Number(event.target.value))} /></label>
          <div className="segmented compact-segmented" role="group" aria-label={t("Audition feel")}><button type="button" aria-pressed={feel === "original"} data-active={feel === "original" || undefined} onClick={() => setFeel("original")}>{t("Original")}</button><button type="button" aria-pressed={feel === "smooth"} data-active={feel === "smooth" || undefined} onClick={() => setFeel("smooth")}>{t("Smooth")}</button><button type="button" aria-pressed={feel === "crisp"} data-active={feel === "crisp" || undefined} onClick={() => setFeel("crisp")}>{t("Crisp")}</button></div>
          <button type="button" className="btn btn-primary" disabled={locked || patternBusy || busyKeys.has(libraryActionKey.motionStart)} onClick={() => void onPlay(pattern.id, speed, feel)}><PlayIcon />{t("Audition")}</button>
        </div>
        <div className="rating-controls" role="group" aria-label={t("Rate {name}", { name: pattern.name })}>
          <button type="button" className="btn btn-secondary" disabled={locked || patternBusy} onClick={() => void onFeedback(pattern.id, 1)}><ThumbUpIcon />{t("More like this")}</button>
          <button type="button" className="btn btn-secondary" disabled={locked || patternBusy} onClick={() => void onFeedback(pattern.id, -1)}><ThumbDownIcon />{t("Less like this")}</button>
          {latest && <button type="button" className="btn btn-secondary" disabled={locked || patternBusy} onClick={() => void onUndo(latest.id)}><UndoIcon />{t("Undo rating")}</button>}
        </div>
      </div>
      <aside className="training-preferences">
        <h2 className="section-title">{t("Preference controls")}</h2>
        <label className="toggle-line"><span className="toggle"><input type="checkbox" checked={autoDisable} disabled={locked || busyKeys.has(libraryActionKey.autoDisable)} onChange={(event) => void onAutoDisable(event.target.checked)} /><span className="track" aria-hidden="true" /></span><span>{t("Auto-disable at low weight")}</span></label>
        <div className="feedback-ledger">
          <h3>{t("Recent ratings")}</h3>
          {feedback.slice(0, 8).map((item) => {
            return <div className="feedback-row" key={item.id} data-reverted={item.reverted || undefined}><span>{patternNames.get(item.pattern_id) ?? item.pattern_id}</span><strong>{item.rating > 0 ? "+" : "-"}{Math.abs(item.weight_after - item.weight_before).toFixed(2)}</strong><span>{item.reverted ? t("Undone") : item.enabled_after ? item.weight_after.toFixed(2) : t("Disabled")}</span></div>;
          })}
          {feedback.length === 0 && <p className="form-status">{t("No ratings yet.")}</p>}
        </div>
      </aside>
    </section>
  );
}
