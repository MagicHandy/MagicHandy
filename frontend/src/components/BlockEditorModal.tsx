import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { api } from "../api/client";
import type { MotionBlock } from "../api/types";
import { BlockHeatmapTrimmer } from "./BlockHeatmapTrimmer";
import { BlockStatsGrid } from "./BlockStatsGrid";
import {
  blockTimelineSpanMs,
  formatEditMs,
  trimBlockActions,
} from "../lib/blockEditor";
import type { FunscriptAction } from "../lib/funscriptHeatmap";

type Props = {
  block: MotionBlock;
  onClose: () => void;
  onSaved: () => void;
  notify: (message: string, kind: "ok" | "error") => void;
};

export function BlockEditorModal({ block, onClose, onSaved, notify }: Props) {
  const { t } = useTranslation();
  const [workingActions, setWorkingActions] = useState<FunscriptAction[]>(
    () => (block.actions ?? []) as FunscriptAction[],
  );
  const [trimStartMs, setTrimStartMs] = useState(0);
  const [trimEndMs, setTrimEndMs] = useState(() =>
    blockTimelineSpanMs((block.actions ?? []) as FunscriptAction[]),
  );
  const [busy, setBusy] = useState(false);

  const spanMs = useMemo(() => blockTimelineSpanMs(workingActions), [workingActions]);
  const previewActions = useMemo(
    () => trimBlockActions(workingActions, trimStartMs, trimEndMs),
    [workingActions, trimStartMs, trimEndMs],
  );

  useEffect(() => {
    setTrimStartMs(0);
    setTrimEndMs(blockTimelineSpanMs(workingActions));
  }, [workingActions]);

  const applyTrim = () => {
    if (previewActions.length < 2) {
      notify(t("block.editor.minTrimPoints"), "error");
      return;
    }
    setWorkingActions(previewActions);
    notify(t("block.editor.trimApplied", { count: previewActions.length }), "ok");
  };

  const save = async (mode: "replace" | "new") => {
    const payload = previewActions.length >= 2 ? previewActions : workingActions;
    if (payload.length < 2) {
      notify(t("block.editor.minActions"), "error");
      return;
    }
    setBusy(true);
    try {
      const result = await api.saveEditedBlock(block.id, {
        actions: payload,
        mode,
      });
      notify(
        mode === "replace"
          ? t("block.editor.replaced", {
              count: result.action_count,
              duration: formatEditMs(result.duration_ms),
            })
          : t("block.editor.created", { id: result.block_id.slice(0, 24) }),
        "ok",
      );
      onSaved();
      onClose();
    } catch (error) {
      notify(error instanceof Error ? error.message : t("block.editor.saveError"), "error");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="modal-backdrop modal-backdrop--editor" onClick={onClose}>
      <div
        className="modal glass block-editor-modal"
        onClick={(event) => event.stopPropagation()}
      >
        <header className="block-editor-head">
          <div>
            <h3>{t("block.editor.title")}</h3>
            <p className="hint">{block.display_name ?? block.id}</p>
          </div>
          <button type="button" className="btn btn-ghost btn-sm" onClick={onClose}>
            {t("common.close")}
          </button>
        </header>

        {block.is_full_script && (
          <p className="hint block-editor-warn">{t("block.editor.fullScriptWarn")}</p>
        )}

        <BlockHeatmapTrimmer
          actions={workingActions}
          trimStartMs={trimStartMs}
          trimEndMs={trimEndMs}
          onTrimChange={(start, end) => {
            setTrimStartMs(start);
            setTrimEndMs(end);
          }}
          height={block.is_full_script ? 160 : 128}
        />

        <BlockStatsGrid
          intensity={block.intensity}
          actions={previewActions.length >= 2 ? previewActions : workingActions}
          durationMs={Math.max(1, trimEndMs - trimStartMs)}
          actionCount={
            previewActions.length >= 2 ? previewActions.length : workingActions.length
          }
          isFullScript={Boolean(block.is_full_script)}
        />

        <p className="hint block-editor-help">{t("block.editor.help")}</p>

        <div className="btn-row block-editor-actions">
          <button
            type="button"
            className="btn btn-ghost btn-sm"
            disabled={busy || previewActions.length < 2 || spanMs <= 0}
            onClick={applyTrim}
          >
            {t("block.editor.applyTrim")}
          </button>
          <button
            type="button"
            className="btn btn-primary btn-sm"
            disabled={busy}
            onClick={() => save("replace")}
          >
            {t("block.editor.replace")}
          </button>
          <button
            type="button"
            className="btn btn-ghost btn-sm"
            disabled={busy}
            onClick={() => save("new")}
          >
            {t("block.editor.saveAsNew")}
          </button>
        </div>
      </div>
    </div>
  );
}
