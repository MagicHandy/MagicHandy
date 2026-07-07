import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router-dom";
import { api, downloadText } from "../api/client";
import type {
  ImportBlockSummary,
  ImportResult,
  MotionBlock,
} from "../api/types";
import { BlockEditorModal } from "../components/BlockEditorModal";
import { BlockHeatmap } from "../components/BlockHeatmap";
import { LibraryBlockCard } from "../components/LibraryBlockCard";
import { Tabs } from "../components/Tabs";
import { UiCheckbox } from "../components/UiCheckbox";
import { useToast } from "../contexts/ToastContext";

const PAGE_SIZE = 24;

type FilterOption = { id: string; label: string };

export function Library() {
  const { t } = useTranslation();
  const [searchParams, setSearchParams] = useSearchParams();
  const initialTab = searchParams.get("tab") === "import" ? "import" : "blocks";
  const [tab, setTab] = useState(initialTab);
  const [blocksRefreshKey, setBlocksRefreshKey] = useState(0);

  const changeTab = (next: string) => {
    setTab(next);
    if (next === "import") {
      setSearchParams({ tab: "import" });
    } else {
      setSearchParams({});
    }
  };

  useEffect(() => {
    const tabParam = searchParams.get("tab") === "import" ? "import" : "blocks";
    setTab(tabParam);
  }, [searchParams]);

  return (
    <div className="page page--fill library-page library-page--pro">
      <header className="page-toolbar library-toolbar">
        <div className="library-toolbar-main">
          <Tabs
            active={tab}
            onChange={changeTab}
            tabs={[
              { id: "blocks", label: t("library.tabs.blocks") },
              { id: "import", label: t("library.tabs.import") },
            ]}
          />
          <p className="hint library-toolbar-hint">
            {tab === "blocks"
              ? t("library.toolbar.blocksHint")
              : t("library.toolbar.importHint")}
          </p>
        </div>
      </header>
      {tab === "blocks" ? (
        <BlocksTab refreshKey={blocksRefreshKey} />
      ) : (
        <div className="library-import-scroll page-scroll">
          <ImportTab
            onImported={() => {
              setBlocksRefreshKey((k) => k + 1);
            }}
          />
        </div>
      )}
    </div>
  );
}

function BlocksTab({ refreshKey }: { refreshKey: number }) {
  const { t } = useTranslation();
  const { notify } = useToast();
  const [searchParams, setSearchParams] = useSearchParams();
  const [blocks, setBlocks] = useState<MotionBlock[]>([]);
  const [total, setTotal] = useState(0);
  const [zone, setZone] = useState("");
  const [speed, setSpeed] = useState("");
  const [rhythm, setRhythm] = useState("");
  const [strokeLength, setStrokeLength] = useState("");
  const [category, setCategory] = useState("");
  const [minInt, setMinInt] = useState(0);
  const [maxInt, setMaxInt] = useState(100);
  const [minDurationSec, setMinDurationSec] = useState<number | "">("");
  const [maxDurationSec, setMaxDurationSec] = useState<number | "">("");
  const [favOnly, setFavOnly] = useState(false);
  const [userRecordedOnly, setUserRecordedOnly] = useState(
    () => searchParams.get("recordings") === "1",
  );
  const [minBpm, setMinBpm] = useState<number | "">("");
  const [maxBpm, setMaxBpm] = useState<number | "">("");
  const [sortBy, setSortBy] = useState("");
  const [nameSearch, setNameSearch] = useState("");
  const [searchInput, setSearchInput] = useState("");
  const [filterMeta, setFilterMeta] = useState<{
    categories: FilterOption[];
    zones: FilterOption[];
    speeds: FilterOption[];
    rhythms: FilterOption[];
    stroke_lengths: FilterOption[];
  }>({
    categories: [],
    zones: [],
    speeds: [],
    rhythms: [],
    stroke_lengths: [],
  });
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [confirmTest, setConfirmTest] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);
  const [confirmBatchDelete, setConfirmBatchDelete] = useState(false);
  const [reclassifying, setReclassifying] = useState(false);
  const [recalculatingTimings, setRecalculatingTimings] = useState(false);
  const [editingBlock, setEditingBlock] = useState<MotionBlock | null>(null);
  const [selectAllBusy, setSelectAllBusy] = useState(false);
  const [page, setPage] = useState(1);

  const filterQuery = useCallback(
    () => ({
      zone,
      speed,
      rhythm,
      stroke_length: strokeLength,
      category,
      min_intensity: minInt,
      max_intensity: maxInt,
      favorites_only: favOnly,
      user_recorded_only: userRecordedOnly,
      ...(minDurationSec !== ""
        ? { min_duration_ms: Math.round(Number(minDurationSec) * 1000) }
        : {}),
      ...(maxDurationSec !== ""
        ? { max_duration_ms: Math.round(Number(maxDurationSec) * 1000) }
        : {}),
      ...(minBpm !== "" ? { min_bpm: minBpm } : {}),
      ...(maxBpm !== "" ? { max_bpm: maxBpm } : {}),
      ...(sortBy ? { sort: sortBy } : {}),
      ...(nameSearch.trim() ? { q: nameSearch.trim() } : {}),
    }),
    [
      zone,
      speed,
      rhythm,
      strokeLength,
      category,
      minInt,
      maxInt,
      favOnly,
      userRecordedOnly,
      minDurationSec,
      maxDurationSec,
      minBpm,
      maxBpm,
      sortBy,
      nameSearch,
    ],
  );

  const filters = useCallback(
    () => ({
      ...filterQuery(),
      offset: (page - 1) * PAGE_SIZE,
      limit: PAGE_SIZE,
    }),
    [filterQuery, page],
  );

  useEffect(() => {
    setPage(1);
  }, [filterQuery]);

  useEffect(() => {
    setUserRecordedOnly(searchParams.get("recordings") === "1");
  }, [searchParams]);

  useEffect(() => {
    api.getPatternMeta().then(setFilterMeta).catch(() => {});
  }, []);

  const load = useCallback(async () => {
    const f = filters();
    const listRes = await api.listPatterns(f);
    setBlocks(listRes.blocks);
    const countRes = await api.countPatterns(f).catch(() => ({ total: 0, count: 0 }));
    setTotal(
      listRes.total ??
        countRes.total ??
        countRes.count ??
        listRes.blocks.length,
    );
  }, [filters]);

  useEffect(() => {
    load().catch((e) =>
      notify(e instanceof Error ? e.message : t("common.error"), "error"),
    );
  }, [load, notify, refreshKey]);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setNameSearch(searchInput.trim());
    }, 350);
    return () => window.clearTimeout(timer);
  }, [searchInput]);

  const toggleSelect = (id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const selectAllOnPage = () => {
    setSelected(new Set(blocks.map((b) => b.id)));
  };

  const selectAllMatchingFilters = async () => {
    setSelectAllBusy(true);
    try {
      const res = await api.listPatternIds(filterQuery());
      setSelected(new Set(res.ids));
      notify(
        res.returned < res.total
          ? t("library.toast.selectAllLimited", {
              returned: res.returned,
              total: res.total,
            })
          : t("library.toast.selectAll", { count: res.total }),
        "ok",
      );
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    } finally {
      setSelectAllBusy(false);
    }
  };

  const clearSelection = () => setSelected(new Set());

  const resetFilters = () => {
    setSearchInput("");
    setNameSearch("");
    setZone("");
    setSpeed("");
    setRhythm("");
    setStrokeLength("");
    setCategory("");
    setMinInt(0);
    setMaxInt(100);
    setMinDurationSec("");
    setMaxDurationSec("");
    setFavOnly(false);
    setUserRecordedOnly(false);
    setSearchParams({});
    setMinBpm("");
    setMaxBpm("");
    setSortBy("");
    setPage(1);
    clearSelection();
  };

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  const defaultZones = useMemo(
    () => [
      { id: "top", label: t("library.zones.top") },
      { id: "middle", label: t("library.zones.middle") },
      { id: "bottom", label: t("library.zones.bottom") },
      { id: "full", label: t("library.zones.full") },
      { id: "mixed", label: t("library.zones.mixed") },
    ],
    [t],
  );

  const defaultSpeeds = useMemo(
    () => [
      { id: "slow", label: t("patterns.speed.slow") },
      { id: "medium", label: t("patterns.speed.medium") },
      { id: "fast", label: t("patterns.speed.fast") },
      { id: "very_fast", label: t("patterns.speed.veryFast") },
    ],
    [t],
  );

  const sessionRoleLabel = (role: "edging" | "milking" | "climax") =>
    ({
      edging: t("block.roles.edge"),
      milking: t("block.roles.milk"),
      climax: t("block.roles.climax"),
    })[role];

  const activeFilterCount = useMemo(() => {
    let n = 0;
    if (nameSearch.trim()) n++;
    if (category) n++;
    if (zone) n++;
    if (speed) n++;
    if (rhythm) n++;
    if (strokeLength) n++;
    if (minInt > 0) n++;
    if (maxInt < 100) n++;
    if (minDurationSec !== "") n++;
    if (maxDurationSec !== "") n++;
    if (minBpm !== "") n++;
    if (maxBpm !== "") n++;
    if (sortBy) n++;
    if (favOnly) n++;
    if (userRecordedOnly) n++;
    return n;
  }, [
    nameSearch,
    category,
    zone,
    speed,
    rhythm,
    strokeLength,
    minInt,
    maxInt,
    minDurationSec,
    maxDurationSec,
    minBpm,
    maxBpm,
    sortBy,
    favOnly,
    userRecordedOnly,
  ]);

  const feedback = async (id: string, kind: string) => {
    try {
      const r = await api.patternFeedback(id, kind);
      notify(t("library.toast.score", { score: r.success_score?.toFixed(2) ?? "—" }), "ok");
      await load();
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  const addToQueue = async (id: string) => {
    try {
      await api.addManualQueueItem({ block_id: id });
      notify(t("library.toast.addedToQueue"), "ok");
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  const toggleSessionRole = async (
    id: string,
    role: "edging" | "milking" | "climax",
    enabled: boolean,
  ) => {
    try {
      await api.setBlockSessionRole(id, role, enabled);
      notify(
        enabled
          ? t("library.toast.roleMarked", { role: sessionRoleLabel(role) })
          : t("library.toast.roleRemoved", { role: sessionRoleLabel(role) }),
        "ok",
      );
      await load();
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  return (
    <>
      <div className="library-workspace">
        <aside className="glass library-filters-rail library-filters-rail--pro" aria-label={t("library.filters.aria")}>
          <header className="library-filters-head">
            <div>
              <span className="section-label">{t("library.filters.title")}</span>
              {activeFilterCount > 0 && (
                <span className="library-filters-active">{t("library.filters.active", { count: activeFilterCount })}</span>
              )}
            </div>
            <span className="hint library-filters-count mono">
              {total} · {page}/{totalPages}
            </span>
          </header>
          <div className="library-filters-body">
            <div className="library-filter-group">
              <span className="library-filter-group-label">{t("library.filters.search")}</span>
              <label className="field-compact library-field-full library-field-pro">
                <span>{t("library.filters.name")}</span>
                <input
                  type="search"
                  className="library-input"
                  placeholder={t("library.filters.searchPlaceholder")}
                  value={searchInput}
                  onChange={(e) => setSearchInput(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") setNameSearch(searchInput.trim());
                  }}
                />
              </label>
            </div>

            <div className="library-filter-group">
              <span className="library-filter-group-label">{t("library.filters.rating")}</span>
              <label className="field-compact library-field-pro">
                <span>{t("library.filters.category")}</span>
                <select
                  className="library-input"
                  value={category}
                  onChange={(e) => setCategory(e.target.value)}
                  aria-label={t("library.filters.categoryAria")}
                >
                  <option value="">{t("library.filters.all")}</option>
                  {filterMeta.categories.map((opt) => (
                    <option key={opt.id} value={opt.id}>
                      {opt.label}
                    </option>
                  ))}
                </select>
              </label>
              <label className="field-compact library-field-pro">
                <span>{t("library.filters.zone")}</span>
                <select
                  className="library-input"
                  value={zone}
                  onChange={(e) => setZone(e.target.value)}
                  aria-label={t("library.filters.zoneAria")}
                >
                  <option value="">{t("library.filters.all")}</option>
                  {(filterMeta.zones.length ? filterMeta.zones : defaultZones).map((opt) => (
                    <option key={opt.id} value={opt.id}>
                      {opt.label}
                    </option>
                  ))}
                </select>
              </label>
              <label className="field-compact library-field-pro">
                <span>{t("library.filters.speed")}</span>
                <select
                  className="library-input"
                  value={speed}
                  onChange={(e) => setSpeed(e.target.value)}
                  aria-label={t("library.filters.speedAria")}
                >
                  <option value="">{t("library.filters.all")}</option>
                  {(filterMeta.speeds.length ? filterMeta.speeds : defaultSpeeds).map((opt) => (
                    <option key={opt.id} value={opt.id}>
                      {opt.label}
                    </option>
                  ))}
                </select>
              </label>
              <label className="field-compact library-field-pro">
                <span>{t("library.filters.rhythm")}</span>
                <select
                  className="library-input"
                  value={rhythm}
                  onChange={(e) => setRhythm(e.target.value)}
                  aria-label={t("library.filters.rhythmAria")}
                >
                  <option value="">{t("library.filters.rhythmAll")}</option>
                  {filterMeta.rhythms.map((opt) => (
                    <option key={opt.id} value={opt.id}>
                      {opt.label}
                    </option>
                  ))}
                </select>
              </label>
              <label className="field-compact library-field-pro">
                <span>{t("library.filters.strokeLength")}</span>
                <select
                  className="library-input"
                  value={strokeLength}
                  onChange={(e) => setStrokeLength(e.target.value)}
                  aria-label={t("library.filters.strokeAria")}
                >
                  <option value="">{t("library.filters.all")}</option>
                  {filterMeta.stroke_lengths.map((opt) => (
                    <option key={opt.id} value={opt.id}>
                      {opt.label}
                    </option>
                  ))}
                </select>
              </label>
            </div>

            <div className="library-filter-group">
              <span className="library-filter-group-label">{t("library.filters.metrics")}</span>
              <div className="library-filter-row">
                <label className="field-compact library-field-pro">
                  <span>{t("library.filters.minDuration")}</span>
                  <input
                    className="library-input"
                    type="number"
                    min={0}
                    step={0.1}
                    placeholder="s"
                    value={minDurationSec}
                    onChange={(e) =>
                      setMinDurationSec(e.target.value === "" ? "" : Number(e.target.value))
                    }
                  />
                </label>
                <label className="field-compact library-field-pro">
                  <span>{t("library.filters.maxDuration")}</span>
                  <input
                    className="library-input"
                    type="number"
                    min={0}
                    step={0.1}
                    placeholder="s"
                    value={maxDurationSec}
                    onChange={(e) =>
                      setMaxDurationSec(e.target.value === "" ? "" : Number(e.target.value))
                    }
                  />
                </label>
              </div>
              <div className="library-filter-row">
                <label className="field-compact library-field-pro">
                  <span>{t("library.filters.minIntensity")}</span>
                  <input
                    className="library-input"
                    type="number"
                    value={minInt}
                    onChange={(e) => setMinInt(Number(e.target.value))}
                  />
                </label>
                <label className="field-compact library-field-pro">
                  <span>{t("library.filters.maxIntensity")}</span>
                  <input
                    className="library-input"
                    type="number"
                    value={maxInt}
                    onChange={(e) => setMaxInt(Number(e.target.value))}
                  />
                </label>
              </div>
              <div className="library-filter-row">
                <label className="field-compact library-field-pro">
                  <span>{t("library.filters.minBpm")}</span>
                  <input
                    className="library-input"
                    type="number"
                    placeholder="—"
                    value={minBpm}
                    onChange={(e) =>
                      setMinBpm(e.target.value === "" ? "" : Number(e.target.value))
                    }
                  />
                </label>
                <label className="field-compact library-field-pro">
                  <span>{t("library.filters.maxBpm")}</span>
                  <input
                    className="library-input"
                    type="number"
                    placeholder="—"
                    value={maxBpm}
                    onChange={(e) =>
                      setMaxBpm(e.target.value === "" ? "" : Number(e.target.value))
                    }
                  />
                </label>
              </div>
            </div>

            <div className="library-filter-group">
              <span className="library-filter-group-label">{t("library.filters.display")}</span>
              <label className="field-compact library-field-pro">
                <span>{t("library.filters.sort")}</span>
                <select
                  className="library-input"
                  value={sortBy}
                  onChange={(e) => setSortBy(e.target.value)}
                  aria-label={t("library.filters.sortAria")}
                >
                  <option value="">{t("library.filters.sortScore")}</option>
                  <option value="duration_desc">{t("library.sort.durationDesc")}</option>
                  <option value="duration">{t("library.sort.durationAsc")}</option>
                  <option value="bpm_desc">{t("library.sort.bpmDesc")}</option>
                  <option value="bpm">{t("library.sort.bpmAsc")}</option>
                </select>
              </label>
              <div className="library-field-pro">
                <UiCheckbox
                  label={t("library.filters.myRecordings")}
                  checked={userRecordedOnly}
                  onChange={(e) => {
                    const on = e.target.checked;
                    setUserRecordedOnly(on);
                    if (on) {
                      setSearchParams({ recordings: "1" });
                    } else if (searchParams.get("recordings")) {
                      setSearchParams({});
                    }
                  }}
                />
              </div>
              <div className="library-field-pro">
                <UiCheckbox
                  label={t("library.filters.favoritesOnly")}
                  checked={favOnly}
                  onChange={(e) => setFavOnly(e.target.checked)}
                />
              </div>
            </div>
          </div>
          <footer className="library-filters-foot">
            <button
              type="button"
              className="btn btn-primary btn-sm library-apply-btn"
              onClick={() => {
                clearSelection();
                load();
              }}
            >
              {t("library.filters.apply")}
            </button>
            <button
              type="button"
              className="btn btn-ghost btn-sm"
              onClick={resetFilters}
              disabled={activeFilterCount === 0}
            >
              {t("library.filters.clear")}
            </button>
            <details className="library-maint-details">
              <summary className="hint">{t("library.maintenance.title")}</summary>
              <div className="library-maint-actions">
                <button
                  type="button"
                  className="btn btn-ghost btn-sm"
                  disabled={recalculatingTimings}
                  onClick={async () => {
                    setRecalculatingTimings(true);
                    try {
                      const r = await api.recalculateBlockTimings();
                      notify(
                        t("library.maintenance.timestamps", {
                          updated: r.updated,
                          trimmed: r.trimmed_actions,
                          unchanged: r.unchanged,
                          skipped: r.skipped,
                        }),
                        "ok",
                      );
                      await load();
                    } catch (e) {
                      notify(e instanceof Error ? e.message : t("common.error"), "error");
                    } finally {
                      setRecalculatingTimings(false);
                    }
                  }}
                >
                  {recalculatingTimings ? t("library.toast.recalculating") : t("library.toast.recalculateTimings")}
                </button>
                <button
                  type="button"
                  className="btn btn-ghost btn-sm"
                  disabled={reclassifying}
                  onClick={async () => {
                    setReclassifying(true);
                    try {
                      const r = await api.reclassifyPatterns();
                      notify(
                        t("library.toast.reclassified", {
                          updated: r.updated,
                          skipped: r.skipped,
                        }),
                        "ok",
                      );
                      await load();
                    } catch (e) {
                      notify(e instanceof Error ? e.message : t("common.error"), "error");
                    } finally {
                      setReclassifying(false);
                    }
                  }}
                >
                  {reclassifying ? t("library.toast.reclassifying") : t("library.toast.reclassify")}
                </button>
              </div>
            </details>
          </footer>
        </aside>

        <div className="library-main">
      <div className="glass library-batch-bar library-batch-bar--pro">
        <UiCheckbox
          label={t("library.batch.all", { count: selected.size })}
          checked={blocks.length > 0 && blocks.every((b) => selected.has(b.id))}
          onChange={(e) => (e.target.checked ? selectAllOnPage() : clearSelection())}
        />
        <button
          type="button"
          className="btn btn-sm btn-ghost"
          disabled={selectAllBusy || total === 0}
          onClick={selectAllMatchingFilters}
        >
          {selectAllBusy ? t("library.batch.selecting") : t("library.batch.allFiltered", { count: total })}
        </button>
        <button
          type="button"
          className="btn btn-sm btn-ghost"
          disabled={selected.size === 0}
          onClick={() => setConfirmBatchDelete(true)}
        >
          {t("library.batch.deleteSelected")}
        </button>
        <button
          type="button"
          className="btn btn-sm btn-ghost"
          disabled={selected.size === 0}
          onClick={clearSelection}
        >
          {t("library.batch.clearSelection")}
        </button>
      </div>

      <div className="library-grid-scroll">
      <div className="blocks-grid blocks-grid--library">
        {blocks.map((b) => (
          <LibraryBlockCard
            key={b.id}
            block={b}
            selected={selected.has(b.id)}
            onToggleSelect={() => toggleSelect(b.id)}
            onFeedback={(kind) => feedback(b.id, kind)}
            onAddToQueue={() => addToQueue(b.id)}
            onToggleSessionRole={(role, enabled) =>
              toggleSessionRole(b.id, role, enabled)
            }
            onEdit={() => setEditingBlock(b)}
            onTest={() => setConfirmTest(b.id)}
            onDelete={() => setConfirmDelete(b.id)}
            onExport={(filename) => notify(t("library.toast.downloaded", { filename }), "ok")}
            onExportError={(message) => notify(message, "error")}
          />
        ))}
      </div>

      {blocks.length === 0 && (
        <div className="library-empty">
          <p className="library-empty-title">{t("library.empty.title")}</p>
          <p className="hint">{t("library.empty.hint")}</p>
        </div>
      )}

      {totalPages > 1 && (
        <div className="pager library-pager">
          <button
            type="button"
            className="btn btn-ghost btn-sm"
            disabled={page <= 1}
            onClick={() => setPage((p) => p - 1)}
          >
            {t("library.pagination.previous")}
          </button>
          <span className="hint">
            {page} / {totalPages}
          </span>
          <button
            type="button"
            className="btn btn-ghost btn-sm"
            disabled={page >= totalPages}
            onClick={() => setPage((p) => p + 1)}
          >
            {t("library.pagination.next")}
          </button>
        </div>
      )}
      </div>
        </div>
      </div>

      {confirmBatchDelete && (
        <div className="modal-backdrop">
          <div className="modal glass">
            <h3>{t("library.delete.batchTitle", { count: selected.size })}</h3>
            <p className="hint">{t("library.delete.batchHint")}</p>
            <div className="btn-row">
              <button
                type="button"
                className="btn btn-danger"
                onClick={async () => {
                  try {
                    const r = await api.deletePatternsBatch([...selected]);
                    notify(t("library.toast.deleted", { count: r.removed }), "ok");
                    clearSelection();
                    await load();
                  } catch (e) {
                    notify(e instanceof Error ? e.message : t("common.error"), "error");
                  } finally {
                    setConfirmBatchDelete(false);
                  }
                }}
              >
                {t("library.delete.batchConfirm")}
              </button>
              <button
                type="button"
                className="btn btn-ghost"
                onClick={() => setConfirmBatchDelete(false)}
              >
                {t("common.cancel")}
              </button>
            </div>
          </div>
        </div>
      )}

      {confirmDelete && (
        <div className="modal-backdrop">
          <div className="modal glass">
            <h3>{t("library.delete.title")}</h3>
            <p className="hint">{t("library.delete.hint")}</p>
            <p className="mono">{confirmDelete.slice(0, 32)}…</p>
            <div className="btn-row">
              <button
                type="button"
                className="btn btn-danger"
                onClick={async () => {
                  try {
                    await api.deletePattern(confirmDelete);
                    notify(t("library.toast.blockDeleted"), "ok");
                    await load();
                  } catch (e) {
                    notify(e instanceof Error ? e.message : t("common.error"), "error");
                  } finally {
                    setConfirmDelete(null);
                  }
                }}
              >
                {t("library.delete.confirm")}
              </button>
              <button
                type="button"
                className="btn btn-ghost"
                onClick={() => setConfirmDelete(null)}
              >
                {t("common.cancel")}
              </button>
            </div>
          </div>
        </div>
      )}

      {confirmTest && (
        <div className="modal-backdrop">
          <div className="modal glass">
            <h3>{t("library.test.title")}</h3>
            <p className="hint">{t("library.test.hint")}</p>
            <p className="mono">{confirmTest.slice(0, 32)}…</p>
            <div className="btn-row">
              <button
                type="button"
                className="btn btn-primary"
                onClick={async () => {
                  try {
                    const r = await api.testPatternDevice(confirmTest);
                    notify(t("library.toast.mockOk", { count: r.actions_played ?? 0 }), "ok");
                  } catch (e) {
                    notify(e instanceof Error ? e.message : t("common.error"), "error");
                  } finally {
                    setConfirmTest(null);
                  }
                }}
              >
                {t("common.confirm")}
              </button>
              <button
                type="button"
                className="btn btn-ghost"
                onClick={() => setConfirmTest(null)}
              >
                {t("common.cancel")}
              </button>
            </div>
          </div>
        </div>
      )}

      {editingBlock && (
        <BlockEditorModal
          block={editingBlock}
          onClose={() => setEditingBlock(null)}
          onSaved={() => void load()}
          notify={notify}
        />
      )}
    </>
  );
}

function ImportPreviewCard({
  title,
  subtitle,
  preview,
  actions,
  bpm,
  badge,
  variant = "block",
  scriptDurationMs,
  heatmapStats,
}: {
  title: string;
  subtitle: string;
  preview: ImportBlockSummary["preview"];
  actions?: ImportBlockSummary["actions"];
  bpm?: number | null;
  badge?: string;
  variant?: "block" | "full";
  scriptDurationMs?: number | null;
  heatmapStats?: ImportBlockSummary["heatmap_stats"];
}) {
  return (
    <article className={`import-preview-card glass import-preview-card--${variant}`}>
      {badge && <span className="import-preview-badge">{badge}</span>}
      <BlockHeatmap
        actions={actions}
        points={preview ?? []}
        bpm={bpm ?? null}
        isFullScript={variant === "full"}
        scriptDurationMs={scriptDurationMs ?? null}
        heatmapStats={heatmapStats ?? null}
      />
      <div className="import-preview-body">
        <strong>{title}</strong>
        <span className="hint">{subtitle}</span>
      </div>
    </article>
  );
}

function ImportResultPanel({
  result,
  onRefreshImports,
}: {
  result: ImportResult;
  onRefreshImports?: () => void;
}) {
  const { t } = useTranslation();
  const { notify } = useToast();
  const blocks = result.imported_blocks ?? [];
  const fullBlock = result.imported_full_block;
  const full = result.full_script;
  const inserted = result.persisted?.blocks_inserted ?? 0;
  const skippedHash = result.persisted?.blocks_skipped_content_hash ?? 0;

  return (
    <div className="glass import-result">
      <h3>{String(result.source?.filename ?? t("library.import.sourceDefault"))}</h3>
      <div className="result-stats">
        <div>
          <span>{t("library.import.stats.inserted")}</span>
          <strong>{inserted}</strong>
        </div>
        <div>
          <span>{t("library.import.stats.inFile")}</span>
          <strong>{result.summary?.block_count ?? blocks.length}</strong>
        </div>
        <div>
          <span>{t("library.import.stats.format")}</span>
          <strong>{String(result.source?.source_format ?? "—")}</strong>
        </div>
        {skippedHash > 0 && (
          <div>
            <span>{t("library.import.stats.duplicates")}</span>
            <strong>{skippedHash}</strong>
          </div>
        )}
      </div>

      {full && (
        <section className="import-preview-section">
          <h4>{t("library.import.fullScriptTitle")}</h4>
          <ImportPreviewCard
            variant="full"
            badge={fullBlock?.inserted === false ? t("library.import.badgeExists") : t("library.import.badgeFull")}
            title={fullBlock?.display_name ?? (full.filename || t("library.import.fullScriptDefault"))}
            subtitle={t("library.import.fullSubtitle", {
              duration: (full.duration_ms / 1000).toFixed(1),
              points: full.action_count,
              bpmSuffix: full.bpm
                ? t("library.import.bpmSuffix", { bpm: full.bpm.toFixed(0) })
                : "",
              librarySuffix: full.block_id ? t("library.import.subtitleInLibrary") : "",
            })}
            preview={fullBlock?.preview ?? full.preview}
            actions={fullBlock?.actions ?? full.actions}
            bpm={fullBlock?.bpm ?? full.bpm}
            scriptDurationMs={fullBlock?.script_duration_ms ?? full.script_duration_ms}
            heatmapStats={fullBlock?.heatmap_stats ?? full.heatmap_stats}
          />
          <div className="btn-row">
            <button
              type="button"
              className="btn btn-primary btn-sm"
              onClick={async () => {
                try {
                  const res = await api.enqueueFullScript(full.file_id);
                  notify(
                    res.enqueued
                      ? t("library.import.enqueued", { count: res.enqueued })
                      : t("library.toast.queueFull"),
                    res.enqueued ? "ok" : "error",
                  );
                } catch (e) {
                  notify(e instanceof Error ? e.message : t("common.error"), "error");
                }
              }}
            >
              {t("library.import.enqueueFull")}
            </button>
            <button
              type="button"
              className="btn btn-ghost btn-sm"
              onClick={async () => {
                try {
                  await api.playFullScript(full.file_id);
                  notify(t("library.import.playingFull"), "ok");
                } catch (e) {
                  notify(e instanceof Error ? e.message : t("common.error"), "error");
                }
              }}
            >
              {t("library.import.playNow")}
            </button>
          </div>
        </section>
      )}

      {blocks.length > 0 && (
        <section className="import-preview-section">
          <h4>{t("library.import.generatedBlocks", { count: blocks.length })}</h4>
          <div className="import-blocks-grid">
            {blocks.map((b) => (
              <ImportPreviewCard
                key={b.id}
                title={b.display_name}
                badge={b.inserted === false ? t("library.import.badgeExists") : undefined}
                subtitle={t("library.import.blockSubtitle", {
                  zone: b.zone ?? "?",
                  speed: b.speed ?? "?",
                  duration: (b.duration_ms / 1000).toFixed(1),
                  intensity: b.intensity?.toFixed(0) ?? "—",
                  points: b.action_count,
                })}
                preview={b.preview}
                actions={b.actions}
                bpm={b.bpm}
              />
            ))}
          </div>
        </section>
      )}

      <div className="btn-row">
        {result.persisted?.file_id &&
          ["funscript", "csv", "json"].map((fmt) => (
            <button
              key={fmt}
              type="button"
              className="btn btn-ghost btn-sm"
              onClick={async () => {
                const id = result.persisted!.file_id;
                try {
                  const { filename, content } = await api.exportImport(id, fmt);
                  downloadText(filename, content);
                  notify(filename, "ok");
                } catch (e) {
                  notify(e instanceof Error ? e.message : t("common.error"), "error");
                }
              }}
            >
              {t("library.import.exportFmt", { fmt })}
            </button>
          ))}
        <button
          type="button"
          className="btn btn-ghost btn-sm"
          onClick={() => onRefreshImports?.()}
        >
          {t("library.import.refreshList")}
        </button>
      </div>
    </div>
  );
}

function ImportTab({ onImported }: { onImported?: () => void }) {
  const { t } = useTranslation();
  const { notify } = useToast();
  const [results, setResults] = useState<ImportResult[]>([]);
  const [uploading, setUploading] = useState(false);
  const [uploadProgress, setUploadProgress] = useState("");
  const [drag, setDrag] = useState(false);
  const [imports, setImports] = useState<import("../api/types").FunscriptFileEntry[]>([]);
  const [rosterEnabled, setRosterEnabled] = useState(false);
  const [rosterBusy, setRosterBusy] = useState(false);
  const [rosterMsg, setRosterMsg] = useState("");

  const loadImports = useCallback(async () => {
    try {
      const res = await api.listImports(40);
      setImports(res.files ?? []);
    } catch {
      /* ignore */
    }
  }, []);

  useEffect(() => {
    loadImports();
    api
      .getSettings()
      .then((s) => {
        const planner = (s.planner ?? {}) as Record<string, unknown>;
        setRosterEnabled(planner.session_roster_enabled === true);
      })
      .catch(() => {});
  }, [loadImports]);

  const importFiles = async (files: FileList | File[]) => {
    const list = [...files].filter((f) => {
      const ext = f.name.toLowerCase();
      return (
        ext.endsWith(".funscript") || ext.endsWith(".json") || ext.endsWith(".csv")
      );
    });
    if (list.length === 0) {
      notify(t("library.import.invalidExt"), "error");
      return;
    }

    setUploading(true);
    const newResults: ImportResult[] = [];
    let totalInserted = 0;

    try {
      for (let i = 0; i < list.length; i++) {
        const file = list[i];
        setUploadProgress(`${i + 1}/${list.length}: ${file.name}`);
        const res = await api.importFile(file);
        newResults.push(res);
        totalInserted += res.persisted?.blocks_inserted ?? 0;
      }
      setResults((prev) => [...newResults, ...prev].slice(0, 12));
      await loadImports();
      onImported?.();
      notify(
        t("library.import.done", { files: list.length, blocks: totalInserted }),
        totalInserted > 0 ? "ok" : "error",
      );
    } catch (err) {
      notify(err instanceof Error ? err.message : t("common.error"), "error");
    } finally {
      setUploading(false);
      setUploadProgress("");
    }
  };

  const enableRoster = async () => {
    try {
      const s = await api.getSettings();
      const planner = (s.planner ?? {}) as Record<string, unknown>;
      await api.saveSettings({
        planner: { ...planner, session_roster_enabled: true },
      });
      setRosterEnabled(true);
      notify(t("library.import.rosterEnabled"), "ok");
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  return (
    <div className="import-zone import-workspace">
      <div className="import-workspace-primary">
      <label
        className={`dropzone glass${drag ? " drag" : ""}`}
        onDragOver={(e) => {
          e.preventDefault();
          setDrag(true);
        }}
        onDragLeave={() => setDrag(false)}
        onDrop={(e) => {
          e.preventDefault();
          setDrag(false);
          if (e.dataTransfer.files.length) void importFiles(e.dataTransfer.files);
        }}
      >
        <input
          type="file"
          accept=".funscript,.json,.csv"
          multiple
          hidden
          disabled={uploading}
          onChange={(e) => {
            if (e.target.files?.length) void importFiles(e.target.files);
            e.target.value = "";
          }}
        />
        <span className="drop-icon">↑</span>
        <strong>{t("library.drop.title")}</strong>
        <span className="hint">{t("library.import.dropHint")}</span>
        {uploading && (
          <span className="pill">
            {uploadProgress
              ? t("library.import.importing", { progress: uploadProgress })
              : t("library.import.processing")}
          </span>
        )}
      </label>

      <section className="glass import-roster-panel">
        <h3>{t("library.import.rosterTitle")}</h3>
        <p className="hint">{t("library.import.sessionHint")}</p>
        {!rosterEnabled ? (
          <button type="button" className="btn btn-sm btn-primary" onClick={enableRoster}>
            {t("library.import.enableRoster")}
          </button>
        ) : (
          <div className="btn-row">
            <button
              type="button"
              className="btn btn-sm btn-primary"
              disabled={rosterBusy || imports.length === 0}
              onClick={async () => {
                setRosterBusy(true);
                setRosterMsg("");
                try {
                  const res = await api.planRosterSession({ message: rosterMsg });
                  notify(
                    res.enqueued
                      ? t("library.import.rosterEnqueued", { count: res.enqueued })
                      : t("library.import.rosterBuilt"),
                    "ok",
                  );
                } catch (e) {
                  notify(e instanceof Error ? e.message : t("common.error"), "error");
                } finally {
                  setRosterBusy(false);
                }
              }}
            >
              {rosterBusy ? t("library.import.buildingRoster") : t("library.import.buildRoster")}
            </button>
            <input
              className="mono"
              placeholder={t("library.import.optionalInstruction")}
              value={rosterMsg}
              onChange={(e) => setRosterMsg(e.target.value)}
            />
          </div>
        )}
      </section>
      </div>

      <div className="import-workspace-side">
      {imports.length > 0 && (
        <section className="glass import-files-panel">
          <h3>{t("library.import.filesTitle", { count: imports.length })}</h3>
          <ul className="import-files-list">
            {imports.map((f) => (
              <li key={f.file_id} className="import-file-row">
                <div>
                  <strong>{f.display_filename ?? f.filename}</strong>
                  <span className="hint">
                    {t("library.import.fileMeta", {
                      duration: f.duration_sec,
                      blocks: f.block_count,
                      points: f.action_count,
                    })}
                  </span>
                </div>
                <div className="btn-row">
                  <button
                    type="button"
                    className="btn btn-sm btn-ghost"
                    onClick={async () => {
                      try {
                        const res = await api.enqueueFullScript(f.file_id);
                        notify(
                          res.enqueued
                            ? t("library.import.enqueued", { count: res.enqueued })
                            : t("library.toast.queueFull"),
                          res.enqueued ? "ok" : "error",
                        );
                      } catch (e) {
                        notify(e instanceof Error ? e.message : t("common.error"), "error");
                      }
                    }}
                  >
                    {t("library.import.enqueue")}
                  </button>
                  <button
                    type="button"
                    className="btn btn-sm btn-ghost"
                    onClick={async () => {
                      try {
                        await api.playFullScript(f.file_id);
                        notify(t("library.import.playingScript"), "ok");
                      } catch (e) {
                        notify(e instanceof Error ? e.message : t("common.error"), "error");
                      }
                    }}
                  >
                    {t("library.import.play")}
                  </button>
                </div>
              </li>
            ))}
          </ul>
        </section>
      )}

      {results.length > 0 && (
        <section className="import-results-stack glass">
          <h3 className="import-results-head">{t("library.import.recentTitle")}</h3>
          {results.map((r, idx) => (
            <ImportResultPanel
              key={`${r.persisted?.file_id ?? r.source?.filename ?? idx}-${idx}`}
              result={r}
              onRefreshImports={loadImports}
            />
          ))}
        </section>
      )}
      </div>
    </div>
  );
}
