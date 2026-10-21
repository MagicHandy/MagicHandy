// Intiface: any linear stroker, so no device drawing. A track with the stroke
// window, a lit bar at the commanded position (top is the tip), and a fading
// trail behind it that carries direction and speed.
//
// The trail is the bar itself, drawn again at falling opacity, each copy
// following the same position on a slightly longer transition. Each lags the
// one before by a fraction of the bar's height even at full speed, so the
// copies always overlap into one continuous tail that stretches when the bar
// moves fast and folds back under it when the bar stops.
import type { CSSProperties } from "react";
import { travelY, type DrawingProps } from "./types";

// Trail copies, nearest the bar first: how far each lags, and its opacity.
const TRAIL = [
  { seconds: 0.165, opacity: 0.36 },
  { seconds: 0.2, opacity: 0.27 },
  { seconds: 0.24, opacity: 0.2 },
  { seconds: 0.28, opacity: 0.14 },
  { seconds: 0.32, opacity: 0.09 },
  { seconds: 0.36, opacity: 0.06 },
] as const;
// Bar centre limits inside the 6–194 track, so the 16-unit bar stays inside.
const BAR_BOTTOM = 185;
const BAR_TOP = 15;

function Bar({ y, x, width, height, radius, trail }: { y: number; x: number; width: number; height: number; radius: number; trail?: (typeof TRAIL)[number] }) {
  const style = { "--viz-carriage-y": `${y}px`, ...(trail && { transitionDuration: `${trail.seconds}s`, opacity: trail.opacity }) } as CSSProperties;
  return <g className={trail ? "viz-carriage viz-scan-trail" : "viz-carriage"} style={style}>
    <rect className={trail ? undefined : "viz-scan-bar"} x={x} y={-height / 2} width={width} height={height} rx={radius} />
  </g>;
}

export function ScanBar(props: DrawingProps) {
  if (props.mini) {
    // Status-bar form: its own geometry, so the track stays legible at 28px tall.
    const my = (percent: number) => travelY(percent, 33, 7);
    return (
      <svg {...props.svgProps} viewBox="0 0 20 40" preserveAspectRatio="xMidYMid meet" aria-hidden="true">
        <rect className="viz-track viz-body" x="5" y="1" width="10" height="38" rx="3" />
        {props.active && <rect className="viz-stroke-range" x="6" y={my(props.max) - 3} width="8" height={Math.max(2, my(props.min) - my(props.max) + 6)} rx="2" />}
        {props.active && TRAIL.map((trail) => <Bar key={trail.seconds} trail={trail} y={my(props.position)} x={6} width={8} height={6} radius={1.5} />)}
        <Bar y={my(props.position)} x={6} width={8} height={props.active ? 6 : 0} radius={1.5} />
      </svg>
    );
  }
  const y = (percent: number) => travelY(percent, BAR_BOTTOM, BAR_TOP);
  const rangeTop = y(props.max) - 9;
  const rangeBottom = y(props.min) + 9;
  return (
    <svg {...props.svgProps} viewBox="0 0 40 200" preserveAspectRatio="xMidYMid meet" aria-hidden="true">
      <rect className="viz-track viz-body" x="10" y="6" width="20" height="188" rx="6" />
      {props.active && <>
        <rect className="viz-stroke-range" x="12" y={rangeTop} width="16" height={Math.max(4, rangeBottom - rangeTop)} rx="3" />
        <path className="viz-range-ticks" d={`M5 ${rangeTop}h4M5 ${rangeBottom}h4`} />
        {TRAIL.map((trail) => <Bar key={trail.seconds} trail={trail} y={y(props.position)} x={12} width={16} height={16} radius={3} />)}
      </>}
      <Bar y={y(props.position)} x={12} width={16} height={props.active ? 16 : 0} radius={3} />
    </svg>
  );
}
