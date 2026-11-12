// The VacuGlide's tempo (ADR 0071): its labels and the person's own tempo
// controls in the Play card. Now playing shows what plays. Every value is the
// backend snapshot; the controls only send requests and render what comes back.
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import { tempoLeads } from "../api/devices";
export { tempoLeads } from "../api/devices";
import type { MotionInfo, PublicSettings, TempoReadout as TempoReadoutData, TempoRoutine, TempoSpec } from "../api/types";
import { vacuglideStroke } from "../devices/AutoblowPage";
import { ValueSlider } from "../devices/fields";
import { useDeviceContext, useDeviceWatch } from "../devices/runtime";
import { t, translateKnown } from "../i18n";
import { useAppState, useMotionState, useToast } from "../state/app-state";

export function showsTempo(settings: PublicSettings | undefined, motion: MotionInfo | null): boolean {
  const engine = motion?.engine;
  if (engine?.running || engine?.paused || engine?.starting || engine?.completing) return Boolean(engine.target?.tempo_only);
  return tempoLeads(settings);
}

export function routineName(id: string | undefined, fallback?: string): string {
  switch (id) {
    case "slow-build": return t("Slow build");
    case "swells": return t("Swells");
    case "bursts": return t("Bursts");
    case "stairs": return t("Stairs");
    case "edge-and-release": return t("Edge and release");
    case "wandering": return t("Wandering");
    default: return fallback ?? id ?? "";
  }
}

export function tempoModeLabel(readout: TempoReadoutData | undefined, playing: boolean): string {
  if (!playing || !readout) return t("Stopped");
  switch (readout.mode) {
    case "follow": return t("Following the movement");
    case "steady": return t("Steady");
    case "ramp": return t("Gliding to a new tempo");
    case "wave": return t("Waves");
    case "pulse": return t("Pulses");
    case "pause": return t("Paused");
    case "sequence": return t("Planned sequence");
    case "routine": return routineName(readout.routine);
    default: return readout.mode;
  }
}

const TEMPO_SEND_MS = 300;

export function TempoControls({ playing, readout }: { playing: boolean; readout?: TempoReadoutData }) {
  const { backendOnline, readOnly, refresh, state } = useAppState();
  const { show } = useToast();
  const [routines, setRoutines] = useState<TempoRoutine[]>([]);
  const [draft, setDraft] = useState<number | null>(null);
  const timer = useRef<number | undefined>(undefined);
  const requestVersion = useRef(0);
  const stopSequence = state?.stop_sequence;
  const locked = !backendOnline || readOnly || stopSequence === undefined;
  const level = draft ?? Math.max(1, Math.round(readout?.level_percent ?? 40));

  useEffect(() => {
    let live = true;
    api.tempoRoutines().then((result) => { if (live) setRoutines(result.routines ?? []); }).catch(() => undefined);
    return () => { live = false; window.clearTimeout(timer.current); requestVersion.current++; };
  }, []);

  useEffect(() => {
    window.clearTimeout(timer.current);
    timer.current = undefined;
    requestVersion.current++;
    setDraft(null);
  }, [locked, playing, stopSequence]);

  const send = (tempo: TempoSpec, admittedSequence = stopSequence) => {
    window.clearTimeout(timer.current);
    timer.current = undefined;
    const version = ++requestVersion.current;
    if (locked || admittedSequence === undefined) return;
    void api.setTempo(tempo, admittedSequence).then(() => refresh()).catch((error: unknown) => {
      if (version === requestVersion.current) {
        show(error instanceof Error ? translateKnown(error.message) : t("The tempo could not be changed."), "error");
      }
    }).finally(() => { if (version === requestVersion.current) setDraft(null); });
  };
  const changeLevel = (value: number) => {
    setDraft(value);
    window.clearTimeout(timer.current);
    const admittedSequence = stopSequence;
    timer.current = window.setTimeout(() => send({ mode: "steady", level_percent: value }, admittedSequence), TEMPO_SEND_MS);
  };

  return (
    <div className="tempo-controls">
      <div className="tempo-routines" role="group" aria-label={t("Routines")}>
        {routines.map((routine) => (
          <button key={routine.id} type="button" className="btn btn-secondary" disabled={locked}
            title={routine.description} aria-pressed={playing && readout?.mode === "routine" && readout.routine === routine.id}
            onClick={() => send({ mode: "routine", routine: routine.id })}>
            {routineName(routine.id, routine.name)}
          </button>
        ))}
      </div>
      <ValueSlider label={t("Tempo")} value={level} min={1} max={100} disabled={locked || !playing}
        valueText={t("{n}%", { n: level })} onChange={changeLevel} />
      <div className="row-actions">
        <button type="button" className="btn btn-secondary" disabled={locked || !playing || readout?.mode === "pause"}
          onClick={() => send({ mode: "pause" })}>{t("Pause the motor")}</button>
      </div>
    </div>
  );
}

/** The Play card while tempo leads: routines, tempo, the stroke and Pause. */
export function TempoPlayCard() {
  useDeviceWatch();
  const ctx = useDeviceContext();
  const motion = useMotionState();
  const engine = motion?.engine;
  const status = ctx.autoblow?.vacuglide;
  if (!status?.connected) return <p className="tempo-play-hint">{t("Connect the VacuGlide to play its routines.")}</p>;
  const playing = Boolean(engine?.running && !engine.paused && engine.target?.tempo);
  const steps = status.stroke_steps ?? 0;
  const strokeLocked = ctx.locked || !ctx.canConfigureHost || ctx.stopSequence === undefined;
  return (
    <div className="tempo-play">
      <TempoControls playing={playing} readout={engine?.tempo} />
      {ctx.canConfigureHost && (
        <div className="control-row tempo-stroke">
          <span className="control-row-label">{t("Stroke")}</span>
          <span className="tempo-stepper">
            <button type="button" className="btn btn-secondary" aria-label={t("Shorter, looser stroke")} disabled={strokeLocked} onClick={() => vacuglideStroke(ctx, false)}>−</button>
            <output aria-live="polite">{steps > 0 ? t("+{n}", { n: steps }) : steps}</output>
            <button type="button" className="btn btn-secondary" aria-label={t("Longer, tighter stroke")} disabled={strokeLocked} onClick={() => vacuglideStroke(ctx, true)}>+</button>
          </span>
        </div>
      )}
    </div>
  );
}
