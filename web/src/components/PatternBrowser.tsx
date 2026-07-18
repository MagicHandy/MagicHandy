import { useEffect, useMemo, useRef, useState } from "react";
import type { LibraryPattern } from "../api/types";
import { DownloadIcon, PlayIcon, ThumbDownIcon, ThumbUpIcon, TrashIcon } from "../shell/icons";
import { libraryActionKey, type LibraryBusyKeys } from "./library-actions";
import { PatternCurve } from "./PatternCurve";

interface Props {
  patterns: LibraryPattern[];
  locked: boolean;
  offline: boolean;
  busyKeys: LibraryBusyKeys;
  onPatch: (id: string, patch: Partial<Pick<LibraryPattern, "enabled" | "weight">>) => Promise<boolean>;
  onPlay: (id: string) => Promise<void>;
  onFeedback: (id: string, rating: -1 | 1) => Promise<void>;
  onExport: (id: string) => Promise<void>;
  onDelete: (id: string) => Promise<void>;
}

export function PatternBrowser({ patterns, locked, offline, busyKeys, onPatch, onPlay, onFeedback, onExport, onDelete }: Props) {
  const [query, setQuery] = useState("");
  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return patterns;
    return patterns.filter((pattern) => `${pattern.name} ${pattern.description ?? ""} ${pattern.tags.join(" ")}`.toLowerCase().includes(needle));
  }, [patterns, query]);
  const enabled = patterns.filter((pattern) => pattern.enabled).length;

  return (
    <section className="library-view" aria-label="Pattern catalog">
      <div className="library-toolbar">
        <div className="curation-readout" data-fallback={enabled === 0 || undefined}>
          <strong>{enabled === 0 ? "Deterministic fallback active" : `${enabled} ${enabled === 1 ? "pattern" : "patterns"} available to chat`}</strong>
          <span>{patterns.length} total</span>
        </div>
        <label className="field compact-field">
          <span className="visually-hidden">Search patterns</span>
          <input type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search patterns" />
        </label>
      </div>

      <div className="pattern-list">
        {filtered.map((pattern) => {
          const mutating = busyKeys.has(libraryActionKey.pattern(pattern.id));
          const startingMotion = busyKeys.has(libraryActionKey.motionStart);
          return <article className="pattern-row" key={pattern.id} data-disabled={!pattern.enabled || undefined}>
            <label className="toggle pattern-enable" title={pattern.enabled ? "Disable pattern" : "Enable pattern"}>
              <input type="checkbox" checked={pattern.enabled} disabled={locked || mutating} aria-label={`Enable ${pattern.name}`} onChange={(event) => void onPatch(pattern.id, { enabled: event.target.checked })} />
              <span className="track" aria-hidden="true" />
            </label>
            <PatternCurve points={pattern.preview_samples} label={`${pattern.name} backend-sampled pattern curve`} />
            <div className="pattern-copy">
              <div className="pattern-title-line">
                <h3>{pattern.name}</h3>
                <span className="origin-label">{pattern.origin}</span>
              </div>
              {pattern.description && <p>{pattern.description}</p>}
              <div className="pattern-meta"><span>{(pattern.cycle_ms / 1000).toFixed(1)} s</span><span>{pattern.kind}</span><span>{pattern.points.length} knots</span></div>
              {pattern.tags.length > 0 && <div className="tag-list">{pattern.tags.map((tag) => <span key={tag}>{tag}</span>)}</div>}
            </div>
            <WeightEditor pattern={pattern} locked={locked || mutating} onCommit={(weight) => onPatch(pattern.id, { weight })} />
            <div className="pattern-actions">
              <button type="button" className="icon-button" title="Audition pattern" aria-label={`Audition ${pattern.name}`} disabled={locked || !pattern.enabled || mutating || startingMotion} onClick={() => void onPlay(pattern.id)}><PlayIcon /></button>
              <button type="button" className="icon-button" title="Rate up" aria-label={`Rate ${pattern.name} up`} disabled={locked || mutating} onClick={() => void onFeedback(pattern.id, 1)}><ThumbUpIcon /></button>
              <button type="button" className="icon-button" title="Rate down" aria-label={`Rate ${pattern.name} down`} disabled={locked || mutating} onClick={() => void onFeedback(pattern.id, -1)}><ThumbDownIcon /></button>
              <button type="button" className="icon-button" title="Export pattern" aria-label={`Export ${pattern.name}`} disabled={offline || busyKeys.has(libraryActionKey.exportPattern(pattern.id))} onClick={() => void onExport(pattern.id)}><DownloadIcon /></button>
              {pattern.origin !== "builtin" && <button type="button" className="icon-button" title="Delete pattern" aria-label={`Delete ${pattern.name}`} disabled={locked || mutating} onClick={() => void onDelete(pattern.id)}><TrashIcon /></button>}
            </div>
          </article>;
        })}
        {filtered.length === 0 && <div className="empty-state compact-empty"><h2>No matching patterns</h2></div>}
      </div>
    </section>
  );
}

function WeightEditor({ pattern, locked, onCommit }: { pattern: LibraryPattern; locked: boolean; onCommit: (weight: number) => Promise<boolean> }) {
  const [value, setValue] = useState(pattern.weight.toFixed(2));
  const [committing, setCommitting] = useState(false);
  const committingRef = useRef(false);
  useEffect(() => {
    if (!committing) setValue(pattern.weight.toFixed(2));
  }, [committing, pattern.weight]);
  async function commit() {
    if (committingRef.current) return;
    const parsed = Number(value);
    if (!Number.isFinite(parsed)) {
      setValue(pattern.weight.toFixed(2));
      return;
    }
    const next = Math.min(3, Math.max(0.1, parsed));
    setValue(next.toFixed(2));
    if (Math.abs(next - pattern.weight) <= 0.0001) return;
    committingRef.current = true;
    setCommitting(true);
    try {
      if (!await onCommit(next)) setValue(pattern.weight.toFixed(2));
    } catch {
      setValue(pattern.weight.toFixed(2));
    } finally {
      committingRef.current = false;
      setCommitting(false);
    }
  }
  return (
    <label className="weight-editor">
      <span>Weight</span>
      <input type="number" min={0.1} max={3} step={0.05} value={value} disabled={locked || committing} onChange={(event) => setValue(event.target.value)} onBlur={() => void commit()} onKeyDown={(event) => { if (event.key === "Enter") event.currentTarget.blur(); }} />
    </label>
  );
}
