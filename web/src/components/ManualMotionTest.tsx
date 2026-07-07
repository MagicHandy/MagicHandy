// Manual motion, explicitly badged "testing": it drives the device directly to
// check the connection. Normal motion comes from chat and modes.
import { useState } from "react";
import { api } from "../api/client";
import { useAppState, useToast } from "../state/app-state";

export function ManualMotionTest() {
  const { backendOnline, readOnly, motion, refresh } = useAppState();
  const { show } = useToast();
  const locked = !backendOnline || readOnly;
  const running = motion?.engine?.running === true;
  const [pattern, setPattern] = useState("stroke");
  const [speed, setSpeed] = useState(50);

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
          {running ? "Restart test" : "Start test"}
        </button>
        <button type="button" className="btn btn-secondary" onClick={() => void stop()} disabled={!backendOnline}>
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
      <label className="field">
        <span className="label">Speed <output>{speed}%</output></span>
        <input type="range" min={1} max={100} value={speed} onChange={(e) => setSpeed(Number(e.target.value))} disabled={locked} />
      </label>
    </div>
  );
}
