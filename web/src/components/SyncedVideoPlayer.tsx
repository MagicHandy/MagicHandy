import { formatNumber, t, translateKnown } from "../i18n";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api, ApiError } from "../api/client";
import type { MediaFunscript, MediaSyncEvent, MediaSyncStatus, MediaVideo } from "../api/types";
import { ChevronUpIcon } from "../shell/icons";
import { formatTimelineTime } from "./ImportTimeline";
import { FunscriptTimeline } from "./FunscriptTimeline";
import { MediaVideoPlayer, type MediaPlaybackEvent } from "./MediaVideoPlayer";
import { PlaybackPanel, formatMillis } from "./PlaybackPanel";
import { useAppState } from "../state/app-state";

const HEARTBEAT_MILLIS = 1_500;
const MEDIA_READY_POLL_MILLIS = 100;
// Browsers may emit `pause` before `seeking` for one scrubber gesture. A pause
// this close to a seek is part of the gesture, so playback intent survives it
// and the video auto-resumes after the seek instead of staying paused.
const SEEK_GESTURE_PAUSE_MILLIS = 400;
// The video is the follower clock: when it deviates from the engine's
// transport-aligned clock by more than this, the video is nudged. Below it,
// a correction would be more visible than the offset.
const CLOCK_ALIGN_THRESHOLD_MILLIS = 150;
// Resuming is not instant: the pre-play alignment targets the engine clock as
// it reads *before* the seek and decoder restart, and the clock keeps running
// through both. That start cost lands entirely in one direction — the video
// begins behind the device — and anything under the steady-state threshold
// would then persist for the whole run, which is exactly what a script running
// slightly ahead of the picture feels like. One tighter pass once the media
// clock is actually advancing removes it; a correction this small is
// imperceptible next to the repositioning that just happened.
const RESUME_ALIGN_THRESHOLD_MILLIS = 40;
const TIMELINE_HIDDEN_KEY = "magichandy-video-timeline-hidden";

interface Props {
  video: MediaVideo;
  locked: boolean;
  stopSequence?: number;
  onVideoUpdate?: (video: MediaVideo) => void;
}

interface PlaybackSession {
  id: string;
  sequence: number;
  stopSequence?: number;
}

type SyncOperationKind = "starting" | "seeking" | "resyncing" | "resuming";

interface SyncOperation {
  kind: SyncOperationKind;
  mediaTimeMillis: number;
}

export function SyncedVideoPlayer({ video, locked, stopSequence, onVideoUpdate }: Props) {
  const session = useMemo<PlaybackSession>(() => ({ id: createMediaSessionID(), sequence: 0 }), [video.id]);
  const activeSessionID = useRef(session.id);
  activeSessionID.current = session.id;
  const playerRef = useRef<HTMLVideoElement | null>(null);
  const lastPlayer = useRef<HTMLVideoElement | null>(null);
  const [script, setScript] = useState<MediaFunscript | null>(null);
  const [scriptError, setScriptError] = useState("");
  const [loadingScript, setLoadingScript] = useState(video.has_funscript);
  const [currentTime, setCurrentTime] = useState(0);
  const [sync, setSync] = useState<MediaSyncStatus>({ active: false, state: "idle" });
  const [syncError, setSyncError] = useState("");
  const [timelineHidden, setTimelineHidden] = useState(readTimelinePreference);
  const [panelOpen, setPanelOpen] = useState(false);
  const [syncOperation, setSyncOperation] = useState<SyncOperation | null>(null);
  const { state, refresh } = useAppState();
  const mounted = useRef(true);
  const generation = useRef(0);
  const desiredPlaying = useRef(false);
  const activeSync = useRef(false);
  const arming = useRef(false);
  const seekInProgress = useRef(false);
  const resumeAfterSeek = useRef(false);
  const seekingStop = useRef<Promise<void>>(Promise.resolve());
  const awaitingMedia = useRef(false);
  const bufferingStop = useRef<Promise<void>>(Promise.resolve());
  const readyArm = useRef<"play" | "seeked" | "ratechange" | "resync">("play");
  const heartbeatPending = useRef(false);
  const heartbeatAbort = useRef<AbortController | null>(null);
  const sessionRequestControllers = useRef(new Set<AbortController>());
  const ignoredPlay = useRef(false);
  const ignoredPause = useRef(false);
  const ignoredPlayTimer = useRef<number>();
  const ignoredPauseTimer = useRef<number>();
  const latestStopSequence = useRef(stopSequence);
  const capturedStopSequence = useRef<number>();
  const playIntentLostAt = useRef(0);
  const alignSeekActive = useRef(false);
  const alignSeekTimer = useRef<number>();
  const resumeAlignPending = useRef(false);
  const engineClock = useRef<{ mediaMs: number; atMs: number; rate: number } | null>(null);
  const pendingArm = useRef<"play" | "seeked" | "ratechange" | "resync" | null>(null);
  const armAbort = useRef<AbortController | null>(null);
  const armPlaybackRef = useRef<(player: HTMLVideoElement, event: "play" | "seeked" | "ratechange" | "resync") => Promise<void>>(async () => undefined);
  const mediaReadyTimer = useRef<number>();
  const mediaReadyGeneration = useRef(0);

  const clearMediaReadyPoll = useCallback(() => {
    window.clearInterval(mediaReadyTimer.current);
    mediaReadyTimer.current = undefined;
  }, []);

  const resumeWhenMediaReady = useCallback((player: HTMLVideoElement, waitGeneration: number) => {
    if (
      waitGeneration !== mediaReadyGeneration.current
      || !mounted.current
      || !awaitingMedia.current
      || !desiredPlaying.current
      || seekInProgress.current
    ) {
      if (!awaitingMedia.current || !desiredPlaying.current || !mounted.current) clearMediaReadyPoll();
      return;
    }
    if (!mediaHasFutureData(player)) return;

    clearMediaReadyPoll();
    const recovery = bufferingStop.current;
    const event = readyArm.current;
    void recovery.then(() => {
      if (
        waitGeneration !== mediaReadyGeneration.current
        || !mounted.current
        || !awaitingMedia.current
        || !desiredPlaying.current
        || seekInProgress.current
        || !mediaHasFutureData(player)
      ) return;
      awaitingMedia.current = false;
      void armPlaybackRef.current(player, event);
    });
  }, [clearMediaReadyPoll]);

  const waitForMediaReady = useCallback((player: HTMLVideoElement) => {
    clearMediaReadyPoll();
    const waitGeneration = ++mediaReadyGeneration.current;
    mediaReadyTimer.current = window.setInterval(
      () => resumeWhenMediaReady(player, waitGeneration),
      MEDIA_READY_POLL_MILLIS,
    );
  }, [clearMediaReadyPoll, resumeWhenMediaReady]);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      mediaReadyGeneration.current += 1;
      clearMediaReadyPoll();
    };
  }, [clearMediaReadyPoll]);

  useEffect(() => {
    const controller = new AbortController();
    const loadGeneration = ++generation.current;
    setScript(null);
    setScriptError("");
    setSyncError("");
    setSync({ active: false, state: "idle" });
    setCurrentTime(0);
    setSyncOperation(null);
    desiredPlaying.current = false;
    activeSync.current = false;
    seekInProgress.current = false;
    awaitingMedia.current = false;
    bufferingStop.current = Promise.resolve();
    readyArm.current = "play";
    capturedStopSequence.current = undefined;
    playIntentLostAt.current = 0;
    alignSeekActive.current = false;
    engineClock.current = null;
    setLoadingScript(video.has_funscript);
    if (!video.has_funscript) return () => controller.abort();

    void api.mediaFunscript(video.id, controller.signal).then((response) => {
      if (!controller.signal.aborted && mounted.current && generation.current === loadGeneration) {
        setScript(response.funscript);
      }
    }).catch((reason) => {
      if (!controller.signal.aborted && mounted.current && generation.current === loadGeneration) {
        setScriptError(reason instanceof Error ? reason.message : "The paired funscript could not be loaded.");
      }
    }).finally(() => {
      if (!controller.signal.aborted && mounted.current && generation.current === loadGeneration) {
        setLoadingScript(false);
      }
    });
    return () => controller.abort();
  }, [video.has_funscript, video.id]);

  const suppressNext = useCallback((kind: "play" | "pause") => {
    const flag = kind === "play" ? ignoredPlay : ignoredPause;
    const timer = kind === "play" ? ignoredPlayTimer : ignoredPauseTimer;
    flag.current = true;
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => {
      flag.current = false;
    }, 1_000);
  }, []);

  const holdVideo = useCallback((player: HTMLVideoElement) => {
    // Any hold ends the resume this alignment belonged to; the next arm sets it
    // again. Leaving it armed would fire a correction against a stale clock.
    resumeAlignPending.current = false;
    if (player.paused) return;
    suppressNext("pause");
    player.pause();
  }, [suppressNext]);

  useEffect(() => {
    const previous = latestStopSequence.current;
    latestStopSequence.current = stopSequence;
    if (previous === undefined || stopSequence === undefined || previous === stopSequence || (!activeSync.current && !arming.current && !awaitingMedia.current)) return;
    desiredPlaying.current = false;
    awaitingMedia.current = false;
    setSyncOperation(null);
    playIntentLostAt.current = 0;
    pendingArm.current = null;
    bufferingStop.current = Promise.resolve();
    generation.current += 1;
    armAbort.current?.abort();
    const player = playerRef.current;
    if (player) holdVideo(player);
    activeSync.current = false;
    setSync({ active: false, state: "stopped", last_event: "emergency_stop", message: "Motion was stopped. Press play to start a new synchronized run." });
  }, [holdVideo, stopSequence]);

  const updateSync = useCallback((status: MediaSyncStatus) => {
    if (!mounted.current) return;
    activeSync.current = status.active;
    if (status.active && typeof status.expected_media_time_ms === "number") {
      engineClock.current = {
        mediaMs: status.expected_media_time_ms,
        atMs: performance.now(),
        rate: status.playback_rate && status.playback_rate > 0 ? status.playback_rate : 1,
      };
    } else if (!status.active) {
      engineClock.current = null;
    }
    setSync(status);
    setSyncError("");
  }, []);

  // The engine's buffered device stream cannot be cheaply re-timed, so once
  // armed the engine clock owns synchronization and the video follows it.
  // Corrections raise alignSeekActive so the resulting seeking/seeked pair is
  // not mistaken for a user seek that must stop motion.
  const alignPlayerToEngineClock = useCallback((
    player: HTMLVideoElement,
    thresholdMillis = CLOCK_ALIGN_THRESHOLD_MILLIS,
  ) => {
    const clock = engineClock.current;
    if (!clock || !activeSync.current || seekInProgress.current || player.seeking) return;
    const projectedMs = clock.mediaMs + (performance.now() - clock.atMs) * clock.rate;
    const deltaMs = projectedMs - mediaTimeMillis(player);
    if (!Number.isFinite(deltaMs) || Math.abs(deltaMs) < thresholdMillis) return;
    alignSeekActive.current = true;
    window.clearTimeout(alignSeekTimer.current);
    alignSeekTimer.current = window.setTimeout(() => {
      alignSeekActive.current = false;
    }, 1_000);
    player.currentTime = Math.max(0, projectedMs) / 1000;
  }, []);

  const showSyncFailure = useCallback((reason: unknown, fallback: string) => {
    if (!mounted.current) return;
    setSyncOperation(null);
    const status = syncStatusFromError(reason);
    activeSync.current = false;
    awaitingMedia.current = false;
    if (status) setSync(status);
    else setSync({ active: false, state: "error", message: fallback });
    setSyncError(reason instanceof Error && reason.message ? reason.message : fallback);
  }, []);

  const syncEvent = useCallback(async (
    player: HTMLVideoElement,
    state: MediaSyncEvent["state"],
    event: MediaSyncEvent["event"],
    sequence: number,
    keepalive = false,
    signal?: AbortSignal,
  ) => {
    const ownedController = signal ? null : new AbortController();
    if (ownedController) sessionRequestControllers.current.add(ownedController);
    lastPlayer.current = player;
    try {
      const response = await api.mediaSync(
        buildSyncEvent(video.id, player, state, event, session),
        sequence,
        signal ?? ownedController?.signal,
        keepalive,
      );
      if (mounted.current && activeSessionID.current === session.id) updateSync(response.sync);
      return response.sync;
    } finally {
      if (ownedController) sessionRequestControllers.current.delete(ownedController);
    }
  }, [session, updateSync, video.id]);

  const stopPlaybackMotion = useCallback(async (
    player: HTMLVideoElement,
    state: "paused" | "seeking" | "ended" | "closed",
    event: MediaSyncEvent["event"],
  ) => {
    activeSync.current = false;
    const sequence = capturedStopSequence.current ?? latestStopSequence.current;
    if (sequence === undefined || locked) return;
    try {
      await syncEvent(player, state, event, sequence, state === "closed");
    } catch (reason) {
      showSyncFailure(reason, "Device motion could not be stopped from the video player.");
    }
  }, [locked, showSyncFailure, syncEvent]);

  const armPlayback = useCallback(async (
    player: HTMLVideoElement,
    event: "play" | "seeked" | "ratechange" | "resync",
  ) => {
    if (!mounted.current || activeSessionID.current !== session.id || !desiredPlaying.current) return;
    if (arming.current) {
      pendingArm.current = event;
      generation.current += 1;
      return;
    }
    if (locked || !script) return;
    const sequence = latestStopSequence.current;
    holdVideo(player);
    if (sequence === undefined) {
      desiredPlaying.current = false;
      showSyncFailure(new Error("The safety state is still loading. Press play again when the app is ready."), "The safety state is unavailable.");
      return;
    }
    if (!supportedPlaybackRate(player.playbackRate)) {
      desiredPlaying.current = false;
      showSyncFailure(new Error("Synchronized playback supports video speeds from 0.25x to 4x."), "The video speed is unsupported.");
      return;
    }
    if (!mediaHasFutureData(player)) {
      awaitingMedia.current = true;
      readyArm.current = event;
      if (activeSync.current) {
        bufferingStop.current = stopPlaybackMotion(player, "paused", "waiting");
      } else {
        setSyncError("");
        setSync({ active: false, video_id: video.id, state: "seeking", last_event: "waiting", message: "Buffering video before motion starts." });
      }
      waitForMediaReady(player);
      return;
    }
    awaitingMedia.current = false;
    const armMediaTimeMillis = mediaTimeMillis(player);
    setSyncOperation({
      kind: event === "play" ? "starting" : "resyncing",
      mediaTimeMillis: armMediaTimeMillis,
    });

    const commandGeneration = ++generation.current;
    capturedStopSequence.current = sequence;
    session.stopSequence = sequence;
    const controller = new AbortController();
    armAbort.current = controller;
    arming.current = true;
    setSyncError("");
    setSync({ active: false, video_id: video.id, state: "seeking", last_event: event, message: "Arming paired-script motion." });
    try {
      const status = await syncEvent(player, "playing", event, sequence, false, controller.signal);
      if (!mounted.current || generation.current !== commandGeneration || !desiredPlaying.current) {
        if (status.active) await stopPlaybackMotion(player, "paused", "pause");
        return;
      }
      if (status.state === "completed") {
        desiredPlaying.current = false;
        setSyncOperation(null);
        suppressNext("play");
        await player.play();
        return;
      }
      if (!status.active) {
        desiredPlaying.current = false;
        setSyncOperation(null);
        return;
      }
      const alignedMediaTimeMillis = typeof status.expected_media_time_ms === "number"
        ? Math.max(0, Math.round(status.expected_media_time_ms))
        : armMediaTimeMillis;
      setSyncOperation({ kind: "resuming", mediaTimeMillis: alignedMediaTimeMillis });
      suppressNext("play");
      try {
        // The engine clock has been running since transport play; move the
        // still-held video onto it so playback resumes already in sync.
        alignPlayerToEngineClock(player);
        await player.play();
        // Seek and decoder restart both happened after that reading, so re-check
        // once the media clock is actually advancing.
        resumeAlignPending.current = true;
        resumeAfterSeek.current = false;
        playIntentLostAt.current = 0;
      } catch (reason) {
        ignoredPlay.current = false;
        desiredPlaying.current = false;
        await stopPlaybackMotion(player, "paused", "pause");
        throw reason;
      }
    } catch (reason) {
      if (controller.signal.aborted) return;
      desiredPlaying.current = false;
      holdVideo(player);
      showSyncFailure(reason, "Paired-script motion could not be synchronized.");
    } finally {
      if (armAbort.current === controller) armAbort.current = null;
      arming.current = false;
      const next = pendingArm.current;
      pendingArm.current = null;
      if (next && desiredPlaying.current && mounted.current) {
        window.queueMicrotask(() => void armPlaybackRef.current(player, next));
      }
    }
  }, [alignPlayerToEngineClock, holdVideo, locked, script, showSyncFailure, stopPlaybackMotion, suppressNext, syncEvent, video.id, waitForMediaReady]);
  armPlaybackRef.current = armPlayback;

  // The media clock advancing is the first honest evidence that playback really
  // resumed, so it is where the tighter post-resume alignment belongs.
  const handleTimeChange = useCallback((timeMillis: number) => {
    setCurrentTime(timeMillis);
    if (!resumeAlignPending.current) return;
    const player = playerRef.current ?? lastPlayer.current;
    if (!player || player.paused || arming.current || seekInProgress.current || !activeSync.current) return;
    resumeAlignPending.current = false;
    alignPlayerToEngineClock(player, RESUME_ALIGN_THRESHOLD_MILLIS);
  }, [alignPlayerToEngineClock]);

  const handlePlaybackEvent = useCallback((event: MediaPlaybackEvent, player: HTMLVideoElement) => {
    lastPlayer.current = player;
    setCurrentTime(mediaTimeMillis(player));
    if (!script) {
      if (loadingScript && event === "play") holdVideo(player);
      return;
    }
    if (locked) return;

    if (event === "playing") {
      if (desiredPlaying.current && activeSync.current) setSyncOperation(null);
      return;
    }
    if (event === "play") {
      if (ignoredPlay.current) {
        ignoredPlay.current = false;
        window.clearTimeout(ignoredPlayTimer.current);
        return;
      }
      if (mediaTimeMillis(player) >= script.duration_ms) {
        desiredPlaying.current = false;
        activeSync.current = false;
        awaitingMedia.current = false;
        setSync({
          active: false,
          video_id: video.id,
          state: "completed",
          last_event: "play",
          media_time_ms: mediaTimeMillis(player),
          message: "The paired script has ended; video playback continues without motion.",
        });
        return;
      }
      desiredPlaying.current = true;
      void armPlayback(player, "play");
      return;
    }
    if (event === "pause") {
      if (ignoredPause.current) {
        ignoredPause.current = false;
        window.clearTimeout(ignoredPauseTimer.current);
        return;
      }
      if (seekInProgress.current || player.ended || (resumeAfterSeek.current && desiredPlaying.current)) return;
      if (desiredPlaying.current) playIntentLostAt.current = performance.now();
      desiredPlaying.current = false;
      resumeAfterSeek.current = false;
      setSyncOperation(null);
      generation.current += 1;
      awaitingMedia.current = false;
      mediaReadyGeneration.current += 1;
      clearMediaReadyPoll();
      armAbort.current?.abort();
      void stopPlaybackMotion(player, "paused", "pause");
      return;
    }
    if (event === "seeking") {
      if (alignSeekActive.current) return;
      if (seekInProgress.current) return;
      seekInProgress.current = true;
      setSyncOperation({ kind: "seeking", mediaTimeMillis: mediaTimeMillis(player) });
      resumeAfterSeek.current = desiredPlaying.current
        || !player.paused
        || (playIntentLostAt.current > 0 && performance.now() - playIntentLostAt.current < SEEK_GESTURE_PAUSE_MILLIS);
      playIntentLostAt.current = 0;
      awaitingMedia.current = false;
      holdVideo(player);
      mediaReadyGeneration.current += 1;
      clearMediaReadyPoll();
      generation.current += 1;
      armAbort.current?.abort();
      seekingStop.current = stopPlaybackMotion(player, "seeking", "seeking");
      return;
    }
    if (event === "seeked") {
      if (alignSeekActive.current) {
        alignSeekActive.current = false;
        window.clearTimeout(alignSeekTimer.current);
        return;
      }
      const shouldResume = resumeAfterSeek.current;
      seekInProgress.current = false;
      if (shouldResume) {
        desiredPlaying.current = true;
        setSyncOperation({ kind: "resyncing", mediaTimeMillis: mediaTimeMillis(player) });
        void seekingStop.current.then(() => armPlayback(player, "seeked"));
      } else {
        desiredPlaying.current = false;
        setSyncOperation(null);
        void seekingStop.current.then(() => stopPlaybackMotion(player, "paused", "seeked"));
      }
      return;
    }
    if (event === "ratechange") {
      if (desiredPlaying.current) {
        generation.current += 1;
        void armPlayback(player, "ratechange");
      }
      return;
    }
    if (event === "ended") {
      desiredPlaying.current = false;
      setSyncOperation(null);
      generation.current += 1;
      awaitingMedia.current = false;
      armAbort.current?.abort();
      void stopPlaybackMotion(player, "ended", "ended");
      return;
    }
    if (event === "canplay") {
      resumeWhenMediaReady(player, mediaReadyGeneration.current);
      return;
    }
    if (event === "stalled") return;
    if (event === "waiting") {
      // A clock-alignment nudge transiently drops readyState and fires
      // `waiting` even inside a fully buffered range. Treating that dip as
      // starvation would stop motion, re-arm, nudge again on the next arm,
      // and loop forever; the nudge's own seeked/canplay completes the seek.
      if (alignSeekActive.current) return;
      if (!desiredPlaying.current && !activeSync.current && !arming.current) return;
      const mustStop = activeSync.current || arming.current;
      desiredPlaying.current = true;
      awaitingMedia.current = true;
      readyArm.current = "resync";
      generation.current += 1;
      armAbort.current?.abort();
      holdVideo(player);
      activeSync.current = false;
      setSyncOperation(null);
      setSyncError("");
      setSync({
        active: false,
        video_id: video.id,
        state: "paused",
        last_event: "waiting",
        media_time_ms: mediaTimeMillis(player),
        message: "Video is buffering; device motion is stopped.",
      });
      bufferingStop.current = mustStop ? stopPlaybackMotion(player, "paused", "waiting") : Promise.resolve();
      waitForMediaReady(player);
      return;
    }
    if (event === "error") {
      if (!desiredPlaying.current && !activeSync.current && !arming.current) return;
      desiredPlaying.current = false;
      awaitingMedia.current = false;
      setSyncOperation(null);
      generation.current += 1;
      armAbort.current?.abort();
      holdVideo(player);
      void stopPlaybackMotion(player, "paused", "error");
    }
  }, [armPlayback, clearMediaReadyPoll, holdVideo, loadingScript, locked, resumeWhenMediaReady, script, stopPlaybackMotion, video.id, waitForMediaReady]);

  useEffect(() => {
    if (!script || locked) return undefined;
    const timer = window.setInterval(() => {
      const player = playerRef.current ?? lastPlayer.current;
      if (!player || !desiredPlaying.current || !activeSync.current || arming.current || heartbeatPending.current || player.paused) return;
      const sequence = capturedStopSequence.current;
      if (sequence === undefined) return;
      const controller = new AbortController();
      const requestGeneration = generation.current;
      const requestSessionID = session.id;
      heartbeatPending.current = true;
      heartbeatAbort.current = controller;
      void syncEvent(player, "playing", "heartbeat", sequence, false, controller.signal).then((status) => {
        if (
          controller.signal.aborted
          || !mounted.current
          || activeSessionID.current !== requestSessionID
          || generation.current !== requestGeneration
          || !desiredPlaying.current
        ) return;
        if (status.requires_reanchor && desiredPlaying.current) {
          void armPlayback(player, "resync");
          return;
        }
        if (status.active) alignPlayerToEngineClock(player);
      }).catch((reason) => {
        if (controller.signal.aborted || !mounted.current || activeSessionID.current !== requestSessionID) return;
        desiredPlaying.current = false;
        holdVideo(player);
        showSyncFailure(reason, "Video synchronization was interrupted; motion stopped.");
      }).finally(() => {
        if (heartbeatAbort.current === controller) {
          heartbeatAbort.current = null;
          heartbeatPending.current = false;
        }
      });
    }, HEARTBEAT_MILLIS);
    return () => window.clearInterval(timer);
  }, [alignPlayerToEngineClock, armPlayback, holdVideo, locked, script, showSyncFailure, syncEvent]);

  useEffect(() => {
    if (!locked || (!activeSync.current && !arming.current && !awaitingMedia.current)) return;
    desiredPlaying.current = false;
    setSyncOperation(null);
    playIntentLostAt.current = 0;
    generation.current += 1;
    armAbort.current?.abort();
    awaitingMedia.current = false;
    const player = playerRef.current;
    if (player) holdVideo(player);
    activeSync.current = false;
    setSync({ active: false, state: "interrupted", message: "Controller access changed; video playback paused and synchronized motion stopped." });
  }, [holdVideo, locked]);

  useEffect(() => {
    const closingVideoID = video.id;
    const closingSession = session;
    return () => {
      // Close admission before canceling requests so no late heartbeat or
      // queued seek continuation can re-arm this session.
      desiredPlaying.current = false;
      resumeAfterSeek.current = false;
      const shouldClose = closingSession.stopSequence !== undefined;
      generation.current += 1;
      pendingArm.current = null;
      armAbort.current?.abort();
      heartbeatAbort.current?.abort();
      heartbeatAbort.current = null;
      heartbeatPending.current = false;
      for (const controller of sessionRequestControllers.current) controller.abort();
      sessionRequestControllers.current.clear();
      awaitingMedia.current = false;
      window.clearTimeout(ignoredPlayTimer.current);
      window.clearTimeout(ignoredPauseTimer.current);
      window.clearTimeout(alignSeekTimer.current);
      const player = playerRef.current ?? lastPlayer.current;
      const sequence = closingSession.stopSequence;
      activeSync.current = false;
      if (activeSessionID.current === closingSession.id) activeSessionID.current = "";
      if (shouldClose && player && sequence !== undefined) {
        void api.mediaSync(buildSyncEvent(closingVideoID, player, "closed", "closed", closingSession), sequence, undefined, true).catch(() => undefined);
      }
      activeSync.current = false;
    };
  }, [session, video.id]);

  function seek(milliseconds: number) {
    const player = playerRef.current;
    if (!player) return;
    lastPlayer.current = player;
    player.currentTime = Math.max(0, milliseconds) / 1000;
    setCurrentTime(milliseconds);
  }

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

  const statusLabel = script ? syncStatusLabel(sync, locked, syncOperation) : "";
  const effectiveOffset = (state?.settings?.media?.script_offset_ms ?? 0) + (video.script_offset_ms ?? 0);
  const durationMismatch = script ? mediaDurationMismatch(video.duration_ms, script.duration_ms) : false;
  return (
    <MediaVideoPlayer
      video={video}
      allowMetadataWrite={!locked}
      controlsEnabled={!loadingScript}
      busy={loadingScript}
      onVideoUpdate={onVideoUpdate}
      onTimeChange={handleTimeChange}
      playerRef={playerRef}
      onPlaybackEvent={video.has_funscript ? handlePlaybackEvent : undefined}
      synchronized={video.has_funscript}
    >
      {loadingScript && <div className="media-script-loading" role="status">{t("Preparing paired script and video")}</div>}
      {!loadingScript && !script && scriptError && <p className="form-status media-playback-error" role="alert">{t("Script unavailable: {error}. Video playback will not command motion.", { error: scriptError })}</p>}
      {script && (
        <section className="media-funscript" aria-label={t("Paired funscript timeline")}>
          <div className="media-funscript-head">
            <div>
              <strong>{t("Paired funscript")}</strong>
              <span>{t("{count} actions / {duration}", { count: formatNumber(script.action_count), duration: formatTimelineTime(script.duration_ms) })}</span>
              {durationMismatch && <span className="media-script-length-warning">{t("Length differs from {duration} video", { duration: formatTimelineTime(video.duration_ms ?? 0) })}</span>}
            </div>
            <button
              type="button"
              className="btn btn-secondary compact-command media-playback-trigger"
              onClick={() => setPanelOpen((open) => !open)}
              aria-expanded={panelOpen}
              aria-haspopup="dialog"
            >{t("Sync {offset}", { offset: formatMillis(effectiveOffset) })}
            </button>
            <button type="button" className="btn btn-secondary compact-command media-timeline-toggle" onClick={toggleTimeline} aria-expanded={!timelineHidden}>
              <ChevronUpIcon />{timelineHidden ? t("Show timeline") : t("Hide timeline")}
            </button>
          </div>
          <FunscriptTimeline script={script} currentTime={currentTime} hidden={timelineHidden} onSeek={seek} />
          <div
            className="media-sync-readout"
            data-state={sync.state}
            data-operation={syncOperation?.kind}
            role="status"
            aria-busy={syncOperation ? true : undefined}
          >
            <span className="media-sync-state"><span aria-hidden="true" />{translateKnown(statusLabel)}</span>
            {sync.active && typeof sync.motion_speed_limit_percent === "number" && <span>{t("{percent}% speed limit", { percent: sync.motion_speed_limit_percent })}</span>}
            {sync.active && typeof sync.drift_ms === "number" && <span aria-hidden="true">{t("{milliseconds} ms drift", { milliseconds: Math.abs(sync.drift_ms) })}</span>}
            <span className="media-sync-time">{formatTimelineTime(currentTime)}</span>
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
              onClose={() => setPanelOpen(false)}
              onVideoUpdate={onVideoUpdate}
              onFiltersChanged={refresh}
            />
          )}
        </section>
      )}
    </MediaVideoPlayer>
  );
}

function buildSyncEvent(
  videoID: string,
  player: HTMLVideoElement,
  state: MediaSyncEvent["state"],
  event: MediaSyncEvent["event"],
  session: PlaybackSession,
): MediaSyncEvent {
  return {
    video_id: videoID,
    session_id: session.id,
    event_sequence: ++session.sequence,
    state,
    event,
    media_time_ms: mediaTimeMillis(player),
    client_time_ms: Date.now(),
    playback_rate: supportedPlaybackRate(player.playbackRate) ? player.playbackRate : 1,
  };
}

function mediaTimeMillis(player: HTMLVideoElement): number {
  return Number.isFinite(player.currentTime) ? Math.max(0, Math.round(player.currentTime * 1000)) : 0;
}

function supportedPlaybackRate(rate: number): boolean {
  return Number.isFinite(rate) && rate >= 0.25 && rate <= 4;
}

function mediaHasFutureData(player: HTMLVideoElement): boolean {
  return player.readyState >= 3;
}

function syncStatusFromError(reason: unknown): MediaSyncStatus | null {
  if (!(reason instanceof ApiError) || !reason.body || typeof reason.body !== "object" || !("sync" in reason.body)) return null;
  const candidate = (reason.body as { sync?: unknown }).sync;
  if (!candidate || typeof candidate !== "object" || !("state" in candidate)) return null;
  return candidate as MediaSyncStatus;
}

function syncStatusLabel(sync: MediaSyncStatus, locked: boolean, operation: SyncOperation | null): string {
  if (locked) return "Timeline only; this tab does not control motion";
  if (operation) {
    const at = formatTimelineTime(operation.mediaTimeMillis);
    switch (operation.kind) {
      case "starting": return `Starting paired motion at ${at}`;
      case "seeking": return `Seeking to ${at}; stopping prior motion`;
      case "resyncing": return `Resyncing motion to script at ${at}`;
      case "resuming": return `Script aligned at ${at}; resuming video`;
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

function createMediaSessionID(): string {
  try {
    return `media-${crypto.randomUUID()}`;
  } catch {
    return `media-${Date.now()}-${Math.round(Math.random() * 100000)}`;
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
