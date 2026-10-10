// Intiface: any linear stroker, so no device drawing. A track with the stroke
// window, a lit bar at the commanded position (top is the tip), and a short
// fading trail of the positions just shown, which carries direction and speed.
import type { CSSProperties } from "react";
import { travelY, type DrawingProps } from "./types";
import { usePositionHistory } from "./usePositionHistory";

const TRAIL_MS = 400;
const TRAIL_LENGTH = 3;
// Bar centre limits inside the 6–194 track, so the 16-unit bar stays inside.
const BAR_BOTTOM = 185;
const BAR_TOP = 15;

export function ScanBar(props: DrawingProps) {
  const history = usePositionHistory(props.position, props.active, TRAIL_MS);
  const y = (percent: number) => travelY(percent, BAR_BOTTOM, BAR_TOP);
  const trail = history.slice(0, -1).slice(-TRAIL_LENGTH);
  const bar = { "--viz-carriage-y": `${y(props.position)}px` } as CSSProperties;
  if (props.mini) {
    // Status-bar form: its own geometry, so the track stays legible at 28px tall.
    const my = (percent: number) => travelY(percent, 33, 7);
    const miniBar = { "--viz-carriage-y": `${my(props.position)}px` } as CSSProperties;
    return (
      <svg {...props.svgProps} viewBox="0 0 20 40" preserveAspectRatio="xMidYMid meet" aria-hidden="true">
        <rect className="viz-track viz-body" x="5" y="1" width="10" height="38" rx="3" />
        {props.active && <>
          <rect className="viz-stroke-range" x="6" y={my(props.max) - 3} width="8" height={Math.max(2, my(props.min) - my(props.max) + 6)} rx="2" />
          <g className="viz-carriage" style={miniBar}>
            <rect className="viz-scan-bar" x="6" y="-3" width="8" height="6" rx="1.5" />
          </g>
        </>}
      </svg>
    );
  }
  const rangeTop = y(props.max) - 9;
  const rangeBottom = y(props.min) + 9;
  return (
    <svg {...props.svgProps} viewBox="0 0 40 200" preserveAspectRatio="xMidYMid meet" aria-hidden="true">
      <rect className="viz-track viz-body" x="10" y="6" width="20" height="188" rx="6" />
      {props.active && <>
        <rect className="viz-stroke-range" x="12" y={rangeTop} width="16" height={Math.max(4, rangeBottom - rangeTop)} rx="3" />
        {!props.mini && <path className="viz-range-ticks" d={`M5 ${rangeTop}h4M5 ${rangeBottom}h4`} />}
        {trail.map((point, index) => (
          <rect key={point.at} className="viz-scan-trail" x="12" y={y(point.position) - 8} width="16" height="16" rx="3"
            style={{ opacity: 0.12 + (0.3 * (index + 1)) / (trail.length + 1) }} />
        ))}
        <g className="viz-carriage" style={bar}>
          <rect className="viz-scan-bar" x="12" y="-8" width="16" height="16" rx="3" />
        </g>
      </>}
    </svg>
  );
}
