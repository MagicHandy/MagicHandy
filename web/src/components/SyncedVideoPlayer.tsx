import { formatNumber, t, translateKnown } from "../i18n";
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { api } from "../api/client";
import type { MediaSyncStatus, MediaVideo, MediaVideoUpdate } from "../api/types";
import { VideoPlaybackController, type SyncOperation, type VideoPlayerHandle } from "../media/playbackController";
import { ChevronUpIcon, GearIcon } from "../shell/icons";
import { formatTimelineTime } from "./ImportTimeline";
import { FunscriptTimeline } from "./FunscriptTimeline";
import { MediaVideoPlayer } from "./MediaVideoPlayer";
import { PlaybackPanel, formatMillis } from "./PlaybackPanel";
import { SynchronizedVideoControls } from "./SynchronizedVideoControls";
import { useAppState } from "../state/app-state";

const TIMELINE_HIDDEN_KEY = "magichandy-video-timeline-hidden";

interface Props {
  video: MediaVideo;
  locked: boolean;
  stopSequence?: number;
  onVideoUpdate?: (video: MediaVideoUpdate) => void;
  /** Offered by the library when the browser refuses to decode this file. */
  onRequestConversion?: () => void;
  conversionBusy?: boolean;
  /** Receives the open video's command surface, and null when it closes. */
  onHandleChange?: (handle: VideoPlayerHandle | null) => void;
  /**
   * Whether the paired script drives the device. Off when the viewer picks
   * another motion source; the video then plays as a plain one.
   */
  synchronized?: boolean;
}

// The view for one open video. Playback decisions live in
// VideoPlaybackController; this component wires the element and the app's
// lock, Stop and duration state into it and renders its snapshot.
export function SyncedVideoPlayer({ video, locked, stopSequence, onVideoUpdate, onRequestConversion, conversionBusy, onHandleChange, synchronized = video.has_funscript }: Props) {
  const { state, refresh } = useAppState();
  const refreshRef = useRef(refresh);
  refreshRef.current = refresh;
  const videoElement = useRef<HTMLVideoElement | null>(null);
  // One controller per opened video and motion source. The props read here
  // only seed it; the effects below keep lock, Stop and duration current.
  // Switching the source replaces the controller on the same element.
  const paired = video.has_funscript && synchronized;
  const controller = useMemo(() => new VideoPlaybackController(
    { videoID: video.id, synchronized: paired, durationMillis: video.duration_ms ?? 0, locked, stopSequence },
    {
      mediaSync: (event, sequence, signal, keepalive) => api.mediaSync(event, sequence, signal, keepalive),
      saveMediaPlayback: (patch) => api.saveMediaPlayback(patch),
      loadScript: async (id, signal) => (await api.mediaFunscript(id, signal)).funscript,
      refresh: () => refreshRef.current(),
    },
  ), [video.id, paired]);
  const snapshot = useSyncExternalStore(controller.subscribe, controller.getSnapshot);
  const [timelineHidden, setTimelineHidden] = useState(readTimelinePreference);
  const [panelOpen, setPanelOpen] = useState(false);

  useEffect(() => {
    controller.connect();
    return () => controller.disconnect();
  }, [controller]);
  useLayoutEffect(() => controller.setLocked(locked), [controller, locked]);
  useLayoutEffect(() => controller.setStopSequence(stopSequence), [controller, stopSequence]);
  // Persisted decoder metadata can arrive during playback. It updates the
  // controls without replacing the script or resetting the active session.
  useEffect(() => controller.setDuration(video.duration_ms ?? 0), [controller, video.duration_ms]);
  useEffect(() => {
    if (!onHandleChange) return undefined;
    onHandleChange(controller);
    return () => onHandleChange(null);
  }, [controller, onHandleChange]);

  const attachPlayer = useCallback((node: HTMLVideoElement | null) => {
    videoElement.current = node;
    controller.attach(node);
  }, [controller]);

  function toggleTimeline() {
    setTimelineHidden((current) => {
      const next = !current;
      try {
        localStorage.setItem(TIMELINE_HIDDEN_KEY, String(next));
      } catch {
        // The preference remains usable for this tab when storage is blocked.
      }
      return next;
    });
  }

  // A surrounding watch layout can mark itself as the fullscreen root so the
  // chat beside the video stays visible in fullscreen.
  function toggleFullscreen() {
    const player = videoElement.current;
    const container = player?.closest<HTMLElement>("[data-fullscreen-root]") ?? player?.closest<HTMLElement>(".media-player");
    if (!container) return;
    if (document.fullscreenElement) {
      if (document.exitFullscreen) void document.exitFullscreen().catch(() => undefined);
    } else if (container.requestFullscreen) {
      void container.requestFullscreen().catch(() => undefined);
    }
  }

  const { script, scriptLoading, scriptError, sync, syncError, operation } = snapshot;
  const statusLabel = script ? syncStatusLabel(sync, locked, operation) : "";
  const effectiveOffset = (state?.settings?.media?.script_offset_ms ?? 0) + (video.script_offset_ms ?? 0);
  const playbackSettingsTitle = [
    t("Playback settings for {display_name}", { display_name: video.display_name }),
    t("Sync {offset}", { offset: formatMillis(effectiveOffset) }),
  ].join(" · ");
  const durationMismatch = script ? mediaDurationMismatch(video.duration_ms, script.duration_ms) : false;
  return (
    <MediaVideoPlayer
      video={video}
      allowMetadataWrite={!locked}
      allowLibraryWrite={!locked && state?.capabilities?.configure_host !== false}
      controlsEnabled={false}
      busy={scriptLoading}
      videoOverlay={!scriptLoading ? (
        <SynchronizedVideoControls
          synchronized={Boolean(script)}
          autoHide={snapshot.playbackIntent && (!script || sync.active) && !operation}
          currentTimeMillis={snapshot.currentTimeMillis}
          durationMillis={snapshot.durationMillis || script?.duration_ms || 0}
          muted={snapshot.muted}
          playbackIntent={snapshot.playbackIntent}
          playbackRate={snapshot.playbackRate}
          volume={snapshot.volume}
          onFullscreen={toggleFullscreen}
          onMuteChange={controller.setMuted}
          onPlaybackRateChange={controller.setRate}
          onSeekCancel={controller.cancelSeek}
          onSeekCommit={controller.commitSeek}
          onSeekStart={controller.beginSeek}
          onTogglePlayback={() => { controller.commands.toggle(); }}
          onVolumeChange={controller.setVolume}
        />
      ) : undefined}
      onDuration={controller.setDuration}
      onVideoUpdate={onVideoUpdate}
      onTimeChange={controller.handleTimeChange}
      playerRef={attachPlayer}
      onPlaybackEvent={controller.handleMediaEvent}
      synchronized={paired}
      onRequestConversion={onRequestConversion}
      conversionBusy={conversionBusy}
    >
      {scriptLoading && <div className="media-script-loading" role="status">{t("Preparing paired script and video")}</div>}
      {!scriptLoading && !script && scriptError && <p className="form-status media-playback-error" role="alert">{t("Script unavailable: {error}. Video playback will not command motion.", { error: scriptError })}</p>}
      {script && (
        <section className="media-funscript" aria-label={t("Paired funscript timeline")}>
          <div className="media-funscript-head">
            <div className="media-funscript-title">
              <strong>{t("Paired funscript")}</strong>
              <span>{t("{count} actions / {duration}", { count: formatNumber(script.action_count), duration: formatTimelineTime(script.duration_ms) })}</span>
              {durationMismatch && <span className="media-script-length-warning">{t("Length differs from {duration} video", { duration: formatTimelineTime(video.duration_ms ?? 0) })}</span>}
            </div>
            <div className="media-funscript-actions">
              <button
                type="button"
                className="icon-button media-timeline-toggle"
                onClick={toggleTimeline}
                aria-expanded={!timelineHidden}
                aria-label={timelineHidden ? t("Show timeline") : t("Hide timeline")}
                title={timelineHidden ? t("Show timeline") : t("Hide timeline")}
              ><ChevronUpIcon size={16} /></button>
              <button
                type="button"
                className="icon-button media-playback-trigger"
                onClick={() => setPanelOpen((open) => !open)}
                aria-expanded={panelOpen}
                aria-haspopup="dialog"
                aria-label={t("Playback settings for {display_name}", { display_name: video.display_name })}
                title={playbackSettingsTitle}
              ><GearIcon size={16} /></button>
            </div>
          </div>
          <FunscriptTimeline
            script={script}
            currentTime={snapshot.currentTimeMillis}
            hidden={timelineHidden}
            onSeek={controller.commitSeek}
            onSeekCancel={controller.cancelSeek}
            onSeekStart={controller.beginSeek}
          />
          <div
            className="media-sync-readout"
            data-state={sync.state}
            data-operation={operation?.kind}
            role="status"
            aria-busy={operation ? true : undefined}
          >
            <span className="media-sync-state"><span aria-hidden="true" />{translateKnown(statusLabel)}</span>
            {sync.active && typeof sync.motion_speed_limit_percent === "number" && <span>{t("{percent}% speed limit", { percent: sync.motion_speed_limit_percent })}</span>}
            {sync.active && typeof sync.drift_ms === "number" && <span aria-hidden="true">{t("{milliseconds} ms drift", { milliseconds: Math.abs(sync.drift_ms) })}</span>}
            <span className="media-sync-time">{formatTimelineTime(snapshot.currentTimeMillis)}</span>
          </div>
          {syncError && <p className="form-status media-playback-error" role="alert">{syncError}</p>}
          {panelOpen && (
            <PlaybackPanel
              video={video}
              sync={sync}
              locked={locked}
              setupOffsetMillis={state?.settings?.media?.script_offset_ms ?? 0}
              smoothingPercent={state?.settings?.media?.script_smoothing_percent ?? 0}
              roundingMillis={state?.settings?.media?.peak_rounding_ms ?? 0}
              limitSpeed={state?.settings?.motion?.apply_video_speed_limit ?? false}
              speedLimitPercent={state?.settings?.motion?.speed_max_percent ?? 100}
              onClose={() => setPanelOpen(false)}
              onVideoUpdate={onVideoUpdate}
              onFiltersChanging={controller.beginFilterChange}
              onFiltersChanged={controller.applyPlaybackFilters}
            />
          )}
        </section>
      )}
    </MediaVideoPlayer>
  );
}

export function syncStatusLabel(sync: MediaSyncStatus, locked: boolean, operation: SyncOperation | null): string {
  if (locked) return "Timeline only; this tab does not control motion";
  if (operation) {
    const at = formatTimelineTime(operation.mediaTimeMillis);
    switch (operation.kind) {
      case "starting": return t("Video held at {time}; starting paired motion", { time: at });
      case "seeking": return t("Video held at {time}; stopping prior motion", { time: at });
      case "resyncing": return t("Video held at {time}; resyncing motion", { time: at });
      case "resuming": return t("Script aligned at {time}; resuming video", { time: at });
    }
  }
  switch (sync.state) {
    case "following": return "Device following video";
    case "seeking":
      if (sync.last_event === "waiting") return "Buffering video before motion starts";
      return sync.last_event === "play" ? "Arming device" : "Motion stopped while seeking";
    case "paused":
      if (sync.last_event === "waiting") return "Buffering video; motion stopped";
      if (sync.last_event === "stalled" || sync.last_event === "error") return "Playback interrupted; motion stopped";
      return "Video paused; motion stopped";
    case "ended":
    case "completed": return "Script playback complete";
    case "drifted": return "Timing changed; re-arming device";
    case "interrupted": return "Synchronized motion interrupted";
    case "timed_out": return "Video heartbeat lost; motion stopped";
    case "stopped": return "Motion stopped";
    case "error": return "Synchronization unavailable";
    default: return "Ready to synchronize on play";
  }
}

function mediaDurationMismatch(videoDuration: number | null, scriptDuration: number): boolean {
  if (videoDuration === null || videoDuration <= 0 || scriptDuration <= 0) return false;
  return Math.abs(videoDuration - scriptDuration) > Math.max(2_000, videoDuration * 0.05);
}

function readTimelinePreference(): boolean {
  try {
    return localStorage.getItem(TIMELINE_HIDDEN_KEY) === "true";
  } catch {
    return false;
  }
}
