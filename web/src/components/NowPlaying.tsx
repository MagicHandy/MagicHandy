// Now playing (docs/ui-design.md, ADR 0071): one status shape for every device
// mix. It says what plays, who chose it, one pace or tempo bar, and which
// devices take part, from the backend snapshot only. Controls live elsewhere;
// every value is a commanded estimate, never a measurement.
import { isHandy, playbackDevices } from "../api/devices";
import type { EngineSnapshot, PublicSettings } from "../api/types";
import { useDeviceContext, useDeviceWatch } from "../devices/runtime";
import type { DeviceContext } from "../devices/types";
import { t, translateKnown } from "../i18n";
import { useAppState, useMotionState } from "../state/app-state";
import { formatClock } from "../util/format";
import { MotionVisualizer } from "./MotionVisualizer";
import { routineName, showsTempo, tempoModeLabel } from "./TempoReadout";
import { StartupRecoveryNotice } from "./StartupRecoveryNotice";
import { showsVibration } from "./VibrationReadout";
import { visualizerDeviceFor } from "./visualizer/device";

type PlayState = "playing" | "paused" | "starting" | "stopping" | "stopped" | "unavailable";

interface DeviceLine {
  id: string;
  name: string;
  value: string;
  note?: string;
}

export interface NowPlayingView {
  state: PlayState;
  stateLabel: string;
  source?: string;
  title: string;
  subtitle?: string;
  tempo: boolean;
  level: number;
  levelLabel: string;
  meterLabel: string;
  ahead?: number[];
  stroke?: { min: number; max: number; position?: number };
  devices: DeviceLine[];
  honest: string;
  details?: string;
  elapsed: string;
}

function playState(engine: EngineSnapshot | undefined, available: boolean): PlayState {
  if (!available) return "unavailable";
  if (engine?.paused) return "paused";
  if (engine?.completing) return "stopping";
  if (engine?.starting) return "starting";
  if (engine?.running) return "playing";
  return "stopped";
}

function stateLabel(state: PlayState): string {
  switch (state) {
    case "playing": return t("Playing");
    case "paused": return t("Paused");
    case "starting": return t("Starting");
    case "stopping": return t("Stopping");
    case "unavailable": return t("Unavailable");
    default: return t("Stopped");
  }
}

function sourceLabel(engine: EngineSnapshot | undefined, autopilot: boolean): string | undefined {
  if (!engine?.running && !engine?.paused) return undefined;
  if (engine?.target?.source === "edge") return t("Edge hold");
  if (autopilot) return t("Autopilot is choosing");
  switch (engine?.target?.source) {
    case "chat": return t("From chat");
    case "manual_ui": return t("Your choice");
    case "media": return t("Following the video");
    default: return undefined;
  }
}

function deviceName(id: string, settings: PublicSettings | undefined): string {
  if (isHandy(id)) {
    const named = settings?.device?.handy_connections?.find((connection) => connection.id === id);
    return named?.name || t("The Handy");
  }
  switch (id) {
    case "autoblow": return t("Autoblow Ultra");
    case "vacuglide": return t("VacuGlide");
    case "vibrator": return t("Vibrator");
    case "coyote": return t("Coyote");
    default: return id;
  }
}

function deviceLines(engine: EngineSnapshot | undefined, settings: PublicSettings | undefined, ctx: DeviceContext, tempo: boolean, running: boolean): DeviceLine[] {
  const position = engine?.current_sample?.position_percent;
  return playbackDevices(settings?.device).map((id) => {
    const name = deviceName(id, settings);
    if (engine?.linked_session?.retired_devices.includes(id)) return { id, name, value: t("Unavailable") };
    if (engine?.linked_session?.idle_devices?.includes(id)) return { id, name, value: t("Idle") };
    if (id === "vacuglide") {
      const motor = ctx.autoblow?.vacuglide?.commanded_percent;
      return { id, name, value: running && motor !== undefined ? t("motor at {percent}%", { percent: Math.round(motor) }) : "–",
        note: !tempo && engine?.target?.tempo?.mode === "follow" ? t("follows the pace") : undefined };
    }
    if (id === "vibrator") {
      const motors = ctx.intiface?.vibrator?.motors ?? [];
      const level = motors.length ? Math.max(...motors.map((motor) => motor.level_percent ?? 0)) : 0;
      return { id, name, value: running ? t("{n}%", { n: Math.round(level) }) : "–" };
    }
    if (id === "coyote") return { id, name, value: running ? t("follows") : "–" };
    return { id, name, value: running && position !== undefined ? t("{n}%", { n: Math.round(position) }) : "–" };
  });
}

function targetTitle(target: EngineSnapshot["target"]): string {
  const label = target?.label?.trim();
  return target?.pattern_name?.trim() || (label ? translateKnown(label) : "");
}

/** The view model both forms render. Every value is read, never inferred. */
export function useNowPlaying(): NowPlayingView {
  useDeviceWatch();
  const ctx = useDeviceContext();
  const { state } = useAppState();
  const motion = useMotionState();
  const engine = motion?.engine;
  const settings = state?.settings;
  const current = playState(engine, motion?.available !== false);
  const running = current === "playing" || current === "stopping";
  const tempo = showsTempo(settings, motion);
  const vibration = !tempo && showsVibration(settings, motion);
  const autopilot = state?.modes?.mode === "autopilot";
  const target = engine?.target;
  const view: NowPlayingView = {
    state: current,
    stateLabel: stateLabel(current),
    source: sourceLabel(engine, autopilot),
    title: t("Nothing playing"),
    tempo,
    level: 0,
    levelLabel: tempo ? t("Tempo") : vibration ? t("Vibration") : t("Pace"),
    meterLabel: "",
    devices: deviceLines(engine, settings, ctx, tempo, running),
    honest: t("Commanded values, not measurements."),
    elapsed: formatClock(engine?.running_ms),
  };
  if (current === "stopped" || current === "unavailable") return view;
  if (tempo) {
    const readout = engine?.tempo;
    view.title = readout?.mode === "routine" ? routineName(readout.routine) : tempoModeLabel(readout, true);
    view.subtitle = readout?.mode === "routine" ? t("VacuGlide routine") : t("VacuGlide tempo");
    view.level = running ? Math.round(readout?.level_percent ?? 0) : 0;
    view.ahead = running ? readout?.ahead : undefined;
    const vacuglide = ctx.autoblow?.vacuglide;
    const parts = [t("Motor {percent}% sent", { percent: Math.round(vacuglide?.commanded_percent ?? 0) })];
    if (vacuglide?.stroke_steps) parts.push(t("stroke {steps}", { steps: vacuglide.stroke_steps > 0 ? `+${vacuglide.stroke_steps}` : String(vacuglide.stroke_steps) }));
    if (vacuglide?.round_trip_ms) parts.push(t("cloud {millis} ms", { millis: vacuglide.round_trip_ms }));
    view.honest = parts.join(" · ");
    view.details = t("Sent through Autoblow's cloud; not measured on the device.");
    view.meterLabel = t("Tempo: {percent}%", { percent: view.level });
    return view;
  }
  if (vibration) {
    // A vibrator without a Handy plays no positions: no drawing, no pace.
    const motors = ctx.intiface?.vibrator?.motors ?? [];
    view.title = targetTitle(target) || t("Vibration");
    view.level = running && motors.length ? Math.round(Math.max(...motors.map((motor) => motor.level_percent ?? 0))) : 0;
    view.honest = t("Last commands sent to Intiface, not measured vibration.");
    view.meterLabel = t("Vibration: {percent}%", { percent: view.level });
    return view;
  }
  view.title = targetTitle(target) || t("Motion");
  const pace = engine?.pace;
  view.level = running ? Math.round(pace?.effective_percent ?? target?.speed_percent ?? 0) : 0;
  view.meterLabel = t("Pace: {percent}%", { percent: view.level });
  const min = Math.max(0, Math.min(100, engine?.settings?.stroke_min_percent ?? 0));
  const max = Math.max(min, Math.min(100, engine?.settings?.stroke_max_percent ?? 100));
  view.stroke = { min, max, position: running ? engine?.current_sample?.position_percent : undefined };
  if (pace) {
    view.details = pace.limited && pace.limiters?.length
      ? t("Effective pace {effective}% of {requested}% asked; limited by {limiters}.", { effective: Math.round(pace.effective_percent), requested: Math.round(pace.requested_percent), limiters: pace.limiters.join(", ").replaceAll("_", " ") })
      : t("Effective pace {effective}% of {requested}% asked.", { effective: Math.round(pace.effective_percent), requested: Math.round(pace.requested_percent) });
  }
  return view;
}

/** The device drawing with its commanded position and stroke range. */
function PositionGraphic() {
  const motion = useMotionState();
  const { state } = useAppState();
  return <MotionVisualizer motion={motion} graphicOnly device={visualizerDeviceFor(state?.settings)} />;
}

function StateDot({ state }: { state: PlayState }) {
  return <span className="now-dot" data-state={state} aria-hidden="true" />;
}

export function NowPlayingCard() {
  const view = useNowPlaying();
  const active = isActive(view.state);
  return (
    <div className="now-playing">
      <div className="now-line">
        <span className="now-state"><StateDot state={view.state} />{view.stateLabel}</span>
        {view.source && <span className="now-source">{view.source}</span>}
        {active && <span className="now-elapsed">{view.elapsed}</span>}
      </div>
      <div className="now-title">
        <strong>{view.title}</strong>
        {view.subtitle && <small>{view.subtitle}</small>}
      </div>
      {active && (
        <>
          <div className="now-bar-row">
            <span>{view.levelLabel}</span>
            <output>{t("{n}%", { n: view.level })}</output>
          </div>
          <meter className="now-bar" min={0} max={100} value={view.level} aria-label={view.meterLabel} />
          {view.ahead && view.ahead.length > 0 && (
            <div className="now-ahead" role="img" aria-label={t("Tempo over the next {seconds} seconds", { seconds: view.ahead.length })}>
              {view.ahead.map((value, index) => <span key={index} style={{ blockSize: `${Math.max(4, value)}%` }} />)}
            </div>
          )}
          {view.stroke && <PositionGraphic />}
        </>
      )}
      {view.devices.length > 1 && (
        <ul className="now-devices" aria-label={t("In this run")}>
          {view.devices.map((device) => (
            <li key={device.id}><span>{device.name}{device.note && <small> {device.note}</small>}</span><output>{device.value}</output></li>
          ))}
        </ul>
      )}
      {active && <p className="now-honest">{view.honest}</p>}
      {view.details && active && (
        <details className="now-details"><summary>{t("Details")}</summary><p>{view.details}</p></details>
      )}
      <StartupRecoveryNotice announce />
    </div>
  );
}

/** The status bar's single readout: state, what plays, its bar and time. */
export function NowPlayingPill() {
  const view = useNowPlaying();
  const active = isActive(view.state);
  return (
    <div className="now-pill" role="status" aria-label={active
      ? t("Now playing: {title}, {label} {percent}%", { title: view.title, label: view.levelLabel, percent: view.level })
      : view.stateLabel}>
      <StateDot state={view.state} />
      <span className="now-pill-title">{active ? view.title : view.stateLabel}</span>
      {active && <span className="now-pill-bar" aria-hidden="true"><span style={{ inlineSize: `${view.level}%` }} /></span>}
      {active && <output>{t("{n}%", { n: view.level })}</output>}
      {active && <span className="now-pill-time">{view.elapsed}</span>}
    </div>
  );
}

/** Something is in a run, so its level, time and honesty line mean something. */
function isActive(state: PlayState): boolean {
  return state !== "stopped" && state !== "unavailable";
}
