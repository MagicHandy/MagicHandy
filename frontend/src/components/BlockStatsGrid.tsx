import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import {
  computeEroScriptsStats,
  mapHeatmapStatsFromApi,
  pointsToActions,
  type FunscriptAction,
} from "../lib/funscriptHeatmap";

type Props = {
  intensity?: number | null;
  actions?: FunscriptAction[];
  preview?: { t_ms: number; pos: number }[];
  heatmapStats?: {
    action_count?: number;
    duration_ms?: number;
    max_speed?: number;
    avg_speed?: number;
  } | null;
  durationMs?: number;
  scriptDurationMs?: number | null;
  actionCount?: number | null;
  isFullScript?: boolean;
};

function formatBlockDuration(ms: number): string {
  if (ms <= 0) return "—";
  const sec = ms / 1000;
  if (sec >= 60) {
    const mm = Math.floor(sec / 60);
    const ss = Math.floor(sec % 60);
    return `${mm}:${String(ss).padStart(2, "0")}`;
  }
  return `${sec.toFixed(1)}s`;
}

export function BlockStatsGrid({
  intensity,
  actions,
  preview,
  heatmapStats,
  durationMs,
  scriptDurationMs,
  actionCount,
  isFullScript = false,
}: Props) {
  const { t } = useTranslation();
  const stats = useMemo(() => {
    if (isFullScript) {
      const fromApi = mapHeatmapStatsFromApi(heatmapStats);
      if (fromApi && fromApi.actionCount > 0) return fromApi;
    }

    const resolved =
      actions && actions.length >= 2
        ? actions
        : preview && preview.length >= 2
          ? pointsToActions(preview)
          : [];
    if (resolved.length < 2) return null;

    const timelineMs =
      isFullScript && scriptDurationMs && scriptDurationMs > 0
        ? scriptDurationMs
        : durationMs && durationMs > 0
          ? durationMs
          : undefined;
    return computeEroScriptsStats(resolved, timelineMs);
  }, [actions, preview, heatmapStats, durationMs, scriptDurationMs, isFullScript]);

  const resolvedDurationMs = isFullScript
    ? (stats?.durationMs && stats.durationMs > 0 ? stats.durationMs : 0) ||
      (scriptDurationMs && scriptDurationMs > 0 ? scriptDurationMs : 0)
    : (durationMs && durationMs > 0 ? durationMs : 0) ||
      (stats?.durationMs && stats.durationMs > 0 ? stats.durationMs : 0);

  const resolvedActionCount = isFullScript
    ? stats?.actionCount ?? actionCount ?? 0
    : actionCount ?? stats?.actionCount ?? (actions?.length ?? 0);

  const maxSpeed =
    (isFullScript
      ? stats?.maxSpeed ?? heatmapStats?.max_speed
      : heatmapStats?.max_speed ?? stats?.maxSpeed) ?? 0;
  const avgSpeed =
    (isFullScript
      ? stats?.avgSpeed ?? heatmapStats?.avg_speed
      : heatmapStats?.avg_speed ?? stats?.avgSpeed) ?? 0;

  return (
    <div className="block-stats-grid" aria-label={t("block.stats.aria")}>
      <div className="block-stat">
        <span className="block-stat-label">{t("block.stats.duration")}</span>
        <span className="block-stat-value">{formatBlockDuration(resolvedDurationMs)}</span>
      </div>
      <div className="block-stat">
        <span className="block-stat-label">{t("block.stats.actions")}</span>
        <span className="block-stat-value">
          {resolvedActionCount > 0 ? resolvedActionCount : "—"}
        </span>
      </div>
      <div className="block-stat">
        <span className="block-stat-label">{t("block.stats.intensity")}</span>
        <span className="block-stat-value">
          {intensity != null ? `${intensity.toFixed(0)}%` : "—"}
        </span>
      </div>
      <div className="block-stat">
        <span className="block-stat-label">{t("block.stats.maxSpeed")}</span>
        <span className="block-stat-value">
          {maxSpeed > 0 ? Math.round(maxSpeed) : "—"}
        </span>
      </div>
      <div className="block-stat block-stat--span">
        <span className="block-stat-label">{t("block.stats.avgSpeed")}</span>
        <span className="block-stat-value">
          {avgSpeed > 0 ? Math.round(avgSpeed) : "—"}
        </span>
      </div>
    </div>
  );
}
