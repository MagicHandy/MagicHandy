import { t, translateKnown } from "../i18n";
import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { CoyoteStatus, CoyoteTransportSnapshot } from "../api/types";
import { useToast } from "../state/app-state";
import { WirelessIcon } from "../shell/icons";

// While the panel is open and a Coyote is connected, strength reports arrive
// faster than the app-state poll.
const STATUS_POLL_MS = 500;

interface CoyotePanelProps {
  locked: boolean;
  canConfigureHost: boolean;
  safetyAcknowledged: boolean;
  initial?: CoyoteTransportSnapshot;
  visible: boolean;
  stopSequence?: number;
  playbackActive?: boolean;
  onChanged: () => void;
}

/**
 * Backend strength snapshots identify their source. Strength steps never
 * initiate output; the separate Test action requires fresh control authority.
 */
export function CoyotePanel({ locked, canConfigureHost, safetyAcknowledged, initial, visible, stopSequence, playbackActive = false, onChanged }: CoyotePanelProps) {
  const { show } = useToast();
  const [snapshot, setSnapshot] = useState(initial);
  const [busy, setBusy] = useState<"connect" | "disconnect" | "safety" | null>(null);
  const [testing, setTesting] = useState(false);
  const status = snapshot?.status;
  const connected = Boolean(status?.connected);

  useEffect(() => {
    if (initial) setSnapshot(initial);
  }, [initial]);

  useEffect(() => {
    if (!visible || !connected) return;
    const timer = window.setInterval(() => {
      void api.coyoteStatus().then(setSnapshot).catch(() => undefined);
    }, STATUS_POLL_MS);
    return () => window.clearInterval(timer);
  }, [visible, connected]);

  if (!visible) return null;

  async function run(action: "connect" | "disconnect") {
    setBusy(action);
    try {
      const next = action === "connect" ? await api.coyoteConnect() : await api.coyoteDisconnect();
      setSnapshot(next);
      show(action === "connect" ? t("Coyote connected. Zero command accepted.") : t("Coyote disconnected. Both channels were set to zero."));
    } catch (error) {
      show(error instanceof Error ? translateKnown(error.message) : t("Coyote request failed."), "error");
    } finally {
      setBusy(null);
      onChanged();
    }
  }

  async function acknowledge() {
    setBusy("safety");
    try {
      await api.setEstimSafety(true);
    } catch (error) {
      show(error instanceof Error ? translateKnown(error.message) : t("Coyote request failed."), "error");
    } finally {
      setBusy(null);
      onChanged();
    }
  }

  async function step(channel: "a" | "b", delta: number) {
    try {
      setSnapshot(await api.coyoteStrength(channel, delta, stopSequence));
    } catch (error) {
      show(error instanceof Error ? translateKnown(error.message) : t("Coyote request failed."), "error");
    }
  }

  async function test(channel: "a" | "b") {
    if (stopSequence === undefined) return;
    setTesting(true);
    try {
      setSnapshot(await api.coyoteTest(channel, stopSequence));
    } catch (error) {
      show(error instanceof Error ? translateKnown(error.message) : t("Coyote request failed."), "error");
    } finally {
      setTesting(false);
    }
  }

  async function zero() {
    try {
      setSnapshot(await api.coyoteZero());
    } catch (error) {
      show(error instanceof Error ? translateKnown(error.message) : t("Coyote request failed."), "error");
    }
  }

  if (!safetyAcknowledged) {
    return (
      <div className="coyote-panel coyote-safety" aria-live="polite">
        <h3 className="coyote-safety-title">{t("Before you connect a Coyote")}</h3>
        <ul>
          <li>{t("Place electrodes below the waist only. Never across the chest or near the heart.")}</li>
          <li>{t("Do not use it with a pacemaker or other implant, a heart condition, epilepsy, or during pregnancy.")}</li>
          <li>{t("Start at zero and raise strength slowly. MagicHandy never raises strength on its own.")}</li>
          <li>{t("Stop and Esc set both channels to zero.")}</li>
        </ul>
        <button type="button" className="btn btn-primary" disabled={locked || !canConfigureHost || busy !== null} onClick={() => void acknowledge()}>
          {t("I have read these notes")}
        </button>
      </div>
    );
  }

  return (
    <div className="coyote-panel" aria-live="polite">
      <div className="transport-summary">
        <span className="status-dot" data-state={connected ? "connected" : "disconnected"} aria-hidden="true" />
        <span>{translateKnown(coyoteStatusText(status, busy))}</span>
        {connected && status?.battery_percent !== undefined && <span className="coyote-battery">{t("Battery {percent}%", { percent: status.battery_percent })}</span>}
      </div>
      {snapshot && !snapshot.available && !status?.simulated && <p className="form-status">{t("The Coyote Bluetooth helper is missing from this installation")}</p>}
      {connected && status && (
        <div className="coyote-channels" role="group" aria-label={t("Coyote strength")}>
          <CoyoteChannel label={t("Channel A")} channel="a" status={status} locked={locked} canRaise={canConfigureHost} onStep={step} onTest={test} testPending={testing || stopSequence === undefined || playbackActive} />
          <CoyoteChannel label={t("Channel B")} channel="b" status={status} locked={locked} canRaise={canConfigureHost} onStep={step} onTest={test} testPending={testing || stopSequence === undefined || playbackActive} />
          {status.strength_source === "zero_command" && <p className="hint-block">{t("Zero command accepted. The device has not reported a strength change.")}</p>}
          <p className="hint-block">{status.reference_pulse
            ? t("Playing a short test pulse on the selected channel.")
            : t("Use Test to play a short pulse while motion is paused. Changing strength never starts playback.")}</p>
          <button type="button" className="btn btn-secondary" disabled={locked} onClick={() => void zero()}>{t("Set both to zero")}</button>
        </div>
      )}
      <div className="row-actions">
        {connected ? (
          <button type="button" className="btn btn-secondary" disabled={locked || busy !== null} onClick={() => void run("disconnect")}>
            <WirelessIcon />{busy === "disconnect" ? t("Disconnecting") : t("Disconnect")}
          </button>
        ) : (
          <button type="button" className="btn btn-primary" disabled={locked || !canConfigureHost || busy !== null} onClick={() => void run("connect")}>
            <WirelessIcon />{busy === "connect" ? t("Connecting") : t("Connect Coyote")}
          </button>
        )}
      </div>
      {!connected && !busy && <p className="hint-block">{t("Turn the Coyote on. The first connection needs pairing mode: pull both switches down.")}</p>}
    </div>
  );
}

function CoyoteChannel({ label, channel, status, locked, canRaise, onStep, onTest, testPending }: {
  label: string;
  channel: "a" | "b";
  status: CoyoteStatus;
  locked: boolean;
  canRaise: boolean;
  onStep: (channel: "a" | "b", delta: number) => Promise<void>;
  onTest: (channel: "a" | "b") => Promise<void>;
  testPending: boolean;
}) {
  const strength = channel === "a" ? status.strength_a : status.strength_b;
  const ceiling = channel === "a" ? status.ceiling_a : status.ceiling_b;
  const atCeiling = strength >= ceiling;
  const ready = status.limits_applied && status.strength_known && !status.strength_pending && status.playback_state !== "stopping";
  return (
    <div className="coyote-channel control-row">
      <span className="control-row-label">
        {label}
        <small>{status.strength_known
          ? t("{strength} of {ceiling}", { strength, ceiling })
          : t("Confirming strength…")}</small>
      </span>
      <div className="coyote-stepper">
        <button
          type="button"
          className="btn btn-secondary coyote-step"
          aria-label={t("Lower {channel}", { channel: label })}
          disabled={locked || (status.strength_known && strength <= 0)}
          onClick={() => void onStep(channel, -1)}
        >−</button>
        <output className="coyote-strength" aria-label={t("{channel} strength", { channel: label })}>{status.strength_known ? strength : "–"}</output>
        <button
          type="button"
          className="btn btn-secondary coyote-step"
          aria-label={t("Raise {channel}", { channel: label })}
          disabled={locked || !canRaise || !ready || atCeiling}
          onClick={() => void onStep(channel, 1)}
        >+</button>
        <button type="button" className="btn btn-start coyote-test" aria-label={t("Test {channel}", { channel: label })}
          disabled={locked || !canRaise || !ready || testPending || status.reference_pulse || status.playback_state !== "idle" || strength <= 0}
          onClick={() => void onTest(channel)}>{t("Test")}</button>
      </div>
    </div>
  );
}

/** The connection line for the panel and the rail trigger. */
export function coyoteStatusText(status: CoyoteStatus | undefined, busy?: string | null) {
  if (busy === "connect" || status?.connecting) return "Connecting to the Coyote";
  if (status?.connected) return "Coyote connected";
  return status?.playback_state === "stale" ? "Coyote connection lost" : "Coyote disconnected";
}

/** A compact strength readout, or an empty string while disconnected. */
export function coyoteStrengthText(status: CoyoteStatus | undefined) {
  if (!status?.connected || !status.strength_known) return "";
  return t("A {a} · B {b}", { a: status.strength_a, b: status.strength_b });
}
