import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { BlockHeatmap } from "./BlockHeatmap";
import type { FunscriptAction } from "../lib/funscriptHeatmap";
import { blockTimelineSpanMs, formatEditMs } from "../lib/blockEditor";

type DragHandle = "start" | "end" | "pan" | null;

type Props = {
  actions: FunscriptAction[];
  trimStartMs: number;
  trimEndMs: number;
  onTrimChange: (startMs: number, endMs: number) => void;
  height?: number;
};

export function BlockHeatmapTrimmer({
  actions,
  trimStartMs,
  trimEndMs,
  onTrimChange,
  height = 120,
}: Props) {
  const { t } = useTranslation();
  const wrapRef = useRef<HTMLDivElement>(null);
  const [dragging, setDragging] = useState<DragHandle>(null);
  const spanMs = useMemo(() => blockTimelineSpanMs(actions), [actions]);

  const msFromClientX = useCallback(
    (clientX: number) => {
      const wrap = wrapRef.current;
      if (!wrap || spanMs <= 0) return 0;
      const rect = wrap.getBoundingClientRect();
      const ratio = Math.max(0, Math.min(1, (clientX - rect.left) / rect.width));
      return Math.round(ratio * spanMs);
    },
    [spanMs],
  );

  useEffect(() => {
    if (!dragging) return;

    const onMove = (event: PointerEvent) => {
      const ms = msFromClientX(event.clientX);
      if (dragging === "start") {
        onTrimChange(Math.min(ms, trimEndMs - 50), trimEndMs);
      } else if (dragging === "end") {
        onTrimChange(trimStartMs, Math.max(ms, trimStartMs + 50));
      } else if (dragging === "pan") {
        const width = trimEndMs - trimStartMs;
        let start = ms - width / 2;
        start = Math.max(0, Math.min(spanMs - width, start));
        onTrimChange(Math.round(start), Math.round(start + width));
      }
    };

    const onUp = () => setDragging(null);
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
    return () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
    };
  }, [dragging, msFromClientX, onTrimChange, spanMs, trimEndMs, trimStartMs]);

  const startPct = spanMs > 0 ? (trimStartMs / spanMs) * 100 : 0;
  const endPct = spanMs > 0 ? (trimEndMs / spanMs) * 100 : 100;

  return (
    <div className="block-trimmer">
      <div ref={wrapRef} className="block-trimmer-canvas">
        <BlockHeatmap actions={actions} height={height} />
        <div className="block-trimmer-shade block-trimmer-shade--left" style={{ width: `${startPct}%` }} />
        <div
          className="block-trimmer-shade block-trimmer-shade--right"
          style={{ left: `${endPct}%`, width: `${100 - endPct}%` }}
        />
        <div
          className="block-trimmer-selection"
          style={{ left: `${startPct}%`, width: `${Math.max(0, endPct - startPct)}%` }}
          onPointerDown={(event) => {
            if ((event.target as HTMLElement).classList.contains("block-trimmer-handle")) return;
            event.preventDefault();
            setDragging("pan");
          }}
        >
          <button
            type="button"
            className="block-trimmer-handle block-trimmer-handle--start"
            aria-label={t("block.heatmapTrimmer.startAria")}
            onPointerDown={(event) => {
              event.preventDefault();
              event.stopPropagation();
              setDragging("start");
            }}
          />
          <button
            type="button"
            className="block-trimmer-handle block-trimmer-handle--end"
            aria-label={t("block.heatmapTrimmer.endAria")}
            onPointerDown={(event) => {
              event.preventDefault();
              event.stopPropagation();
              setDragging("end");
            }}
          />
        </div>
      </div>
      <div className="block-trimmer-meta mono">
        <span>{t("block.heatmapTrimmer.cropLabel", { start: formatEditMs(trimStartMs), end: formatEditMs(trimEndMs) })}</span>
        <span>{t("block.heatmapTrimmer.duration", { duration: formatEditMs(trimEndMs - trimStartMs) })}</span>
      </div>
    </div>
  );
}
