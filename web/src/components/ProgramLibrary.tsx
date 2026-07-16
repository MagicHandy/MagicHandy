import { useEffect, useState } from "react";
import type { EngineSnapshot, LibraryProgram } from "../api/types";
import { DownloadIcon, PauseIcon, PlayIcon, StopIcon, TrashIcon, UploadIcon } from "../shell/icons";
import { formatClock } from "../util/format";
import { PatternCurve } from "./PatternCurve";

interface Props {
  programs: LibraryProgram[];
  engine?: EngineSnapshot;
  locked: boolean;
  offline: boolean;
  busyId: string;
  maxIntensity: number;
  onImport: (file: File, asKind: "pattern" | "program") => Promise<void>;
  onPlay: (id: string, intensity: number) => Promise<void>;
  onPause: () => Promise<void>;
  onResume: () => Promise<void>;
  onStop: () => Promise<void>;
  onExport: (id: string) => Promise<void>;
  onDelete: (id: string) => Promise<void>;
}

export function ProgramLibrary({ programs, engine, locked, offline, busyId, maxIntensity, onImport, onPlay, onPause, onResume, onStop, onExport, onDelete }: Props) {
  const [importAs, setImportAs] = useState<"pattern" | "program">("program");
  const [intensity, setIntensity] = useState(Math.min(30, maxIntensity));
  const activeProgram = engine?.target?.program_id;
  const progress = activeProgram ? Math.round((engine?.phase ?? 0) * 100) : 0;
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
  useEffect(() => setIntensity((value) => Math.min(value, maxIntensity)), [maxIntensity]);

  return (
    <section className="library-view" aria-label="Programs and funscripts">
      <div className="program-toolbar">
        <div className="segmented compact-segmented" role="group" aria-label="Import as">
          <button type="button" aria-pressed={importAs === "program"} data-active={importAs === "program" || undefined} onClick={() => setImportAs("program")}>Program</button>
          <button type="button" aria-pressed={importAs === "pattern"} data-active={importAs === "pattern" || undefined} onClick={() => setImportAs("pattern")}>Loop pattern</button>
        </div>
        <label className="btn btn-secondary file-button">
          <UploadIcon /> Import file
          <input type="file" accept=".funscript,.json" disabled={locked} onChange={(event) => {
            const file = event.target.files?.[0];
            event.currentTarget.value = "";
            if (file) void onImport(file, importAs);
          }} />
        </label>
        <label className="inline-slider">
          <span>Intensity <strong>{intensity}%</strong></span>
          <input type="range" min={1} max={maxIntensity} value={intensity} disabled={locked} onChange={(event) => setIntensity(Number(event.target.value))} />
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
            {engine?.paused ? <button type="button" className="icon-button" title="Resume program" aria-label="Resume program" disabled={locked} onClick={() => void onResume()}><PlayIcon /></button> : <button type="button" className="icon-button" title="Pause program" aria-label="Pause program" disabled={locked || !engine?.running} onClick={() => void onPause()}><PauseIcon /></button>}
            <button type="button" className="icon-button stop-icon-button" title="Stop program" aria-label="Stop program" disabled={offline || busyId === "player"} onClick={() => void onStop()}><StopIcon /></button>
          </div>
        </div>
      )}

      <div className="program-list">
        {programs.map((program) => (
          <article className="program-row" key={program.id}>
            <PatternCurve points={program.preview_samples} label={`${program.name} backend-sampled program curve`} />
            <div className="pattern-copy">
              <h3>{program.name}</h3>
              <div className="pattern-meta"><span>{formatClock(program.duration_ms)}</span><span>{program.points.length} knots</span><span>{program.origin}</span></div>
            </div>
            <div className="pattern-actions">
              <button type="button" className="btn btn-primary compact-command" disabled={locked || busyId === program.id} onClick={() => void onPlay(program.id, intensity)}><PlayIcon /> Play</button>
              <button type="button" className="icon-button" title="Export program" aria-label={`Export ${program.name}`} disabled={offline || busyId === `export-${program.id}`} onClick={() => void onExport(program.id)}><DownloadIcon /></button>
              <button type="button" className="icon-button" title="Delete program" aria-label={`Delete ${program.name}`} disabled={locked || busyId === program.id} onClick={() => void onDelete(program.id)}><TrashIcon /></button>
            </div>
          </article>
        ))}
        {programs.length === 0 && <div className="empty-state compact-empty"><h2>No programs imported</h2></div>}
      </div>
    </section>
  );
}
