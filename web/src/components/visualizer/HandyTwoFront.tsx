// Handy 2 front view: the body with its control panel and dot display, and the
// attachment beside it riding the side slot. Published but not enabled: the dot
// display (HandyDotDisplay) is the Handy 2's visualizer. See DISABLED_VISUALIZER_KINDS.
import type { CSSProperties } from "react";
import { dotText } from "./dotFont";
import { travelY, type DrawingProps } from "./types";

const TRAVEL_BOTTOM = 158;
const TRAVEL_TOP = 62;

export function HandyTwoFront(props: DrawingProps) {
  const y = (percent: number) => travelY(percent, TRAVEL_BOTTOM, TRAVEL_TOP);
  const carriage = { "--viz-carriage-y": `${y(props.position)}px` } as CSSProperties;
  const digits = dotText(props.active ? String(Math.round(props.position)) : "--", 104, 138, 2.9);
  return (
    <svg {...props.svgProps} viewBox="0 0 160 220" preserveAspectRatio="xMidYMid meet" aria-hidden="true">
      {!props.mini && <path className="viz-track" d={`M9 ${TRAVEL_TOP}V${TRAVEL_BOTTOM}`} />}
      {!props.mini && props.active && <path className="viz-stroke-range" d={`M6 ${y(props.max)}h6M9 ${y(props.max)}V${y(props.min)}M6 ${y(props.min)}h6`} />}
      <rect className="viz-body" x="64" y="10" width="76" height="200" rx="30" />
      <rect className="viz-slot" x="68" y="30" width="3" height="160" rx="1.5" />
      <rect className="viz-panel" x="84" y="54" width="40" height="132" rx="12" />
      {!props.mini && <g className="viz-keys">
        <rect x="101" y="63" width="6" height="9" rx="3" />
        <rect x="101" y="84" width="6" height="9" rx="3" />
        <rect x="89" y="75" width="9" height="6" rx="3" />
        <rect x="110" y="75" width="9" height="6" rx="3" />
      </g>}
      <circle className="viz-device-led" cx="104" cy="78" r={props.mini ? 9 : 2.8} />
      {!props.mini && <g className="viz-digits">
        {digits.map((d) => <circle key={`${d.x},${d.y}`} className={d.lit ? "viz-digit-on" : "viz-digit-off"} cx={d.x} cy={d.y} r="1.05" />)}
      </g>}
      <g className="viz-carriage" style={carriage}>
        {!props.mini && props.active && <path className="viz-marker" d="M11 0l-5-3.5v7Z" />}
        <rect className="viz-sleeve" x="22" y="-44" width="34" height="88" rx="12" />
        <rect className="viz-band" x="18" y="-13" width="42" height="26" rx="6" />
        <rect className="viz-bracket" x="60" y="-5" width="10" height="10" rx="2" />
      </g>
    </svg>
  );
}
