import { useTranslation } from "react-i18next";
import type { MotionBlock } from "../api/types";
import {
  EMPTY_PLAYER_BLOCK_FILTERS,
  formatBlockDuration,
  getBpmRangeBuckets,
  getDurationRangeBuckets,
  getSpeedFilterOptions,
  speedDisplayLabel,
  type PlayerBlockFilters,
} from "../utils/rangeBuckets";
import { BlockHeatmap } from "./BlockHeatmap";
import { UiCheckbox } from "./UiCheckbox";

export type { PlayerBlockFilters };

export function PlayerBlockBrowser({
  blocks,
  totalBlocks,
  page,
  totalPages,
  searchInput,
  filters,
  defaultLoop,
  busy,
  onSearchInputChange,
  onFiltersChange,
  onDefaultLoopChange,
  onPageChange,
  onAdd,
}: {
  blocks: MotionBlock[];
  totalBlocks: number;
  page: number;
  totalPages: number;
  searchInput: string;
  filters: PlayerBlockFilters;
  defaultLoop: boolean;
  busy?: boolean;
  onSearchInputChange: (value: string) => void;
  onFiltersChange: (patch: Partial<PlayerBlockFilters>) => void;
  onDefaultLoopChange: (value: boolean) => void;
  onPageChange: (page: number) => void;
  onAdd: (block: MotionBlock) => void;
}) {
  const { t } = useTranslation();
  const speedOptions = getSpeedFilterOptions(t);
  const bpmBuckets = getBpmRangeBuckets(t);
  const durationBuckets = getDurationRangeBuckets(t);

  const hasFilters =
    Boolean(filters.speed) ||
    Boolean(filters.bpmRange) ||
    Boolean(filters.durationRange) ||
    Boolean(searchInput.trim());

  const clearFilters = () => {
    onFiltersChange(EMPTY_PLAYER_BLOCK_FILTERS);
    onSearchInputChange("");
  };

  return (
    <div className="player-block-browser">
      <div className="player-block-toolbar">
        <div className="player-block-search-field">
          <span className="section-label">{t("player.browser.searchLabel")}</span>
          <input
            type="search"
            placeholder={t("player.browser.searchPlaceholder")}
            value={searchInput}
            onChange={(e) => onSearchInputChange(e.target.value)}
            autoComplete="off"
          />
        </div>

        <div className="player-block-filters">
          <label className="player-block-filter">
            <span>{t("player.browser.speed")}</span>
            <select
              value={filters.speed}
              onChange={(e) => onFiltersChange({ speed: e.target.value })}
            >
              {speedOptions.map((opt) => (
                <option key={opt.id || "all"} value={opt.id}>
                  {opt.label}
                </option>
              ))}
            </select>
          </label>
          <label className="player-block-filter">
            <span>{t("player.browser.bpm")}</span>
            <select
              value={filters.bpmRange}
              onChange={(e) => onFiltersChange({ bpmRange: e.target.value })}
            >
              {bpmBuckets.map((b) => (
                <option key={b.id || "any"} value={b.id}>
                  {b.label}
                </option>
              ))}
            </select>
          </label>
          <label className="player-block-filter">
            <span>{t("player.browser.duration")}</span>
            <select
              value={filters.durationRange}
              onChange={(e) =>
                onFiltersChange({ durationRange: e.target.value })
              }
            >
              {durationBuckets.map((b) => (
                <option key={b.id || "any"} value={b.id}>
                  {b.label}
                </option>
              ))}
            </select>
          </label>
        </div>

        <div className="player-block-toolbar-foot">
          <label className="player-block-loop">
            <UiCheckbox
              label={t("player.browser.loopOnAdd")}
              checked={defaultLoop}
              onChange={(e) => onDefaultLoopChange(e.target.checked)}
            />
          </label>
          {hasFilters && (
            <button
              type="button"
              className="btn btn-ghost btn-sm"
              onClick={clearFilters}
            >
              {t("player.browser.clearFilters")}
            </button>
          )}
        </div>
      </div>

      <div className="player-block-meta">
        <span className="hint">
          {t("player.browser.blockCount", { count: totalBlocks.toLocaleString() })}
          {hasFilters ? ` · ${t("player.browser.filtered")}` : ""}
        </span>
        <span className="hint">
          {t("player.browser.page", { page, total: totalPages })}
        </span>
      </div>

      {blocks.length === 0 ? (
        <div className="player-block-empty">
          <p className="hint">{t("player.browser.empty")}</p>
        </div>
      ) : (
        <div className="player-block-grid">
          {blocks.map((block) => {
            const durSec = (block.duration_ms ?? 0) / 1000;
            const speed = block.speed ?? "";
            return (
              <article key={block.id} className="player-block-card glass">
                <button
                  type="button"
                  className="player-block-card-hit"
                  disabled={busy}
                  title={t("player.browser.addTitle", {
                    speed: speedDisplayLabel(block.speed, t),
                    duration: formatBlockDuration(durSec),
                    bpm: block.bpm != null ? ` · ${Math.round(block.bpm)} bpm` : "",
                  })}
                  onClick={() => onAdd(block)}
                >
                  <div className="player-block-card-viz">
                    <BlockHeatmap
                      actions={block.actions}
                      points={block.preview ?? []}
                      height={72}
                      bpm={block.bpm ?? null}
                    />
                    <div className="player-block-card-stats" aria-hidden>
                      <span
                        className={`player-stat player-stat--speed${speed ? ` player-stat--${speed}` : ""}`}
                      >
                        {speedDisplayLabel(block.speed, t)}
                      </span>
                      <span className="player-stat player-stat--dur">
                        {formatBlockDuration(durSec)}
                      </span>
                      <span className="player-stat player-stat--bpm">
                        {block.bpm != null && block.bpm > 0
                          ? `${Math.round(block.bpm)} bpm`
                          : "— bpm"}
                      </span>
                    </div>
                  </div>
                  <div className="player-block-card-body">
                    <strong
                      className="player-block-card-title"
                      title={block.id}
                    >
                      {block.display_name ?? block.source_filename ?? block.id}
                    </strong>
                  </div>
                  <span className="player-block-card-add">+</span>
                </button>
              </article>
            );
          })}
        </div>
      )}

      <div className="pager player-block-pager">
        <button
          type="button"
          className="btn btn-ghost btn-sm"
          disabled={page <= 1}
          onClick={() => onPageChange(page - 1)}
        >
          {t("common.previous")}
        </button>
        <span className="hint">
          {page} / {totalPages}
        </span>
        <button
          type="button"
          className="btn btn-ghost btn-sm"
          disabled={page >= totalPages}
          onClick={() => onPageChange(page + 1)}
        >
          {t("common.next")}
        </button>
      </div>
    </div>
  );
}
