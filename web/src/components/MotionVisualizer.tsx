// The single authoritative visualizer. It renders engine state only and labels
// position as a commanded estimate — never a guessed or device-confirmed value.
import type { CSSProperties } from "react";
import type { MotionInfo } from "../api/types";
import { clampPercent } from "../util/format";

export function MotionVisualizer({ motion, mini = false }: { motion: MotionInfo | null; mini?: boolean }) {
  const engine = motion?.engine;
  const running = engine?.running === true;
  const paused = engine?.paused === true;
  const pos = clampPercent(engine?.last_sample?.position_percent, 50);
  const firstBound = clampPercent(engine?.settings?.stroke_min_percent, 0);
  const secondBound = clampPercent(engine?.settings?.stroke_max_percent, 100);
  const min = Math.min(firstBound, secondBound);
  const max = Math.max(firstBound, secondBound);
  const hasError = Boolean(engine?.last_error || motion?.error);
  const state = motion?.available === false
    ? "unavailable"
    : hasError ? "error" : paused ? "paused" : engine?.completing ? "completing" : running ? "running" : "idle";
  const stateLabel = state === "completing" ? "Completing" : state.charAt(0).toUpperCase() + state.slice(1);
  const roundedPosition = Math.round(pos);
  const speed = typeof engine?.target?.speed_percent === "number" ? `${Math.round(clampPercent(engine.target.speed_percent, 0))}%` : "--";
  const target = engine?.target?.label?.trim() || (running ? "Engine motion" : "No active motion");
  // The stroking sleeve rides a vertical channel on the body's right edge, the
  // way The Handy 2's sleeve carriage travels. 100% is the top of the channel.
  const travelTop = 30;
  const travelBottom = 104;
  const toRailY = (percent: number) => travelBottom - ((travelBottom - travelTop) * percent) / 100;
  const rangeTop = toRailY(max);
  const rangeBottom = toRailY(min);
  const carriageStyle = { "--viz-carriage-y": `${toRailY(pos)}px` } as CSSProperties;
  const label = `Motion ${state}; commanded position estimate ${roundedPosition} percent; stroke range ${Math.round(min)} to ${Math.round(max)} percent`;

  return (
    <div className={`visualizer${mini ? " mini" : ""}`} data-state={state} role="img" aria-label={label}>
      <svg
        className="viz-device"
        data-position={roundedPosition}
        data-range-min={Math.round(min)}
        data-range-max={Math.round(max)}
        viewBox="0 0 96 132"
        preserveAspectRatio="xMidYMid meet"
        aria-hidden="true"
      >
        {/* Body: the vertical charcoal capsule you hold, styled after The Handy 2. */}
        <rect className="viz-body" x="20" y="10" width="40" height="112" rx="18" />
        <path className="viz-grip" d="M27 34l24-9M27 46l24-9M27 58l24-9" />
        <rect className="viz-screen" x="30" y="66" width="19" height="26" rx="4" />
        <circle className="viz-device-led" cx="39.5" cy="36" r="2.6" />
        {/* Belt channel on the right edge, with the active stroke range inside it. */}
        <rect className="viz-track" x="55" y="24" width="9" height="84" rx="4.5" />
        <rect className="viz-stroke-range" x="55.6" y={rangeTop} width="7.8" height={Math.max(3, rangeBottom - rangeTop)} rx="3.6" />
        {/* Sleeve carriage: the clear ribbed sleeve held by a collar, moving vertically. */}
        <g className="viz-carriage" style={carriageStyle}>
          <rect className="viz-carriage-sleeve" x="62" y="-9" width="14" height="18" rx="6.5" />
          <path className="viz-sleeve-rib" d="M65-3.5h9M65 0h9M65 3.5h9" />
          <rect className="viz-carriage-collar" x="51" y="-7.5" width="16" height="15" rx="5" />
        </g>
      </svg>
      {!mini && (
        <div className="viz-telemetry">
          <div className="viz-primary">
            <span>Commanded position estimate</span>
            <strong>{roundedPosition}%</strong>
          </div>
          <dl className="viz-metrics">
            <div>
              <dt>State</dt>
              <dd><span className="viz-state-dot" aria-hidden="true" />{stateLabel}</dd>
            </div>
            <div>
              <dt>Stroke</dt>
              <dd>{Math.round(min)}-{Math.round(max)}%</dd>
            </div>
            <div>
              <dt>Speed</dt>
              <dd>{speed}</dd>
            </div>
            <div>
              <dt>Target</dt>
              <dd title={target}>{target}</dd>
            </div>
          </dl>
        </div>
      )}
    </div>
  );
}
