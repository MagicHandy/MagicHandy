// The session owner: one mounted player's identity in the media sync protocol.
// A random session id and an increasing event sequence let the backend fence a
// delayed arm or close from an older player, and every event carries the
// Emergency Stop sequence that was current when its run was admitted.

import type { MediaSyncEvent, MediaSyncStatus } from "../api/types";
import { mediaTimeMillis, supportedPlaybackRate } from "./mediaElement";

export type MediaSyncSender = (
  event: MediaSyncEvent,
  stopSequence: number,
  signal?: AbortSignal,
  keepalive?: boolean,
) => Promise<{ sync: MediaSyncStatus }>;

export interface SyncExchange {
  request: MediaSyncEvent;
  status: MediaSyncStatus;
  signal?: AbortSignal;
}

export class SyncSession {
  readonly id = createMediaSessionID();
  /** Set by the first arm; only an armed session reports its close. */
  armedStopSequence: number | undefined;
  private sequence = 0;
  private readonly owned = new Set<AbortController>();

  constructor(private readonly videoID: string, private readonly sender: MediaSyncSender) {}

  get latestSequence(): number {
    return this.sequence;
  }

  /** Sends one event. Requests without a caller signal are aborted by abortAll. */
  async post(
    player: HTMLVideoElement,
    state: MediaSyncEvent["state"],
    event: MediaSyncEvent["event"],
    stopSequence: number,
    keepalive = false,
    signal?: AbortSignal,
  ): Promise<SyncExchange> {
    const owned = signal ? null : new AbortController();
    if (owned) this.owned.add(owned);
    const request = this.build(player, state, event);
    const effectiveSignal = signal ?? owned?.signal;
    try {
      const response = await this.sender(request, stopSequence, effectiveSignal, keepalive);
      return { request, status: response.sync, signal: effectiveSignal };
    } finally {
      if (owned) this.owned.delete(owned);
    }
  }

  /** Reports an armed session's end; delivery survives page teardown. */
  close(player: HTMLVideoElement): void {
    const sequence = this.armedStopSequence;
    if (sequence === undefined) return;
    void this.sender(this.build(player, "closed", "closed"), sequence, undefined, true).catch(() => undefined);
  }

  abortAll(): void {
    for (const controller of this.owned) controller.abort();
    this.owned.clear();
  }

  private build(player: HTMLVideoElement, state: MediaSyncEvent["state"], event: MediaSyncEvent["event"]): MediaSyncEvent {
    return {
      video_id: this.videoID,
      session_id: this.id,
      event_sequence: ++this.sequence,
      state,
      event,
      media_time_ms: mediaTimeMillis(player),
      client_time_ms: Date.now(),
      playback_rate: supportedPlaybackRate(player.playbackRate) ? player.playbackRate : 1,
    };
  }
}

function createMediaSessionID(): string {
  try {
    return `media-${crypto.randomUUID()}`;
  } catch {
    return `media-${Date.now()}-${Math.round(Math.random() * 100000)}`;
  }
}
