import { t, translateKnown } from "../i18n";
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
  maxSpeed: number;
  onPlay: (id: string, speedPercent: number) => Promise<void>;
  onPause: () => Promise<void>;
  onResume: () => Promise<void>;
  onStop: () => Promise<void>;
  onExport: (id: string) => Promise<void>;
  onDelete: (id: string) => Promise<void>;
}

export function ProgramLibrary({ programs, engine, locked, offline, busyKeys, maxSpeed, onPlay, onPause, onResume, onStop, onExport, onDelete }: Props) {
  const speedCap = clampSpeedCap(maxSpeed);
  const [speed, setSpeed] = useState(Math.min(30, speedCap));
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
  useEffect(() => setSpeed((value) => Math.min(value, speedCap)), [speedCap]);

  return (
    <section className="library-view" aria-label={t("Programs and funscripts")}>
      {programs.length > 0 && <h2 className="visually-hidden">{t("Programs")}</h2>}
      <div className="program-toolbar">
        <label className="inline-slider">
          <span>{t("Speed")}<strong>{speed}%</strong></span>
          <input type="range" min={1} max={speedCap} value={speed} disabled={locked} onChange={(event) => setSpeed(Number(event.target.value))} />
        </label>
      </div>

      {activeProgram && (
        <div className="program-player" aria-label={t("Program player")}>
          <div>
            <strong>{engine?.target?.label ?? t("Program")}</strong>
            <span>{translateKnown(playbackState)} / {engine?.running || engine?.paused ? formatClock(engine.running_ms) : <>{progress}%</>}</span>
          </div>
          <div className="program-progress" role="progressbar" aria-label={t("Program progress")} aria-valuemin={0} aria-valuemax={100} aria-valuenow={progress}><span style={{ width: `${progress}%` }} /></div>
          <div className="row-actions">
            {engine?.paused ? <button type="button" className="icon-button" title={t("Resume program")} aria-label={t("Resume program")} disabled={locked || busyKeys.has(libraryActionKey.playerControl)} onClick={() => void onResume()}><PlayIcon /></button> : <button type="button" className="icon-button" title={t("Pause program")} aria-label={t("Pause program")} disabled={locked || !engine?.running || busyKeys.has(libraryActionKey.playerControl)} onClick={() => void onPause()}><PauseIcon /></button>}
            <button type="button" className="icon-button stop-icon-button" title={t("Stop program")} aria-label={t("Stop program")} disabled={offline || busyKeys.has(libraryActionKey.playerStop)} onClick={() => void onStop()}><StopIcon /></button>
          </div>
        </div>
      )}

      <div className="program-list">
        {programs.map((program) => {
          const mutating = busyKeys.has(libraryActionKey.program(program.id));
          return <article className="program-row" key={program.id}>
            <PatternCurve points={program.preview_samples} label={t("Backend-sampled program curve for {name}", { name: program.name })} />
            <div className="pattern-copy">
              <h3>{program.name}</h3>
              <div className="pattern-meta"><span>{formatClock(program.duration_ms)}</span><span>{t("{count} knots", { count: program.points.length })}</span><span>{program.origin}</span></div>
            </div>
            <div className="pattern-actions">
              <button type="button" className="btn btn-primary compact-command" disabled={locked || mutating || busyKeys.has(libraryActionKey.motionStart)} onClick={() => void onPlay(program.id, speed)}><PlayIcon />{t("Play")}</button>
              <button type="button" className="icon-button" title={t("Export program")} aria-label={t("Export {name}", { name: program.name })} disabled={offline || busyKeys.has(libraryActionKey.exportProgram(program.id))} onClick={() => void onExport(program.id)}><DownloadIcon /></button>
              <button type="button" className="icon-button" title={t("Delete program")} aria-label={t("Delete {name}", { name: program.name })} disabled={locked || mutating} onClick={() => void onDelete(program.id)}><TrashIcon /></button>
            </div>
          </article>;
        })}
        {programs.length === 0 && <div className="empty-state compact-empty"><h2>{t("No programs imported")}</h2><p>{t("Use the Import tab to bring in a funscript or share file.")}</p></div>}
      </div>
    </section>
  );
}

function clampSpeedCap(value: number): number {
  return Math.max(1, Math.min(100, Number.isFinite(value) ? Math.round(value) : 100));
}
