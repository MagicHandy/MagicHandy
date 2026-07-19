import { useRef, useState } from "react";
import { api } from "../api/client";
import { PauseIcon, PlayIcon } from "../shell/icons";
import { useAppState, useToast } from "../state/app-state";

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
  const canPause = Boolean(motion?.engine?.running || motion?.engine?.paused);
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
    if (pendingRef.current || locked || !active) return;
    const paused = motion?.engine?.paused === true;
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
  } else if (active && motion?.engine?.paused) {
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
    <div className="chat-autopilot" data-active={active || undefined} aria-busy={Boolean(pending) || undefined}>
      <span className="chat-autopilot-dot" aria-hidden="true" />
      <div className="chat-autopilot-copy">
        <strong>Autopilot</strong>
        <span role="status">{status}</span>
      </div>
      <div className="chat-autopilot-actions">
        {active && (
          <button
            type="button"
            className="icon-button"
            aria-label={motion?.engine?.paused ? "Resume Autopilot" : "Pause Autopilot"}
            title={!canPause ? "Motion has not started" : motion?.engine?.paused ? "Resume Autopilot" : "Pause Autopilot"}
            disabled={locked || Boolean(pending) || !canPause}
            onClick={() => void togglePause()}
          >
            {motion?.engine?.paused ? <PlayIcon /> : <PauseIcon />}
          </button>
        )}
        <button
          type="button"
          className={`btn ${active ? "btn-secondary" : "btn-start"} chat-autopilot-action`}
          disabled={locked || Boolean(pending)}
          onClick={() => void toggle()}
        >
          {pending === "start" ? "Starting..." : pending === "stop" ? "Stopping..." : active ? "Stop Autopilot" : "Start Autopilot"}
        </button>
      </div>
    </div>
  );
}
