import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type { ManualQueuePreview } from "../api/types";
import {
  computeHeatmapStats,
  mapHeatmapStatsFromApi,
  type FunscriptAction,
  type HeatmapStats,
} from "../lib/funscriptHeatmap";
import { useAnimatedPlayheadMs } from "../lib/useAnimatedPlayheadMs";
import { BlockHeatmap, type CurvePoint } from "./BlockHeatmap";
import { QueueSegmentBar } from "./QueueSegmentBar";

type ApiHeatmapStats = {
  action_count?: number;
  duration_ms?: number;
  max_speed?: number;
  avg_speed?: number;
};

type Props = {
  actions?: FunscriptAction[];
  points?: CurvePoint[];
  timelineDurationMs?: number | null;
  scriptDurationMs?: number | null;
  heatmapStats?: ApiHeatmapStats | null;
  playheadMs?: number | null;
  segments?: ManualQueuePreview["segments"];
  durationMs?: number;
  currentSegmentIndex?: number;
  live?: boolean;
};

const STAT_ITEMS: { key: keyof HeatmapStats | "durationMs"; labelKey: string; tone: string }[] = [
  { key: "durationMs", labelKey: "playerPreview.stats.duration", tone: "cyan" },
  { key: "actionCount", labelKey: "playerPreview.stats.actions", tone: "violet" },
  { key: "maxSpeed", labelKey: "playerPreview.stats.maxSpeed", tone: "lime" },
  { key: "avgSpeed", labelKey: "playerPreview.stats.avgSpeed", tone: "amber" },
];

function formatDurMs(ms: number): string {
  const durSec = ms / 1000;
  const mm = Math.floor(durSec / 60);
  const ss = Math.floor(durSec % 60);
  return mm > 0 ? `${mm}:${String(ss).padStart(2, "0")}` : `${ss}s`;
}

function statValue(key: (typeof STAT_ITEMS)[number]["key"], stats: HeatmapStats): string {
  if (key === "durationMs") return formatDurMs(stats.durationMs);
  if (key === "actionCount") return String(stats.actionCount);
  if (key === "maxSpeed") return String(Math.round(stats.maxSpeed));
  return String(Math.round(stats.avgSpeed));
}

function PreviewHeatmapStatsBar({ stats }: { stats: HeatmapStats }) {
  const { t } = useTranslation();
  return (
    <div className="player-preview-stats-bar" role="group" aria-label={t("playerPreview.stats.aria")}>
      {STAT_ITEMS.map(({ key, labelKey, tone }) => (
        <div key={key} className={`player-preview-stat player-preview-stat--${tone}`}>
          <span className="player-preview-stat-label">{t(labelKey)}</span>
          <strong className="player-preview-stat-value">{statValue(key, stats)}</strong>
        </div>
      ))}
    </div>
  );
}

function resolveStats(
  actions: FunscriptAction[] | undefined,
  heatmapStats: ApiHeatmapStats | null | undefined,
): HeatmapStats | null {
  const fromApi = mapHeatmapStatsFromApi(heatmapStats);
  if (fromApi) return fromApi;
  if (actions && actions.length >= 2) {
    return computeHeatmapStats(actions);
  }
  return null;
}

export function PlayerPreviewChart({
  actions,
  points,
  timelineDurationMs = null,
  scriptDurationMs = null,
  heatmapStats = null,
  playheadMs = null,
  segments,
  durationMs = 0,
  currentSegmentIndex,
  live = false,
}: Props) {
  const vizRef = useRef<HTMLDivElement>(null);
  const [chartH, setChartH] = useState(280);

  const stats = useMemo(
    () => resolveStats(actions, heatmapStats),
    [actions, heatmapStats],
  );

  const chartTimelineMs =
    timelineDurationMs && timelineDurationMs > 0
      ? timelineDurationMs
      : scriptDurationMs && scriptDurationMs > 0
        ? scriptDurationMs
        : durationMs > 0
          ? durationMs
          : null;

  const animatedPlayheadMs = useAnimatedPlayheadMs(
    playheadMs,
    live,
    chartTimelineMs ?? durationMs,
  );

  useEffect(() => {
    const el = vizRef.current;
    if (!el) return;

    const sync = () => setChartH(Math.max(200, el.clientHeight));
    sync();
    const ro = new ResizeObserver(sync);
    ro.observe(el);
    return () => ro.disconnect();
  }, [live]);

  const hasScript = Boolean(actions && actions.length >= 2) || Boolean(points && points.length >= 2);
  if (!hasScript) return null;

  const showSegments = Boolean(segments?.length && durationMs > 0);
  const showPlayhead = live && playheadMs != null;

  return (
    <div className={`player-preview-stage${live ? " player-preview-stage--live" : ""}`}>
      {stats && <PreviewHeatmapStatsBar stats={stats} />}
      <div
        ref={vizRef}
        className={`player-preview-viz${live ? " player-preview-viz--live" : ""}`}
      >
        <div className="player-preview-viz-glow" aria-hidden />
        <div className="player-preview-chart">
          <BlockHeatmap
            actions={actions}
            points={points}
            height={chartH}
            showStats={false}
            scriptDurationMs={chartTimelineMs}
            heatmapStats={heatmapStats}
            playheadMs={showPlayhead ? animatedPlayheadMs : null}
            className="player-preview-heatmap"
          />
        </div>
      </div>
      {showSegments && (
        <QueueSegmentBar
          segments={segments!}
          durationMs={durationMs}
          playheadMs={showPlayhead ? animatedPlayheadMs : undefined}
          currentIndex={currentSegmentIndex}
        />
      )}
    </div>
  );
}
