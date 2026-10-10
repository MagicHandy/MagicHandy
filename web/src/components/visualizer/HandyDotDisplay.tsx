// Handy 2 Standard and Pro: the device's own dot display, enlarged. A dot
// column shows full travel with the stroke window lit and a gliding lit row at
// the commanded position, the position in large dots, millimetres beneath,
// and the status LED in the corner.
import type { CSSProperties } from "react";
import { dotText } from "./dotFont";
import type { DrawingProps } from "./types";

const ROWS = 18;
const MINI_ROWS = 10;

interface ColumnDot {
  x: number;
  y: number;
  kind: "off" | "range";
}

interface Column {
  rows: number;
  left: number;
  top: number;
  pitch: number;
  columns: number;
}

// The static column: full travel, with the stroke window lit. It changes only
// when the window does, never with the position.
function columnDots(props: DrawingProps, { rows, left, top, pitch, columns }: Column): ColumnDot[] {
  const step = 100 / (rows - 1);
  const dots: ColumnDot[] = [];
  for (let row = 0; row < rows; row += 1) {
    const percent = 100 - row * step;
    const kind = props.active && percent >= props.min - 0.01 && percent <= props.max + 0.01 ? "range" : "off";
    for (let col = 0; col < columns; col += 1) dots.push({ x: left + col * pitch, y: top + row * pitch, kind });
  }
  return dots;
}

// The position is one lit row that glides over the column on a CSS transform,
// like the original Handy's carriage, so it moves smoothly between engine
// samples instead of jumping a whole row at a time. It lines up with the
// column's dots at each row.
function DotColumn({ props, column, radius }: { props: DrawingProps; column: Column; radius: number }) {
  const { rows, left, top, pitch, columns } = column;
  const y = top + ((100 - props.position) / 100) * (rows - 1) * pitch;
  const carriage = { "--viz-carriage-y": `${y}px` } as CSSProperties;
  const dots = columnDots(props, column);
  return <>
    <g className="viz-track">{dots.map((d) => d.kind === "off" && <circle key={`${d.x},${d.y}`} cx={d.x} cy={d.y} r={radius} />)}</g>
    <g className="viz-stroke-range">{dots.map((d) => d.kind === "range" && <circle key={`${d.x},${d.y}`} cx={d.x} cy={d.y} r={radius} />)}</g>
    <g className="viz-carriage" style={carriage}>
      {props.active && Array.from({ length: columns }, (_, col) => <circle key={col} cx={left + col * pitch} cy={0} r={radius} />)}
    </g>
  </>;
}

export function HandyDotDisplay(props: DrawingProps) {
  if (props.mini) {
    return (
      <svg {...props.svgProps} viewBox="0 0 30 40" preserveAspectRatio="xMidYMid meet" aria-hidden="true">
        <rect className="viz-body" x="0.5" y="0.5" width="29" height="39" rx="5" />
        <DotColumn props={props} column={{ rows: MINI_ROWS, left: 9.6, top: 5.4, pitch: 3.3, columns: 3 }} radius={1.2} />
        <circle className="viz-device-led" cx="24" cy="6" r="2" />
      </svg>
    );
  }
  const digits = props.active ? String(Math.round(props.position)) : "--";
  const millimetres = props.active ? String(Math.round((props.position / 100) * props.travelMillimetres)) : "";
  const big = dotText(digits, 80, 9.5, digits.length > 2 ? 3.6 : 4.4);
  const small = dotText(millimetres, 80, 46.5, 2.4);
  const bigRadius = digits.length > 2 ? 1.35 : 1.6;
  return (
    <svg {...props.svgProps} viewBox="0 0 160 70" preserveAspectRatio="xMidYMid meet" aria-hidden="true">
      <rect className="viz-body" x="0.5" y="0.5" width="159" height="69" rx="8" />
      <DotColumn props={props} column={{ rows: ROWS, left: 10, top: 7, pitch: 3.3, columns: 3 }} radius={1.15} />
      <g className="viz-digits">
        {big.map((d) => <circle key={`${d.x},${d.y}`} className={d.lit ? "viz-digit-on" : "viz-digit-off"} cx={d.x} cy={d.y} r={bigRadius} />)}
      </g>
      <g className="viz-digits-small">
        {small.filter((d) => d.lit).map((d) => <circle key={`${d.x},${d.y}`} cx={d.x} cy={d.y} r="0.85" />)}
      </g>
      <circle className="viz-device-led" cx="149" cy="10" r="2.6" />
    </svg>
  );
}
