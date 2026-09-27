// The control owner for one open video. It turns intent (play, pause, seek,
// speed, script filters) into the media sync protocol for paired videos and
// into plain element operations for unpaired ones, and it publishes one
// snapshot for rendering. On-screen controls, keyboard gestures and remote
// commands all go through the same commands, so no caller needs to know how
// a synchronized run is armed or stopped.
//
// Paired playback rules (docs/video-playback.md): the browser owns intent and
// the engine owns the running clock. A discontinuity stops motion and re-arms
// at the exact media time; it never re-times points the device already holds.

import { ApiError } from "../api/client";
import type { MediaFunscript, MediaPlaybackSettings, MediaSyncEvent, MediaSyncStatus } from "../api/types";
import type { MediaPlaybackPatch } from "../components/PlaybackPanel";
import { MediaElementDriver, mediaHasFutureData, mediaTimeMillis, supportedPlaybackRate, type MediaPlaybackEvent } from "./mediaElement";
import { SyncSession, type MediaSyncSender } from "./syncSession";

const HEARTBEAT_MILLIS = 1_500;
const MEDIA_READY_POLL_MILLIS = 100;
// The video is the follower clock: when it deviates from the engine's
// transport-aligned clock by more than this, the video is nudged. Below it,
// a correction would be more visible than the offset.
const CLOCK_ALIGN_THRESHOLD_MILLIS = 150;
// Resuming is not instant: the pre-play alignment targets the engine clock as
// it reads before the seek and decoder restart, and the clock keeps running
// through both. That start cost lands in one direction — the video begins
// behind the device — and anything under the steady-state threshold would then
// persist for the whole run. One tighter pass once the media clock is actually
// advancing removes it.
const RESUME_ALIGN_THRESHOLD_MILLIS = 40;

export type SyncOperationKind = "starting" | "seeking" | "resyncing" | "resuming";

export interface SyncOperation {
  kind: SyncOperationKind;
  mediaTimeMillis: number;
}

type ArmEvent = "play" | "seeked" | "ratechange" | "resync";

interface ControlledRestart {
  id: number;
  mediaTimeMillis: number;
  resume: boolean;
  stop: Promise<boolean>;
  committed?: boolean;
}

export interface PlaybackSnapshot {
  /** Whether the viewer wants the video playing; drives the play/pause control. */
  playbackIntent: boolean;
  sync: MediaSyncStatus;
  syncError: string;
  operation: SyncOperation | null;
  currentTimeMillis: number;
  durationMillis: number;
  volume: number;
  muted: boolean;
  playbackRate: number;
  script: MediaFunscript | null;
  scriptLoading: boolean;
  scriptError: string;
}

/** The command surface shared by on-screen controls and remote control. */
export interface VideoPlayerCommands {
  play(): boolean;
  pause(): boolean;
  toggle(): boolean;
  seekTo(milliseconds: number): boolean;
  seekBy(deltaMillis: number): boolean;
  setVolume(volume: number): boolean;
  setMuted(muted: boolean): boolean;
  setRate(rate: number): boolean;
}

export interface VideoPlayerHandle {
  readonly videoID: string;
  readonly synchronized: boolean;
  readonly commands: VideoPlayerCommands;
  getSnapshot(): PlaybackSnapshot;
  subscribe(listener: () => void): () => void;
}

export interface PlaybackDependencies {
  mediaSync: MediaSyncSender;
  saveMediaPlayback: (patch: MediaPlaybackPatch) => Promise<MediaPlaybackSettings>;
  loadScript: (videoID: string, signal: AbortSignal) => Promise<MediaFunscript>;
  refresh: () => Promise<unknown>;
}

export interface PlaybackOptions {
  videoID: string;
  /** The catalog paired a script; motion follows the video once it loads. */
  synchronized: boolean;
  durationMillis: number;
  locked: boolean;
  stopSequence?: number;
}

export class VideoPlaybackController implements VideoPlayerHandle {
  readonly videoID: string;
  readonly synchronized: boolean;
  readonly commands: VideoPlayerCommands;

  private readonly deps: PlaybackDependencies;
  private readonly media = new MediaElementDriver();
  private readonly session: SyncSession;
  private readonly listeners = new Set<() => void>();
  private snapshot: PlaybackSnapshot;

  private connected = false;
  private locked: boolean;
  private latestStopSequence: number | undefined;
  private capturedStopSequence: number | undefined;
  // Every request, continuation and wait carries a generation. Advancing one
  // invalidates work started under the old value without tracking it.
  private generation = 0;
  private mediaReadyGeneration = 0;
  private controlledRestartID = 0;
  private desiredPlaying = false;
  private activeSync = false;
  private arming = false;
  private pendingArm: ArmEvent | null = null;
  private armAbort: AbortController | null = null;
  private armAnchor = 0;
  private seekInProgress = false;
  private resumeAfterSeek = false;
  private seekingStop: Promise<boolean> = Promise.resolve(true);
  private awaitingMedia = false;
  private bufferingStop: Promise<boolean> = Promise.resolve(true);
  private readyArm: ArmEvent = "play";
  private mediaReadyTimer: number | undefined;
  private heartbeatTimer: number | undefined;
  private heartbeatPending = false;
  private heartbeatAbort: AbortController | null = null;
  private resumeAlignPending = false;
  private engineClock: { mediaMs: number; atMs: number; rate: number } | null = null;
  private seekGesture: ControlledRestart | null = null;
  private filterRestart: ControlledRestart | null = null;
  private filterWriteChain: Promise<unknown> = Promise.resolve();
  private scriptAbort: AbortController | null = null;

  constructor(options: PlaybackOptions, deps: PlaybackDependencies) {
    this.videoID = options.videoID;
    this.synchronized = options.synchronized;
    this.locked = options.locked;
    this.latestStopSequence = options.stopSequence;
    this.deps = deps;
    this.session = new SyncSession(options.videoID, deps.mediaSync);
    this.snapshot = {
      playbackIntent: false,
      sync: { active: false, state: "idle" },
      syncError: "",
      operation: null,
      currentTimeMillis: 0,
      durationMillis: options.durationMillis,
      volume: 1,
      muted: false,
      playbackRate: 1,
      script: null,
      scriptLoading: options.synchronized,
      scriptError: "",
    };
    this.commands = {
      play: () => this.play(),
      pause: () => this.pause(),
      toggle: () => (this.snapshot.playbackIntent ? this.pause() : this.play()),
      seekTo: (milliseconds) => this.seekTo(milliseconds),
      seekBy: (deltaMillis) => this.seekTo(this.snapshot.currentTimeMillis + deltaMillis),
      setVolume: (volume) => this.setVolume(volume),
      setMuted: (muted) => this.setMuted(muted),
      setRate: (rate) => this.setRate(rate),
    };
  }

  // --- Store -----------------------------------------------------------

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };

  getSnapshot = (): PlaybackSnapshot => this.snapshot;

  private update(patch: Partial<PlaybackSnapshot>): void {
    this.snapshot = { ...this.snapshot, ...patch };
    for (const listener of this.listeners) listener();
  }

  // --- Lifecycle -------------------------------------------------------

  /** Mounted: load the paired script and resume the heartbeat. */
  connect(): void {
    this.connected = true;
    this.loadScript();
    this.syncHeartbeat();
  }

  /**
   * Unmounted: close admission before canceling requests so no late heartbeat
   * or queued continuation can re-arm this session, then report an armed
   * session's end.
   */
  disconnect(): void {
    this.connected = false;
    this.desiredPlaying = false;
    this.resumeAfterSeek = false;
    this.controlledRestartID += 1;
    this.seekGesture = null;
    this.filterRestart = null;
    this.invalidatePlaybackRequests();
    this.session.abortAll();
    this.awaitingMedia = false;
    this.mediaReadyGeneration += 1;
    this.clearMediaReadyPoll();
    this.stopHeartbeat();
    this.scriptAbort?.abort();
    this.scriptAbort = null;
    this.media.dispose();
    this.activeSync = false;
    const player = this.media.element;
    if (player) this.session.close(player);
  }

  attach = (element: HTMLVideoElement | null): void => {
    this.media.attach(element);
  };

  setDuration = (durationMillis: number): void => {
    if (durationMillis === this.snapshot.durationMillis) return;
    this.update({ durationMillis });
  };

  /** A Stop from anywhere ends this run; the viewer presses play to start a new one. */
  setStopSequence(stopSequence: number | undefined): void {
    const previous = this.latestStopSequence;
    this.latestStopSequence = stopSequence;
    if (previous === undefined || stopSequence === undefined || previous === stopSequence || !this.synchronized) return;
    this.abandonRun();
    this.bufferingStop = Promise.resolve(true);
    this.activeSync = false;
    this.update({ sync: { active: false, state: "stopped", last_event: "emergency_stop", message: "Motion was stopped. Press play to start a new synchronized run." } });
  }

  /** Losing control holds the picture; the tab can no longer command motion. */
  setLocked(locked: boolean): void {
    const wasLocked = this.locked;
    this.locked = locked;
    this.syncHeartbeat();
    if (!locked || wasLocked || !this.synchronized) return;
    this.abandonRun();
    this.activeSync = false;
    this.update({ sync: { active: false, state: "interrupted", message: "Controller access changed; video playback paused and synchronized motion stopped." } });
  }

  private abandonRun(): void {
    this.setDesiredPlayback(false);
    this.update({ operation: null });
    this.seekGesture = null;
    this.filterRestart = null;
    this.seekInProgress = false;
    this.resumeAfterSeek = false;
    this.invalidatePlaybackRequests();
    this.awaitingMedia = false;
    this.mediaReadyGeneration += 1;
    this.clearMediaReadyPoll();
    const player = this.media.mounted;
    if (player) this.holdVideo(player);
  }

  private loadScript(): void {
    if (!this.synchronized || this.snapshot.script || this.scriptAbort) return;
    const controller = new AbortController();
    this.scriptAbort = controller;
    void this.deps.loadScript(this.videoID, controller.signal).then((script) => {
      if (controller.signal.aborted || !this.connected) return;
      this.update({ script });
      this.syncHeartbeat();
    }).catch((reason) => {
      if (controller.signal.aborted || !this.connected) return;
      this.update({ scriptError: reason instanceof Error ? reason.message : "The paired funscript could not be loaded." });
    }).finally(() => {
      if (this.scriptAbort === controller) this.scriptAbort = null;
      if (!controller.signal.aborted && this.connected) this.update({ scriptLoading: false });
    });
  }

  // --- Commands ----------------------------------------------------------

  /** The on-screen play/pause control for a paired video. */
  togglePlayback = (): void => {
    const player = this.media.element;
    const script = this.snapshot.script;
    if (!player || !script) return;
    if (this.snapshot.playbackIntent) {
      this.controlledRestartID += 1;
      this.seekGesture = null;
      this.filterRestart = null;
      this.seekInProgress = false;
      this.resumeAfterSeek = false;
      this.awaitingMedia = false;
      this.mediaReadyGeneration += 1;
      this.clearMediaReadyPoll();
      this.invalidatePlaybackRequests();
      this.setDesiredPlayback(false);
      this.update({ operation: null });
      this.holdVideo(player);
      if (this.activeSync || this.arming) void this.stopPlaybackMotion(player, "paused", "pause");
      return;
    }

    this.update({ playbackIntent: true });
    if (this.locked || mediaTimeMillis(player) >= script.duration_ms) {
      this.desiredPlaying = false;
      void player.play().catch((reason) => {
        this.update({ playbackIntent: false });
        this.showSyncFailure(reason, "Video playback could not start.");
      });
      return;
    }
    this.setDesiredPlayback(true);
    this.holdVideoAt(player, mediaTimeMillis(player));
    void this.armPlayback(player, "play");
  };

  private pairedReady(): boolean {
    return this.synchronized && this.snapshot.script !== null;
  }

  /** Paired videos wait for their script; an unloadable one plays as a plain video. */
  private commandsBlocked(): boolean {
    return this.synchronized && this.snapshot.scriptLoading;
  }

  private play(): boolean {
    const player = this.media.element;
    if (!player || this.commandsBlocked()) return false;
    if (this.pairedReady()) {
      if (!this.snapshot.playbackIntent) this.togglePlayback();
      return true;
    }
    void player.play().catch(() => undefined);
    return true;
  }

  private pause(): boolean {
    const player = this.media.element;
    if (!player || this.commandsBlocked()) return false;
    if (this.pairedReady()) {
      if (this.snapshot.playbackIntent) this.togglePlayback();
      return true;
    }
    player.pause();
    return true;
  }

  private seekTo(milliseconds: number): boolean {
    const player = this.media.element;
    if (!player || this.commandsBlocked() || !Number.isFinite(milliseconds)) return false;
    const duration = this.snapshot.durationMillis || this.snapshot.script?.duration_ms || 0;
    const target = Math.max(0, duration > 0 ? Math.min(duration, milliseconds) : milliseconds);
    if (this.pairedReady()) {
      this.beginSeek();
      this.commitSeek(target);
      return true;
    }
    player.currentTime = target / 1000;
    this.update({ currentTimeMillis: Math.round(target) });
    return true;
  }

  setVolume = (volume: number): boolean => {
    const player = this.media.element;
    const next = Math.max(0, Math.min(1, Number.isFinite(volume) ? volume : 1));
    this.update({ volume: next });
    if (!player) return false;
    player.volume = next;
    if (next > 0 && player.muted) {
      player.muted = false;
      this.update({ muted: false });
    }
    return true;
  };

  setMuted = (muted: boolean): boolean => {
    const player = this.media.element;
    this.update({ muted });
    if (!player) return false;
    player.muted = muted;
    return true;
  };

  /** A paired run re-arms on the resulting ratechange; the device never re-times. */
  setRate = (rate: number): boolean => {
    const player = this.media.element;
    if (!player || !supportedPlaybackRate(rate)) return false;
    player.playbackRate = rate;
    this.update({ playbackRate: rate });
    return true;
  };

  // --- Seek and filter gestures -----------------------------------------------

  /** Freezes the picture and stops motion; one commit re-arms at the target. */
  beginSeek = (): void => {
    const player = this.media.element;
    if (!player || !this.snapshot.script || this.locked || (this.seekGesture && !this.seekGesture.committed)) return;
    const previousStop = this.seekGesture?.stop;
    const atMillis = mediaTimeMillis(player);
    const resume = this.desiredPlaying || this.activeSync || this.arming || !player.paused;
    const id = ++this.controlledRestartID;
    const mustStop = this.activeSync || this.arming;

    this.resumeAfterSeek = resume;
    this.seekInProgress = true;
    this.awaitingMedia = false;
    this.mediaReadyGeneration += 1;
    this.clearMediaReadyPoll();
    this.invalidatePlaybackRequests();
    this.holdVideoAt(player, atMillis);
    this.update({ operation: { kind: "seeking", mediaTimeMillis: atMillis } });
    const stop = mustStop
      ? this.stopPlaybackMotion(player, "seeking", "seeking")
      : previousStop ?? Promise.resolve(true);
    this.seekGesture = { id, mediaTimeMillis: atMillis, resume, stop };
  };

  commitSeek = (milliseconds: number, event: "seeked" | "ratechange" = "seeked"): void => {
    const player = this.media.element;
    if (!player) return;
    const script = this.snapshot.script;
    if (!script || this.locked) {
      this.setPlayerTime(player, milliseconds);
      return;
    }
    if (!this.seekGesture) this.beginSeek();
    if (!this.seekGesture) return;
    // Each commit replaces the previous continuation, including repeated keys
    // or a new scrub released before the original Stop has been acknowledged.
    const gesture = { ...this.seekGesture, id: ++this.controlledRestartID, committed: true };
    this.seekGesture = gesture;

    const target = Math.max(0, Math.min(this.snapshot.durationMillis || script.duration_ms, Math.round(milliseconds)));
    this.setPlayerTime(player, target);
    this.update({ operation: { kind: "seeking", mediaTimeMillis: target } });
    void gesture.stop.then(async (stopped) => {
      if (!this.connected || this.seekGesture?.id !== gesture.id) return;
      this.seekGesture = null;
      this.seekInProgress = false;
      this.resumeAfterSeek = false;

      if (!stopped || !gesture.resume) {
        this.setDesiredPlayback(false);
        this.update({ operation: null });
        return;
      }
      if (target >= script.duration_ms) {
        this.desiredPlaying = false;
        this.update({ playbackIntent: true, operation: null });
        try {
          await this.media.play(player);
        } catch (reason) {
          this.update({ playbackIntent: false });
          this.showSyncFailure(reason, "Video playback could not resume after seeking.");
        }
        return;
      }

      this.setDesiredPlayback(true);
      this.update({ operation: { kind: "resyncing", mediaTimeMillis: target } });
      await this.armPlayback(player, event);
    });
  };

  cancelSeek = (): void => {
    const gesture = this.seekGesture;
    if (!gesture) return;
    this.commitSeek(gesture.mediaTimeMillis);
  };

  /** Accepted points cannot be rewritten, so a filter change stops the run first. */
  beginFilterChange = (): void => {
    const player = this.media.element;
    if (!player || !this.snapshot.script || this.locked || this.filterRestart) return;
    const atMillis = mediaTimeMillis(player);
    const resume = this.desiredPlaying || this.activeSync || this.arming || !player.paused;
    const id = ++this.controlledRestartID;
    const mustStop = this.activeSync || this.arming;

    this.awaitingMedia = false;
    this.mediaReadyGeneration += 1;
    this.clearMediaReadyPoll();
    this.invalidatePlaybackRequests();
    this.holdVideoAt(player, atMillis);
    if (resume) this.update({ operation: { kind: "resyncing", mediaTimeMillis: atMillis } });
    const stop = mustStop
      ? this.stopPlaybackMotion(player, "paused", "pause")
      : Promise.resolve(true);
    this.filterRestart = { id, mediaTimeMillis: atMillis, resume, stop };
  };

  applyPlaybackFilters = (patch: MediaPlaybackPatch): Promise<MediaPlaybackSettings> => {
    const operation = async () => {
      const ownsPlayer = this.connected;
      if (ownsPlayer && !this.filterRestart) this.beginFilterChange();
      const restart = ownsPlayer ? this.filterRestart : null;
      if (!restart) {
        const saved = await this.deps.saveMediaPlayback(patch);
        await this.deps.refresh();
        return saved;
      }

      let writeError: unknown;
      let saved!: MediaPlaybackSettings;
      let stopped = false;
      try {
        stopped = await restart.stop;
        saved = await this.deps.saveMediaPlayback(patch);
        await this.deps.refresh();
      } catch (reason) {
        writeError = reason;
        // A runtime error can follow a successful settings save. Reconcile
        // the actual backend values while keeping the original failure visible.
        await this.deps.refresh().catch(() => undefined);
      }

      if (this.connected && this.filterRestart?.id === restart.id) {
        this.filterRestart = null;
        const player = this.media.element;
        if (player && restart.resume && stopped && !writeError) {
          this.holdVideoAt(player, restart.mediaTimeMillis);
          this.setDesiredPlayback(true);
          this.update({ operation: { kind: "resyncing", mediaTimeMillis: restart.mediaTimeMillis } });
          await this.armPlayback(player, "resync");
        } else {
          this.setDesiredPlayback(false);
          this.update({ operation: null });
        }
      }
      if (writeError) throw writeError;
      return saved;
    };

    const queued = this.filterWriteChain.then(operation, operation);
    this.filterWriteChain = queued.catch(() => undefined);
    return queued;
  };

  // --- Media element events ---------------------------------------------------

  /**
   * The media clock advancing is the first honest evidence that playback
   * really resumed, so it is where the tighter post-resume alignment belongs.
   */
  handleTimeChange = (timeMillis: number): void => {
    this.update({ currentTimeMillis: timeMillis });
    if (!this.resumeAlignPending) return;
    const player = this.media.element;
    if (!player || player.paused || this.arming || this.seekInProgress || !this.activeSync) return;
    this.resumeAlignPending = false;
    this.alignPlayerToEngineClock(player, RESUME_ALIGN_THRESHOLD_MILLIS);
  };

  handleMediaEvent = (event: MediaPlaybackEvent, player: HTMLVideoElement): void => {
    this.media.observe(player);
    this.update({ currentTimeMillis: mediaTimeMillis(player) });
    if (event === "ratechange") this.update({ playbackRate: player.playbackRate });
    if (event === "volumechange") {
      this.update({ volume: player.volume, muted: player.muted });
      return;
    }
    const script = this.snapshot.script;
    if (!this.synchronized || !script) {
      if (this.synchronized && this.snapshot.scriptLoading && event === "play") {
        this.holdVideo(player);
        return;
      }
      this.recordPlainIntent(event);
      return;
    }
    if (this.locked) {
      this.recordPlainIntent(event);
      return;
    }

    switch (event) {
      case "playing":
        this.update({ playbackIntent: true });
        if (this.desiredPlaying && this.activeSync) this.update({ operation: null });
        return;
      case "play":
        this.handlePlay(player, script);
        return;
      case "pause":
        this.handlePause(player);
        return;
      case "seeking":
        this.handleSeeking(player);
        return;
      case "seeked":
        this.handleSeeked(player);
        return;
      case "ratechange":
        if (this.desiredPlaying) {
          this.beginSeek();
          this.commitSeek(mediaTimeMillis(player), "ratechange");
        }
        return;
      case "ended":
        this.setDesiredPlayback(false);
        this.update({ operation: null });
        this.invalidatePlaybackRequests();
        this.awaitingMedia = false;
        void this.stopPlaybackMotion(player, "ended", "ended");
        return;
      case "canplay":
        this.resumeWhenMediaReady(player, this.mediaReadyGeneration);
        return;
      case "stalled":
        // Advisory only: a player that still has buffered data keeps playing.
        return;
      case "waiting":
        this.handleWaiting(player);
        return;
      case "error":
        if (!this.desiredPlaying && !this.activeSync && !this.arming) return;
        this.setDesiredPlayback(false);
        this.awaitingMedia = false;
        this.update({ operation: null });
        this.invalidatePlaybackRequests();
        this.holdVideo(player);
        void this.stopPlaybackMotion(player, "paused", "error");
        return;
    }
  };

  /** Unpaired or read-only playback: the element's own state is the intent. */
  private recordPlainIntent(event: MediaPlaybackEvent): void {
    if (event === "play" || event === "playing") this.update({ playbackIntent: true });
    if (event === "pause" || event === "ended" || event === "error") this.update({ playbackIntent: false });
  }

  private handlePlay(player: HTMLVideoElement, script: MediaFunscript): void {
    if (this.media.consume("play")) {
      this.update({ playbackIntent: true });
      return;
    }
    if (this.arming) {
      // The browser started early; hold at the timestamp being armed.
      this.holdVideoAt(player, this.armAnchor);
      return;
    }
    if (mediaTimeMillis(player) >= script.duration_ms) {
      this.desiredPlaying = false;
      this.activeSync = false;
      this.awaitingMedia = false;
      this.update({
        playbackIntent: true,
        sync: {
          active: false,
          video_id: this.videoID,
          state: "completed",
          last_event: "play",
          media_time_ms: mediaTimeMillis(player),
          message: "The paired script has ended; video playback continues without motion.",
        },
      });
      return;
    }
    this.setDesiredPlayback(true);
    void this.armPlayback(player, "play");
  }

  private handlePause(player: HTMLVideoElement): void {
    if (this.media.consume("pause")) return;
    if (this.seekInProgress || player.ended || (this.resumeAfterSeek && this.desiredPlaying)) return;
    this.setDesiredPlayback(false);
    this.resumeAfterSeek = false;
    this.update({ operation: null });
    this.invalidatePlaybackRequests();
    this.awaitingMedia = false;
    this.mediaReadyGeneration += 1;
    this.clearMediaReadyPoll();
    void this.stopPlaybackMotion(player, "paused", "pause");
  }

  private handleSeeking(player: HTMLVideoElement): void {
    if (this.media.seekingInternally) return;
    if (this.seekInProgress) return;
    this.seekInProgress = true;
    this.update({ operation: { kind: "seeking", mediaTimeMillis: mediaTimeMillis(player) } });
    this.resumeAfterSeek = this.desiredPlaying || !player.paused;
    this.awaitingMedia = false;
    this.holdVideo(player);
    this.mediaReadyGeneration += 1;
    this.clearMediaReadyPoll();
    this.invalidatePlaybackRequests();
    this.seekingStop = this.stopPlaybackMotion(player, "seeking", "seeking");
  }

  private handleSeeked(player: HTMLVideoElement): void {
    if (this.media.seekingInternally) {
      this.media.finishInternalSeek();
      this.resumeWhenMediaReady(player, this.mediaReadyGeneration);
      return;
    }
    if (this.seekGesture) return;
    if (!this.seekInProgress) {
      this.resumeWhenMediaReady(player, this.mediaReadyGeneration);
      return;
    }
    const shouldResume = this.resumeAfterSeek;
    const seekGeneration = this.generation;
    this.seekInProgress = false;
    if (shouldResume) {
      this.setDesiredPlayback(true);
      this.update({ operation: { kind: "resyncing", mediaTimeMillis: mediaTimeMillis(player) } });
      void this.seekingStop.then((stopped) => {
        if (stopped && this.generation === seekGeneration) return this.armPlayback(player, "seeked");
      });
    } else {
      this.setDesiredPlayback(false);
      this.update({ operation: null });
      void this.seekingStop.then((stopped) => {
        if (stopped && this.generation === seekGeneration) return this.stopPlaybackMotion(player, "paused", "seeked");
      });
    }
  }

  private handleWaiting(player: HTMLVideoElement): void {
    // A clock-alignment nudge transiently drops readyState and fires `waiting`
    // even inside a fully buffered range. Treating that dip as starvation would
    // stop motion, re-arm, nudge again on the next arm, and loop forever.
    if (this.media.seekingInternally) return;
    if (!this.desiredPlaying && !this.activeSync && !this.arming) return;
    const mustStop = this.activeSync || this.arming;
    this.desiredPlaying = true;
    this.awaitingMedia = true;
    this.readyArm = "resync";
    this.invalidatePlaybackRequests();
    this.holdVideo(player);
    this.activeSync = false;
    this.update({
      operation: null,
      syncError: "",
      sync: {
        active: false,
        video_id: this.videoID,
        state: "paused",
        last_event: "waiting",
        media_time_ms: mediaTimeMillis(player),
        message: "Video is buffering; device motion is stopped.",
      },
    });
    if (mustStop) this.bufferingStop = this.stopPlaybackMotion(player, "paused", "waiting");
    this.waitForMediaReady(player);
  }

  // --- Synchronized run --------------------------------------------------------

  private async armPlayback(player: HTMLVideoElement, event: ArmEvent): Promise<void> {
    if (!this.connected || !this.desiredPlaying) return;
    if (this.arming) {
      this.pendingArm = event;
      this.generation += 1;
      return;
    }
    if (this.locked || !this.snapshot.script) return;
    const sequence = this.latestStopSequence;
    const armMediaTimeMillis = mediaTimeMillis(player);
    this.armAnchor = armMediaTimeMillis;
    this.holdVideoAt(player, armMediaTimeMillis);
    if (sequence === undefined) {
      this.setDesiredPlayback(false);
      this.showSyncFailure(new Error("The safety state is still loading. Press play again when the app is ready."), "The safety state is unavailable.");
      return;
    }
    if (!supportedPlaybackRate(player.playbackRate)) {
      this.setDesiredPlayback(false);
      this.showSyncFailure(new Error("Synchronized playback supports video speeds from 0.25x to 4x."), "The video speed is unsupported.");
      return;
    }
    if (!mediaHasFutureData(player)) {
      this.awaitingMedia = true;
      this.readyArm = event;
      if (this.activeSync) {
        this.bufferingStop = this.stopPlaybackMotion(player, "paused", "waiting");
      } else {
        this.update({ syncError: "", sync: { active: false, video_id: this.videoID, state: "seeking", last_event: "waiting", message: "Buffering video before motion starts." } });
      }
      this.waitForMediaReady(player);
      return;
    }
    this.awaitingMedia = false;
    this.update({ operation: { kind: event === "play" ? "starting" : "resyncing", mediaTimeMillis: armMediaTimeMillis } });

    const commandGeneration = ++this.generation;
    this.capturedStopSequence = sequence;
    this.session.armedStopSequence = sequence;
    const controller = new AbortController();
    this.armAbort = controller;
    this.arming = true;
    this.update({ syncError: "", sync: { active: false, video_id: this.videoID, state: "seeking", last_event: event, message: "Arming paired-script motion." } });
    try {
      const status = await this.syncEvent(player, "playing", event, sequence, false, controller.signal);
      if (!this.connected || this.generation !== commandGeneration || !this.desiredPlaying) {
        if (status.active) await this.stopPlaybackMotion(player, "paused", "pause");
        return;
      }
      if (status.state === "completed") {
        this.desiredPlaying = false;
        this.update({ playbackIntent: true, operation: null });
        await this.media.play(player);
        return;
      }
      if (!status.active) {
        this.setDesiredPlayback(false);
        this.update({ operation: null });
        return;
      }
      const alignedMediaTimeMillis = typeof status.expected_media_time_ms === "number"
        ? Math.max(0, Math.round(status.expected_media_time_ms))
        : armMediaTimeMillis;
      this.update({ operation: { kind: "resuming", mediaTimeMillis: alignedMediaTimeMillis } });
      try {
        // The engine clock has been running since transport play; move the
        // still-held video onto it so playback resumes already in sync.
        this.alignPlayerToEngineClock(player);
        await this.media.play(player);
        // Seek and decoder restart both happened after that reading, so
        // re-check once the media clock is actually advancing.
        this.resumeAlignPending = true;
        this.resumeAfterSeek = false;
      } catch (reason) {
        this.setDesiredPlayback(false);
        await this.stopPlaybackMotion(player, "paused", "pause");
        throw reason;
      }
    } catch (reason) {
      if (controller.signal.aborted) return;
      this.setDesiredPlayback(false);
      this.holdVideoAt(player, armMediaTimeMillis);
      this.showSyncFailure(reason, "Paired-script motion could not be synchronized.");
    } finally {
      if (this.armAbort === controller) this.armAbort = null;
      this.arming = false;
      const next = this.pendingArm;
      this.pendingArm = null;
      if (next && this.desiredPlaying && this.connected) {
        window.queueMicrotask(() => void this.armPlayback(player, next));
      }
    }
  }

  private async stopPlaybackMotion(
    player: HTMLVideoElement,
    state: "paused" | "seeking" | "ended" | "closed",
    event: MediaSyncEvent["event"],
  ): Promise<boolean> {
    this.activeSync = false;
    const requestGeneration = this.generation;
    const sequence = this.capturedStopSequence ?? this.latestStopSequence;
    if (sequence === undefined || this.locked) return false;
    try {
      await this.syncEvent(player, state, event, sequence, state === "closed");
      return true;
    } catch (reason) {
      if (this.generation === requestGeneration && this.connected) {
        this.setDesiredPlayback(false);
        this.showSyncFailure(reason, "Device motion could not be stopped from the video player.");
      }
      return false;
    }
  }

  private async syncEvent(
    player: HTMLVideoElement,
    state: MediaSyncEvent["state"],
    event: MediaSyncEvent["event"],
    sequence: number,
    keepalive = false,
    signal?: AbortSignal,
  ): Promise<MediaSyncStatus> {
    this.media.observe(player);
    const requestGeneration = this.generation;
    const exchange = await this.session.post(player, state, event, sequence, keepalive, signal);
    if (this.connected
      && this.generation === requestGeneration
      && exchange.request.event_sequence === this.session.latestSequence
      && !exchange.signal?.aborted) this.updateSync(exchange.status);
    return exchange.status;
  }

  private updateSync(status: MediaSyncStatus): void {
    if (!this.connected) return;
    this.activeSync = status.active;
    if (status.active && typeof status.expected_media_time_ms === "number") {
      this.engineClock = {
        mediaMs: status.expected_media_time_ms,
        atMs: performance.now(),
        rate: status.playback_rate && status.playback_rate > 0 ? status.playback_rate : 1,
      };
    } else if (!status.active) {
      this.engineClock = null;
    }
    this.update({ sync: status, syncError: "" });
  }

  // The engine's buffered device stream cannot be cheaply re-timed, so once
  // armed the engine clock owns synchronization and the video follows it.
  // Corrections are marked internal so the resulting seeking/seeked pair is not
  // mistaken for a user seek that must stop motion.
  private alignPlayerToEngineClock(player: HTMLVideoElement, thresholdMillis = CLOCK_ALIGN_THRESHOLD_MILLIS): void {
    const clock = this.engineClock;
    if (!clock || !this.activeSync || this.seekInProgress || player.seeking) return;
    const projectedMs = clock.mediaMs + (performance.now() - clock.atMs) * clock.rate;
    const deltaMs = projectedMs - mediaTimeMillis(player);
    if (!Number.isFinite(deltaMs) || Math.abs(deltaMs) < thresholdMillis) return;
    this.media.markInternalSeek();
    player.currentTime = Math.max(0, projectedMs) / 1000;
  }

  private showSyncFailure(reason: unknown, fallback: string): void {
    if (!this.connected) return;
    this.activeSync = false;
    this.awaitingMedia = false;
    const status = syncStatusFromError(reason);
    this.update({
      operation: null,
      sync: status ?? { active: false, state: "error", message: fallback },
      syncError: reason instanceof Error && reason.message ? reason.message : fallback,
    });
  }

  private syncHeartbeat(): void {
    const running = this.connected && this.snapshot.script !== null && !this.locked;
    if (running && this.heartbeatTimer === undefined) {
      this.heartbeatTimer = window.setInterval(() => this.heartbeat(), HEARTBEAT_MILLIS);
    } else if (!running) {
      this.stopHeartbeat();
    }
  }

  private stopHeartbeat(): void {
    if (this.heartbeatTimer === undefined) return;
    window.clearInterval(this.heartbeatTimer);
    this.heartbeatTimer = undefined;
  }

  /** A heartbeat can validate or stop a run; it never starts one. */
  private heartbeat(): void {
    const player = this.media.element;
    if (!player || !this.desiredPlaying || !this.activeSync || this.arming || this.heartbeatPending || player.paused) return;
    const sequence = this.capturedStopSequence;
    if (sequence === undefined) return;
    const controller = new AbortController();
    const requestGeneration = this.generation;
    this.heartbeatPending = true;
    this.heartbeatAbort = controller;
    void this.syncEvent(player, "playing", "heartbeat", sequence, false, controller.signal).then((status) => {
      if (controller.signal.aborted || !this.connected || this.generation !== requestGeneration || !this.desiredPlaying) return;
      if (status.requires_reanchor && this.desiredPlaying) {
        void this.armPlayback(player, "resync");
        return;
      }
      if (status.active) this.alignPlayerToEngineClock(player);
    }).catch((reason) => {
      if (controller.signal.aborted || !this.connected || this.generation !== requestGeneration) return;
      this.setDesiredPlayback(false);
      this.holdVideo(player);
      this.showSyncFailure(reason, "Video synchronization was interrupted; motion stopped.");
    }).finally(() => {
      if (this.heartbeatAbort === controller) {
        this.heartbeatAbort = null;
        this.heartbeatPending = false;
      }
    });
  }

  // --- Media readiness ---------------------------------------------------------

  private resumeWhenMediaReady(player: HTMLVideoElement, waitGeneration: number): void {
    if (
      waitGeneration !== this.mediaReadyGeneration
      || !this.connected
      || !this.awaitingMedia
      || !this.desiredPlaying
      || this.seekInProgress
    ) {
      if (!this.awaitingMedia || !this.desiredPlaying || !this.connected) this.clearMediaReadyPoll();
      return;
    }
    if (!mediaHasFutureData(player)) return;

    this.clearMediaReadyPoll();
    const recovery = this.bufferingStop;
    const event = this.readyArm;
    void recovery.then((stopped) => {
      if (
        !stopped
        || waitGeneration !== this.mediaReadyGeneration
        || !this.connected
        || !this.awaitingMedia
        || !this.desiredPlaying
        || this.seekInProgress
        || !mediaHasFutureData(player)
      ) return;
      this.awaitingMedia = false;
      void this.armPlayback(player, event);
    });
  }

  // Browsers can advance readyState without another canplay event, so a
  // short readiness poll backs up the event while motion waits for data.
  private waitForMediaReady(player: HTMLVideoElement): void {
    this.clearMediaReadyPoll();
    const waitGeneration = ++this.mediaReadyGeneration;
    this.mediaReadyTimer = window.setInterval(() => this.resumeWhenMediaReady(player, waitGeneration), MEDIA_READY_POLL_MILLIS);
  }

  private clearMediaReadyPoll(): void {
    window.clearInterval(this.mediaReadyTimer);
    this.mediaReadyTimer = undefined;
  }

  // --- Small helpers -------------------------------------------------------------

  private invalidatePlaybackRequests(): void {
    this.generation += 1;
    this.pendingArm = null;
    this.armAbort?.abort();
    this.heartbeatAbort?.abort();
    this.heartbeatAbort = null;
    this.heartbeatPending = false;
  }

  private setDesiredPlayback(desired: boolean): void {
    this.desiredPlaying = desired;
    this.update({ playbackIntent: desired });
  }

  private holdVideo(player: HTMLVideoElement): void {
    // Any hold ends the resume this alignment belonged to; the next arm sets
    // it again. Leaving it armed would fire a correction against a stale clock.
    this.resumeAlignPending = false;
    this.media.hold(player);
  }

  private setPlayerTime(player: HTMLVideoElement, milliseconds: number): void {
    this.update({ currentTimeMillis: this.media.setTime(player, milliseconds) });
  }

  private holdVideoAt(player: HTMLVideoElement, milliseconds: number): void {
    this.holdVideo(player);
    this.setPlayerTime(player, milliseconds);
  }
}

function syncStatusFromError(reason: unknown): MediaSyncStatus | null {
  if (!(reason instanceof ApiError) || !reason.body || typeof reason.body !== "object" || !("sync" in reason.body)) return null;
  const candidate = (reason.body as { sync?: unknown }).sync;
  if (!candidate || typeof candidate !== "object" || !("state" in candidate)) return null;
  return candidate as MediaSyncStatus;
}
