import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api, ApiError } from "../api/client";
import type { MediaFunscript, MediaSyncStatus, MediaVideo } from "../api/types";
import { SyncedVideoPlayer } from "./SyncedVideoPlayer";

vi.mock("../api/client", async (importOriginal) => {
  const original = await importOriginal<typeof import("../api/client")>();
  return {
    ...original,
    api: {
      mediaFunscript: vi.fn(),
      mediaSync: vi.fn(),
      mediaStreamURL: (id: string) => `/stream/${id}`,
      saveMediaDuration: vi.fn(),
    },
  };
});

const mediaFunscript = vi.mocked(api.mediaFunscript);
const mediaSync = vi.mocked(api.mediaSync);
const following: MediaSyncStatus = {
  active: true,
  video_id: "paired",
  state: "following",
  motion_scale_percent: 40,
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

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    play = vi.spyOn(HTMLMediaElement.prototype, "play").mockImplementation(function (this: HTMLMediaElement) {
      window.queueMicrotask(() => fireEvent.play(this));
      return Promise.resolve();
    });
    pause = vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => undefined);
    mediaFunscript.mockResolvedValue({ funscript: script });
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
    expect(screen.getByText("40% motion scale")).toBeInTheDocument();
  });

  it("clears motion on pause and seek, then explicitly re-arms a playing seek", async () => {
    render(<SyncedVideoPlayer video={video()} locked={false} stopSequence={9} />);
    const player = await screen.findByLabelText("Paired session") as HTMLVideoElement;

    fireEvent.play(player);
    await waitFor(() => expect(mediaSync).toHaveBeenCalledWith(expect.objectContaining({ event: "play" }), 9, expect.any(AbortSignal), false));
    fireEvent.pause(player);
    await waitFor(() => expect(mediaSync).toHaveBeenCalledWith(expect.objectContaining({ state: "paused", event: "pause" }), 9, undefined, false));

    fireEvent.play(player);
    await waitFor(() => expect(mediaSync.mock.calls.filter(([event]) => event.event === "play")).toHaveLength(2));
    fireEvent.seeking(player);
    fireEvent.seeked(player);
    await waitFor(() => expect(mediaSync).toHaveBeenCalledWith(expect.objectContaining({ state: "seeking", event: "seeking" }), 9, undefined, false));
    await waitFor(() => expect(mediaSync).toHaveBeenCalledWith(expect.objectContaining({ state: "playing", event: "seeked" }), 9, expect.any(AbortSignal), false));
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
    expect(screen.getByLabelText("Paired session")).toHaveAttribute("controls");
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

  it("stops synchronized motion immediately when media playback stalls", async () => {
    render(<SyncedVideoPlayer video={video()} locked={false} stopSequence={7} />);
    const player = await screen.findByLabelText("Paired session") as HTMLVideoElement;
    fireEvent.play(player);
    await waitFor(() => expect(screen.getByText("Device following video")).toBeInTheDocument());

    fireEvent.waiting(player);

    await waitFor(() => expect(mediaSync).toHaveBeenCalledWith(expect.objectContaining({
      state: "paused",
      event: "waiting",
    }), 7, undefined, false));
    expect(screen.getByText("Playback interrupted; motion stopped")).toBeInTheDocument();
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
