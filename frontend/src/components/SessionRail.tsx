import { useTranslation } from "react-i18next";
import type { StatusSnapshot } from "../api/types";
import { MotionQueuePanel } from "./MotionQueuePanel";
import { SceneStatusCard } from "./SceneStatusCard";

type QueueBlock = {
  block_id: string;
  duration_ms: number;
  intensity: number;
  bpm?: number | null;
  loop_pattern?: boolean;
  source?: string;
  enqueue_seq?: number;
};

export function SessionRail({
  snap,
  queueBlocks,
  queueEmptyMessage,
}: {
  snap: StatusSnapshot | null;
  queueBlocks?: QueueBlock[];
  queueEmptyMessage: string;
}) {
  const { t } = useTranslation();
  const buffer =
    snap?.buffer_remaining_sec ?? snap?.buffer_sec ?? undefined;

  return (
    <aside className="session-rail" aria-label={t("session.railAria")}>
      <div className="session-rail-top">
        <SceneStatusCard snap={snap} dense />
      </div>
      <MotionQueuePanel
        blocks={queueBlocks}
        bufferSec={buffer}
        blockCount={snap?.queue_blocks}
        emptyMessage={queueEmptyMessage}
        refillBusy={snap?.planner_refill_busy}
      />
    </aside>
  );
}
