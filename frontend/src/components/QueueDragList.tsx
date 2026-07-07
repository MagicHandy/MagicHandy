import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { ManualQueueItem } from "../api/types";
import { UiCheckbox } from "./UiCheckbox";

export function QueueDragList({
  items,
  disabled,
  onReorder,
  onPatch,
  onRemove,
}: {
  items: ManualQueueItem[];
  disabled?: boolean;
  onReorder: (from: number, to: number) => void | Promise<void>;
  onPatch: (
    index: number,
    patch: { duration_sec?: number; loop?: boolean },
  ) => void | Promise<void>;
  onRemove: (index: number) => void | Promise<void>;
}) {
  const { t } = useTranslation();
  const [dragIndex, setDragIndex] = useState<number | null>(null);
  const [overIndex, setOverIndex] = useState<number | null>(null);

  const finishDrag = () => {
    setDragIndex(null);
    setOverIndex(null);
  };

  const handleDrop = (toIndex: number) => {
    if (dragIndex == null || dragIndex === toIndex) {
      finishDrag();
      return;
    }
    void onReorder(dragIndex, toIndex);
    finishDrag();
  };

  return (
    <ol className="queue-drag-list" aria-label={t("player.queue.dragAria")}>
      {items.map((item, index) => {
        const minDuration = Math.max(1, item.script_duration_sec ?? 1);
        const dragging = dragIndex === index;
        const dropBefore = overIndex === index && dragIndex != null && dragIndex !== index;

        return (
          <li
            key={`${item.block_id}-${index}`}
            className={`queue-drag-item${dragging ? " queue-drag-item--dragging" : ""}${dropBefore ? " queue-drag-item--drop-target" : ""}`}
            onDragOver={(e) => {
              if (disabled || dragIndex == null) return;
              e.preventDefault();
              e.dataTransfer.dropEffect = "move";
              setOverIndex(index);
            }}
            onDragLeave={(e) => {
              if (!e.currentTarget.contains(e.relatedTarget as Node)) {
                setOverIndex((prev) => (prev === index ? null : prev));
              }
            }}
            onDrop={(e) => {
              e.preventDefault();
              handleDrop(index);
            }}
          >
            <button
              type="button"
              className="queue-drag-handle"
              title={t("player.queue.dragHandle")}
              disabled={disabled}
              draggable={!disabled}
              aria-grabbed={dragging}
              onDragStart={(e) => {
                if (disabled) return;
                setDragIndex(index);
                e.dataTransfer.effectAllowed = "move";
                e.dataTransfer.setData("text/plain", String(index));
              }}
              onDragEnd={finishDrag}
            >
              <span aria-hidden>⋮⋮</span>
            </button>

            <span className="queue-drag-index">{index + 1}</span>

            <div className="queue-drag-main">
              <strong className="queue-drag-title" title={item.display_name ?? item.block_id}>
                {item.display_name ?? item.block_id}
              </strong>
              <div className="queue-drag-controls">
                <label className="queue-drag-duration">
                  <span>{t("player.queue.duration")}</span>
                  <input
                    type="number"
                    min={minDuration}
                    step={1}
                    disabled={disabled}
                    value={item.duration_sec}
                    onChange={(e) => {
                      const raw = Number(e.target.value);
                      if (!Number.isFinite(raw)) return;
                      void onPatch(index, {
                        duration_sec: Math.max(minDuration, raw),
                      });
                    }}
                    title={t("player.queue.minDuration", { sec: minDuration })}
                  />
                  <span className="queue-drag-unit">s</span>
                </label>
                <UiCheckbox
                  id={`queue-loop-${index}`}
                  label={t("player.queue.loop")}
                  disabled={disabled}
                  checked={item.loop}
                  onChange={(e) => void onPatch(index, { loop: e.target.checked })}
                />
              </div>
            </div>

            <button
              type="button"
              className="btn btn-ghost btn-sm queue-drag-remove"
              title={t("player.queue.remove")}
              disabled={disabled}
              onClick={() => void onRemove(index)}
            >
              ✕
            </button>
          </li>
        );
      })}

      {items.length > 0 && dragIndex != null && (
        <li
          className={`queue-drag-drop-end${overIndex === items.length ? " queue-drag-drop-end--active" : ""}`}
          onDragOver={(e) => {
            if (disabled) return;
            e.preventDefault();
            setOverIndex(items.length);
          }}
          onDrop={(e) => {
            e.preventDefault();
            handleDrop(items.length - 1);
          }}
        />
      )}
    </ol>
  );
}
