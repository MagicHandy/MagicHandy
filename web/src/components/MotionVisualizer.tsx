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
  const travelTop = 12;
  const travelBottom = 78;
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
        viewBox="0 0 132 96"
        preserveAspectRatio="xMidYMid meet"
        aria-hidden="true"
      >
        <path className="viz-floor" d="M9 87.5h116" />
        <path className="viz-device-base" d="M14 84V70c0-7.2 5.8-13 13-13h38c7.2 0 13 5.8 13 13v14Z" />
        <rect className="viz-device-tower" x="57" y="20" width="27" height="64" rx="13.5" />
        <rect className="viz-device-bridge" x="75" y="25" width="26" height="10" rx="5" />
        <rect className="viz-device-rail" x="96" y="8" width="8" height="76" rx="4" />
        <path className="viz-rail-ticks" d="M90 12h4M90 45h4M90 78h4" />
        <rect className="viz-stroke-range" x="94" y={rangeTop} width="12" height={Math.max(2, rangeBottom - rangeTop)} rx="6" />
        <path className="viz-rail-line" d="M100 11v70" />
        <g className="viz-carriage" style={carriageStyle}>
          <rect className="viz-carriage-body" x="87" y="-5" width="26" height="10" rx="3" />
          <path className="viz-carriage-arm" d="M110-2h15v4h-15Z" />
          <rect className="viz-carriage-coupler" x="123" y="-4" width="6" height="8" rx="2" />
        </g>
        <circle className="viz-device-led" cx="69.5" cy="72" r="3" />
        <path className="viz-device-seam" d="M25 66h27M25 72h23" />
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
