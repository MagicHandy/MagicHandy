import { t, translateKnown } from "../i18n";
// The Handy's Test: a short pattern that drives the device directly to check
// the connection, within the limits set in the device panel. Normal motion
// comes from chat and modes.
import { useState } from "react";
import { api } from "../api/client";
import { useAppState, useToast , useMotionState } from "../state/app-state";
import { ownsActiveMotion } from "../util/motion";
import { ValueSlider } from "../devices/fields";

export function ManualMotionTest() {
  const { backendOnline, readOnly, refresh } = useAppState();
  const motion = useMotionState();
  const { show } = useToast();
  const locked = !backendOnline || readOnly;
  const engine = motion?.engine;
  const manualActive = ownsActiveMotion(engine, "manual_ui");
  const [pattern, setPattern] = useState("stroke");
  const [speed, setSpeed] = useState(50);

  async function start() {
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
          <option value="stroke">{t("Stroke")}</option>
          <option value="pulse">{t("Pulse")}</option>
          <option value="tease">{t("Tease")}</option>
        </select>
      </label>
      <div className="form-row">
        <span className="form-row-label"><strong>{t("Speed")}</strong></span>
        <div className="device-option-control">
          <ValueSlider label={t("Speed")} hideLabel value={speed} min={1} max={100} disabled={locked}
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
