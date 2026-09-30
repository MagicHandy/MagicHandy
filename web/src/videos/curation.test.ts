import { describe, expect, it } from "vitest";
import type { MediaVideo } from "../api/types";
import { canonicalTag, filterVideos, filtersActive, noVideoFilters, sortVideos, splitTagInput, tagCounts, videoTitle } from "./curation";

function video(id: string, overrides: Partial<MediaVideo> = {}): MediaVideo {
  return {
    id,
    location_path: "C:/media",
    display_name: id,
    size_bytes: 1,
    modified_at: "2026-09-01T00:00:00Z",
    duration_ms: 60_000,
    has_funscript: false,
    missing: false,
    scanned_at: "2026-09-01T00:00:00Z",
    tags: [],
    ...overrides,
  };
}

describe("video curation helpers", () => {
  it("shows a title before the file name and ignores a blank one", () => {
    expect(videoTitle(video("clip", { title: "Evening take" }))).toBe("Evening take");
    expect(videoTitle(video("clip", { title: "   " }))).toBe("clip");
    expect(videoTitle(video("clip", { title: null }))).toBe("clip");
  });

  it("filters by text, every chosen tag, rating and script", () => {
    const videos = [
      video("a", { title: "Warm-up", tags: ["Calm", "Short"], rating: 4, has_funscript: true }),
      video("b", { tags: ["calm"], rating: 2, notes: "balcony light" }),
      video("c", { tags: ["Fast"] }),
    ];
    const ids = (filters: Partial<typeof noVideoFilters>) => filterVideos(videos, { ...noVideoFilters, ...filters }).map((entry) => entry.id);
    expect(ids({ query: "warm" })).toEqual(["a"]);
    expect(ids({ query: "balcony" })).toEqual(["b"]);
    expect(ids({ query: "fast" })).toEqual(["c"]);
    expect(ids({ tags: ["CALM"] })).toEqual(["a", "b"]);
    expect(ids({ tags: ["calm", "short"] })).toEqual(["a"]);
    expect(ids({ minimumRating: 3 })).toEqual(["a"]);
    expect(ids({ script: "paired" })).toEqual(["a"]);
    expect(ids({ script: "unpaired" })).toEqual(["b", "c"]);
    expect(filtersActive(noVideoFilters)).toBe(false);
    expect(filtersActive({ ...noVideoFilters, script: "paired" })).toBe(true);
  });

  it("sorts by title, recency or rating with unavailable entries last", () => {
    const videos = [
      video("zeta", { rating: 5 }),
      video("alpha", { title: "Omega", modified_at: "2026-09-10T00:00:00Z" }),
      video("beta", { missing: true, rating: 5 }),
      video("gamma", { rating: 3, modified_at: "2026-09-05T00:00:00Z" }),
    ];
    expect(sortVideos(videos, "name").map((entry) => entry.id)).toEqual(["gamma", "alpha", "zeta", "beta"]);
    expect(sortVideos(videos, "recent").map((entry) => entry.id)).toEqual(["alpha", "gamma", "zeta", "beta"]);
    expect(sortVideos(videos, "rating").map((entry) => entry.id)).toEqual(["zeta", "gamma", "alpha", "beta"]);
  });

  it("counts tags in the library's spelling and splits typed tags", () => {
    expect(tagCounts([video("a", { tags: ["Calm", "Build"] }), video("b", { tags: ["calm"] })])).toEqual([
      { tag: "Build", count: 1 },
      { tag: "Calm", count: 2 },
    ]);
    expect(splitTagInput(" slow   build, calm ,, ")).toEqual(["slow build", "calm"]);
    expect(canonicalTag("CALM", ["Calm", "Build"])).toBe("Calm");
    expect(canonicalTag("new", ["Calm"])).toBe("new");
  });
});
