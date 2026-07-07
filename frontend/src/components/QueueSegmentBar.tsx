import { useTranslation } from "react-i18next";
import type { ManualQueuePreview } from "../api/types";

const SEGMENT_COLORS = [
  "rgba(167, 139, 250, 0.72)",
  "rgba(99, 102, 241, 0.68)",
  "rgba(74, 222, 128, 0.62)",
  "rgba(251, 191, 36, 0.62)",
  "rgba(248, 113, 113, 0.58)",
];

export function QueueSegmentBar({
  segments,
  durationMs,
  playheadMs,
  currentIndex,
}: {
  segments: ManualQueuePreview["segments"];
  durationMs: number;
  playheadMs?: number;
  currentIndex?: number;
}) {
  const { t } = useTranslation();
  if (!segments.length || durationMs <= 0) return null;

  return (
    <div className="manual-queue-segment-bar-wrap manual-queue-segment-bar-wrap--below">
      <div className="manual-queue-segment-bar" title={t("player.queue.segmentBarTitle")}>
        {segments.map((seg) => (
          <div
            key={`${seg.block_id}-${seg.index}`}
            className={`manual-queue-segment-chunk${currentIndex === seg.index ? " manual-queue-segment-chunk--active" : ""}`}
            style={{
              width: `${(seg.duration_ms / durationMs) * 100}%`,
              background: SEGMENT_COLORS[seg.index % SEGMENT_COLORS.length],
            }}
            title={`${seg.index + 1}. ${seg.display_name ?? seg.block_id}`}
          />
        ))}
      </div>
      {playheadMs != null && playheadMs >= 0 && (
        <div
          className="manual-queue-playhead"
          style={{ left: `${Math.min(100, (playheadMs / durationMs) * 100)}%` }}
        />
      )}
    </div>
  );
}
