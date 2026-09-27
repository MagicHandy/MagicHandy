// The decoder owner. Every write the app makes to the <video> element goes
// through here. Holding the picture and correcting its clock raise media
// events the browser cannot tell apart from the user's, so the driver
// remembers the ones the app caused and lets the controller consume them.

const SELF_EVENT_WINDOW_MILLIS = 1_000;

/** The element events the player forwards to its playback controller. */
export type MediaPlaybackEvent =
  | "play" | "playing" | "pause" | "seeking" | "seeked" | "ended" | "ratechange"
  | "volumechange" | "waiting" | "stalled" | "canplay" | "error";

export type SelfEvent = "play" | "pause";

export function mediaTimeMillis(player: HTMLVideoElement): number {
  return Number.isFinite(player.currentTime) ? Math.max(0, Math.round(player.currentTime * 1000)) : 0;
}

export function supportedPlaybackRate(rate: number): boolean {
  return Number.isFinite(rate) && rate >= 0.25 && rate <= 4;
}

export function mediaHasFutureData(player: HTMLVideoElement): boolean {
  return !player.seeking && player.readyState >= 3;
}

export class MediaElementDriver {
  private attached: HTMLVideoElement | null = null;
  private lastSeen: HTMLVideoElement | null = null;
  private readonly selfEvents: Record<SelfEvent, boolean> = { play: false, pause: false };
  private readonly selfEventTimers: Partial<Record<SelfEvent, number>> = {};
  private internalSeek = false;
  private internalSeekTimer: number | undefined;

  /** Follows the mounted element; null when React detaches it. */
  attach(element: HTMLVideoElement | null): void {
    this.attached = element;
  }

  /** Remembers the element that last produced an event or request. */
  observe(element: HTMLVideoElement): void {
    this.lastSeen = element;
  }

  /** The mounted element, falling back to the last one seen during teardown. */
  get element(): HTMLVideoElement | null {
    return this.attached ?? this.lastSeen;
  }

  get mounted(): HTMLVideoElement | null {
    return this.attached;
  }

  /** Marks the next play or pause event as one the app caused. */
  expect(kind: SelfEvent): void {
    this.selfEvents[kind] = true;
    window.clearTimeout(this.selfEventTimers[kind]);
    this.selfEventTimers[kind] = window.setTimeout(() => {
      this.selfEvents[kind] = false;
    }, SELF_EVENT_WINDOW_MILLIS);
  }

  /** Returns true, once, when an arriving event was one the app caused. */
  consume(kind: SelfEvent): boolean {
    if (!this.selfEvents[kind]) return false;
    this.selfEvents[kind] = false;
    window.clearTimeout(this.selfEventTimers[kind]);
    return true;
  }

  /** Forgets an expected event that will not arrive, such as a rejected play. */
  forget(kind: SelfEvent): void {
    this.selfEvents[kind] = false;
  }

  /** Freezes the picture without that pause reading as the user's. */
  hold(player: HTMLVideoElement): void {
    if (player.paused) return;
    this.expect("pause");
    player.pause();
  }

  /** Moves the picture without that seek reading as the user's. */
  setTime(player: HTMLVideoElement, milliseconds: number): number {
    const target = Math.max(0, milliseconds);
    if (Math.abs(mediaTimeMillis(player) - target) > 1) {
      this.markInternalSeek();
      player.currentTime = target / 1000;
    }
    return target;
  }

  markInternalSeek(): void {
    this.internalSeek = true;
    window.clearTimeout(this.internalSeekTimer);
    this.internalSeekTimer = window.setTimeout(() => {
      this.internalSeek = false;
    }, SELF_EVENT_WINDOW_MILLIS);
  }

  /** True while the seeking/seeked pair of an app correction is in flight. */
  get seekingInternally(): boolean {
    return this.internalSeek;
  }

  finishInternalSeek(): void {
    this.internalSeek = false;
    window.clearTimeout(this.internalSeekTimer);
  }

  /** Starts playback the app requested; a rejection clears the expectation. */
  async play(player: HTMLVideoElement): Promise<void> {
    this.expect("play");
    try {
      await player.play();
    } catch (reason) {
      this.forget("play");
      throw reason;
    }
  }

  dispose(): void {
    window.clearTimeout(this.selfEventTimers.play);
    window.clearTimeout(this.selfEventTimers.pause);
    window.clearTimeout(this.internalSeekTimer);
  }
}
