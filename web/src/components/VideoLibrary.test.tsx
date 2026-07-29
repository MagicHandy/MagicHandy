import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { MediaJobState, MediaScanState, MediaVideo } from "../api/types";
import { VideoLibrary } from "./VideoLibrary";

vi.mock("../api/client", () => ({
  api: {
    mediaVideos: vi.fn(),
    mediaScan: vi.fn(),
    startMediaScan: vi.fn(),
    cancelMediaScan: vi.fn(),
    saveMediaDuration: vi.fn(),
    mediaStreamURL: (id: string) => `/stream/${id}`,
    mediaFunscript: vi.fn(),
    mediaSync: vi.fn(),
    mediaTools: vi.fn(),
    mediaJob: vi.fn(),
    cancelMediaJob: vi.fn(),
    convertMedia: vi.fn(),
    mediaThumbnailURL: (video: MediaVideo) => `/thumb/${video.id}`,
    reportMediaCompatibility: vi.fn(),
    saveMediaThumbnail: vi.fn(),
  },
}));

const mediaVideos = vi.mocked(api.mediaVideos);
const mediaScan = vi.mocked(api.mediaScan);
const startMediaScan = vi.mocked(api.startMediaScan);
const cancelMediaScan = vi.mocked(api.cancelMediaScan);
const mediaFunscript = vi.mocked(api.mediaFunscript);
const mediaSync = vi.mocked(api.mediaSync);
const mediaTools = vi.mocked(api.mediaTools);
const mediaJob = vi.mocked(api.mediaJob);
const convertMedia = vi.mocked(api.convertMedia);

const idleJob = {
  running: false, cancellable: false, cancelled: false,
  total: 0, processed: 0, succeeded: 0, failed: 0, item_percent: 0, issues: [],
};

const idleScan: MediaScanState = {
  running: false,
  cancellable: false,
  cancelled: false,
  files_visited: 0,
  videos_found: 0,
  summary: { locations: 1, added: 0, updated: 0, missing: 0, removed: 0, skipped: 0, issues: [] },
};

function video(id: string, name: string, modified: string, paired = false, missing = false): MediaVideo {
  return {
    id,
    location_path: "C:/media",
    display_name: name,
    size_bytes: 2048,
    modified_at: modified,
    duration_ms: 65000,
    has_funscript: paired,
    missing,
    scanned_at: modified,
  };
}

describe("VideoLibrary", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    mediaScan.mockResolvedValue({ scan: idleScan });
    mediaTools.mockResolvedValue({ tools: { configured: true, available: true, version: "ffmpeg version 7.1" } });
    mediaJob.mockResolvedValue({ job: idleJob });
    convertMedia.mockResolvedValue({ job: { ...idleJob, running: true, kind: "conversion", cancellable: true, total: 1 } });
    startMediaScan.mockResolvedValue({ scan: { ...idleScan, running: true, cancellable: true } });
    cancelMediaScan.mockResolvedValue({ scan: { ...idleScan, running: true, cancellable: false } });
    mediaFunscript.mockResolvedValue({ funscript: {
      video_id: "alpha",
      name: "Alpha session",
      duration_ms: 65_000,
      action_count: 2,
      actions: [{ at: 0, pos: 20 }, { at: 65_000, pos: 80 }],
    } });
    mediaSync.mockResolvedValue({ sync: { active: false, state: "idle" } });
  });

  it("searches the catalog and opens paired video playback with its timeline", async () => {
    mediaVideos.mockResolvedValue({ videos: [
      video("zeta", "Zeta session", "2026-07-18T12:00:00Z"),
      video("alpha", "Alpha session", "2026-07-19T12:00:00Z", true),
    ] });
    render(<VideoLibrary locked={false} stopSequence={7} />);

    const grid = await screen.findByRole("button", { name: "Play Alpha session" });
    expect(grid).toBeInTheDocument();
    expect(within(grid).getByText("media", { selector: ".media-card-location" })).toHaveAttribute("title", "C:/media");
    fireEvent.change(screen.getByRole("searchbox", { name: "Search videos" }), { target: { value: "alpha" } });
    expect(screen.queryByRole("button", { name: "Play Zeta session" })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Play Alpha session" }));
    const playerView = screen.getByRole("region", { name: "Video playback" });
    expect(within(playerView).getByRole("heading", { name: "Alpha session" })).toBeInTheDocument();
    expect(await within(playerView).findByLabelText("Alpha session")).toHaveAttribute("src", "/stream/alpha");
    expect(within(playerView).getByRole("slider", { name: /funscript timeline/i })).toBeInTheDocument();
    expect(within(playerView).getByText("Ready to synchronize on play")).toBeInTheDocument();

    fireEvent.click(within(playerView).getByRole("button", { name: "Videos" }));
    expect(await screen.findByRole("button", { name: "Play Alpha session" })).toBeInTheDocument();
  });

  it("offers an explicit scan from the empty state", async () => {
    mediaVideos.mockResolvedValue({ videos: [] });
    render(<VideoLibrary locked={false} />);

    const scanButton = await screen.findByRole("button", { name: "Scan library" });
    fireEvent.click(scanButton);

    await waitFor(() => expect(startMediaScan).toHaveBeenCalledOnce());
    expect(screen.getByRole("status")).toHaveTextContent("Scanning");
  });

  it("keeps scan and cancel commands available when the catalog is populated", async () => {
    mediaVideos.mockResolvedValue({ videos: [video("session", "Session", "2026-07-19T12:00:00Z")] });
    render(<VideoLibrary locked={false} />);

    fireEvent.click(await screen.findByRole("button", { name: "Scan library" }));
    await waitFor(() => expect(startMediaScan).toHaveBeenCalledOnce());
    fireEvent.click(await screen.findByRole("button", { name: "Cancel scan" }));

    await waitFor(() => expect(cancelMediaScan).toHaveBeenCalledOnce());
  });

  it("keeps loaded videos visible when scan status is temporarily unavailable", async () => {
    mediaVideos.mockResolvedValue({ videos: [video("session", "Session", "2026-07-19T12:00:00Z")] });
    mediaScan.mockRejectedValueOnce(new Error("scan endpoint unavailable"));
    render(<VideoLibrary locked={false} />);

    expect(await screen.findByRole("button", { name: "Play Session" })).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent("scan endpoint unavailable");
  });

  it("retries scan polling after a transient status failure", async () => {
    vi.useFakeTimers();
    mediaVideos.mockResolvedValue({ videos: [video("session", "Session", "2026-07-19T12:00:00Z")] });
    mediaScan
      .mockResolvedValueOnce({ scan: { ...idleScan, running: true, cancellable: true } })
      .mockRejectedValueOnce(new Error("temporary scan failure"))
      .mockResolvedValueOnce({ scan: idleScan });
    render(<VideoLibrary locked={false} />);

    await act(async () => Promise.resolve());
    await act(async () => vi.advanceTimersByTimeAsync(500));
    expect(screen.getByRole("alert")).toHaveTextContent("temporary scan failure");
    await act(async () => vi.advanceTimersByTimeAsync(1500));
    expect(mediaScan).toHaveBeenCalledTimes(3);
    vi.useRealTimers();
  });

  it("keeps unavailable entries after playable videos for every sort", async () => {
    mediaVideos.mockResolvedValue({ videos: [
      video("missing", "Alpha missing", "2026-07-20T12:00:00Z", false, true),
      video("playable", "Zeta playable", "2026-07-18T12:00:00Z"),
    ] });
    render(<VideoLibrary locked={false} />);

    await screen.findByRole("button", { name: "Play Zeta playable" });
    const catalogButtons = () => screen.getAllByRole("button", { name: /^(Play|Unavailable) / });
    expect(catalogButtons().map((button) => button.getAttribute("aria-label"))).toEqual([
      "Play Zeta playable",
      "Unavailable Alpha missing",
    ]);
    expect(screen.getByRole("button", { name: "Unavailable Alpha missing" })).toHaveAttribute("aria-disabled", "true");

    fireEvent.change(screen.getByRole("combobox", { name: "Sort" }), { target: { value: "recent" } });
    expect(catalogButtons().map((button) => button.getAttribute("aria-label"))).toEqual([
      "Play Zeta playable",
      "Unavailable Alpha missing",
    ]);
  });

  it("unmounts playback when the Videos route is left", async () => {
    mediaVideos.mockResolvedValue({ videos: [video("session", "Session", "2026-07-19T12:00:00Z")] });
    const result = render(<VideoLibrary locked={false} />);

    fireEvent.click(await screen.findByRole("button", { name: "Play Session" }));

// The player reads media playback settings straight from app state, the way
// QuickSettings and ChatPanel do; these tests render it outside the provider.
vi.mock("../state/app-state", () => ({
  useAppState: () => ({ state: { settings: { media: {}, motion: {} } }, refresh: vi.fn() }),
}));
    expect(screen.getByLabelText("Session")).toBeInTheDocument();

    result.unmount();
    expect(screen.queryByLabelText("Session")).not.toBeInTheDocument();
  });

  // Converting the video you are watching replaces it with a differently named
  // row, so without follow-through the page reports "Video unavailable" the
  // instant the repair succeeds — the worst possible moment to say that.
  it("follows a conversion of the open video to the repaired file", async () => {
    const broken = { ...video("broken", "Clip", "2026-07-19T12:00:00Z"), compatibility: "unsupported_codec" as const, container_type: "video/mp4" };
    const repaired = { ...video("repaired", "Clip_MHConverted", "2026-07-19T12:05:00Z"), container_type: "video/mp4" };
    // A same-name converted video may already exist elsewhere under the same
    // library root. Follow the row created by this run, not the first name match.
    const olderNamesake = { ...video("older-repaired", "Clip_MHConverted", "2026-07-18T12:00:00Z"), container_type: "video/mp4" };
    mediaVideos
      .mockResolvedValueOnce({ videos: [broken, olderNamesake] })
      .mockResolvedValue({ videos: [olderNamesake, repaired] });
    const startedAt = "2026-07-29T00:00:00Z";
    convertMedia.mockResolvedValue({ job: {
      ...idleJob,
      running: true,
      kind: "conversion" as const,
      cancellable: true,
      total: 1,
      started_at: startedAt,
    } });
    mediaJob
      .mockResolvedValueOnce({ job: { ...idleJob, failed: 1, error: "previous job failed", started_at: "2026-07-28T00:00:00Z", completed_at: "2026-07-28T00:01:00Z" } })
      .mockResolvedValue({ job: { ...idleJob, succeeded: 1, started_at: startedAt, completed_at: "2026-07-29T00:01:00Z" } });

    // The repair offer only appears once the browser has actually refused the
    // file, so the failure is raised the way a real one arrives: an error event
    // carrying MEDIA_ERR_SRC_NOT_SUPPORTED, with the bytes still reachable.
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: true, body: null }));
    render(<VideoLibrary locked={false} />);
    fireEvent.click(await screen.findByRole("button", { name: "Play Clip" }));
    expect(await screen.findByRole("heading", { name: "Clip" })).toBeInTheDocument();

    const player = screen.getByLabelText("Clip") as HTMLVideoElement;
    Object.defineProperty(player, "error", { configurable: true, value: { code: 4 } });
    fireEvent.error(player);

    // The conversion hides the original and publishes the repaired copy.
    fireEvent.click(await screen.findByRole("button", { name: "Convert this video" }));

    expect(await screen.findByRole("heading", { name: "Clip_MHConverted" })).toBeInTheDocument();
    expect(screen.getByLabelText("Clip_MHConverted")).toHaveAttribute("src", "/stream/repaired");
    expect(screen.queryByText("Video unavailable")).not.toBeInTheDocument();
    vi.unstubAllGlobals();
  });

  it("ignores a pre-conversion job status response that arrives late", async () => {
    const broken = {
      ...video("broken", "Delayed status", "2026-07-29T00:00:00Z"),
      compatibility: "unsupported_codec" as const,
      container_type: "video/mp4",
    };
    mediaVideos.mockResolvedValue({ videos: [broken] });
    let resolveInitialJob!: (value: { job: MediaJobState }) => void;
    mediaJob.mockImplementationOnce(() => new Promise((resolve) => {
      resolveInitialJob = resolve;
    }));
    convertMedia.mockResolvedValue({ job: {
      ...idleJob,
      running: true,
      kind: "conversion",
      cancellable: true,
      total: 1,
      started_at: "2026-07-29T00:01:00Z",
    } });

    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: true, body: null }));
    render(<VideoLibrary locked={false} />);
    fireEvent.click(await screen.findByRole("button", { name: "Play Delayed status" }));
    const player = screen.getByLabelText("Delayed status") as HTMLVideoElement;
    Object.defineProperty(player, "error", { configurable: true, value: { code: 4 } });
    fireEvent.error(player);
    fireEvent.click(await screen.findByRole("button", { name: "Convert this video" }));
    expect(await screen.findByRole("button", { name: "Converting" })).toBeDisabled();

    await act(async () => resolveInitialJob({ job: idleJob }));

    expect(screen.getByRole("button", { name: "Converting" })).toBeDisabled();
    vi.unstubAllGlobals();
  });

});
