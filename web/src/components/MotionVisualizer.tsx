// The single authoritative visualizer. It renders engine state only and labels
// position as a commanded estimate — never a guessed or device-confirmed value.
import type { MotionInfo } from "../api/types";
import { clampPercent } from "../util/format";

export function MotionVisualizer({ motion, mini = false }: { motion: MotionInfo | null; mini?: boolean }) {
  const engine = motion?.engine;
  const running = engine?.running === true;
  const paused = engine?.paused === true;
  const pos = clampPercent(engine?.last_sample?.position_percent, 50);
  const min = clampPercent(engine?.settings?.stroke_min_percent, 0);
  const max = clampPercent(engine?.settings?.stroke_max_percent, 100);
  const label = paused ? "paused" : running ? "running" : motion?.available === false ? "unavailable" : "idle";

  return (
    <div className={`visualizer${mini ? " mini" : ""}`} role="img" aria-label={`Motion ${label}`}>
      <span className="viz-track" aria-hidden="true">
        <span className="viz-range" style={{ left: `${min}%`, width: `${Math.max(0, max - min)}%` }} />
        <span className="viz-pos" data-active={running} style={{ left: `${pos}%` }} />
      </span>
      {!mini && <span className="viz-caption">Commanded position estimate</span>}
    </div>
  );
}
