import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { MediaVideo } from "../api/types";
import { useMediaCatalog } from "./useMediaCatalog";

vi.mock("../api/client", () => ({ api: { mediaVideos: vi.fn() } }));

const video: MediaVideo = {
  id: "one", display_name: "One", location_path: "", size_bytes: 1, modified_at: "", duration_ms: 1000, has_funscript: false, missing: false, scanned_at: "", tags: [],
};

describe("catalog refresh admission", () => {
  it("merges player metadata without overwriting curation saved while it was pending", async () => {
    vi.mocked(api.mediaVideos).mockResolvedValueOnce({ videos: [video] } as Awaited<ReturnType<typeof api.mediaVideos>>);
    const { result } = renderHook(useMediaCatalog);
    await waitFor(() => expect(result.current.loading).toBe(false));
    act(() => result.current.replaceVideo({ ...video, title: "Saved title", tags: ["Été"] }));
    act(() => result.current.replaceVideo({ id: video.id, duration_ms: 2000 }));
    expect(result.current.videos[0]).toMatchObject({ title: "Saved title", tags: ["Été"], duration_ms: 2000 });
  });
  it.each(["one", "bulk"])("keeps an acknowledged %s edit when an older read finishes", async (kind) => {
    vi.mocked(api.mediaVideos).mockResolvedValueOnce({ videos: [video] } as Awaited<ReturnType<typeof api.mediaVideos>>);
    const { result } = renderHook(useMediaCatalog);
    await waitFor(() => expect(result.current.loading).toBe(false));
    let finish!: (value: Awaited<ReturnType<typeof api.mediaVideos>>) => void;
    vi.mocked(api.mediaVideos).mockImplementationOnce(() => new Promise((resolve) => { finish = resolve; }));
    let reading!: Promise<void>;
    act(() => { reading = result.current.reload(); });
    act(() => {
      const edited = { ...video, title: "Saved title", tags: ["Été"] };
      if (kind === "one") result.current.replaceVideo(edited);
      else result.current.replaceVideos([edited]);
    });
    await act(async () => { finish({ videos: [video] } as Awaited<ReturnType<typeof api.mediaVideos>>); await reading; });
    expect(result.current.videos[0]).toMatchObject({ title: "Saved title", tags: ["Été"] });
    expect(result.current.loading).toBe(false);
  });
});
