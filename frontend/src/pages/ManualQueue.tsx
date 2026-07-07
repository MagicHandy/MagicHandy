import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, downloadText } from "../api/client";
import type {
  ManualQueueDraft,
  ManualQueuePreview,
  MotionBlock,
  SavedQueueSummary,
  SignalPreset,
  StatusSnapshot,
} from "../api/types";
import { PlayerPreviewChart } from "../components/PlayerPreviewChart";
import { PlayerBlockBrowser } from "../components/PlayerBlockBrowser";
import type { PlayerBlockFilters } from "../components/PlayerBlockBrowser";
import {
  PlayerCollapsiblePanel,
  PlayerPanelToggles,
  type PlayerPanelKey,
} from "../components/PlayerCollapsiblePanel";
import { QueueDragList } from "../components/QueueDragList";
import { UiCheckbox } from "../components/UiCheckbox";
import { useToast } from "../contexts/ToastContext";
import {
  bucketToBpmParams,
  bucketToDurationParams,
  EMPTY_PLAYER_BLOCK_FILTERS,
} from "../utils/rangeBuckets";

const LIB_PAGE = 30;

function formatDuration(sec: number) {
  const m = Math.floor(sec / 60);
  const s = Math.round(sec % 60);
  return m > 0 ? `${m}m ${s}s` : `${s}s`;
}

function exportQueue(
  fetcher: (fmt: string) => Promise<{ filename: string; content: string }>,
  fmt: string,
  notify: (msg: string, kind: "ok" | "error") => void,
  t: (key: string, opts?: Record<string, unknown>) => string,
) {
  fetcher(fmt)
    .then(({ filename, content }) => {
      downloadText(filename, content);
      notify(t("player.downloaded", { filename }), "ok");
    })
    .catch((e) => notify(e instanceof Error ? e.message : t("common.error"), "error"));
}

type SignalKind = "edging" | "climax" | "milking";

const SIGNAL_DEFAULT_SEC: Record<SignalKind, number> = {
  edging: 90,
  milking: 120,
  climax: 60,
};

function signalLabel(t: (key: string) => string, kind: SignalKind) {
  return t(`player.signals.${kind}`);
}

function SignalButtonBar({
  presets,
  busy,
  onPlay,
  onConfigure,
}: {
  presets: Record<string, SignalPreset>;
  busy: boolean;
  onPlay: (kind: SignalKind) => void;
  onConfigure: (kind: SignalKind) => void;
}) {
  const { t } = useTranslation();
  const kinds: SignalKind[] = ["edging", "milking", "climax"];
  return (
    <div className="manual-queue-signal-bar player-signals">
      <span className="section-label player-signals-label">{t("player.signals.label")}</span>
      {kinds.map((kind) => {
        const preset = presets[kind];
        const label = signalLabel(t, kind);
        const configured = Boolean(preset?.configured && preset.block_id);
        return (
          <div key={kind} className="signal-btn">
            <button
              type="button"
              className={`btn btn-sm signal-btn-play${configured ? " btn-primary" : " btn-ghost"}`}
              disabled={busy}
              title={
                configured
                  ? t("player.signals.playTitle", {
                      label,
                      name: preset?.display_name ?? preset?.block_id ?? "",
                    })
                  : t("player.signals.configureTitle", { label })
              }
              onClick={() => onPlay(kind)}
            >
              ▶ {label}
            </button>
            <button
              type="button"
              className="btn btn-ghost btn-sm signal-btn-config"
              title={t("player.signals.configureTitle", { label })}
              disabled={busy}
              onClick={() => onConfigure(kind)}
            >
              ⚙
            </button>
          </div>
        );
      })}
    </div>
  );
}

function QueuePlayerPanel({
  progress,
  preview,
  autoloop,
  onAutoloop,
  onPause,
  onResume,
  onStop,
  onSkip,
  busy,
}: {
  progress: StatusSnapshot;
  preview: ManualQueuePreview | null;
  autoloop: boolean;
  onAutoloop: (v: boolean) => void;
  onPause: () => void;
  onResume: () => void;
  onStop: () => void;
  onSkip: () => void;
  busy: boolean;
}) {
  const { t } = useTranslation();
  const paused = progress.manual_queue_paused ?? false;
  const scriptMode = progress.manual_queue_playback_mode === "script";
  const playheadMs = progress.manual_queue_playhead_ms ?? 0;
  const currentSeg = progress.manual_queue_current_segment_index ?? 0;
  const segCount = progress.manual_queue_segment_count ?? 0;
  const signalLabel = progress.manual_queue_signal_label;

  return (
    <section className="manual-queue-player manual-queue-player--live">
      <div className="manual-queue-player-top manual-queue-player-top--compact">
        <div>
          <strong className="manual-queue-player-title">
            {progress.manual_queue_signal_active && signalLabel
              ? t("player.live.signal", { label: signalLabel })
              : progress.manual_queue_name ?? t("player.live.queue")}
          </strong>
          <span className="hint manual-queue-player-meta">
            {scriptMode ? t("player.live.hsspMode") : ""}
            {t("player.live.block", {
              current: Math.min(currentSeg + 1, Math.max(segCount, 1)),
              total: Math.max(segCount, 1),
            })}
            {formatDuration(progress.manual_queue_elapsed_sec ?? 0)} /{" "}
            {formatDuration(progress.manual_queue_duration_sec ?? 0)} · {t("player.live.pos")}{" "}
            {(progress.motion_position_pct ?? 0).toFixed(0)}%
          </span>
        </div>
        <label className="hint manual-queue-autoloop-toggle">
          <UiCheckbox
            label={t("player.live.autoloop")}
            checked={autoloop}
            onChange={(e) => onAutoloop(e.target.checked)}
          />
        </label>
      </div>

      {preview?.ok && preview.actions && (
        <PlayerPreviewChart
          actions={preview.actions}
          points={preview.preview}
          timelineDurationMs={preview.duration_ms}
          scriptDurationMs={preview.script_duration_ms ?? preview.duration_ms}
          heatmapStats={preview.heatmap_stats ?? null}
          playheadMs={playheadMs}
          segments={preview.segments}
          durationMs={preview.duration_ms}
          currentSegmentIndex={currentSeg}
          live
        />
      )}

      <div className="manual-queue-player-controls">
        {!scriptMode &&
          (paused ? (
            <button type="button" className="btn btn-primary" disabled={busy} onClick={onResume}>
              {t("player.live.resume")}
            </button>
          ) : (
            <button type="button" className="btn btn-primary" disabled={busy} onClick={onPause}>
              {t("player.live.pause")}
            </button>
          ))}
        {!scriptMode && (
          <button type="button" className="btn btn-ghost" disabled={busy} onClick={onSkip}>
            {t("player.live.skip")}
          </button>
        )}
        <button type="button" className="btn btn-ghost btn-danger-outline" disabled={busy} onClick={onStop}>
          {t("player.live.stop")}
        </button>
      </div>
    </section>
  );
}

export function ManualQueue() {
  const { t } = useTranslation();
  const { notify } = useToast();
  const [draft, setDraft] = useState<ManualQueueDraft | null>(null);
  const [saved, setSaved] = useState<SavedQueueSummary[]>([]);
  const [blocks, setBlocks] = useState<MotionBlock[]>([]);
  const [totalBlocks, setTotalBlocks] = useState(0);
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [searchInput, setSearchInput] = useState("");
  const [blockFilters, setBlockFilters] = useState<PlayerBlockFilters>(
    EMPTY_PLAYER_BLOCK_FILTERS,
  );
  const [panelsVisible, setPanelsVisible] = useState<Record<PlayerPanelKey, boolean>>({
    preview: true,
    queue: true,
    library: true,
  });
  const [panelsExpanded, setPanelsExpanded] = useState<Record<PlayerPanelKey, boolean>>({
    preview: true,
    queue: true,
    library: true,
  });

  const togglePanelVisible = (key: PlayerPanelKey) => {
    setPanelsVisible((prev) => {
      const next = { ...prev, [key]: !prev[key] };
      if (!next.preview && !next.queue && !next.library) return prev;
      return next;
    });
  };

  const togglePanelExpanded = (key: PlayerPanelKey) => {
    setPanelsExpanded((prev) => ({ ...prev, [key]: !prev[key] }));
  };
  const [defaultLoop, setDefaultLoop] = useState(false);
  const [saveName, setSaveName] = useState("");
  const [showSave, setShowSave] = useState(false);
  const [playing, setPlaying] = useState(false);
  const [busy, setBusy] = useState(false);
  const [preview, setPreview] = useState<ManualQueuePreview | null>(null);
  const [playProgress, setPlayProgress] = useState<StatusSnapshot | null>(null);
  const [autoloop, setAutoloop] = useState(false);
  const [signalPresets, setSignalPresets] = useState<Record<string, SignalPreset>>({});
  const [signalModal, setSignalModal] = useState<SignalKind | null>(null);
  const [signalBlockId, setSignalBlockId] = useState("");
  const [signalDurationSec, setSignalDurationSec] = useState(90);
  const [pickerSearch, setPickerSearch] = useState("");
  const [pickerBlocks, setPickerBlocks] = useState<MotionBlock[]>([]);
  const pollRef = useRef<number | null>(null);

  const loadSignalPresets = useCallback(async () => {
    try {
      const r = await api.getManualQueueSignalPresets();
      setSignalPresets(r.presets ?? {});
    } catch {
      /* ignore */
    }
  }, []);

  const stopProgressPoll = () => {
    if (pollRef.current != null) {
      window.clearInterval(pollRef.current);
      pollRef.current = null;
    }
  };

  const startProgressPoll = () => {
    stopProgressPoll();
    pollRef.current = window.setInterval(() => {
      api
        .getStatus()
        .then((status) => {
          setPlayProgress(status);
          if (!status.manual_queue_playing) {
            setPlaying(false);
            stopProgressPoll();
          }
        })
        .catch(() => {});
    }, 250);
  };

  useEffect(() => () => stopProgressPoll(), []);

  const loadDraft = useCallback(async () => {
    const d = await api.getManualQueueDraft();
    setDraft(d);
  }, []);

  const loadSaved = useCallback(async () => {
    const r = await api.listSavedQueues();
    setSaved(r.queues);
  }, []);

  const loadBlocks = useCallback(async () => {
    const offset = (page - 1) * LIB_PAGE;
    const params: Record<string, string | number | boolean> = {
      offset,
      limit: LIB_PAGE,
      hide_blocked: true,
    };
    if (search.trim()) params.q = search.trim();
    if (blockFilters.speed) params.speed = blockFilters.speed;
    Object.assign(params, bucketToBpmParams(blockFilters.bpmRange));
    Object.assign(params, bucketToDurationParams(blockFilters.durationRange));
    const listRes = await api.listPatterns(params);
    setBlocks(listRes.blocks);
    const countRes = await api.countPatterns(params).catch(() => ({ total: 0 }));
    setTotalBlocks(
      listRes.total ?? countRes.total ?? listRes.blocks.length,
    );
  }, [page, search, blockFilters]);

  useEffect(() => {
    loadDraft().catch((e) =>
      notify(e instanceof Error ? e.message : t("common.error"), "error"),
    );
    loadSaved().catch(() => {});
    loadSignalPresets().catch(() => {});
  }, [loadDraft, loadSaved, loadSignalPresets, notify]);

  useEffect(() => {
    loadBlocks().catch((e) =>
      notify(e instanceof Error ? e.message : t("common.error"), "error"),
    );
  }, [loadBlocks, notify]);

  useEffect(() => {
    setPage(1);
  }, [blockFilters]);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setPage(1);
      setSearch(searchInput.trim());
    }, 350);
    return () => window.clearTimeout(timer);
  }, [searchInput]);

  useEffect(() => {
    if (!signalModal) return;
    const timer = window.setTimeout(() => {
      const params: Record<string, string | number | boolean> = {
        offset: 0,
        limit: 80,
        hide_blocked: true,
      };
      if (pickerSearch.trim()) params.q = pickerSearch.trim();
      api
        .listPatterns(params)
        .then((r) => setPickerBlocks(r.blocks))
        .catch(() => setPickerBlocks([]));
    }, 220);
    return () => window.clearTimeout(timer);
  }, [signalModal, pickerSearch]);

  const items = draft?.items ?? [];

  useEffect(() => {
    if (!items.length) {
      setPreview(null);
      return;
    }
    const timer = window.setTimeout(() => {
      api
        .getManualQueuePreview()
        .then(setPreview)
        .catch(() => setPreview(null));
    }, 350);
    return () => window.clearTimeout(timer);
  }, [draft]);

  const addBlock = async (block: MotionBlock) => {
    setBusy(true);
    try {
      const scriptSec = Math.max(1, Math.ceil((block.duration_ms ?? 0) / 1000));
      const d = await api.addManualQueueItem({
        block_id: block.id,
        duration_sec: scriptSec,
        loop: defaultLoop,
      });
      setDraft(d);
      notify(t("player.toast.added"), "ok");
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    } finally {
      setBusy(false);
    }
  };

  const patchItem = async (
    index: number,
    patch: { duration_sec?: number; loop?: boolean },
  ) => {
    try {
      const d = await api.patchManualQueueItem(index, patch);
      setDraft(d);
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  const moveItem = async (from: number, to: number) => {
    if (to < 0 || !draft || to >= draft.items.length) return;
    try {
      const d = await api.reorderManualQueue(from, to);
      setDraft(d);
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  const removeItem = async (index: number) => {
    try {
      const d = await api.removeManualQueueItem(index);
      setDraft(d);
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  const runPlay = async (
    playFn: () => Promise<{ sync?: { offset_ms?: number }; duration_ms?: number; autoloop?: boolean }>,
    label: string,
  ) => {
    setPlaying(true);
    setPlayProgress(null);
    startProgressPoll();
    try {
      const r = await playFn();
      if (r.autoloop != null) setAutoloop(r.autoloop);
      const syncMs = r.sync?.offset_ms;
      notify(
        t("player.toast.playStarted", {
          label,
          sync: syncMs != null ? t("player.toast.playSync", { ms: syncMs }) : "",
        }),
        "ok",
      );
    } catch (e) {
      stopProgressPoll();
      setPlaying(false);
      setPlayProgress(null);
      notify(e instanceof Error ? e.message : t("player.toast.playError"), "error");
    }
  };

  const onPause = async () => {
    setBusy(true);
    try {
      await api.pauseManualQueuePlayer();
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    } finally {
      setBusy(false);
    }
  };

  const onResume = async () => {
    setBusy(true);
    try {
      await api.resumeManualQueuePlayer();
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    } finally {
      setBusy(false);
    }
  };

  const onStopPlayer = async () => {
    setBusy(true);
    try {
      await api.stopManualQueuePlayer();
      stopProgressPoll();
      setPlaying(false);
      setPlayProgress(null);
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    } finally {
      setBusy(false);
    }
  };

  const onSkip = async () => {
    setBusy(true);
    try {
      await api.skipManualQueueSegment();
      notify(t("player.toast.nextBlock"), "ok");
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    } finally {
      setBusy(false);
    }
  };

  const onAutoloopChange = async (enabled: boolean) => {
    setAutoloop(enabled);
    try {
      await api.setManualQueueAutoloop(enabled);
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  const fireSignal = async (
    kind: SignalKind,
    blockId: string,
    durationSec: number,
  ) => {
    setBusy(true);
    try {
      await api.manualQueueSignal({
        signal: kind,
        block_id: blockId,
        duration_sec: durationSec,
      });
      if (!playing) {
        setPlaying(true);
        startProgressPoll();
      }
      await loadSignalPresets();
      notify(t("player.toast.signalFired", { label: signalLabel(t, kind), sec: durationSec }), "ok");
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    } finally {
      setBusy(false);
    }
  };

  const openSignalModal = (kind: SignalKind) => {
    const preset = signalPresets[kind];
    setSignalDurationSec(
      preset?.duration_sec ?? SIGNAL_DEFAULT_SEC[kind],
    );
    setSignalBlockId(preset?.block_id ?? "");
    setPickerSearch("");
    setSignalModal(kind);
  };

  const handleSignalPlay = async (kind: SignalKind) => {
    const preset = signalPresets[kind];
    if (preset?.configured && preset.block_id) {
      await fireSignal(
        kind,
        preset.block_id,
        preset.duration_sec ?? SIGNAL_DEFAULT_SEC[kind],
      );
      return;
    }
    openSignalModal(kind);
  };

  const confirmSignal = async () => {
    if (!signalModal) return;
    if (!signalBlockId) {
      notify(t("player.toast.pickScript"), "error");
      return;
    }
    await fireSignal(signalModal, signalBlockId, signalDurationSec);
    setSignalModal(null);
  };

  const onPlay = async () => {
    if (!draft?.items.length) {
      notify(t("player.toast.buildQueue"), "error");
      return;
    }
    await runPlay(() => api.playManualQueue(), t("player.toast.queueDone"));
  };

  const onSave = async () => {
    if (!saveName.trim()) {
      notify(t("player.toast.enterName"), "error");
      return;
    }
    setBusy(true);
    try {
      const r = await api.saveManualQueue(saveName.trim());
      notify(t("player.toast.savedAs", { name: r.name }), "ok");
      setShowSave(false);
      setSaveName("");
      await loadSaved();
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    } finally {
      setBusy(false);
    }
  };

  const onLoadSaved = async (id: string) => {
    try {
      const d = await api.loadSavedQueue(id);
      setDraft(d);
      notify(t("player.toast.queueLoaded"), "ok");
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  const onPlaySaved = async (id: string, name: string) => {
    await runPlay(() => api.playSavedQueue(id), name);
  };

  const onDeleteSaved = async (id: string) => {
    try {
      await api.deleteSavedQueue(id);
      await loadSaved();
      notify(t("player.toast.queueRemoved"), "ok");
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  const totalPages = Math.max(1, Math.ceil(totalBlocks / LIB_PAGE));
  const showPlayer =
    playing &&
    playProgress?.manual_queue_playing &&
    playProgress != null;

  return (
    <div className={`page page--fill player-page${showPlayer ? " player-page--playing" : ""}`}>
      <header className="page-toolbar player-toolbar">
        <div className="player-toolbar-main">
          <div className="player-toolbar-info">
            <span className="section-label">{t("player.title")}</span>
            <span className="hint">
              {t("player.toolbar.blocks", {
                count: items.length,
                duration: formatDuration(draft?.total_duration_sec ?? 0),
              })}
            </span>
          </div>
          <div className="player-toolbar-actions">
            <button
              type="button"
              className="btn btn-primary"
              disabled={playing || !items.length}
              onClick={onPlay}
            >
              {playing ? t("player.toolbar.playing") : t("player.toolbar.play")}
            </button>
            {playing && (
              <button
                type="button"
                className="btn btn-ghost"
                disabled={busy}
                onClick={onStopPlayer}
              >
                {t("player.toolbar.stop")}
              </button>
            )}
            <button
              type="button"
              className="btn btn-ghost"
              disabled={!items.length}
              onClick={() => setShowSave(true)}
            >
              {t("player.toolbar.save")}
            </button>
            <select
              className="export-select"
              defaultValue=""
              disabled={!items.length}
              title={t("player.toolbar.exportTitle")}
              onChange={(e) => {
                if (!e.target.value) return;
                exportQueue(
                  (fmt) => api.exportManualQueueDraft(fmt),
                  e.target.value,
                  notify,
                  t,
                );
                e.target.value = "";
              }}
            >
              <option value="">{t("player.toolbar.export")}</option>
              <option value="funscript">.funscript</option>
              <option value="csv">.csv</option>
              <option value="json">.json</option>
            </select>
            <button
              type="button"
              className="btn btn-ghost btn-sm"
              disabled={!items.length}
              onClick={async () => {
                try {
                  const d = await api.clearManualQueue();
                  setDraft(d);
                } catch (e) {
                  notify(e instanceof Error ? e.message : t("common.error"), "error");
                }
              }}
            >
              {t("player.toolbar.clear")}
            </button>
          </div>
        </div>
        <SignalButtonBar
          presets={signalPresets}
          busy={busy}
          onPlay={handleSignalPlay}
          onConfigure={openSignalModal}
        />
        <PlayerPanelToggles panels={panelsVisible} onToggle={togglePanelVisible} />
      </header>

      <div className="player-body">
      {showSave && (
        <div className="glass player-save-bar">
          <input
            type="text"
            placeholder={t("player.toolbar.saveNamePlaceholder")}
            value={saveName}
            onChange={(e) => setSaveName(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && onSave()}
          />
          <button
            type="button"
            className="btn btn-primary btn-sm"
            disabled={busy}
            onClick={onSave}
          >
            {t("player.toolbar.confirm")}
          </button>
          <button
            type="button"
            className="btn btn-ghost btn-sm"
            onClick={() => setShowSave(false)}
          >
            {t("common.cancel")}
          </button>
        </div>
      )}

      <div className="player-panel-stack">
        {panelsVisible.preview && (
          <PlayerCollapsiblePanel
            variant="preview"
            title={t("player.panels.preview")}
            subtitle={
              showPlayer
                ? t("player.preview.playingNow")
                : preview?.ok
                  ? t("player.preview.actionsDuration", {
                      count: preview.action_count ?? 0,
                      duration: formatDuration(preview.total_duration_sec ?? 0),
                    })
                  : t("player.preview.queueScript")
            }
            open={panelsExpanded.preview}
            onToggle={() => togglePanelExpanded("preview")}
          >
            <div className="player-preview-content">
              {showPlayer && playProgress ? (
                <QueuePlayerPanel
                  progress={playProgress}
                  preview={preview}
                  autoloop={autoloop || Boolean(playProgress.manual_queue_autoloop)}
                  onAutoloop={onAutoloopChange}
                  onPause={onPause}
                  onResume={onResume}
                  onStop={onStopPlayer}
                  onSkip={onSkip}
                  busy={busy}
                />
              ) : preview?.ok && preview.preview.length > 0 ? (
                <PlayerPreviewChart
                  actions={preview.actions}
                  points={preview.preview}
                  timelineDurationMs={preview.duration_ms}
                  scriptDurationMs={preview.script_duration_ms ?? preview.duration_ms}
                  heatmapStats={preview.heatmap_stats ?? null}
                  segments={preview.segments}
                  durationMs={preview.duration_ms}
                />
              ) : (
                <div className="player-queue-empty">
                  <p className="hint">
                    {t("player.preview.empty")}
                  </p>
                </div>
              )}
            </div>
          </PlayerCollapsiblePanel>
        )}

        {panelsVisible.queue && (
          <PlayerCollapsiblePanel
            variant="queue"
            title={t("player.queue.title")}
            subtitle={t("player.queue.subtitle", {
              count: items.length,
              duration: formatDuration(draft?.total_duration_sec ?? 0),
            })}
            open={panelsExpanded.queue}
            onToggle={() => togglePanelExpanded("queue")}
          >
            {!items.length ? (
              <div className="player-queue-empty">
                <p className="hint empty-hint">
                  {t("player.queue.empty")}
                </p>
              </div>
            ) : (
              <QueueDragList
                items={items}
                disabled={busy || playing}
                onReorder={moveItem}
                onPatch={patchItem}
                onRemove={removeItem}
              />
            )}
            {saved.length > 0 && (
              <details className="player-saved-details">
                <summary className="player-saved-summary">
                  {t("player.queue.saved")} <span className="pill pill-muted">{saved.length}</span>
                </summary>
                <ul className="manual-queue-saved-list player-saved-list">
                  {saved.map((q) => (
                    <li key={q.id} className="manual-queue-saved-item">
                      <div>
                        <strong>{q.name}</strong>
                        <span className="hint">
                          {q.item_count} bl ·{" "}
                          {formatDuration((q.duration_ms ?? 0) / 1000)}
                        </span>
                      </div>
                      <div className="manual-queue-saved-actions">
                        <button
                          type="button"
                          className="btn btn-ghost btn-sm"
                          onClick={() => onLoadSaved(q.id)}
                        >
                          {t("player.queue.load")}
                        </button>
                        <select
                          className="export-select"
                          defaultValue=""
                          title={t("player.queue.export")}
                          onChange={(e) => {
                            if (!e.target.value) return;
                            exportQueue(
                              (fmt) => api.exportSavedQueue(q.id, fmt),
                              e.target.value,
                              notify,
                              t,
                            );
                            e.target.value = "";
                          }}
                        >
                          <option value="">↓</option>
                          <option value="funscript">.funscript</option>
                          <option value="csv">.csv</option>
                          <option value="json">.json</option>
                        </select>
                        <button
                          type="button"
                          className="btn btn-sm btn-primary"
                          disabled={playing}
                          onClick={() => onPlaySaved(q.id, q.name)}
                        >
                          ▶
                        </button>
                        <button
                          type="button"
                          className="btn btn-ghost btn-sm"
                          onClick={() => onDeleteSaved(q.id)}
                        >
                          ✕
                        </button>
                      </div>
                    </li>
                  ))}
                </ul>
              </details>
            )}
          </PlayerCollapsiblePanel>
        )}

        {panelsVisible.library && (
          <PlayerCollapsiblePanel
            variant="library"
            title={t("player.panels.library")}
            subtitle={t("player.library.subtitle", {
              count: totalBlocks.toLocaleString(),
              page,
              total: totalPages,
            })}
            open={panelsExpanded.library}
            onToggle={() => togglePanelExpanded("library")}
          >
            <PlayerBlockBrowser
              blocks={blocks}
              totalBlocks={totalBlocks}
              page={page}
              totalPages={totalPages}
              searchInput={searchInput}
              filters={blockFilters}
              defaultLoop={defaultLoop}
              busy={busy}
              onSearchInputChange={setSearchInput}
              onFiltersChange={(patch) =>
                setBlockFilters((prev) => ({ ...prev, ...patch }))
              }
              onDefaultLoopChange={setDefaultLoop}
              onPageChange={setPage}
              onAdd={addBlock}
            />
          </PlayerCollapsiblePanel>
        )}
      </div>

      {signalModal && (
        <div className="glass manual-queue-signal-modal">
          <h3 className="panel-subtitle">
            {t("player.signalModal.title", { label: signalLabel(t, signalModal) })}
          </h3>
          <label className="hint">
            {t("player.signalModal.searchScript")}
            <input
              type="search"
              placeholder={t("player.signalModal.searchPlaceholder")}
              value={pickerSearch}
              onChange={(e) => setPickerSearch(e.target.value)}
              autoFocus
            />
          </label>
          <ul className="manual-queue-signal-picker">
            {pickerBlocks.map((b) => (
              <li key={b.id}>
                <button
                  type="button"
                  className={`manual-queue-signal-pick${signalBlockId === b.id ? " manual-queue-signal-pick--active" : ""}`}
                  onClick={() => setSignalBlockId(b.id)}
                >
                  <strong>{b.display_name ?? b.id}</strong>
                  <span className="hint">
                    {b.script_number != null ? `#${String(b.script_number).padStart(3, "0")} · ` : ""}
                    {Math.round((b.duration_ms ?? 0) / 1000)}s
                    {b.session_roles?.includes(signalModal) ? " · ★" : ""}
                  </span>
                </button>
              </li>
            ))}
            {!pickerBlocks.length && (
              <li className="hint empty-hint">{t("player.signalModal.noScripts")}</li>
            )}
          </ul>
          <label className="hint">
            {t("player.signalModal.loopDuration")}
            <input
              type="number"
              min={5}
              max={600}
              value={signalDurationSec}
              onChange={(e) => setSignalDurationSec(Number(e.target.value))}
            />
          </label>
          <div className="manual-queue-signal-modal-actions">
            <button
              type="button"
              className="btn btn-primary btn-sm"
              disabled={busy || !signalBlockId}
              onClick={confirmSignal}
            >
              {t("player.signalModal.play")}
            </button>
            <button
              type="button"
              className="btn btn-ghost btn-sm"
              onClick={() => setSignalModal(null)}
            >
              {t("common.cancel")}
            </button>
          </div>
        </div>
      )}

      </div>
    </div>
  );
}
