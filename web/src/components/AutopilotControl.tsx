import { useRef, useState } from "react";
import { api } from "../api/client";
import { PauseIcon, PlayIcon } from "../shell/icons";
import { useAppState, useToast } from "../state/app-state";
import { ownsActiveMotion } from "../util/motion";

const decisionSourceCopy: Record<string, string> = {
  model: "Assistant selected",
  fallback: "Planner fallback",
  hold: "Continuing current pattern",
};

const errorMessage = (error: unknown) =>
  error instanceof Error ? error.message : "Autopilot request failed.";

export function AutopilotControl() {
  const { state, backendOnline, readOnly, motion, refresh } = useAppState();
  const { show } = useToast();
  const modes = state?.modes;
  const active = modes?.mode === "autopilot" || modes?.active_mode === "autopilot";
  const engine = motion?.engine;
  const autopilotMotionActive = ownsActiveMotion(engine, "autopilot");
  const autopilotPaused = autopilotMotionActive && engine?.paused === true;
  const canPause = active && autopilotMotionActive && Boolean(engine?.running || engine?.paused);
  const locked = !backendOnline || !state || readOnly;
  const [pending, setPending] = useState<"start" | "stop" | "pause" | "resume" | "">("");
  const pendingRef = useRef(false);

  async function toggle() {
    if (pendingRef.current || locked) return;
    pendingRef.current = true;
    setPending(active ? "stop" : "start");
    try {
      if (active) {
        await api.stopMode();
        show("Autopilot stopped.");
      } else {
        await api.startMode("autopilot");
        show("Autopilot started.");
      }
    } catch (error) {
      show(errorMessage(error), "error");
    } finally {
      pendingRef.current = false;
      setPending("");
      refresh();
    }
  }

  async function togglePause() {
    if (pendingRef.current || locked || !canPause) return;
    const paused = autopilotPaused;
    pendingRef.current = true;
    setPending(paused ? "resume" : "pause");
    try {
      if (paused) {
        await api.resumeMotion();
        show("Motion resumed.");
      } else {
        await api.pauseMotion();
        show("Motion paused.");
      }
    } catch (error) {
      show(errorMessage(error), "error");
    } finally {
      pendingRef.current = false;
      setPending("");
      refresh();
    }
  }

  const segment = modes?.segment_index ?? 0;
  let status = "Off";
  if (pending) {
    status = { start: "Starting", stop: "Stopping", pause: "Pausing", resume: "Resuming" }[pending];
  } else if (active && autopilotPaused) {
    status = "Paused";
  } else if (active && segment === 0) {
    status = "Choosing first segment";
  } else if (active) {
    const source = modes?.decision_source
      ? decisionSourceCopy[modes.decision_source] ?? modes.decision_source
      : "Active";
    status = `Segment ${segment} · ${source}`;
  }

  return (
    <div className="autopilot-control" data-active={active || undefined} aria-busy={Boolean(pending) || undefined}>
      <span className="autopilot-control-dot" aria-hidden="true" />
      <div className="autopilot-control-copy">
        <strong>Autopilot</strong>
        <span role="status">{status}</span>
      </div>
      <div className="autopilot-control-actions">
        {active && (
          <button
            type="button"
            className="icon-button"
            aria-label={autopilotPaused ? "Resume Autopilot" : "Pause Autopilot"}
            title={!canPause ? "Autopilot motion has not started" : autopilotPaused ? "Resume Autopilot" : "Pause Autopilot"}
            disabled={locked || Boolean(pending) || !canPause}
            onClick={() => void togglePause()}
          >
            {autopilotPaused ? <PlayIcon /> : <PauseIcon />}
          </button>
        )}
        <button
          type="button"
          className={`btn ${active ? "btn-secondary" : "btn-start"} autopilot-control-action`}
          disabled={locked || Boolean(pending)}
          onClick={() => void toggle()}
        >
          {pending === "start" ? "Starting..." : pending === "stop" ? "Stopping..." : active ? "Stop Autopilot" : "Start Autopilot"}
        </button>
      </div>
    </div>
  );
}
