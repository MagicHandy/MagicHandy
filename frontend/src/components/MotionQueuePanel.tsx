import { useTranslation } from "react-i18next";

type QueueBlock = {
  block_id: string;
  duration_ms: number;
  intensity: number;
  bpm?: number | null;
  loop_pattern?: boolean;
  source?: string;
  enqueue_seq?: number;
};

export function MotionQueuePanel({
  blocks,
  bufferSec,
  blockCount,
  emptyMessage,
  refillBusy,
}: {
  blocks?: QueueBlock[];
  bufferSec?: number;
  blockCount?: number;
  emptyMessage: string;
  refillBusy?: boolean;
}) {
  const { t } = useTranslation();
  const count = blockCount ?? blocks?.length ?? 0;
  const hasBlocks = (blocks?.length ?? 0) > 0;

  return (
    <section className="glass motion-queue-panel" aria-label={t("session.queue.aria")}>
      <header className="motion-queue-head">
        <span className="section-label">{t("session.queue.title")}</span>
        <div className="motion-queue-meta">
          {bufferSec != null && (
            <span className="motion-queue-stat" title={t("session.queue.bufferTitle")}>
              <span className="motion-queue-stat-label">{t("session.queue.buf")}</span>
              <span className="mono">{bufferSec.toFixed(0)}s</span>
            </span>
          )}
          <span className="motion-queue-stat" title={t("session.queue.countTitle")}>
            <span className="motion-queue-stat-label">{t("session.queue.queue")}</span>
            <span className="mono">{count}</span>
          </span>
          {refillBusy != null && (
            <span
              className={`motion-queue-stat${refillBusy ? " motion-queue-stat--warn" : ""}`}
              title={t("session.queue.refillTitle")}
            >
              <span className="motion-queue-stat-label">{t("session.scene.ai")}</span>
              <span>{refillBusy ? "…" : t("session.queue.ok")}</span>
            </span>
          )}
        </div>
      </header>

      <div className="motion-queue-body">
        {hasBlocks ? (
          <ul className="motion-queue-list">
            {blocks!.map((item, index) => (
              <li
                key={`${item.block_id}-${item.enqueue_seq ?? index}`}
                className="motion-queue-item"
              >
                <span className="motion-queue-index">{index + 1}</span>
                <div className="motion-queue-item-main">
                  <span className="mono motion-queue-id" title={item.block_id}>
                    {item.block_id}
                  </span>
                  <span className="motion-queue-detail">
                    {(item.duration_ms / 1000).toFixed(1)}s · {item.intensity}%
                    {item.bpm != null ? ` · ${Math.round(item.bpm)} bpm` : ""}
                    {item.loop_pattern ? ` · ${t("session.queue.loop")}` : ""}
                    {item.source ? ` · ${item.source}` : ""}
                  </span>
                </div>
              </li>
            ))}
          </ul>
        ) : (
          <div className="motion-queue-empty">
            <p className="hint">{emptyMessage}</p>
          </div>
        )}
      </div>
    </section>
  );
}
