// Status-only top bar: compact dot+text readouts, run timer, and the mini
// visualizer. No controls live here (docs/ui-design-guidelines.md).
import { MotionVisualizer } from "../components/MotionVisualizer";
import { useAppState } from "../state/app-state";
import { formatClock } from "../util/format";
import { ClockIcon } from "./icons";

export function StatusBar() {
  const { backendOnline, motion, readOnly } = useAppState();
  const engine = motion?.engine;
  const phaseState = engine?.paused ? "paused" : engine?.running ? "running" : "idle";
  const phaseLabel = engine?.paused
    ? "paused"
    : engine?.running
      ? engine.phase && engine.phase !== "" ? engine.phase : "running"
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
      <span className="status-readout status-readout-controller">
        <span className="status-dot" data-state={readOnly ? "warn" : "ok"} />
        <span className="status-text">{readOnly ? "read-only" : "controller: you"}</span>
      </span>
      <span className="status-divider" aria-hidden="true" />
      <span className="status-timer">
        <ClockIcon />
        <span className="value">{formatClock(engine?.running_ms)}</span>
      </span>
      <span className="status-spacer" />
      <MotionVisualizer motion={motion} mini />
    </div>
  );
}
