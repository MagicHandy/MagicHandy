import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { MediaScanState, MediaVideo } from "../api/types";
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
  },
}));

const mediaVideos = vi.mocked(api.mediaVideos);
const mediaScan = vi.mocked(api.mediaScan);
const startMediaScan = vi.mocked(api.startMediaScan);
const cancelMediaScan = vi.mocked(api.cancelMediaScan);
const mediaFunscript = vi.mocked(api.mediaFunscript);
const mediaSync = vi.mocked(api.mediaSync);

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
    expect(screen.getByLabelText("Session")).toBeInTheDocument();

    result.unmount();
    expect(screen.queryByLabelText("Session")).not.toBeInTheDocument();
  });
});
