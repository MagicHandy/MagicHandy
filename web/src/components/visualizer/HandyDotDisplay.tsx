// Handy 2 Standard and Pro: the device's own dot display, enlarged. A dot
// column shows full travel with the stroke window lit, the commanded position
// in large dots, millimetres beneath, and the status LED in the corner.
import { dotText } from "./dotFont";
import type { DrawingProps } from "./types";

const ROWS = 18;
const MINI_ROWS = 10;

interface ColumnDot {
  x: number;
  y: number;
  kind: "off" | "range" | "on";
}

function column(props: DrawingProps, rows: number, left: number, top: number, pitch: number, columns: number): ColumnDot[] {
  const step = 100 / (rows - 1);
  const dots: ColumnDot[] = [];
  for (let row = 0; row < rows; row += 1) {
    const percent = 100 - row * step;
    const kind = !props.active ? "off"
      : Math.abs(percent - props.position) < step ? "on"
        : percent >= props.min - 0.01 && percent <= props.max + 0.01 ? "range" : "off";
    for (let col = 0; col < columns; col += 1) dots.push({ x: left + col * pitch, y: top + row * pitch, kind });
  }
  return dots;
}

function ColumnDots({ dots, radius }: { dots: ColumnDot[]; radius: number }) {
  return <>
    <g className="viz-track">{dots.filter((d) => d.kind === "off").map((d) => <circle key={`${d.x},${d.y}`} cx={d.x} cy={d.y} r={radius} />)}</g>
    <g className="viz-stroke-range">{dots.filter((d) => d.kind === "range").map((d) => <circle key={`${d.x},${d.y}`} cx={d.x} cy={d.y} r={radius} />)}</g>
    <g className="viz-carriage">{dots.filter((d) => d.kind === "on").map((d) => <circle key={`${d.x},${d.y}`} cx={d.x} cy={d.y} r={radius} />)}</g>
  </>;
}

export function HandyDotDisplay(props: DrawingProps) {
  if (props.mini) {
    return (
      <svg {...props.svgProps} viewBox="0 0 30 40" preserveAspectRatio="xMidYMid meet" aria-hidden="true">
        <rect className="viz-body" x="0.5" y="0.5" width="29" height="39" rx="5" />
        <ColumnDots dots={column(props, MINI_ROWS, 9.6, 5.4, 3.3, 3)} radius={1.2} />
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
      <ColumnDots dots={column(props, ROWS, 10, 7, 3.3, 3)} radius={1.15} />
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
