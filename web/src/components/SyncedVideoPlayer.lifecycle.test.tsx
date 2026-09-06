import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { MediaSyncStatus, MediaVideo } from "../api/types";
import { SyncedVideoPlayer } from "./SyncedVideoPlayer";

vi.mock("../api/client", () => ({
  ApiError: class extends Error {},
  api: {
    mediaFunscript: vi.fn(), mediaSync: vi.fn(), saveMediaDuration: vi.fn(),
    mediaStreamURL: (id: string) => `/stream/${id}`,
  },
}));
vi.mock("../state/app-state", () => ({
  useAppState: () => ({ state: { settings: { media: {}, motion: {} } }, refresh: vi.fn() }),
}));

const video: MediaVideo = {
  id: "paired", display_name: "Seek fixture", location_path: "C:/fixtures",
  size_bytes: 1024, modified_at: "2026-09-06T00:00:00Z", scanned_at: "2026-09-06T00:00:00Z",
  has_funscript: true, missing: false, duration_ms: 30_000,
};
const following: MediaSyncStatus = { active: true, state: "following", video_id: video.id };
const mediaSync = vi.mocked(api.mediaSync);

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

describe("synchronized video lifecycle", () => {
  let play: ReturnType<typeof vi.spyOn>;
  let ready = 3;
  let seeking = false;

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    ready = 3;
    seeking = false;
    vi.spyOn(HTMLMediaElement.prototype, "readyState", "get").mockImplementation(() => ready);
    vi.spyOn(HTMLMediaElement.prototype, "seeking", "get").mockImplementation(() => seeking);
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => undefined);
    play = vi.spyOn(HTMLMediaElement.prototype, "play").mockImplementation(function (this: HTMLMediaElement) {
      queueMicrotask(() => { fireEvent.play(this); fireEvent.playing(this); });
      return Promise.resolve();
    });
    vi.mocked(api.mediaFunscript).mockResolvedValue({ funscript: {
      video_id: video.id, name: video.display_name, duration_ms: 20_000, action_count: 3,
      actions: [{ at: 0, pos: 20 }, { at: 10_000, pos: 80 }, { at: 20_000, pos: 20 }],
    } });
    mediaSync.mockImplementation(async (event) => ({ sync: event.state === "playing" ? following : {
      active: false, state: event.state === "closed" ? "idle" : event.state, video_id: video.id,
    } }));
  });

  afterEach(() => { cleanup(); vi.useRealTimers(); vi.restoreAllMocks(); });

  async function start() {
    const result = render(<SyncedVideoPlayer video={video} locked={false} stopSequence={7} />);
    fireEvent.click(await screen.findByRole("button", { name: "Play video with paired motion" }));
    await waitFor(() => expect(play).toHaveBeenCalledOnce());
    mediaSync.mockClear();
    return { ...result, player: screen.getByLabelText(video.display_name) as HTMLVideoElement };
  }

  function scrub(at = 5000) {
    const slider = screen.getByRole("slider", { name: "Video position" });
    fireEvent.pointerDown(slider, { pointerId: 1 });
    fireEvent.change(slider, { target: { value: String(at) } });
    fireEvent.pointerUp(slider, { pointerId: 1 });
  }

  it.each(["Stop", "controller loss"])("cancels a committed seek when %s arrives before Stop completes", async (reason) => {
    const result = await start();
    const stopping = deferred<{ sync: MediaSyncStatus }>();
    mediaSync.mockImplementationOnce(() => stopping.promise);
    scrub();
    result.rerender(<SyncedVideoPlayer video={video} locked={reason === "controller loss"} stopSequence={reason === "Stop" ? 8 : 7} />);
    await act(async () => stopping.resolve({ sync: { active: false, state: "seeking" } }));
    expect(mediaSync.mock.calls.filter(([event]) => event.state === "playing")).toHaveLength(0);
    expect(play).toHaveBeenCalledOnce();
    expect(screen.getByRole("button", { name: "Play video with paired motion" })).toBeInTheDocument();
  });

  it("does not reset live synchronization when duration metadata is saved", async () => {
    const result = await start();
    result.rerender(<SyncedVideoPlayer video={{ ...video, duration_ms: 31_000 }} locked={false} stopSequence={7} />);
    await act(async () => undefined);
    expect(api.mediaFunscript).toHaveBeenCalledOnce();
    expect(screen.getByText("Device following video")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Pause video and motion" })).toBeInTheDocument();
  });

  it("finishes loading the script after Stop cancels playback admission", async () => {
    const loading = deferred<Awaited<ReturnType<typeof api.mediaFunscript>>>();
    vi.mocked(api.mediaFunscript).mockImplementationOnce(() => loading.promise);
    const result = render(<SyncedVideoPlayer video={video} locked={false} stopSequence={7} />);
    result.rerender(<SyncedVideoPlayer video={video} locked={false} stopSequence={8} />);
    await act(async () => loading.resolve({ funscript: {
      video_id: video.id, name: video.display_name, duration_ms: 20_000, action_count: 2,
      actions: [{ at: 0, pos: 20 }, { at: 20_000, pos: 80 }],
    } }));
    expect(screen.getByRole("button", { name: "Play video with paired motion" })).toBeEnabled();
    expect(mediaSync).not.toHaveBeenCalled();
  });

  it("keeps an unpaired native video independent of motion ownership", () => {
    const plain = { ...video, has_funscript: false };
    const result = render(<SyncedVideoPlayer video={plain} locked={false} stopSequence={7} />);
    const player = screen.getByLabelText(video.display_name);
    Object.defineProperty(player, "paused", { configurable: true, get: () => false });
    result.rerender(<SyncedVideoPlayer video={plain} locked stopSequence={8} />);
    expect(HTMLMediaElement.prototype.pause).not.toHaveBeenCalled();
    expect(mediaSync).not.toHaveBeenCalled();
    expect(player).toHaveAttribute("controls");
  });

  it("retains the pending Stop across repeated buffering events", async () => {
    const { player } = await start();
    const stopping = deferred<{ sync: MediaSyncStatus }>();
    mediaSync.mockImplementationOnce(() => stopping.promise);
    ready = 2;
    fireEvent.waiting(player);
    fireEvent.waiting(player);
    ready = 3;
    fireEvent.canPlay(player);
    await act(async () => undefined);
    expect(mediaSync.mock.calls.filter(([event]) => event.state === "playing")).toHaveLength(0);
    await act(async () => stopping.resolve({ sync: { active: false, state: "paused" } }));
    expect(mediaSync.mock.calls.filter(([event]) => event.state === "playing")).toHaveLength(1);
    expect(play).toHaveBeenCalledTimes(2);
  });

  it("waits for the decoder and re-arms on seeked without a readiness polling delay", async () => {
    const { player } = await start();
    vi.useFakeTimers();
    seeking = true;
    scrub();
    await act(async () => undefined);
    expect(mediaSync.mock.calls.filter(([event]) => event.state === "playing")).toHaveLength(0);
    seeking = false;
    await act(async () => fireEvent.seeked(player));
    expect(mediaSync.mock.calls.filter(([event]) => event.state === "playing")).toHaveLength(1);
    expect(play).toHaveBeenCalledTimes(2);
  });

  it("resumes only the latest committed seek while a prior Stop is pending", async () => {
    const { player } = await start();
    const stopping = deferred<{ sync: MediaSyncStatus }>();
    mediaSync.mockImplementationOnce(() => stopping.promise);
    scrub(25_000); // Outside the script; superseded by the next seek.
    scrub(6000);
    expect(player.currentTime).toBe(6);
    await act(async () => stopping.resolve({ sync: { active: false, state: "seeking" } }));
    expect(mediaSync.mock.calls.filter(([event]) => event.state === "playing").map(([event]) => event.media_time_ms)).toEqual([6000]);
    expect(play).toHaveBeenCalledTimes(2);
  });

  it("keeps playback held when a seek Stop request fails", async () => {
    await start();
    mediaSync.mockRejectedValueOnce(new Error("Stop was not acknowledged"));
    scrub();
    await act(async () => undefined);
    expect(mediaSync.mock.calls.filter(([event]) => event.state === "playing")).toHaveLength(0);
    expect(play).toHaveBeenCalledOnce();
    expect(screen.getByRole("alert")).toHaveTextContent("Stop was not acknowledged");
  });

  it.each(["response", "error"])("ignores an obsolete heartbeat %s after seeking", async (outcome) => {
    const { player } = await start();
    const heartbeat = deferred<{ sync: MediaSyncStatus }>();
    Object.defineProperty(player, "paused", { configurable: true, get: () => false });
    mediaSync.mockImplementationOnce(() => heartbeat.promise);
    await waitFor(() => expect(mediaSync).toHaveBeenCalled(), { timeout: 2500 });
    expect(mediaSync.mock.calls[0][0].event).toBe("heartbeat");
    scrub();
    await act(async () => undefined);
    const count = play.mock.calls.length;
    await act(async () => {
      if (outcome === "response") heartbeat.resolve({ sync: { active: false, state: "drifted", requires_reanchor: true } });
      else heartbeat.reject(new Error("Obsolete heartbeat failed"));
    });
    expect(screen.getByText("Device following video")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(play).toHaveBeenCalledTimes(count);
  });
});
