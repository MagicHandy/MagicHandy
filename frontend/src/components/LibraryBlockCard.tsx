import { useTranslation } from "react-i18next";
import type { MotionBlock } from "../api/types";
import { api, downloadText } from "../api/client";
import { BlockHeatmap } from "./BlockHeatmap";
import { BlockStatsGrid } from "./BlockStatsGrid";
import { UiCheckbox } from "./UiCheckbox";

type Props = {
  block: MotionBlock;
  selected: boolean;
  onToggleSelect: () => void;
  onFeedback: (kind: string) => void;
  onAddToQueue: () => void;
  onToggleSessionRole: (
    role: "edging" | "milking" | "climax",
    enabled: boolean,
  ) => void;
  onEdit: () => void;
  onTest: () => void;
  onDelete: () => void;
  onExport: (filename: string) => void;
  onExportError: (message: string) => void;
};

export function LibraryBlockCard({
  block: b,
  selected,
  onToggleSelect,
  onFeedback,
  onAddToQueue,
  onToggleSessionRole,
  onEdit,
  onTest,
  onDelete,
  onExport,
  onExportError,
}: Props) {
  const { t } = useTranslation();
  const title = b.display_name ?? b.source_filename ?? b.id.slice(0, 36);

  const feedbackActions = [
    ["👍", "like", t("block.feedback.like")],
    ["👎", "dislike", t("block.feedback.dislike")],
    ["★", "favorite", t("block.feedback.favorite")],
    ["⛔", "block", t("block.feedback.block")],
  ] as const;

  const roleActions = [
    [t("block.roles.edge"), "edging"],
    [t("block.roles.milk"), "milking"],
    [t("block.roles.climax"), "climax"],
  ] as const;

  return (
    <article
      className={`block-card block-card--library glass${b.favorite ? " block-card--fav" : ""}${b.blocked ? " block-card--blocked" : ""}${selected ? " block-card--selected" : ""}`}
    >
      <div className="block-card-media">
        <div className="block-select-wrap">
          <UiCheckbox
            compact
            checked={selected}
            onChange={onToggleSelect}
            aria-label={t("block.select", { title })}
          />
        </div>
        <BlockHeatmap
          actions={b.actions}
          points={b.preview ?? []}
          bpm={b.bpm ?? null}
          isFullScript={Boolean(b.is_full_script)}
          scriptDurationMs={b.script_duration_ms ?? null}
          heatmapStats={b.heatmap_stats ?? null}
        />
        {b.bpm != null && b.bpm > 0 && (
          <span
            className={`block-card-bpm-pill bpm-badge bpm-badge--${b.pace ?? "medium"}`}
            title={t("block.bpmMeta", {
              legs: b.stroke_legs ?? "?",
              reversals: b.stroke_reversals ?? "?",
            })}
          >
            {b.bpm.toFixed(0)} BPM
          </span>
        )}
      </div>

      <div className="block-card-body">
        <div className="block-card-title-row">
          <h3 className="block-title" title={b.id}>
            {title}
          </h3>
          <span className="block-score mono" title={t("block.successScore")}>
            {(b.success_score ?? 0).toFixed(2)}
          </span>
        </div>

        {b.source_filename &&
          b.display_name &&
          b.source_filename !== b.display_name && (
            <p className="mono block-source hint" title={b.source_filename}>
              {b.source_filename}
            </p>
          )}

        <div className="block-card-tags">
          {b.is_user_recorded && (
            <span className="lib-tag lib-tag--recorded" title={t("block.recordedTitle")}>
              {t("block.myRecording")}
            </span>
          )}
          {b.is_full_script && (
            <span className="lib-tag lib-tag--full" title={t("block.fullScriptTitle")}>
              {t("block.fullScript")}
            </span>
          )}
          <span className="lib-tag lib-tag--zone">{b.zone ?? "?"}</span>
          <span className="lib-tag">{b.stroke_length ?? "—"}</span>
          <span className="lib-tag">{b.speed ?? "?"}</span>
          {b.rhythm && <span className="lib-tag lib-tag--muted">{b.rhythm}</span>}
          {b.pace_label && (
            <span className="lib-tag lib-tag--pace">{b.pace_label}</span>
          )}
        </div>

        <BlockStatsGrid
          intensity={b.intensity}
          actions={b.actions}
          preview={b.preview ?? []}
          heatmapStats={b.heatmap_stats ?? null}
          durationMs={
            b.heatmap_stats?.duration_ms ??
            b.script_duration_ms ??
            b.duration_ms
          }
          scriptDurationMs={b.is_full_script ? b.script_duration_ms ?? null : null}
          actionCount={b.playback_action_count ?? b.action_count}
          isFullScript={Boolean(b.is_full_script)}
        />
      </div>

      <footer className="block-card-footer">
        <div className="block-card-actions-primary">
          <button
            type="button"
            className="btn btn-sm btn-primary"
            onClick={onAddToQueue}
          >
            {t("block.addToQueue")}
          </button>
          <button
            type="button"
            className="btn btn-sm btn-ghost"
            title={t("block.testOnHandy")}
            onClick={onTest}
          >
            {t("block.test")}
          </button>
          <button
            type="button"
            className="btn btn-sm btn-ghost"
            title={t("block.editTrim")}
            onClick={onEdit}
          >
            {t("block.trim")}
          </button>
        </div>
        <div className="block-card-actions-secondary">
          {feedbackActions.map(([icon, kind, label]) => (
            <button
              key={kind}
              type="button"
              className="block-action-chip"
              title={label}
              onClick={() => onFeedback(kind)}
            >
              {icon}
            </button>
          ))}
          {roleActions.map(([label, role]) => {
            const active = b.session_roles?.includes(role);
            return (
              <button
                key={role}
                type="button"
                className={`block-action-chip block-action-chip--role${active ? " block-action-chip--active" : ""}`}
                title={
                  active
                    ? t("block.removeRole", { role })
                    : t("block.saveForRole", { role })
                }
                onClick={() => onToggleSessionRole(role, !active)}
              >
                {label}
              </button>
            );
          })}
          <select
            className="block-export-select"
            defaultValue=""
            aria-label={t("block.export")}
            onChange={(e) => {
              if (!e.target.value) return;
              api
                .exportPattern(b.id, e.target.value)
                .then(({ filename, content }) => {
                  downloadText(filename, content);
                  onExport(filename);
                })
                .catch((err) =>
                  onExportError(err instanceof Error ? err.message : t("common.error")),
                );
              e.target.value = "";
            }}
          >
            <option value="">{t("block.export")}</option>
            <option value="funscript">.funscript</option>
            <option value="csv">.csv</option>
            <option value="json">.json</option>
          </select>
          <button
            type="button"
            className="block-action-chip block-action-chip--danger"
            title={t("block.delete")}
            onClick={onDelete}
          >
            🗑
          </button>
        </div>
      </footer>
    </article>
  );
}
