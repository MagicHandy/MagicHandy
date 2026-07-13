// Compact status readouts, run timer, mini visualizer, and the shell-level
// connection disclosure. Motion controls remain in their routed workspaces.
import { MotionVisualizer } from "../components/MotionVisualizer";
import { useAppState } from "../state/app-state";
import { formatClock } from "../util/format";
import { ConnectionManager } from "./ConnectionManager";
import { ClockIcon } from "./icons";

export function StatusBar() {
  const { backendOnline, motion, readOnly, state } = useAppState();
  const engine = motion?.engine;

  // Voice earns a readout only when it is enabled and unhealthy: a crashed
  // worker, or speak-replies promised while the TTS worker cannot deliver.
  // Healthy or disabled voice stays out of the bar entirely.
  const voiceSettings = state?.settings?.voice;
  const voiceWorkers = state?.voice?.workers;
  const voiceCrashed = Boolean(voiceSettings?.enabled && (voiceWorkers?.tts?.state === "crashed" || voiceWorkers?.asr?.state === "crashed"));
  const speakNotReady = Boolean(
    voiceSettings?.enabled &&
      voiceSettings.speak_replies &&
      voiceSettings.tts_provider &&
      voiceSettings.tts_provider !== "none" &&
      !(voiceWorkers?.tts?.state === "running" && voiceWorkers?.tts?.model_state === "ready"),
  );
  const phaseState = engine?.paused ? "paused" : engine?.running ? "running" : "idle";
  const phaseLabel = engine?.paused
    ? "paused"
    : engine?.running
      ? engine.target?.label || "running"
      : motion?.available === false ? "unavailable" : "idle";

  return (
    <div className="status-bar" role="region" aria-label="Status">
      <span className="status-readout">
        <span className="status-dot" data-state={phaseState} />
        <span className="status-text">{phaseLabel}</span>
      </span>
      <span className="status-divider" aria-hidden="true" />
      <span className="status-readout">
        <span className="status-dot" data-state={backendOnline ? "ok" : "error"} />
        <span className="status-text">{backendOnline ? "core ok" : "core offline"}</span>
      </span>
      <span
        className="status-readout status-readout-controller"
        title={readOnly ? "Read-only client" : "This tab is the controller"}
        aria-label={readOnly ? "Read-only client" : "This tab is the controller"}
      >
        <span className="status-dot" data-state={readOnly ? "warn" : "ok"} />
        <span className="status-text">{readOnly ? "read-only" : "controller: you"}</span>
      </span>
      {(voiceCrashed || speakNotReady) && (
        <span className="status-readout">
          <span className="status-dot" data-state={voiceCrashed ? "error" : "warn"} />
          <span className="status-text">{voiceCrashed ? "voice crashed" : "voice not ready"}</span>
        </span>
      )}
      <span className="status-divider" aria-hidden="true" />
      <span className="status-timer">
        <ClockIcon />
        <span className="value">{formatClock(engine?.running_ms)}</span>
      </span>
      <span className="status-spacer" />
      <MotionVisualizer motion={motion} mini />
      <ConnectionManager />
    </div>
  );
}
