import { t, translateKnown, type MessageKey } from "../i18n";
import { useRef, useState } from "react";
import { api } from "../api/client";
import { PauseIcon, PlayIcon } from "../shell/icons";
import { useAppState, useToast } from "../state/app-state";
import { ownsActiveMotion } from "../util/motion";

const decisionSourceCopy: Partial<Record<string, MessageKey>> = {
  model: "Assistant selected",
  fallback: "Planner fallback",
  hold: "Continuing current pattern",
};

const errorMessage = (error: unknown) =>
  error instanceof Error ? translateKnown(error.message) : t("Autopilot request failed.");

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
        show(t("Autopilot stopped."));
      } else {
        await api.startMode("autopilot");
        show(t("Autopilot started."));
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
        show(t("Motion resumed."));
      } else {
        await api.pauseMotion();
        show(t("Motion paused."));
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
  let status = t("Off");
  if (pending) {
    status = { start: t("Starting"), stop: t("Stopping"), pause: t("Pausing"), resume: t("Resuming") }[pending];
  } else if (active && autopilotPaused) {
    status = t("Paused");
  } else if (active && segment === 0) {
    status = t("Choosing first segment");
  } else if (active) {
    const source = modes?.decision_source
      ? decisionSourceCopy[modes.decision_source] ?? modes.decision_source
      : t("Active");
    status = t("Segment {segment} · {source}", { segment, source: translateKnown(source) });
  }

  return (
    <div className="autopilot-control" data-active={active || undefined} aria-busy={Boolean(pending) || undefined}>
      <span className="autopilot-control-dot" aria-hidden="true" />
      <div className="autopilot-control-copy">
        <strong>{t("Autopilot")}</strong>
        <span role="status">{status}</span>
      </div>
      <div className="autopilot-control-actions">
        {active && (
          <button
            type="button"
            className="icon-button"
            aria-label={autopilotPaused ? t("Resume Autopilot") : t("Pause Autopilot")}
            title={!canPause ? t("Autopilot motion has not started") : autopilotPaused ? t("Resume Autopilot") : t("Pause Autopilot")}
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
          {pending === "start" ? t("Starting…") : pending === "stop" ? t("Stopping…") : active ? t("Stop Autopilot") : t("Start Autopilot")}
        </button>
      </div>
    </div>
  );
}
