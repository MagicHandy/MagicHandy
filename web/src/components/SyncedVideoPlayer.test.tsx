import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api, ApiError } from "../api/client";
import type { MediaFunscript, MediaSyncStatus, MediaVideo } from "../api/types";
import { SyncedVideoPlayer } from "./SyncedVideoPlayer";

const { refreshAppState } = vi.hoisted(() => ({
  refreshAppState: vi.fn(),
}));

vi.mock("../api/client", async (importOriginal) => {
  const original = await importOriginal<typeof import("../api/client")>();
  return {
    ...original,
    api: {
      mediaFunscript: vi.fn(),
      mediaSync: vi.fn(),
      mediaStreamURL: (id: string) => `/stream/${id}`,
      saveMediaDuration: vi.fn(),
      saveMediaPlayback: vi.fn(),
    },
  };
});

vi.mock("../state/app-state", () => ({
  useAppState: () => ({
    state: {
      settings: {
        media: {},
        motion: {
          apply_video_speed_limit: false,
          speed_max_percent: 40,
        },
      },
    },
    refresh: refreshAppState,
  }),
}));

const mediaFunscript = vi.mocked(api.mediaFunscript);
const mediaSync = vi.mocked(api.mediaSync);
const saveMediaPlayback = vi.mocked(api.saveMediaPlayback);
const following: MediaSyncStatus = {
  active: true,
  video_id: "paired",
  state: "following",
  motion_speed_limit_percent: 40,
  drift_ms: 12,
};
const script: MediaFunscript = {
  video_id: "paired",
  name: "Paired session",
  duration_ms: 12_000,
  action_count: 3,
  actions: [{ at: 0, pos: 20 }, { at: 6_000, pos: 80 }, { at: 12_000, pos: 30 }],
};

function video(paired = true, duration = 12_000): MediaVideo {
  return {
    id: paired ? "paired" : "plain",
    location_path: "C:/media",
    display_name: paired ? "Paired session" : "Plain session",
    size_bytes: 1024,
    modified_at: "2026-07-19T12:00:00Z",
    duration_ms: duration,
    has_funscript: paired,
    missing: false,
    scanned_at: "2026-07-19T12:00:00Z",
  };
}

describe("SyncedVideoPlayer", () => {
  let play: ReturnType<typeof vi.spyOn>;
  let pause: ReturnType<typeof vi.spyOn>;
  let mediaReadyState = 3;
  let restoreReadyState: () => void = () => undefined;

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    mediaReadyState = 3;
    const readyState = vi.spyOn(HTMLMediaElement.prototype, "readyState", "get").mockImplementation(() => mediaReadyState);
    restoreReadyState = () => readyState.mockRestore();
    play = vi.spyOn(HTMLMediaElement.prototype, "play").mockImplementation(function (this: HTMLMediaElement) {
      window.queueMicrotask(() => {
        fireEvent.play(this);
        fireEvent.playing(this);
      });
      return Promise.resolve();
    });
    pause = vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => undefined);
    mediaFunscript.mockResolvedValue({ funscript: script });
    saveMediaPlayback.mockResolvedValue({ status: "saved" });
    refreshAppState.mockResolvedValue(undefined);
    mediaSync.mockImplementation(async (event) => ({
      sync: event.state === "playing" ? { ...following, last_event: event.event } : {
        active: false,
        video_id: event.video_id,
        state: event.state === "closed" ? "idle" : event.state,
        last_event: event.event,
      },
    }));
  });

  afterEach(() => {
    play.mockRestore();
    pause.mockRestore();
    restoreReadyState();
  });

  it("starts buffering paired media while its script is still loading", async () => {
    let releaseScript!: () => void;
    mediaFunscript.mockImplementationOnce(() => new Promise<{ funscript: MediaFunscript }>((resolve) => {
      releaseScript = () => resolve({ funscript: script });
    }));

    render(<SyncedVideoPlayer video={video()} locked={false} stopSequence={6} />);

    const player = screen.getByLabelText("Paired session") as HTMLVideoElement;
    const shell = player.closest(".media-player");
    expect(player).toHaveAttribute("preload", "auto");
    expect(player).not.toHaveAttribute("controls");
    expect(shell).toHaveAttribute("aria-busy", "true");
    expect(screen.getByRole("status")).toHaveTextContent("Preparing paired script and video");

    releaseScript();
    expect(await screen.findByRole("slider", { name: /funscript timeline/i })).toBeInTheDocument();
    expect(screen.getByLabelText("Paired session")).toBe(player);
    expect(player).not.toHaveAttribute("controls");
    const transport = screen.getByRole("group", { name: "Synchronized video controls" });
    expect(transport).toContainElement(screen.getByRole("button", { name: "Play video with paired motion" }));
    expect(player.closest(".media-video-frame")).toContainElement(transport);
    expect(shell).not.toHaveAttribute("aria-busy");
  });

  it("holds the exact click timestamp until paired motion is armed", async () => {
    let releaseArm!: () => void;
    mediaSync.mockImplementation((event) => {
      if (event.state === "playing" && event.event === "play") {
        return new Promise<{ sync: MediaSyncStatus }>((resolve) => {
          releaseArm = () => resolve({ sync: { ...following, last_event: event.event } });
        });
      }
      const state: MediaSyncStatus["state"] = event.state === "closed"
        ? "idle"
        : event.state === "playing"
          ? "interrupted"
          : event.state;
      return Promise.resolve({
        sync: {
          active: false,
          video_id: event.video_id,
          state,
          last_event: event.event,
        },
      });
    });
    render(<SyncedVideoPlayer video={video()} locked={false} stopSequence={8} />);
    const player = await screen.findByLabelText("Paired session") as HTMLVideoElement;
    let currentTime = 3.25;
    let paused = true;
    Object.defineProperty(player, "currentTime", {
      configurable: true,
      get: () => currentTime,
      set: (value: number) => { currentTime = value; },
    });
    Object.defineProperty(player, "paused", { configurable: true, get: () => paused });

    fireEvent.click(screen.getByRole("button", { name: "Play video with paired motion" }));

    await waitFor(() => expect(mediaSync).toHaveBeenCalledWith(expect.objectContaining({
      event: "play",
      media_time_ms: 3_250,
    }), 8, expect.any(AbortSignal), false));
    expect(play).not.toHaveBeenCalled();

    // Even if the browser starts advancing unexpectedly, the app freezes and
    // rewinds it to the timestamp whose device arm is still pending.
    paused = false;
    currentTime = 4;
    fireEvent.play(player);
    expect(pause).toHaveBeenCalled();
    expect(currentTime).toBeCloseTo(3.25, 3);
    expect(play).not.toHaveBeenCalled();

    releaseArm();
    await waitFor(() => expect(play).toHaveBeenCalledOnce());
  });

  it("loads the same-name script, shows its curve, and arms motion before resuming video", async () => {
    render(<SyncedVideoPlayer video={video()} locked={false} stopSequence={7} />);

    expect(await screen.findByRole("slider", { name: /funscript timeline/i })).toBeInTheDocument();
    expect(mediaFunscript).toHaveBeenCalledWith("paired", expect.any(AbortSignal));
    const player = screen.getByLabelText("Paired session") as HTMLVideoElement;
    Object.defineProperty(player, "currentTime", { configurable: true, writable: true, value: 1.25 });
    fireEvent.play(player);

    await waitFor(() => expect(mediaSync).toHaveBeenCalledWith(expect.objectContaining({
      video_id: "paired",
      session_id: expect.stringMatching(/^media-/),
      event_sequence: 1,
      state: "playing",
      event: "play",
      media_time_ms: 1_250,
    }), 7, expect.any(AbortSignal), false));
    await waitFor(() => expect(play).toHaveBeenCalledOnce());
    expect(screen.getByText("Device following video")).toBeInTheDocument();
    expect(screen.getByText("40% speed limit")).toBeInTheDocument();
  });

  it("clears motion on pause and seek, then explicitly re-arms a playing seek", async () => {
    render(<SyncedVideoPlayer video={video()} locked={false} stopSequence={9} />);
    const player = await screen.findByLabelText("Paired session") as HTMLVideoElement;

    fireEvent.play(player);
    await waitFor(() => expect(mediaSync).toHaveBeenCalledWith(expect.objectContaining({ event: "play" }), 9, expect.any(AbortSignal), false));
    fireEvent.pause(player);
    await waitFor(() => expect(mediaSync).toHaveBeenCalledWith(expect.objectContaining({ state: "paused", event: "pause" }), 9, expect.any(AbortSignal), false));

    fireEvent.play(player);
    await waitFor(() => expect(mediaSync.mock.calls.filter(([event]) => event.event === "play")).toHaveLength(2));
    fireEvent.seeking(player);
    fireEvent.seeked(player);
    await waitFor(() => expect(mediaSync).toHaveBeenCalledWith(expect.objectContaining({ state: "seeking", event: "seeking" }), 9, expect.any(AbortSignal), false));
    await waitFor(() => expect(mediaSync).toHaveBeenCalledWith(expect.objectContaining({ state: "playing", event: "seeked" }), 9, expect.any(AbortSignal), false));
  });

  it("ignores a delayed native pause from a playing seek and still re-arms", async () => {
    render(<SyncedVideoPlayer video={video()} locked={false} stopSequence={10} />);
    const player = await screen.findByLabelText("Paired session") as HTMLVideoElement;

    fireEvent.play(player);
    await waitFor(() => expect(screen.getByText("Device following video")).toBeInTheDocument());
    mediaSync.mockClear();

    fireEvent.seeking(player);
    fireEvent.seeked(player);
    fireEvent.pause(player);

    await waitFor(() => expect(mediaSync).toHaveBeenCalledWith(
      expect.objectContaining({ state: "playing", event: "seeked" }),
      10,
      expect.any(AbortSignal),
      false,
    ));
    expect(mediaSync.mock.calls.some(([event]) => event.event === "pause")).toBe(false);
  });

  it("preserves play intent across a long explicit scrub and re-arms once at its exact target", async () => {
    let releaseSeekArm!: () => void;
    mediaSync.mockImplementation((event) => {
      if (event.state === "playing" && event.event === "seeked") {
        return new Promise<{ sync: MediaSyncStatus }>((resolve) => {
          releaseSeekArm = () => resolve({ sync: { ...following, last_event: event.event } });
        });
      }
      return Promise.resolve({
        sync: event.state === "playing" ? { ...following, last_event: event.event } : {
          active: false,
          video_id: event.video_id,
          state: event.state === "closed" ? "idle" : event.state,
          last_event: event.event,
        },
      });
    });
    render(<SyncedVideoPlayer video={video()} locked={false} stopSequence={12} />);
    await screen.findByLabelText("Paired session");

    fireEvent.click(screen.getByRole("button", { name: "Play video with paired motion" }));
    await waitFor(() => expect(screen.getByText("Device following video")).toBeInTheDocument());
    expect(play).toHaveBeenCalledOnce();

    mediaSync.mockClear();
    const scrubber = screen.getByRole("slider", { name: "Video position" });
    fireEvent.pointerDown(scrubber, { pointerId: 1 });
    await waitFor(() => expect(mediaSync).toHaveBeenCalledWith(
      expect.objectContaining({ state: "seeking", event: "seeking" }),
      12,
      expect.any(AbortSignal),
      false,
    ));
    fireEvent.change(scrubber, { target: { value: "4500" } });
    // The old implementation lost this intent after 400 ms. Explicit gesture
    // state has no timing window, so an arbitrarily long drag remains a resume.
    await new Promise((resolve) => window.setTimeout(resolve, 450));
    fireEvent.pointerUp(scrubber, { pointerId: 1 });
    expect(await screen.findByText("Video held at 00:04.500; resyncing motion")).toBeInTheDocument();

    await waitFor(() => expect(mediaSync).toHaveBeenCalledWith(expect.objectContaining({
      state: "playing",
      event: "seeked",
      media_time_ms: 4_500,
    }), 12, expect.any(AbortSignal), false));
    expect(mediaSync.mock.calls.filter(([event]) => event.state === "seeking")).toHaveLength(1);
    expect(mediaSync.mock.calls.filter(([event]) => event.state === "playing")).toHaveLength(1);
    releaseSeekArm();
    await waitFor(() => expect(play).toHaveBeenCalledTimes(2));
    expect(screen.getByText("Device following video")).toBeInTheDocument();
  });

  it("keeps an explicitly paused video paused after scrubbing", async () => {
    render(<SyncedVideoPlayer video={video()} locked={false} stopSequence={13} />);
    await screen.findByLabelText("Paired session");

    fireEvent.click(screen.getByRole("button", { name: "Play video with paired motion" }));
    await waitFor(() => expect(screen.getByText("Device following video")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "Pause video and motion" }));
    await waitFor(() => expect(mediaSync).toHaveBeenCalledWith(expect.objectContaining({ state: "paused", event: "pause" }), 13, expect.any(AbortSignal), false));

    mediaSync.mockClear();
    const scrubber = screen.getByRole("slider", { name: "Video position" });
    fireEvent.pointerDown(scrubber, { pointerId: 1 });
    fireEvent.change(scrubber, { target: { value: "4500" } });
    fireEvent.pointerUp(scrubber, { pointerId: 1 });

    await waitFor(() => expect(scrubber).toHaveValue("4500"));
    expect(mediaSync).not.toHaveBeenCalled();
    expect(play).toHaveBeenCalledOnce();
  });

  it("freezes and re-arms the exact timestamp when a script filter changes", async () => {
    render(<SyncedVideoPlayer video={video()} locked={false} stopSequence={16} />);
    const player = await screen.findByLabelText("Paired session") as HTMLVideoElement;
    let currentTime = 2.75;
    let paused = true;
    Object.defineProperty(player, "currentTime", {
      configurable: true,
      get: () => currentTime,
      set: (value: number) => { currentTime = value; },
    });
    Object.defineProperty(player, "paused", { configurable: true, get: () => paused });

    fireEvent.click(screen.getByRole("button", { name: "Play video with paired motion" }));
    await waitFor(() => expect(screen.getByText("Device following video")).toBeInTheDocument());
    paused = false;
    pause.mockClear();
    mediaSync.mockClear();

    fireEvent.click(screen.getByRole("button", { name: "Sync 0 ms" }));
    fireEvent.click(screen.getByRole("checkbox", { name: /Round peaks/ }));

    expect(pause).toHaveBeenCalledOnce();
    expect(currentTime).toBeCloseTo(2.75, 3);
    await waitFor(() => expect(mediaSync).toHaveBeenCalledWith(
      expect.objectContaining({ state: "paused", event: "pause", media_time_ms: 2_750 }),
      16,
      expect.any(AbortSignal),
      false,
    ));
    await waitFor(() => expect(saveMediaPlayback).toHaveBeenCalledWith({ peak_rounding_ms: 60 }));
    await waitFor(() => expect(mediaSync).toHaveBeenCalledWith(
      expect.objectContaining({ state: "playing", event: "resync", media_time_ms: 2_750 }),
      16,
      expect.any(AbortSignal),
      false,
    ));
    expect(mediaSync.mock.calls.filter(([event]) => event.state === "paused")).toHaveLength(1);
    expect(mediaSync.mock.calls.filter(([event]) => event.state === "playing")).toHaveLength(1);
  });

  it("locks the held video onto the engine clock before resuming playback", async () => {
    mediaSync.mockImplementation(async (event) => ({
      sync: event.state === "playing"
        ? { ...following, last_event: event.event, expected_media_time_ms: 5_000, playback_rate: 1 }
        : {
            active: false,
            video_id: event.video_id,
            state: event.state === "closed" ? "idle" : event.state,
            last_event: event.event,
          },
    }));
    render(<SyncedVideoPlayer video={video()} locked={false} stopSequence={14} />);
    const player = await screen.findByLabelText("Paired session") as HTMLVideoElement;
    let currentTime = 0;
    Object.defineProperty(player, "currentTime", {
      configurable: true,
      get: () => currentTime,
      set: (value: number) => { currentTime = value; },
    });

    fireEvent.play(player);
    await waitFor(() => expect(screen.getByText("Device following video")).toBeInTheDocument());

    // The engine clock started at transport play; the video is moved onto it.
    expect(currentTime).toBeCloseTo(5, 1);

    // The correction's own seeking/seeked pair must not read as a user seek
    // that stops motion and re-arms.
    mediaSync.mockClear();
    fireEvent.seeking(player);
    fireEvent.seeked(player);
    await Promise.resolve();
    expect(mediaSync.mock.calls.some(([event]) => event.state === "seeking" || event.event === "seeked")).toBe(false);
    expect(screen.getByText("Device following video")).toBeInTheDocument();
  });

  it("re-checks the engine clock once playback is really advancing", async () => {
    mediaSync.mockImplementation(async (event) => ({
      sync: event.state === "playing"
        ? { ...following, last_event: event.event, expected_media_time_ms: 5_000, playback_rate: 1 }
        : {
            active: false,
            video_id: event.video_id,
            state: event.state === "closed" ? "idle" : event.state,
            last_event: event.event,
          },
    }));
    render(<SyncedVideoPlayer video={video()} locked={false} stopSequence={14} />);
    const player = await screen.findByLabelText("Paired session") as HTMLVideoElement;
    let currentTime = 0;
    Object.defineProperty(player, "currentTime", {
      configurable: true,
      get: () => currentTime,
      set: (value: number) => { currentTime = value; },
    });
    Object.defineProperty(player, "paused", { configurable: true, get: () => false });

    fireEvent.play(player);
    await waitFor(() => expect(screen.getByText("Device following video")).toBeInTheDocument());
    expect(currentTime).toBeCloseTo(5, 1);

    // Seeking and restarting the decoder cost real time that the pre-play
    // reading could not include, so the video starts behind the device. Left
    // alone this sits under the steady-state threshold forever and reads as a
    // script running slightly ahead of the picture.
    currentTime = 4.93;
    fireEvent.timeUpdate(player);
    await Promise.resolve();
    expect(currentTime).toBeCloseTo(5, 1);

    // Only once: a later small deviation is steady-state drift, which the
    // 150ms band deliberately tolerates rather than correcting visibly.
    currentTime = 4.93;
    fireEvent.timeUpdate(player);
    await Promise.resolve();
    expect(currentTime).toBeCloseTo(4.93, 2);
  });

  it("does not treat the correction seek's buffering dip as starvation", async () => {
    mediaSync.mockImplementation(async (event) => ({
      sync: event.state === "playing"
        ? { ...following, last_event: event.event, expected_media_time_ms: 5_000, playback_rate: 1 }
        : {
            active: false,
            video_id: event.video_id,
            state: event.state === "closed" ? "idle" : event.state,
            last_event: event.event,
          },
    }));
    render(<SyncedVideoPlayer video={video()} locked={false} stopSequence={15} />);
    const player = await screen.findByLabelText("Paired session") as HTMLVideoElement;
    let currentTime = 0;
    Object.defineProperty(player, "currentTime", {
      configurable: true,
      get: () => currentTime,
      set: (value: number) => { currentTime = value; },
    });

    fireEvent.play(player);
    await waitFor(() => expect(screen.getByText("Device following video")).toBeInTheDocument());
    expect(currentTime).toBeCloseTo(5, 1);

    // The nudge's own seeking/waiting/seeked sequence — Chrome drops
    // readyState during any programmatic seek, even inside a buffered range —
    // must not stop motion and enter a stop/re-arm/nudge loop.
    mediaSync.mockClear();
    fireEvent.seeking(player);
    fireEvent.waiting(player);
    fireEvent.seeked(player);
    await Promise.resolve();
    expect(mediaSync).not.toHaveBeenCalled();
    expect(screen.getByText("Device following video")).toBeInTheDocument();

    // Genuine starvation after the correction completes still tears down.
    mediaReadyState = 2;
    fireEvent.waiting(player);
    await waitFor(() => expect(mediaSync).toHaveBeenCalledWith(expect.objectContaining({
      state: "paused",
      event: "waiting",
    }), 15, expect.any(AbortSignal), false));
  });

  it("cancels an obsolete arm and re-arms at the latest seek timestamp", async () => {
    let initialArmSignal: AbortSignal | undefined;
    mediaSync.mockImplementation((event, _sequence, signal) => {
      if (event.event === "play") {
        initialArmSignal = signal;
        return new Promise<{ sync: MediaSyncStatus }>((_, reject) => {
          signal?.addEventListener("abort", () => reject(new DOMException("Aborted", "AbortError")), { once: true });
        });
      }
      const sync: MediaSyncStatus = event.state === "playing"
        ? { ...following, last_event: event.event }
        : {
            active: false,
            video_id: event.video_id,
            state: event.state === "closed" ? "idle" : event.state,
            last_event: event.event,
          };
      return Promise.resolve({ sync });
    });
    render(<SyncedVideoPlayer video={video()} locked={false} stopSequence={11} />);
    const player = await screen.findByLabelText("Paired session") as HTMLVideoElement;

    fireEvent.play(player);
    await waitFor(() => expect(mediaSync).toHaveBeenCalledWith(expect.objectContaining({ event: "play" }), 11, expect.any(AbortSignal), false));
    Object.defineProperty(player, "currentTime", { configurable: true, writable: true, value: 4.25 });
    fireEvent.seeking(player);
    fireEvent.seeked(player);

    await waitFor(() => expect(initialArmSignal?.aborted).toBe(true));
    await waitFor(() => expect(mediaSync).toHaveBeenCalledWith(expect.objectContaining({
      state: "playing",
      event: "seeked",
      media_time_ms: 4_250,
    }), 11, expect.any(AbortSignal), false));
    expect(screen.getByText("Device following video")).toBeInTheDocument();
  });

  it("keeps paired playback visualization-only for a read-only tab", async () => {
    render(<SyncedVideoPlayer video={video()} locked stopSequence={7} />);
    const player = await screen.findByLabelText("Paired session");
    expect(screen.getByText("Timeline only; this tab does not control motion")).toBeInTheDocument();

    fireEvent.play(player);
    expect(mediaSync).not.toHaveBeenCalled();
  });

  it("surfaces a likely pairing mismatch without disabling playback", async () => {
    render(<SyncedVideoPlayer video={video(true, 8_000)} locked={false} stopSequence={7} />);

    expect(await screen.findByText("Length differs from 00:08 video")).toBeInTheDocument();
    expect(screen.getByLabelText("Paired session")).not.toHaveAttribute("controls");
    expect(screen.getByRole("button", { name: "Play video with paired motion" })).toBeEnabled();
  });

  it("leaves the video paused and reports a synchronization failure", async () => {
    mediaSync.mockRejectedValueOnce(new ApiError("transport unavailable", 502, { error: "transport unavailable" }));

    render(<SyncedVideoPlayer video={video()} locked={false} stopSequence={7} />);
    const player = await screen.findByLabelText("Paired session");

    fireEvent.play(player);

    expect(await screen.findByRole("alert")).toHaveTextContent("transport unavailable");
    expect(play).not.toHaveBeenCalled();
  });

  it("closes an armed media session when the player unmounts", async () => {
    const result = render(<SyncedVideoPlayer video={video()} locked={false} stopSequence={7} />);
    const player = await screen.findByLabelText("Paired session");
    fireEvent.play(player);
    await waitFor(() => expect(screen.getByText("Device following video")).toBeInTheDocument());

    result.unmount();

    await waitFor(() => expect(mediaSync).toHaveBeenCalledWith(expect.objectContaining({ state: "closed", event: "closed" }), 7, undefined, true));
    const playEvent = mediaSync.mock.calls.find(([event]) => event.event === "play")?.[0];
    const closeEvent = mediaSync.mock.calls.find(([event]) => event.event === "closed")?.[0];
    expect(closeEvent?.session_id).toBe(playEvent?.session_id);
    expect(closeEvent?.event_sequence).toBeGreaterThan(playEvent?.event_sequence ?? 0);
  });

  it("aborts an in-flight heartbeat and cannot re-arm after unmount", async () => {
    let heartbeatSignal: AbortSignal | undefined;
    let releaseHeartbeat!: () => void;
    mediaSync.mockImplementation((event, _sequence, signal) => {
      if (event.event === "heartbeat") {
        heartbeatSignal = signal;
        return new Promise<{ sync: MediaSyncStatus }>((resolve) => {
          releaseHeartbeat = () => resolve({
            sync: { ...following, last_event: "heartbeat", requires_reanchor: true },
          });
        });
      }
      return Promise.resolve({
        sync: event.state === "playing" ? { ...following, last_event: event.event } : {
          active: false,
          video_id: event.video_id,
          state: event.state === "closed" ? "idle" : event.state,
          last_event: event.event,
        },
      });
    });

    const result = render(<SyncedVideoPlayer video={video()} locked={false} stopSequence={7} />);
    const player = await screen.findByLabelText("Paired session") as HTMLVideoElement;
    Object.defineProperty(player, "paused", { configurable: true, get: () => false });
    fireEvent.play(player);
    await waitFor(() => expect(screen.getByText("Device following video")).toBeInTheDocument());
    await waitFor(
      () => expect(mediaSync.mock.calls.some(([event]) => event.event === "heartbeat")).toBe(true),
      { timeout: 2_500 },
    );

    result.unmount();
    expect(heartbeatSignal?.aborted).toBe(true);
    releaseHeartbeat();
    await Promise.resolve();
    await Promise.resolve();

    expect(mediaSync.mock.calls.some(([event]) => event.event === "resync")).toBe(false);
  });

  it("recovers when readyState advances without another canplay event", async () => {
    mediaReadyState = 2;
    render(<SyncedVideoPlayer video={video()} locked={false} stopSequence={7} />);
    const player = await screen.findByLabelText("Paired session") as HTMLVideoElement;

    fireEvent.play(player);

    expect(await screen.findByText("Buffering video before motion starts")).toBeInTheDocument();
    expect(player).toHaveAttribute("preload", "auto");
    expect(mediaSync).not.toHaveBeenCalled();

    mediaReadyState = 3;

    await waitFor(() => expect(mediaSync).toHaveBeenCalledWith(
      expect.objectContaining({ state: "playing", event: "play" }),
      7,
      expect.any(AbortSignal),
      false,
    ));
  });

  it("stops on media starvation and automatically re-arms when playback is ready", async () => {
    render(<SyncedVideoPlayer video={video()} locked={false} stopSequence={7} />);
    const player = await screen.findByLabelText("Paired session") as HTMLVideoElement;
    fireEvent.play(player);
    await waitFor(() => expect(screen.getByText("Device following video")).toBeInTheDocument());

    mediaReadyState = 2;
    fireEvent.waiting(player);

    await waitFor(() => expect(mediaSync).toHaveBeenCalledWith(expect.objectContaining({
      state: "paused",
      event: "waiting",
    }), 7, expect.any(AbortSignal), false));
    expect(screen.getByText("Buffering video; motion stopped")).toBeInTheDocument();

    mediaReadyState = 3;
    fireEvent.canPlay(player);

    await waitFor(() => expect(mediaSync).toHaveBeenCalledWith(
      expect.objectContaining({ state: "playing", event: "resync" }),
      7,
      expect.any(AbortSignal),
      false,
    ));
    expect(screen.getByText("Device following video")).toBeInTheDocument();
  });

  it("does not tear down synchronized motion for a buffered fetch stall", async () => {
    render(<SyncedVideoPlayer video={video()} locked={false} stopSequence={7} />);
    const player = await screen.findByLabelText("Paired session") as HTMLVideoElement;
    fireEvent.play(player);
    await waitFor(() => expect(screen.getByText("Device following video")).toBeInTheDocument());

    fireEvent.stalled(player);
    await Promise.resolve();

    expect(mediaSync.mock.calls.some(([event]) => event.event === "stalled" || event.state === "paused")).toBe(false);
    expect(screen.getByText("Device following video")).toBeInTheDocument();
  });

  it("continues video without motion after the paired script ends", async () => {
    mediaSync.mockResolvedValueOnce({ sync: {
      active: false,
      video_id: "paired",
      state: "completed",
      message: "The paired script has ended; video playback can continue without motion.",
    } });
    render(<SyncedVideoPlayer video={video()} locked={false} stopSequence={7} />);
    const player = await screen.findByLabelText("Paired session") as HTMLVideoElement;
    Object.defineProperty(player, "currentTime", { configurable: true, writable: true, value: 11.999 });

    fireEvent.play(player);

    await waitFor(() => expect(screen.getByText("Script playback complete")).toBeInTheDocument());
    expect(mediaSync).toHaveBeenCalledOnce();
    expect(play).toHaveBeenCalledOnce();
    player.currentTime = 12;
    fireEvent.play(player);
    expect(mediaSync).toHaveBeenCalledOnce();
  });
});
