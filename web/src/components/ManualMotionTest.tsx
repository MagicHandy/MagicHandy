import { t, translateKnown } from "../i18n";
// The Handy's Test sends a semantic pattern to the shared motion engine,
// within the saved limits. Normal motion comes from chat and modes.
import { useState } from "react";
import { api } from "../api/client";
import { useAppState, useToast, useMotionState } from "../state/app-state";
import { ownsActiveMotion } from "../util/motion";
import { ValueSlider } from "../devices/fields";

export function ManualMotionTest() {
  const { state, backendOnline, readOnly, refresh } = useAppState();
  const motion = useMotionState();
  const { show } = useToast();
  const limits = state?.settings?.motion;
  const locked = !backendOnline || readOnly || !limits;
  const engine = motion?.engine;
  const manualActive = ownsActiveMotion(engine, "manual_ui");
  const [pattern, setPattern] = useState("flow-full-sweeps");
  const [requestedSpeed, setSpeed] = useState(25);
  const minSpeed = limits?.speed_min_percent ?? 1;
  const maxSpeed = limits?.speed_max_percent ?? 100;
  const speed = Math.max(minSpeed, Math.min(maxSpeed, requestedSpeed));

  async function start() {
    if (locked) return;
    try {
      await api.startManualTest({ pattern, speed_percent: speed });
      show(t("Test motion started."));
    } catch (e) {
      show(e instanceof Error ? translateKnown(e.message) : t("Could not start test."), "error");
    } finally {
      refresh();
    }
  }
  async function stop() {
    try {
      await api.stopMotion();
    } finally {
      refresh();
    }
  }

  return (
    <div className="form-rows device-rows manual-motion-test">
      <p className="hint-block form-row-note">{t("A short pattern within the limits set in the device panel. Stop and Esc end it.")}</p>
      <label className="form-row">
        <span className="form-row-label"><strong>{t("Pattern")}</strong></span>
        <select value={pattern} onChange={(e) => setPattern(e.target.value)} disabled={locked}>
          <option value="flow-full-sweeps">{t("Full sweeps")}</option>
          <option value="flow-base-anchored">{t("Base-anchored variety")}</option>
          <option value="flow-pace-wave">{t("Pace wave")}</option>
        </select>
      </label>
      <div className="form-row">
        <span className="form-row-label"><strong>{t("Speed")}</strong></span>
        <div className="device-option-control">
          <ValueSlider label={t("Speed")} hideLabel value={speed} min={minSpeed} max={maxSpeed} disabled={locked}
            valueText={t("{n}%", { n: speed })} onChange={setSpeed} />
        </div>
      </div>
      <div className="row-actions form-row-note">
        <button type="button" className="btn btn-start" onClick={() => void start()} disabled={locked}>
          {manualActive ? t("Restart test") : t("Start test")}
        </button>
        <button type="button" className="btn btn-secondary" onClick={() => void stop()} disabled={!backendOnline || !manualActive}>{t("Stop test")}</button>
      </div>
    </div>
  );
}
