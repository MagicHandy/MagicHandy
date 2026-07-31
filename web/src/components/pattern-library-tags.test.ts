import { describe, expect, it } from "vitest";
import { collectPatternTags, patternMatchesQuery, patternMatchesTags, visiblePatternTags } from "./pattern-library-tags";

describe("pattern-library-tags", () => {
  it("hides internal catalog tags from the browser", () => {
    expect(visiblePatternTags(["steady", "curated", "imported", "pose-blowjob", "zone-tip", "full"])).toEqual([
      "steady",
      "full",
    ]);
  });

  it("filters patterns by visible tags", () => {
    const patterns = [
      { name: "A", tags: ["tip", "rhythmic"] },
      { name: "B", tags: ["full", "steady"] },
    ];
    expect(patternMatchesTags(patterns[0], new Set(["tip"]))).toBe(true);
    expect(patternMatchesTags(patterns[1], new Set(["tip"]))).toBe(false);
    expect(collectPatternTags(patterns)).toEqual(["full", "rhythmic", "steady", "tip"]);
    expect(patternMatchesQuery(patterns[0], "rhythmic")).toBe(true);
  });
});
