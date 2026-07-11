import type { CurvePoint } from "../api/types";

interface Props {
  points?: CurvePoint[];
  label: string;
  className?: string;
  showKnots?: boolean;
}

const WIDTH = 240;
const HEIGHT = 72;
const PAD = 5;

export function PatternCurve({ points, label, className = "", showKnots = false }: Props) {
  const samples = points ?? [];
  const duration = Math.max(1, samples[samples.length - 1]?.time_ms ?? 1);
  const projected = samples.map((point) => ({
    x: PAD + (point.time_ms / duration) * (WIDTH - PAD * 2),
    y: PAD + ((100 - point.position_percent) / 100) * (HEIGHT - PAD * 2),
  }));
  const path = projected.map((point, index) => `${index === 0 ? "M" : "L"}${point.x.toFixed(2)} ${point.y.toFixed(2)}`).join(" ");

  return (
    <svg className={`pattern-curve ${className}`} viewBox={`0 0 ${WIDTH} ${HEIGHT}`} role="img" aria-label={label} preserveAspectRatio="none">
      <line x1={PAD} y1={HEIGHT / 2} x2={WIDTH - PAD} y2={HEIGHT / 2} className="pattern-grid-line" />
      {path && <path d={path} className="pattern-curve-line" />}
      {showKnots && projected.map((point, index) => <circle key={index} cx={point.x} cy={point.y} r="2.5" className="pattern-knot" />)}
    </svg>
  );
}
