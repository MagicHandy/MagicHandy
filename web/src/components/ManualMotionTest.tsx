// Manual motion, explicitly badged "testing": it drives the device directly to
// check the connection. Normal motion comes from chat and modes.
import { useId, useState } from "react";
import { api } from "../api/client";
import { useAppState, useToast } from "../state/app-state";

export function ManualMotionTest() {
  const { backendOnline, readOnly, motion, refresh } = useAppState();
  const { show } = useToast();
  const locked = !backendOnline || readOnly;
  const engine = motion?.engine;
  const manualActive = engine?.target?.source === "manual_ui" && Boolean(
    engine.running || engine.starting || engine.paused || engine.completing,
  );
  const [pattern, setPattern] = useState("stroke");
  const [speed, setSpeed] = useState(50);
  const speedID = useId();

  async function start() {
    try {
      await api.startManualTest({ pattern, speed_percent: speed });
      show("Test motion started.");
    } catch (e) {
      show(e instanceof Error ? e.message : "Could not start test.", "error");
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
      <h3 className="group-title">
        Manual motion <span className="badge">testing</span>
      </h3>
      <p className="hint-block">
        Drives the device directly to test the connection. Normal motion comes from chat and modes.
      </p>
      <div className="row-actions hint-block">
        <button type="button" className="btn btn-start" onClick={() => void start()} disabled={locked}>
          {manualActive ? "Restart test" : "Start test"}
        </button>
        <button type="button" className="btn btn-secondary" onClick={() => void stop()} disabled={!backendOnline || !manualActive}>
          Stop test
        </button>
      </div>
      <label className="field">
        <span className="label">Pattern</span>
        <select value={pattern} onChange={(e) => setPattern(e.target.value)} disabled={locked}>
          <option value="stroke">Stroke</option>
          <option value="pulse">Pulse</option>
          <option value="tease">Tease</option>
        </select>
      </label>
      <label className="field" htmlFor={speedID}>
        <span className="label">Speed <output htmlFor={speedID}>{speed}%</output></span>
        <input id={speedID} aria-label="Speed" type="range" min={1} max={100} value={speed} onChange={(e) => setSpeed(Number(e.target.value))} disabled={locked} />
      </label>
    </div>
  );
}
