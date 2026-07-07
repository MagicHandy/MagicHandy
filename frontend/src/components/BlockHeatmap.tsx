import { useEffect, useMemo, useRef } from "react";
import {
  type FunscriptAction,
  type HeatmapStats,
  mapHeatmapStatsFromApi,
  pointsToActions,
  renderFunscriptHeatmap,
  roundActions,
} from "../lib/funscriptHeatmap";

export interface CurvePoint {
  t_ms: number;
  pos: number;
}

type ApiHeatmapStats = {
  action_count?: number;
  duration_ms?: number;
  max_speed?: number;
  avg_speed?: number;
};

type Props = {
  points?: CurvePoint[];
  actions?: FunscriptAction[];
  height?: number;
  className?: string;
  bpm?: number | null;
  showStats?: boolean;
  /** Script completo — heatmap grande com stats estilo EroScripts. */
  isFullScript?: boolean;
  /** Duração do vídeo/eixo X (metadata.duration). */
  scriptDurationMs?: number | null;
  /** Stats pré-calculados pela API. */
  heatmapStats?: ApiHeatmapStats | null;
  /** Posição atual do playhead (ms desde o início). */
  playheadMs?: number | null;
};

export function BlockHeatmap({
  points,
  actions,
  height,
  className = "",
  bpm: _bpm,
  showStats = false,
  isFullScript = false,
  scriptDurationMs = null,
  heatmapStats = null,
  playheadMs = null,
}: Props) {
  const wrapRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);

  const resolvedHeight = height ?? (isFullScript ? 96 : 72);

  const scriptActions = useMemo(() => {
    if (actions && actions.length >= 2) {
      return roundActions(actions);
    }
    if (points && points.length >= 2) {
      return roundActions(pointsToActions(points));
    }
    return [];
  }, [actions, points]);

  const timelineDurationMs = scriptDurationMs ?? undefined;
  const statsOverride = useMemo(
    () => mapHeatmapStatsFromApi(heatmapStats),
    [heatmapStats],
  );

  const usingActions = Boolean(actions && actions.length >= 2);
  const withStats = showStats;

  useEffect(() => {
    const wrap = wrapRef.current;
    const canvas = canvasRef.current;
    if (!wrap || !canvas || scriptActions.length < 2) return;

    const paint = () => {
      const w = Math.max(160, wrap.clientWidth);
      renderFunscriptHeatmap(canvas, scriptActions, w, resolvedHeight, {
        showStats: withStats,
        stripRatio: withStats ? 0.24 : 0.26,
        timelineDurationMs: timelineDurationMs ?? undefined,
        statsOverride,
        skipLeadingHoldLine: !isFullScript,
        playheadMs: playheadMs ?? undefined,
      });
    };

    paint();
    const ro = new ResizeObserver(paint);
    ro.observe(wrap);
    return () => ro.disconnect();
  }, [scriptActions, resolvedHeight, withStats, timelineDurationMs, statsOverride, playheadMs]);

  if (scriptActions.length < 2) {
    return (
      <div
        className={`block-heatmap block-heatmap--empty ${className}`.trim()}
        style={{ height: resolvedHeight }}
      >
        <span className="hint">Sem preview</span>
      </div>
    );
  }

  return (
    <div
      ref={wrapRef}
      className={`block-heatmap ${isFullScript ? "block-heatmap--full" : ""} ${className}`.trim()}
      style={{ height: resolvedHeight }}
      title={
        usingActions
          ? isFullScript
            ? "Heatmap EroScripts — script completo"
            : "Heatmap — mesma timeline enviada ao Handy"
          : "Preview reduzido — reimporte ou reinicie o backend"
      }
    >
      <canvas
        ref={canvasRef}
        className="block-heatmap-canvas"
        aria-label="Heatmap de velocidade do funscript"
      />
    </div>
  );
}

export type { HeatmapStats };
