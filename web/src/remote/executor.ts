// The desktop half of the phone remote (ADR 0032). The tab that holds control
// reports what it shows and carries out the phone's commands with the same
// commands its own controls use: a video command goes to the open player, a
// chat message to the open conversation. Nothing here talks to the device;
// every effect still passes the player's and chat's own admission and Stop.

import type { RemoteChatPresence, RemoteCommand, RemoteOutcome, RemotePresence, RemoteVideoPresence } from "../api/remote-types";
import type { MessageKey } from "../i18n";
import type { VideoPlayerHandle } from "../media/playbackController";
import { videoRoute } from "../videos/route";

export interface RemoteVideoSurface {
  handle: VideoPlayerHandle;
  title: string;
  /** Opens the conversation beside this video. */
  openChat?: () => void;
}

export interface RemoteChatSurface {
  sessionId: string;
  personaName: string;
  /** A reply is streaming; the next message waits for it. */
  busy: boolean;
  /** The composer could send now: history loaded, this tab not read-only. */
  ready: boolean;
  send: (text: string) => void;
}

export interface RemoteExecutorDependencies {
  report: (presence: RemotePresence) => Promise<unknown>;
  withdraw: () => void;
  navigate: (hash: string) => void;
  /**
   * Whether this page may start playback with sound. Browsers refuse it until
   * someone has clicked the page; defaults to the page's user activation.
   */
  canPlaySound?: () => boolean;
  now?: () => number;
}

// Outcomes are canonical English catalog strings; the phone translates them.
export const REMOTE_OUTCOMES = {
  noVideo: "No video is open on the desktop.",
  notReady: "The video is not ready for that yet.",
  needsClick: "The desktop browser blocks sound until someone clicks the MagicHandy page there. Click it once, or mute the video first.",
  noChat: "No chat is open on the desktop.",
  chatBusy: "The desktop is still answering. Send again when the reply finishes.",
  chatUnavailable: "The desktop chat cannot send right now.",
  unknown: "The desktop does not know that command.",
} as const satisfies Record<string, MessageKey>;

const KEEPALIVE_MS = 5_000;
const MIN_REPORT_GAP_MS = 200;
// A refused report (control changing hands, or offline) retries with backoff.
const RETRY_BASE_MS = 500;
// A position this far from where playback should be is a seek or a stall, and
// the phone needs to hear about it; smaller drift waits for the keepalive.
const POSITION_JUMP_MS = 1_000;
const SEEN_LIMIT = 128;
const OUTCOME_LIMIT = 64;

type Result = { ok: true } | { ok: false; error: string };
const done: Result = { ok: true };
const failure = (error: string): Result => ({ ok: false, error });

export function videoPresence(surface: RemoteVideoSurface): RemoteVideoPresence {
  const snapshot = surface.handle.getSnapshot();
  return {
    video_id: surface.handle.videoID,
    title: surface.title,
    playing: snapshot.playbackIntent,
    position_ms: Math.max(0, Math.round(snapshot.currentTimeMillis)),
    duration_ms: Math.max(0, Math.round(snapshot.durationMillis)),
    volume: snapshot.volume,
    muted: snapshot.muted,
    rate: snapshot.playbackRate,
    synchronized: surface.handle.synchronized,
    ...(surface.handle.synchronized ? { sync_state: snapshot.sync.state } : {}),
    ready: !snapshot.scriptLoading,
  };
}

export class RemoteExecutor {
  private readonly deps: RemoteExecutorDependencies;
  private readonly now: () => number;
  private readonly canPlaySound: () => boolean;
  private eligible = false;
  private disposed = false;
  private route = "";
  private video: RemoteVideoSurface | null = null;
  private stopWatchingVideo: (() => void) | null = null;
  private chat: RemoteChatSurface | null = null;
  private outcomes: RemoteOutcome[] = [];
  private readonly seen = new Set<string>();
  private registered = false;
  private inFlight = false;
  private failures = 0;
  private dirty = false;
  private timer: ReturnType<typeof setTimeout> | undefined;
  private lastReportAt = Number.NEGATIVE_INFINITY;
  private reportedVideo = "";
  private reportedPosition = { ms: 0, at: 0, rate: 1, playing: false };

  constructor(deps: RemoteExecutorDependencies) {
    this.deps = deps;
    this.now = deps.now ?? (() => Date.now());
    this.canPlaySound = deps.canPlaySound ?? (() => navigator.userActivation?.hasBeenActive !== false);
  }

  /** Only a visible tab that holds control reports and takes commands. */
  setEligible(eligible: boolean): void {
    if (eligible === this.eligible || this.disposed) return;
    this.eligible = eligible;
    if (eligible) this.schedule(0);
    else this.stopReporting();
  }

  setRoute(route: string): void {
    if (route === this.route) return;
    this.route = route;
    this.schedule(0);
  }

  setVideo(surface: RemoteVideoSurface | null): void {
    if (surface === this.video) return;
    this.stopWatchingVideo?.();
    this.stopWatchingVideo = null;
    this.video = surface;
    if (surface) this.stopWatchingVideo = surface.handle.subscribe(this.videoChanged);
    this.schedule(0);
  }

  setChat(surface: RemoteChatSurface | null): void {
    const previous = this.chat;
    this.chat = surface;
    if (previous?.sessionId !== surface?.sessionId || previous?.personaName !== surface?.personaName ||
      previous?.busy !== surface?.busy || previous?.ready !== surface?.ready) {
      this.schedule(0);
    }
  }

  /** Runs a delivered command once; a redelivery after a reconnect is ignored. */
  execute(command: RemoteCommand): void {
    if (!command?.id || this.seen.has(command.id) || this.disposed) return;
    this.seen.add(command.id);
    if (this.seen.size > SEEN_LIMIT) this.seen.delete(this.seen.values().next().value!);
    // Not ours to run now: the queue drops it when control or presence moves.
    if (!this.eligible) return;
    let result: Result;
    try {
      result = command.target === "chat" ? this.runChat(command) : this.runVideo(command);
    } catch {
      result = failure(REMOTE_OUTCOMES.notReady);
    }
    this.outcomes.push(result.ok ? { command_id: command.id, ok: true } : { command_id: command.id, ok: false, error: result.error });
    this.schedule(0);
  }

  dispose(): void {
    this.stopReporting();
    this.stopWatchingVideo?.();
    this.stopWatchingVideo = null;
    this.disposed = true;
  }

  private runVideo(command: RemoteCommand): Result {
    if (command.target !== "video") return failure(REMOTE_OUTCOMES.unknown);
    if (command.action === "open") {
      if (!command.video_id) return failure(REMOTE_OUTCOMES.unknown);
      this.deps.navigate(videoRoute(command.video_id));
      return done;
    }
    if (command.action === "close") {
      if (this.video) this.deps.navigate(videoRoute(""));
      return done;
    }
    const video = this.video;
    if (!video) return failure(REMOTE_OUTCOMES.noVideo);
    const controls = video.handle.commands;
    // A refused play() would arm paired motion and then stop it again. Ask for
    // the click instead of trying.
    const snapshot = video.handle.getSnapshot();
    const starts = command.action === "play" || (command.action === "toggle" && !snapshot.playbackIntent);
    if (starts && !snapshot.muted && !this.canPlaySound()) return failure(REMOTE_OUTCOMES.needsClick);
    let accepted: boolean;
    switch (command.action) {
      case "play": accepted = controls.play(); break;
      case "pause": accepted = controls.pause(); break;
      case "toggle": accepted = controls.toggle(); break;
      case "seek": accepted = typeof command.ms === "number" && controls.seekTo(command.ms); break;
      case "seek_by": accepted = typeof command.ms === "number" && controls.seekBy(command.ms); break;
      case "volume": accepted = typeof command.value === "number" && controls.setVolume(command.value); break;
      case "rate": accepted = typeof command.value === "number" && controls.setRate(command.value); break;
      case "mute": accepted = typeof command.flag === "boolean" && controls.setMuted(command.flag); break;
      default: return failure(REMOTE_OUTCOMES.unknown);
    }
    return accepted ? done : failure(REMOTE_OUTCOMES.notReady);
  }

  private runChat(command: RemoteCommand): Result {
    if (command.action === "open") {
      if (this.chat) return done;
      if (this.video?.openChat) this.video.openChat();
      else this.deps.navigate("#/chat");
      return done;
    }
    if (command.action !== "send") return failure(REMOTE_OUTCOMES.unknown);
    const chat = this.chat;
    const text = command.text?.trim() ?? "";
    if (!chat) return failure(REMOTE_OUTCOMES.noChat);
    if (chat.busy) return failure(REMOTE_OUTCOMES.chatBusy);
    if (!chat.ready || !text) return failure(REMOTE_OUTCOMES.chatUnavailable);
    chat.send(text);
    return done;
  }

  private readonly videoChanged = (): void => {
    const video = this.video;
    if (!video || !this.eligible) return;
    const presence = videoPresence(video);
    const reported = this.reportedPosition;
    const expected = reported.playing ? reported.ms + (this.now() - reported.at) * reported.rate : reported.ms;
    if (videoSignature(presence) !== this.reportedVideo || Math.abs(presence.position_ms - expected) > POSITION_JUMP_MS) {
      this.schedule(0);
    }
  };

  private schedule(delay: number): void {
    if (!this.eligible || this.disposed) return;
    if (this.inFlight) {
      this.dirty = true;
      return;
    }
    clearTimeout(this.timer);
    const wait = Math.max(delay, this.lastReportAt + MIN_REPORT_GAP_MS - this.now(), 0);
    this.timer = setTimeout(() => void this.flush(), wait);
  }

  private async flush(): Promise<void> {
    this.timer = undefined;
    if (!this.eligible || this.disposed || this.inFlight) return;
    this.inFlight = true;
    this.dirty = false;
    const outcomes = this.outcomes;
    this.outcomes = [];
    const presence: RemotePresence = { route: this.route };
    if (this.video) presence.video = videoPresence(this.video);
    if (this.chat) presence.chat = chatPresence(this.chat);
    if (outcomes.length) presence.outcomes = outcomes;
    const at = this.now();
    this.lastReportAt = at;
    let reported = false;
    try {
      await this.deps.report(presence);
      reported = true;
      this.failures = 0;
      this.registered = true;
      this.reportedVideo = presence.video ? videoSignature(presence.video) : "";
      this.reportedPosition = presence.video
        ? { ms: presence.video.position_ms, at, rate: presence.video.rate, playing: presence.video.playing }
        : { ms: 0, at, rate: 1, playing: false };
    } catch {
      // Refused (control is changing) or offline: keep the outcomes for the
      // next report.
      this.failures += 1;
      this.outcomes = [...outcomes, ...this.outcomes].slice(-OUTCOME_LIMIT);
    } finally {
      this.inFlight = false;
    }
    if (!this.eligible || this.disposed) {
      // Control or visibility changed while this report was in flight.
      if (reported) this.deps.withdraw();
      this.registered = false;
      return;
    }
    if (this.failures > 0) this.schedule(Math.min(KEEPALIVE_MS, RETRY_BASE_MS * 2 ** (this.failures - 1)));
    else this.schedule(this.dirty || this.outcomes.length ? 0 : KEEPALIVE_MS);
  }

  private stopReporting(): void {
    clearTimeout(this.timer);
    this.timer = undefined;
    this.dirty = false;
    this.failures = 0;
    this.outcomes = [];
    if (this.registered) {
      this.registered = false;
      this.deps.withdraw();
    }
  }
}

function chatPresence(chat: RemoteChatSurface): RemoteChatPresence {
  return { session_id: chat.sessionId, persona_name: chat.personaName, busy: chat.busy, ready: chat.ready };
}

function videoSignature(video: RemoteVideoPresence): string {
  return [video.video_id, video.playing, video.volume, video.muted, video.rate, video.ready, video.sync_state ?? "", video.duration_ms].join("|");
}
