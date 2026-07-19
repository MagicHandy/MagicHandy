import { useEffect, useState } from "react";
import type { EngineSnapshot, LibraryProgram } from "../api/types";
import { DownloadIcon, PauseIcon, PlayIcon, StopIcon, TrashIcon } from "../shell/icons";
import { formatClock } from "../util/format";
import { libraryActionKey, type LibraryBusyKeys } from "./library-actions";
import { PatternCurve } from "./PatternCurve";

interface Props {
  programs: LibraryProgram[];
  engine?: EngineSnapshot;
  locked: boolean;
  offline: boolean;
  busyKeys: LibraryBusyKeys;
  maxIntensity: number;
  onPlay: (id: string, intensity: number) => Promise<void>;
  onPause: () => Promise<void>;
  onResume: () => Promise<void>;
  onStop: () => Promise<void>;
  onExport: (id: string) => Promise<void>;
  onDelete: (id: string) => Promise<void>;
}

export function ProgramLibrary({ programs, engine, locked, offline, busyKeys, maxIntensity, onPlay, onPause, onResume, onStop, onExport, onDelete }: Props) {
  const intensityCap = clampIntensityCap(maxIntensity);
  const [intensity, setIntensity] = useState(Math.min(30, intensityCap));
  const activeProgram = engine?.target?.program_id;
  const rawPhase = Number.isFinite(engine?.phase) ? engine?.phase ?? 0 : 0;
  const progress = activeProgram ? Math.round(Math.min(1, Math.max(0, rawPhase)) * 100) : 0;
  let playbackState = progress >= 100 ? "Complete" : "Stopped";
  if (engine?.completing) {
    playbackState = "Stopping";
  } else if (engine?.paused) {
    playbackState = "Paused";
  } else if (engine?.starting) {
    playbackState = "Starting";
  } else if (engine?.running) {
    playbackState = "Playing";
  }
  useEffect(() => setIntensity((value) => Math.min(value, intensityCap)), [intensityCap]);

  return (
    <section className="library-view" aria-label="Programs and funscripts">
      {programs.length > 0 && <h2 className="visually-hidden">Programs</h2>}
      <div className="program-toolbar">
        <label className="inline-slider">
          <span>Intensity <strong>{intensity}%</strong></span>
          <input type="range" min={1} max={intensityCap} value={intensity} disabled={locked} onChange={(event) => setIntensity(Number(event.target.value))} />
        </label>
      </div>

      {activeProgram && (
        <div className="program-player" aria-label="Program player">
          <div>
            <strong>{engine?.target?.label ?? "Program"}</strong>
            <span>{playbackState} / {engine?.running || engine?.paused ? formatClock(engine.running_ms) : `${progress}%`}</span>
          </div>
          <div className="program-progress" role="progressbar" aria-label="Program progress" aria-valuemin={0} aria-valuemax={100} aria-valuenow={progress}><span style={{ width: `${progress}%` }} /></div>
          <div className="row-actions">
            {engine?.paused ? <button type="button" className="icon-button" title="Resume program" aria-label="Resume program" disabled={locked || busyKeys.has(libraryActionKey.playerControl)} onClick={() => void onResume()}><PlayIcon /></button> : <button type="button" className="icon-button" title="Pause program" aria-label="Pause program" disabled={locked || !engine?.running || busyKeys.has(libraryActionKey.playerControl)} onClick={() => void onPause()}><PauseIcon /></button>}
            <button type="button" className="icon-button stop-icon-button" title="Stop program" aria-label="Stop program" disabled={offline || busyKeys.has(libraryActionKey.playerStop)} onClick={() => void onStop()}><StopIcon /></button>
          </div>
        </div>
      )}

      <div className="program-list">
        {programs.map((program) => {
          const mutating = busyKeys.has(libraryActionKey.program(program.id));
          return <article className="program-row" key={program.id}>
            <PatternCurve points={program.preview_samples} label={`${program.name} backend-sampled program curve`} />
            <div className="pattern-copy">
              <h3>{program.name}</h3>
              <div className="pattern-meta"><span>{formatClock(program.duration_ms)}</span><span>{program.points.length} knots</span><span>{program.origin}</span></div>
            </div>
            <div className="pattern-actions">
              <button type="button" className="btn btn-primary compact-command" disabled={locked || mutating || busyKeys.has(libraryActionKey.motionStart)} onClick={() => void onPlay(program.id, intensity)}><PlayIcon /> Play</button>
              <button type="button" className="icon-button" title="Export program" aria-label={`Export ${program.name}`} disabled={offline || busyKeys.has(libraryActionKey.exportProgram(program.id))} onClick={() => void onExport(program.id)}><DownloadIcon /></button>
              <button type="button" className="icon-button" title="Delete program" aria-label={`Delete ${program.name}`} disabled={locked || mutating} onClick={() => void onDelete(program.id)}><TrashIcon /></button>
            </div>
          </article>;
        })}
        {programs.length === 0 && <div className="empty-state compact-empty"><h2>No programs imported</h2><p>Use the Import tab to bring in a funscript or share file.</p></div>}
      </div>
    </section>
  );
}

function clampIntensityCap(value: number): number {
  return Math.max(1, Math.min(100, Number.isFinite(value) ? Math.round(value) : 100));
}
