// Intiface ribbon timeline: the positions shown during the last eight seconds,
// newest at the right edge, across the stroke window. Published but not
// enabled: the scanning bar (ScanBar) is the Intiface visualizer. See
// DISABLED_VISUALIZER_KINDS.
import { travelY, type DrawingProps } from "./types";
import { usePositionHistory } from "./usePositionHistory";

const WINDOW_MS = 8000;
const WIDTH = 262;
const TOP = 8;
const BOTTOM = 84;

export function RibbonTrace(props: DrawingProps) {
  const history = usePositionHistory(props.position, props.active, WINDOW_MS);
  const y = (percent: number) => travelY(percent, BOTTOM, TOP);
  const newest = history.length ? history[history.length - 1].at : 0;
  const points = history.map((point) => `${(WIDTH - ((newest - point.at) / WINDOW_MS) * WIDTH).toFixed(1)},${y(point.position).toFixed(1)}`);
  const nowY = y(props.position);
  return (
    <svg {...props.svgProps} viewBox={props.mini ? "0 0 262 92" : "0 0 300 92"} preserveAspectRatio="xMidYMid meet" aria-hidden="true">
      <rect className="viz-track viz-body" x="0" y="4" width={WIDTH} height="84" rx="4" />
      {props.active && <>
        <path className="viz-stroke-range viz-range-lines" d={`M0 ${y(props.max)}H${WIDTH}M0 ${y(props.min)}H${WIDTH}`} />
        {points.length > 1 && <polygon className="viz-ribbon-fill" points={`${points[0].split(",")[0]},88 ${points.join(" ")} ${WIDTH},88`} />}
        {points.length > 1 && <polyline className="viz-ribbon-line" points={points.join(" ")} />}
        <g className="viz-carriage"><circle className="viz-ribbon-now" cx={WIDTH} cy={nowY} r="3" /></g>
      </>}
    </svg>
  );
}
