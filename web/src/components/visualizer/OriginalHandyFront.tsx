// Original Handy: the front view drawn from the product. A near-flat cap and a
// base section set off by seams, the waisted front panel with power, cross pad
// and dash keys, and the status LED in its lower lobe. The attachment (sleeve
// and band on a bracket) moves along the travel; depth comes from flat tone
// steps on the surface ladder, never gradients.
import { useId, type CSSProperties } from "react";
import { travelY, type DrawingProps } from "./types";

const BODY = "M72 34C72 16 78 10 104 10C130 10 136 16 136 34V196Q136 212 120 212H88Q72 212 72 196Z";
const PANEL = "M104 42C118 42 125 47 125 58V98C125 108 120 112 120 120C120 128 124 132 124 142C124 154 116 160 104 160C92 160 84 154 84 142C84 132 88 128 88 120C88 112 83 108 83 98V58C83 47 90 42 104 42Z";
// The attachment's band centre travels between these heights (100% at the top).
const TRAVEL_BOTTOM = 158;
const TRAVEL_TOP = 62;

export function OriginalHandyFront(props: DrawingProps) {
  const clipId = useId().replace(/:/g, "");
  const y = (percent: number) => travelY(percent, TRAVEL_BOTTOM, TRAVEL_TOP);
  const carriage = { "--viz-carriage-y": `${y(props.position)}px` } as CSSProperties;
  if (props.mini) {
    return (
      <svg {...props.svgProps} viewBox="0 0 160 220" preserveAspectRatio="xMidYMid meet" aria-hidden="true">
        <path className="viz-body viz-body-mini" d={BODY} />
        <path className="viz-panel" d={PANEL} />
        <circle className="viz-device-led" cx="104" cy="140" r="11" />
        <path className="viz-track" d="M38 62V158" />
        {props.active && <path className="viz-stroke-range" d={`M38 ${y(props.max)}V${y(props.min)}`} />}
        <g className="viz-carriage" style={carriage}>
          <rect className="viz-sleeve" x="16" y="-48" width="44" height="96" rx="14" />
          <rect className="viz-band" x="10" y="-17" width="56" height="34" rx="6" />
        </g>
      </svg>
    );
  }
  return (
    <svg {...props.svgProps} viewBox="0 0 160 220" preserveAspectRatio="xMidYMid meet" aria-hidden="true">
      <path className="viz-track" d={`M9 ${TRAVEL_TOP}V${TRAVEL_BOTTOM}`} />
      {props.active && <path className="viz-stroke-range" d={`M6 ${y(props.max)}h6M9 ${y(props.max)}V${y(props.min)}M6 ${y(props.min)}h6`} />}
      <defs>
        <clipPath id={clipId}><path d={BODY} /></clipPath>
      </defs>
      <path className="viz-body" d={BODY} />
      <g clipPath={`url(#${clipId})`}>
        <path className="viz-tone-cap" d="M60 0H150V36Q104 43 60 36Z" />
        <path className="viz-tone-base" d="M60 168Q104 186 148 168V220H60Z" />
        <path className="viz-seam" d="M60 168Q104 186 148 168" />
        <rect className="viz-tone-shade" x="126" y="0" width="14" height="220" />
        <rect className="viz-tone-light" x="76" y="40" width="4" height="124" rx="2" />
      </g>
      <path className="viz-seam" d="M72 36Q104 43 136 36" />
      <path className="viz-body-outline" d={BODY} />
      <path className="viz-panel" d={PANEL} />
      <path className="viz-panel-edge" d="M90 47C96 44.5 112 44.5 118 47" />
      <g className="viz-keys">
        <circle cx="104" cy="54" r="3.2" className="viz-key-ring" />
        <path d="M104 51.5v2.4" className="viz-key-ring" />
        <rect x="101.5" y="62.5" width="5" height="9" rx="2.5" />
        <rect x="101.5" y="80.5" width="5" height="9" rx="2.5" />
        <rect x="91.5" y="73.5" width="9" height="5" rx="2.5" />
        <rect x="107.5" y="73.5" width="9" height="5" rx="2.5" />
        <rect x="100" y="96" width="8" height="3" rx="1.5" />
      </g>
      <circle className="viz-device-led-ring" cx="104" cy="140" r="5.5" />
      <circle className="viz-device-led" cx="104" cy="140" r="2.4" />
      <g className="viz-carriage" style={carriage}>
        {props.active && <path className="viz-marker" d="M12 0l-5.5-4v8Z" />}
        <rect className="viz-bracket" x="60" y="-15" width="13" height="30" rx="3" />
        <rect className="viz-sleeve" x="24" y="-46" width="34" height="92" rx="14" />
        <path className="viz-sleeve-light" d="M30.5 -36V-20M30.5 20V36" />
        <rect className="viz-band" x="20" y="-15" width="42" height="30" rx="5" />
        <path className="viz-band-edge" d="M25 -11H57" />
      </g>
    </svg>
  );
}
