import { t, translateKnown } from "../i18n";
// Manual motion, explicitly badged "testing": it drives the device directly to
// check the connection. Normal motion comes from chat and modes.
import { useId, useState } from "react";
import { api } from "../api/client";
import { useAppState, useToast } from "../state/app-state";
import { ownsActiveMotion } from "../util/motion";

export function ManualMotionTest() {
  const { backendOnline, readOnly, motion, refresh } = useAppState();
  const { show } = useToast();
  const locked = !backendOnline || readOnly;
  const engine = motion?.engine;
  const manualActive = ownsActiveMotion(engine, "manual_ui");
  const [pattern, setPattern] = useState("stroke");
  const [speed, setSpeed] = useState(50);
  const speedID = useId();

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
    <div className="group">
      <h3 className="group-title">{t("Manual motion")}<span className="badge">{t("Testing")}</span>
      </h3>
      <p className="hint-block">{t("Drives the device directly to test the connection. Normal motion comes from chat and modes.")}</p>
      <div className="row-actions hint-block">
        <button type="button" className="btn btn-start" onClick={() => void start()} disabled={locked}>
          {manualActive ? t("Restart test") : t("Start test")}
        </button>
        <button type="button" className="btn btn-secondary" onClick={() => void stop()} disabled={!backendOnline || !manualActive}>{t("Stop test")}</button>
      </div>
      <label className="field">
        <span className="label">{t("Pattern")}</span>
        <select value={pattern} onChange={(e) => setPattern(e.target.value)} disabled={locked}>
          <option value="stroke">{t("Stroke")}</option>
          <option value="pulse">{t("Pulse")}</option>
          <option value="tease">{t("Tease")}</option>
        </select>
      </label>
      <label className="field" htmlFor={speedID}>
        <span className="label">{t("Speed")}<output htmlFor={speedID}>{speed}%</output></span>
        <input id={speedID} aria-label={t("Speed")} type="range" min={1} max={100} value={speed} onChange={(e) => setSpeed(Number(e.target.value))} disabled={locked} />
      </label>
    </div>
  );
}
