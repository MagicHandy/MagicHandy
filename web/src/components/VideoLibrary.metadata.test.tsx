import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { MediaVideo } from "../api/types";
import { VideoLibrary } from "./VideoLibrary";

vi.mock("../api/client", () => ({
  api: {
    mediaVideos: vi.fn(),
    mediaScan: vi.fn(),
    mediaTools: vi.fn(),
    mediaJob: vi.fn(),
    mediaFunscript: vi.fn(),
    mediaSync: vi.fn(),
    saveMediaDuration: vi.fn(),
    saveMediaMetadata: vi.fn(),
    tagMediaVideos: vi.fn(),
    renameMediaTag: vi.fn(),
    deleteMediaTag: vi.fn(),
    mediaStreamURL: (id: string) => `/stream/${id}`,
    mediaThumbnailURL: (video: MediaVideo) => `/thumb/${video.id}`,
    reportMediaCompatibility: vi.fn(),
    saveMediaThumbnail: vi.fn(),
  },
}));

vi.mock("../state/app-state", () => ({
  useAppState: () => ({ state: { settings: { media: {}, motion: {} } }, refresh: vi.fn() }),
}));

const idleScan = {
  running: false, cancellable: false, cancelled: false, files_visited: 0, videos_found: 0,
  summary: { locations: 1, added: 0, updated: 0, missing: 0, removed: 0, skipped: 0, issues: [] },
};
const idleJob = { running: false, cancellable: false, cancelled: false, total: 0, processed: 0, succeeded: 0, failed: 0, item_percent: 0, issues: [] };

function video(id: string, name: string, overrides: Partial<MediaVideo> = {}): MediaVideo {
  return {
    id, location_path: "C:/media", display_name: name, size_bytes: 2048, modified_at: "2026-09-01T00:00:00Z",
    duration_ms: 65_000, has_funscript: false, missing: false, scanned_at: "2026-09-01T00:00:00Z", tags: [], ...overrides,
  };
}

describe("VideoLibrary curation", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    vi.mocked(api.mediaScan).mockResolvedValue({ scan: idleScan });
    vi.mocked(api.mediaTools).mockResolvedValue({ tools: { configured: false, available: false } });
    vi.mocked(api.mediaJob).mockResolvedValue({ job: idleJob });
  });

  it("edits a video's details and sends only what changed", async () => {
    vi.mocked(api.mediaVideos).mockResolvedValue({ videos: [video("alpha", "alpha_take_07", { tags: ["Calm"] })] });
    vi.mocked(api.saveMediaMetadata).mockImplementation(async (id, patch) => ({
      video: video(id, "alpha_take_07", { title: patch.title, rating: patch.rating, tags: patch.tags ?? ["Calm"] }),
    }));
    render(<VideoLibrary locked={false} />);

    fireEvent.click(await screen.findByRole("button", { name: "Edit details for alpha_take_07" }));
    const dialog = screen.getByRole("dialog", { name: "Video details" });
    expect(within(dialog).getByLabelText("Title")).toHaveFocus();
    fireEvent.change(within(dialog).getByLabelText("Title"), { target: { value: "Evening take" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Rate 4 of 5" }));
    const tagInput = within(dialog).getByLabelText("Tags");
    fireEvent.change(tagInput, { target: { value: "slow build" } });
    fireEvent.keyDown(tagInput, { key: "Enter" });
    fireEvent.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => expect(api.saveMediaMetadata).toHaveBeenCalledWith("alpha", { title: "Evening take", rating: 4, tags: ["Calm", "slow build"] }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(await screen.findByRole("button", { name: "Play Evening take" })).toHaveTextContent("alpha_take_07");
  });

  it("filters the grid by tag and clears the filter from its chip", async () => {
    vi.mocked(api.mediaVideos).mockResolvedValue({ videos: [
      video("alpha", "Alpha", { tags: ["Calm"] }),
      video("beta", "Beta", { tags: ["Fast"] }),
    ] });
    render(<VideoLibrary locked={false} />);
    await screen.findByRole("button", { name: "Play Alpha" });

    fireEvent.change(screen.getByRole("combobox", { name: "Tag" }), { target: { value: "Calm" } });
    expect(screen.queryByRole("button", { name: "Play Beta" })).not.toBeInTheDocument();
    expect(screen.getByText("1 of 2 videos")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Stop filtering by Calm" }));
    expect(screen.getByRole("button", { name: "Play Beta" })).toBeInTheDocument();
  });

  it("tags several selected videos in one request", async () => {
    vi.mocked(api.mediaVideos).mockResolvedValue({ videos: [video("alpha", "Alpha"), video("beta", "Beta"), video("gamma", "Gamma")] });
    vi.mocked(api.tagMediaVideos).mockImplementation(async (ids, add) => ({
      videos: ids.map((id) => video(id, id === "alpha" ? "Alpha" : "Beta", { tags: add })),
    }));
    render(<VideoLibrary locked={false} />);
    await screen.findByRole("button", { name: "Play Alpha" });

    fireEvent.click(screen.getByRole("button", { name: "Select" }));
    fireEvent.click(screen.getByRole("button", { name: "Select Alpha" }));
    fireEvent.click(screen.getByRole("button", { name: "Select Beta" }));
    expect(screen.getByText("2 selected")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Tag to add or remove"), { target: { value: "Evening" } });
    fireEvent.click(screen.getByRole("button", { name: "Add tag" }));

    await waitFor(() => expect(api.tagMediaVideos).toHaveBeenCalledWith(["alpha", "beta"], ["Evening"], []));
    expect(api.saveMediaMetadata).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.getAllByText("Evening").length).toBeGreaterThanOrEqual(2));
  });

  it("renames a tag library-wide and reloads the catalog", async () => {
    vi.mocked(api.mediaVideos).mockResolvedValue({ videos: [video("alpha", "Alpha", { tags: ["calm"] })] });
    vi.mocked(api.renameMediaTag).mockResolvedValue({ renamed: 1, tags: [{ tag: "Slow", count: 1 }] });
    render(<VideoLibrary locked={false} />);
    await screen.findByRole("button", { name: "Play Alpha" });

    fireEvent.click(screen.getByRole("button", { name: "Manage tags" }));
    const dialog = screen.getByRole("dialog", { name: "Manage tags" });
    fireEvent.click(within(dialog).getByRole("button", { name: "Rename calm" }));
    fireEvent.change(within(dialog).getByLabelText("New name for calm"), { target: { value: "Slow" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Save name" }));

    await waitFor(() => expect(api.renameMediaTag).toHaveBeenCalledWith("calm", "Slow"));
    await waitFor(() => expect(api.mediaVideos).toHaveBeenCalledTimes(2));
  });

  it("offers no curation controls without control permission", async () => {
    vi.mocked(api.mediaVideos).mockResolvedValue({ videos: [video("alpha", "Alpha", { tags: ["Calm"], rating: 3, title: "Evening" })] });
    render(<VideoLibrary locked canCurate={false} />);
    const card = await screen.findByRole("button", { name: "Play Evening" });

    expect(within(card).getByText("Calm")).toBeInTheDocument();
    expect(within(card).getByRole("img", { name: "Rated 3 of 5" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Edit details/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Select" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Manage tags" })).not.toBeInTheDocument();
  });

  it("shows curation on the player page and edits it there", async () => {
    vi.mocked(api.mediaVideos).mockResolvedValue({ videos: [video("alpha", "alpha_take_07", { title: "Evening take", notes: "Good first half.", tags: ["Calm"] })] });
    render(<VideoLibrary locked={false} />);
    fireEvent.click(await screen.findByRole("button", { name: "Play Evening take" }));

    const player = screen.getByRole("region", { name: "Video playback" });
    expect(within(player).getByRole("heading", { name: "Evening take" })).toBeInTheDocument();
    expect(within(player).getByText("alpha_take_07")).toBeInTheDocument();
    expect(within(player).getByText("Good first half.")).toBeInTheDocument();
    fireEvent.click(within(player).getByRole("button", { name: "Edit details" }));
    expect(screen.getByRole("dialog", { name: "Video details" })).toBeInTheDocument();
  });
});
