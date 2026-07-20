import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { MediaScanState, MediaVideo } from "../api/types";
import { VideoLibrary } from "./VideoLibrary";

vi.mock("../api/client", () => ({
  api: {
    mediaVideos: vi.fn(),
    mediaScan: vi.fn(),
    startMediaScan: vi.fn(),
    saveMediaDuration: vi.fn(),
    mediaStreamURL: (id: string) => `/stream/${id}`,
  },
}));

const mediaVideos = vi.mocked(api.mediaVideos);
const mediaScan = vi.mocked(api.mediaScan);
const startMediaScan = vi.mocked(api.startMediaScan);

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
  });

  it("searches the catalog and opens plain video playback without starting motion", async () => {
    mediaVideos.mockResolvedValue({ videos: [
      video("zeta", "Zeta session", "2026-07-18T12:00:00Z"),
      video("alpha", "Alpha session", "2026-07-19T12:00:00Z", true),
    ] });
    render(<VideoLibrary active locked={false} />);

    const grid = await screen.findByRole("button", { name: "Play Alpha session" });
    expect(grid).toBeInTheDocument();
    fireEvent.change(screen.getByRole("searchbox", { name: "Search videos" }), { target: { value: "alpha" } });
    expect(screen.queryByRole("button", { name: "Play Zeta session" })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Play Alpha session" }));
    const playerView = screen.getByRole("region", { name: "Video playback" });
    expect(within(playerView).getByRole("heading", { name: "Alpha session" })).toBeInTheDocument();
    expect(within(playerView).getByLabelText("Alpha session")).toHaveAttribute("src", "/stream/alpha");
    expect(screen.queryByText(/device following/i)).not.toBeInTheDocument();

    fireEvent.click(within(playerView).getByRole("button", { name: "Videos" }));
    expect(await screen.findByRole("button", { name: "Play Alpha session" })).toBeInTheDocument();
  });

  it("offers an explicit scan from the empty state", async () => {
    mediaVideos.mockResolvedValue({ videos: [] });
    render(<VideoLibrary active locked={false} />);

    const scanButton = await screen.findByRole("button", { name: "Scan library" });
    fireEvent.click(scanButton);

    await waitFor(() => expect(startMediaScan).toHaveBeenCalledOnce());
    expect(screen.getByRole("status")).toHaveTextContent("Scanning");
  });

  it("keeps unavailable entries after playable videos for every sort", async () => {
    mediaVideos.mockResolvedValue({ videos: [
      video("missing", "Alpha missing", "2026-07-20T12:00:00Z", false, true),
      video("playable", "Zeta playable", "2026-07-18T12:00:00Z"),
    ] });
    render(<VideoLibrary active locked={false} />);

    await screen.findByRole("button", { name: "Play Zeta playable" });
    const catalogButtons = () => screen.getAllByRole("button", { name: /^(Play|Unavailable) / });
    expect(catalogButtons().map((button) => button.getAttribute("aria-label"))).toEqual([
      "Play Zeta playable",
      "Unavailable Alpha missing",
    ]);

    fireEvent.change(screen.getByRole("combobox", { name: "Sort" }), { target: { value: "recent" } });
    expect(catalogButtons().map((button) => button.getAttribute("aria-label"))).toEqual([
      "Play Zeta playable",
      "Unavailable Alpha missing",
    ]);
  });
});
