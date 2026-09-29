import { fireEvent, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { MediaFunscript, MediaSyncEvent, MediaSyncStatus } from "../api/types";
import type { MediaPlaybackEvent } from "./mediaElement";
import { VideoPlaybackController, type PlaybackDependencies } from "./playbackController";

const script: MediaFunscript = {
  video_id: "paired",
  name: "Paired",
  duration_ms: 20_000,
  action_count: 3,
  actions: [{ at: 0, pos: 20 }, { at: 10_000, pos: 80 }, { at: 20_000, pos: 20 }],
};

const forwarded: MediaPlaybackEvent[] = ["play", "playing", "pause", "seeking", "seeked", "ended", "ratechange", "volumechange", "waiting", "canplay", "error"];

function mountVideo(controller: VideoPlaybackController): HTMLVideoElement {
  const element = document.createElement("video");
  document.body.append(element);
  for (const event of forwarded) element.addEventListener(event, () => controller.handleMediaEvent(event, element));
  controller.attach(element);
  return element;
}

describe("VideoPlaybackController commands", () => {
  let mediaSync: ReturnType<typeof vi.fn>;
  let deps: PlaybackDependencies;
  let ready = 3;

  beforeEach(() => {
    ready = 3;
    vi.spyOn(HTMLMediaElement.prototype, "readyState", "get").mockImplementation(() => ready);
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => undefined);
    vi.spyOn(HTMLMediaElement.prototype, "play").mockImplementation(function (this: HTMLMediaElement) {
      queueMicrotask(() => { fireEvent.play(this); fireEvent.playing(this); });
      return Promise.resolve();
    });
    mediaSync = vi.fn(async (event: MediaSyncEvent) => ({
      sync: (event.state === "playing"
        ? { active: true, state: "following", video_id: event.video_id, last_event: event.event }
        : { active: false, state: event.state === "closed" ? "idle" : event.state, video_id: event.video_id }) as MediaSyncStatus,
    }));
    deps = {
      mediaSync,
      saveMediaPlayback: vi.fn(),
      loadScript: vi.fn(async () => script),
      refresh: vi.fn(async () => undefined),
    };
  });

  afterEach(() => {
    vi.restoreAllMocks();
    document.body.replaceChildren();
  });

  it("takes over an element in use when the motion source changes", () => {
    const element = document.createElement("video");
    Object.defineProperty(element, "paused", { configurable: true, get: () => false });
    element.volume = 0.4;
    element.muted = true;
    const plain = new VideoPlaybackController({ videoID: "paired", synchronized: false, durationMillis: 20_000, locked: false, stopSequence: 1 }, deps);
    plain.attach(element);
    expect(plain.getSnapshot()).toMatchObject({ playbackIntent: true, volume: 0.4, muted: true });
    // A paired run only plays once armed, so it starts from rest.
    const paired = new VideoPlaybackController({ videoID: "paired", synchronized: true, durationMillis: 20_000, locked: false, stopSequence: 1 }, deps);
    paired.attach(element);
    expect(paired.getSnapshot()).toMatchObject({ playbackIntent: false, volume: 0.4, muted: true });
  });

  it("awaits the old script's Stop and blocks re-arming during a source handoff", async () => {
    const controller = new VideoPlaybackController({ videoID: "paired", synchronized: true, durationMillis: 20_000, locked: false, stopSequence: 5 }, deps);
    controller.connect();
    const element = mountVideo(controller);
    await waitFor(() => expect(controller.getSnapshot().scriptLoading).toBe(false));
    controller.commands.play();
    await waitFor(() => expect(controller.getSnapshot().sync.active).toBe(true));
    let finish!: (value: { sync: MediaSyncStatus }) => void;
    mediaSync.mockImplementationOnce(() => new Promise((resolve) => { finish = resolve; }));
    let resolved = false;
    const releasing = controller.releaseMotion(5).then((value) => { resolved = true; return value; });
    expect(controller.commands.play()).toBe(false);
    fireEvent.play(element);
    await Promise.resolve();
    expect(resolved).toBe(false);
    expect(controller.getSnapshot().playbackIntent).toBe(false);
    finish({ sync: { active: false, state: "paused" } });
    expect(await releasing).toBe(true);
    const starts = mediaSync.mock.calls.filter(([event]) => event.state === "playing");
    expect(starts).toHaveLength(1);
    controller.disconnect();
  });

  it("keeps a failed handoff paused and does not need another transport Stop after Emergency Stop", async () => {
    const controller = new VideoPlaybackController({ videoID: "paired", synchronized: true, durationMillis: 20_000, locked: false, stopSequence: 5 }, deps);
    controller.connect();
    mountVideo(controller);
    await waitFor(() => expect(controller.getSnapshot().scriptLoading).toBe(false));
    controller.commands.play();
    await waitFor(() => expect(controller.getSnapshot().sync.active).toBe(true));
    mediaSync.mockRejectedValueOnce(new Error("transport unavailable"));
    expect(await controller.releaseMotion(5)).toBe(false);
    expect(controller.getSnapshot().playbackIntent).toBe(false);
    controller.setStopSequence(6);
    mediaSync.mockClear();
    expect(await controller.releaseMotion(6)).toBe(true);
    expect(mediaSync).not.toHaveBeenCalled();
    controller.disconnect();
  });

  it("drives an unpaired video directly and reports element state", () => {
    const controller = new VideoPlaybackController({ videoID: "plain", synchronized: false, durationMillis: 60_000, locked: false, stopSequence: 1 }, deps);
    controller.connect();
    const element = mountVideo(controller);

    expect(controller.commands.play()).toBe(true);
    expect(HTMLMediaElement.prototype.play).toHaveBeenCalledOnce();
    expect(controller.commands.seekTo(90_000)).toBe(true);
    expect(element.currentTime).toBe(60);
    expect(controller.commands.setVolume(0.4)).toBe(true);
    expect(element.volume).toBeCloseTo(0.4);
    expect(controller.commands.setRate(1.5)).toBe(true);
    expect(controller.getSnapshot().playbackRate).toBe(1.5);
    expect(controller.commands.setRate(8)).toBe(false);
    expect(controller.commands.pause()).toBe(true);
    expect(HTMLMediaElement.prototype.pause).toHaveBeenCalled();
    expect(mediaSync).not.toHaveBeenCalled();

    element.muted = true;
    fireEvent.volumeChange(element);
    expect(controller.getSnapshot().muted).toBe(true);
    controller.disconnect();
  });

  it("refuses commands for a paired video until its script loads", async () => {
    let release!: (value: MediaFunscript) => void;
    deps.loadScript = vi.fn(() => new Promise<MediaFunscript>((resolve) => { release = resolve; }));
    const controller = new VideoPlaybackController({ videoID: "paired", synchronized: true, durationMillis: 20_000, locked: false, stopSequence: 3 }, deps);
    controller.connect();
    mountVideo(controller);

    expect(controller.commands.play()).toBe(false);
    release(script);
    await waitFor(() => expect(controller.getSnapshot().script).not.toBeNull());
    expect(controller.commands.play()).toBe(true);
    await waitFor(() => expect(controller.getSnapshot().sync.state).toBe("following"));
    controller.disconnect();
  });

  it("arms a paired run on play and re-arms once at a remote seek target", async () => {
    const controller = new VideoPlaybackController({ videoID: "paired", synchronized: true, durationMillis: 20_000, locked: false, stopSequence: 5 }, deps);
    const updates = vi.fn();
    controller.subscribe(updates);
    controller.connect();
    mountVideo(controller);
    await waitFor(() => expect(controller.getSnapshot().script).not.toBeNull());

    controller.commands.play();
    await waitFor(() => expect(controller.getSnapshot().sync.state).toBe("following"));
    expect(mediaSync.mock.calls.filter(([event]) => event.event === "play")).toHaveLength(1);
    expect(updates).toHaveBeenCalled();

    mediaSync.mockClear();
    expect(controller.commands.seekTo(4_500)).toBe(true);
    await waitFor(() => expect(mediaSync).toHaveBeenCalledWith(
      expect.objectContaining({ state: "playing", event: "seeked", media_time_ms: 4_500 }),
      5,
      expect.any(AbortSignal),
      false,
    ));
    expect(mediaSync.mock.calls.filter(([event]) => event.state === "seeking")).toHaveLength(1);
    await waitFor(() => expect(controller.getSnapshot().operation).toBeNull());

    mediaSync.mockClear();
    expect(controller.commands.pause()).toBe(true);
    await waitFor(() => expect(mediaSync).toHaveBeenCalledWith(expect.objectContaining({ state: "paused", event: "pause" }), 5, expect.any(AbortSignal), false));
    expect(controller.getSnapshot().playbackIntent).toBe(false);
    controller.disconnect();
  });

  it("ignores a play command while a paired video is already playing", async () => {
    const controller = new VideoPlaybackController({ videoID: "paired", synchronized: true, durationMillis: 20_000, locked: false, stopSequence: 9 }, deps);
    controller.connect();
    mountVideo(controller);
    await waitFor(() => expect(controller.getSnapshot().script).not.toBeNull());
    controller.commands.play();
    await waitFor(() => expect(controller.getSnapshot().sync.state).toBe("following"));
    mediaSync.mockClear();

    controller.commands.play();
    await Promise.resolve();
    expect(mediaSync).not.toHaveBeenCalled();
    controller.disconnect();
  });

  it("stops a paired run when Stop advances and reports it once", async () => {
    const controller = new VideoPlaybackController({ videoID: "paired", synchronized: true, durationMillis: 20_000, locked: false, stopSequence: 2 }, deps);
    controller.connect();
    mountVideo(controller);
    await waitFor(() => expect(controller.getSnapshot().script).not.toBeNull());
    controller.commands.play();
    await waitFor(() => expect(controller.getSnapshot().sync.state).toBe("following"));

    controller.setStopSequence(3);
    expect(controller.getSnapshot().sync.state).toBe("stopped");
    expect(controller.getSnapshot().playbackIntent).toBe(false);
    controller.disconnect();
    // The armed session reports its end with the Stop sequence it ran under.
    expect(mediaSync).toHaveBeenLastCalledWith(expect.objectContaining({ state: "closed" }), 2, undefined, true);
  });
});
