import { t, translateKnown } from "../i18n";
import { useEffect, useMemo, useState } from "react";
import { api } from "../api/client";
import type { IntifaceTransportSnapshot } from "../api/types";
import { useToast } from "../state/app-state";
import { WirelessIcon } from "../shell/icons";

// While the panel is open and the vibrator is connected, levels change
// faster than the app-state poll.
const STATUS_POLL_MS = 500;
const STEP_PERCENT = 5;

type Busy = "connect" | "disconnect" | "scan" | "select" | "test" | null;

interface VibratorPanelProps {
  visible: boolean;
  locked: boolean;
  canConfigureHost: boolean;
  initial?: IntifaceTransportSnapshot;
  stopSequence?: number;
  playbackActive?: boolean;
  onChanged: () => void;
}

/**
 * The vibrator row of the connection manager. The backend owns the shared
 * Intiface connection, the selection and the live intensity: this panel only
 * renders its snapshot and sends explicit user actions. The intensity starts
 * at zero on every connection and never rises on its own.
 */
export function VibratorPanel({ visible, locked, canConfigureHost, initial, stopSequence, playbackActive = false, onChanged }: VibratorPanelProps) {
  const { show } = useToast();
  const [snapshot, setSnapshot] = useState(initial);
  const [busy, setBusy] = useState<Busy>(null);
  const [choice, setChoice] = useState("");
  const leased = Boolean(snapshot?.status.connected && snapshot.leases?.includes("vibrator"));
  const vibrator = snapshot?.vibrator;
  const selected = leased && Boolean(vibrator?.selected);

  useEffect(() => {
    if (initial) setSnapshot(initial);
  }, [initial]);

  useEffect(() => {
    if (!visible || !leased) return;
    const timer = window.setInterval(() => {
      void api.intifaceStatus().then(setSnapshot).catch(() => undefined);
    }, STATUS_POLL_MS);
    return () => window.clearInterval(timer);
  }, [visible, leased]);

  const candidates = useMemo(() => (snapshot?.status.devices ?? []).filter((device) =>
    (device.scalar_actuators ?? []).some((actuator) => actuator.actuator_type === "Vibrate" && (actuator.step_count ?? 0) > 0)),
  [snapshot?.status.devices]);

  useEffect(() => {
    if (vibrator?.device_index !== undefined) setChoice(String(vibrator.device_index));
    else if (candidates.length === 1) setChoice(String(candidates[0].device_index));
  }, [candidates, vibrator?.device_index]);

  if (!visible) return null;

  async function run(action: Exclude<Busy, null>, request: () => Promise<IntifaceTransportSnapshot>, success?: string) {
    setBusy(action);
    try {
      setSnapshot(await request());
      if (success) show(success);
    } catch (error) {
      show(error instanceof Error ? translateKnown(error.message) : t("Vibrator request failed."), "error");
    } finally {
      setBusy(null);
      onChanged();
    }
  }

  async function setIntensity(percent: number) {
    const current = vibrator?.intensity_percent ?? 0;
    try {
      setSnapshot(await api.vibratorIntensity(percent, percent > current ? stopSequence : undefined));
    } catch (error) {
      show(error instanceof Error ? translateKnown(error.message) : t("Vibrator request failed."), "error");
    }
  }

  const intensity = vibrator?.intensity_percent ?? 0;
  const ceiling = vibrator?.ceiling_percent ?? 0;
  const effective = vibrator?.effective_ceiling_percent ?? ceiling;
  const scanning = Boolean(snapshot?.status.scanning);

  return (
    <div className="vibrator-panel" aria-live="polite">
      <div className="transport-summary">
        <span className="status-dot" data-state={leased ? "connected" : "disconnected"} aria-hidden="true" />
        <span>{translateKnown(vibratorStatusText(snapshot, busy))}</span>
      </div>
      {leased && snapshot?.leases?.includes("handy") && <p className="hint-block">{t("Uses the same Intiface connection as The Handy.")}</p>}
      {leased && snapshot?.ping_watchdog === false && (
        <p className="hint-block">{t("The Intiface keepalive timeout is off, so if MagicHandy stops responding only a closed connection stops the vibrator.")}</p>
      )}

      {leased && !selected && (
        <div className="transport-selection">
          <label className="field">
            <span className="label">{t("Vibrator")}</span>
            <select value={choice} disabled={locked || !canConfigureHost || busy !== null} onChange={(event) => setChoice(event.target.value)}>
              <option value="">{t("Choose a device")}</option>
              {candidates.map((device) => {
                const motors = (device.scalar_actuators ?? []).filter((actuator) => actuator.actuator_type === "Vibrate").length;
                return <option key={device.device_index} value={device.device_index}>{device.device_name} · {t("{count} motors", { count: motors })}</option>;
              })}
            </select>
          </label>
          <button type="button" className="btn btn-secondary" disabled={locked || !canConfigureHost || busy !== null || !choice}
            onClick={() => void run("select", () => api.vibratorSelect(Number(choice)), t("Vibrator ready. Raise the intensity when you want it."))}>
            {t("Use vibrator")}
          </button>
          {candidates.length === 0 && !scanning && <p className="form-status">{t("No vibrators found. Make the vibrator available to Intiface Central, then scan.")}</p>}
        </div>
      )}

      {selected && vibrator && (
        <div className="vibrator-controls" role="group" aria-label={t("Vibrator intensity")}>
          <p className="vibrator-device">
            <strong>{vibrator.device_name}</strong>
            <small>{t("{count} motors", { count: vibrator.motors.length })}</small>
          </p>
          <div className="control-row vibrator-intensity">
            <span className="control-row-label">
              {t("Intensity")}
              <small>{t("{intensity}% of {ceiling}%", { intensity, ceiling })}</small>
            </span>
            <div className="coyote-stepper">
              <button type="button" className="btn btn-secondary coyote-step" aria-label={t("Lower the vibrator intensity")}
                disabled={locked || intensity <= 0} onClick={() => void setIntensity(Math.max(0, intensity - STEP_PERCENT))}>−</button>
              <output className="coyote-strength" aria-label={t("Vibrator intensity")}>{intensity}%</output>
              <button type="button" className="btn btn-secondary coyote-step" aria-label={t("Raise the vibrator intensity")}
                disabled={locked || !canConfigureHost || intensity >= ceiling}
                onClick={() => void setIntensity(Math.min(ceiling, intensity + STEP_PERCENT))}>+</button>
              <button type="button" className="btn btn-start coyote-test" aria-label={t("Test the vibrator")}
                disabled={locked || !canConfigureHost || busy !== null || intensity <= 0 || playbackActive || vibrator.testing || stopSequence === undefined}
                onClick={() => stopSequence !== undefined && void run("test", () => api.vibratorTest(stopSequence))}>{t("Test")}</button>
            </div>
          </div>
          {effective < ceiling && <p className="hint-block">{t("With these motors' steps, the highest level is {percent}%.", { percent: effective })}</p>}
          <p className="hint-block">{t("Use Test for one second at up to 25% while motion is paused. Changing the intensity never starts playback.")}</p>
          {vibrator.last_error && <p className="form-status">{translateKnown(vibrator.last_error)}</p>}
        </div>
      )}

      <div className="row-actions">
        {leased ? (
          <>
            <button type="button" className="btn btn-secondary" disabled={locked || busy !== null}
              onClick={() => void run("disconnect", () => api.intifaceDisconnect("vibrator"), t("Vibrator disconnected. It was stopped before closing."))}>
              <WirelessIcon />{busy === "disconnect" ? t("Disconnecting") : t("Disconnect")}
            </button>
            {!selected && <button type="button" className="btn btn-secondary" disabled={locked || !canConfigureHost || busy !== null}
              onClick={() => void run("scan", scanning ? api.intifaceStopScan : api.intifaceStartScan)}>
              {scanning ? t("Stop scan") : t("Scan devices")}
            </button>}
          </>
        ) : (
          <button type="button" className="btn btn-primary" disabled={locked || !canConfigureHost || busy !== null}
            onClick={() => void run("connect", () => api.intifaceConnect("vibrator"))}>
            <WirelessIcon />{busy === "connect" ? t("Connecting") : t("Connect vibrator")}
          </button>
        )}
      </div>
      {!leased && busy === null && <p className="hint-block">{t("Start Intiface Central and make the vibrator available to it, then connect.")}</p>}
    </div>
  );
}

/** The vibrator's connection line for the panel and the rail trigger. */
export function vibratorStatusText(snapshot: IntifaceTransportSnapshot | undefined, busy?: Busy): string {
  if (busy === "connect") return "Connecting to Intiface";
  const leased = Boolean(snapshot?.status.connected && snapshot.leases?.includes("vibrator"));
  if (!leased) return "Vibrator disconnected";
  const vibrator = snapshot?.vibrator;
  if (!vibrator?.selected) return vibrator?.playback_state === "stale" ? "Vibrator connection lost" : "Intiface connected - choose a vibrator";
  if (vibrator.playback_state === "stale") return "Vibrator connection lost";
  return "Vibrator connected";
}
