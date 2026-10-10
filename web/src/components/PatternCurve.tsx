import type { ReactNode } from "react";
import type { CurvePoint } from "../api/types";
import { formatNumber, t } from "../i18n";

interface Props {
  points?: CurvePoint[];
  knots?: CurvePoint[];
  label: string;
  className?: string;
  showKnots?: boolean;
  /** Expanded plots draw edge to edge and carry their scales (PatternPlotAxes). */
  axes?: boolean;
}

const WIDTH = 240;
const HEIGHT = 72;
const PAD = 5;

export function PatternCurve({ points, knots, label, className = "", showKnots = false, axes = false }: Props) {
  const samples = normalizedCurvePoints(points, knots);
  const duration = Math.max(1, ...samples.map((point) => point.time_ms));
  const pad = axes ? 0 : PAD;
  const projected = samples.map((point) => ({
    x: pad + (point.time_ms / duration) * (WIDTH - pad * 2),
    y: pad + ((100 - point.position_percent) / 100) * (HEIGHT - pad * 2),
  }));
  const path = projected.map((point, index) => `${index === 0 ? "M" : "L"}${point.x.toFixed(2)} ${point.y.toFixed(2)}`).join(" ");

  const curve = (
    <svg className={`pattern-curve ${className}`} viewBox={`0 0 ${WIDTH} ${HEIGHT}`} role="img" aria-label={label} preserveAspectRatio="none">
      <line x1={pad} y1={HEIGHT / 2} x2={WIDTH - pad} y2={HEIGHT / 2} className="pattern-grid-line" />
      {path && <path d={path} className="pattern-curve-line" />}
      {showKnots && projected.map((point, index) => <circle key={index} cx={point.x} cy={point.y} r="2.5" className="pattern-knot" />)}
    </svg>
  );
  return axes && samples.length > 1 ? <PatternPlotAxes durationMs={duration}>{curve}</PatternPlotAxes> : curve;
}

/** Top, middle and bottom labels of a plot's value scale. */
export type PlotScale = [ReactNode, ReactNode, ReactNode];

// The scales of an expanded plot, as readable HTML text beside and under it
// (never scaled with the drawing): position from Base 0 to Tip 100 unless
// other `values` are given, and the time domain the plot actually draws. The
// plot spans the box edge to edge, so the top and bottom values sit on its
// top and bottom edges.
export function PatternPlotAxes({ durationMs, values, children }: { durationMs: number; values?: PlotScale; children: ReactNode }) {
  const seconds = (millis: number) => t("{seconds} s", { seconds: formatNumber(Math.round(millis / 100) / 10) });
  const [top, middle, bottom] = values ?? [
    <>{t("Tip")} <span className="pattern-plot-value">100</span></>,
    <span className="pattern-plot-value">50</span>,
    <>{t("Base")} <span className="pattern-plot-value">0</span></>,
  ];
  return (
    <div className="pattern-plot">
      <div className="pattern-plot-y" aria-hidden="true">
        <span>{top}</span>
        <span>{middle}</span>
        <span>{bottom}</span>
      </div>
      {children}
      <div className="pattern-plot-x" aria-hidden="true">
        <span>{seconds(0)}</span>
        <span>{seconds(durationMs / 2)}</span>
        <span>{seconds(durationMs)}</span>
      </div>
    </div>
  );
}

function normalizedCurvePoints(samples?: CurvePoint[], knots?: CurvePoint[]): CurvePoint[] {
  const byTime = new Map<number, number>();
  for (const source of [samples, knots]) {
    for (const point of source ?? []) {
      if (!Number.isFinite(point.time_ms) || !Number.isFinite(point.position_percent)) continue;
      byTime.set(
        Math.max(0, point.time_ms),
        Math.min(100, Math.max(0, point.position_percent)),
      );
    }
  }
  return Array.from(byTime, ([time_ms, position_percent]) => ({ time_ms, position_percent }))
    .sort((left, right) => left.time_ms - right.time_ms);
}
