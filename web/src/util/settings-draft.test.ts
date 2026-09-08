import { describe, expect, it } from "vitest";
import { rebaseSettingsDraft } from "./settings-draft";

describe("settings refresh after a managed runtime update", () => {
  it("adopts the activated runtime while preserving edits and explicit clears", () => {
    const baseline = { voice: { root: "old-runtime", reference: "saved", enabled: false }, folders: ["one"], locale: "en" };
    const draft = { ...baseline, voice: { ...baseline.voice, reference: "", enabled: true }, folders: ["two"] };
    const snapshot = { ...baseline, voice: { ...baseline.voice, root: "new-runtime" }, locale: "ja" };
    expect(rebaseSettingsDraft(baseline, draft, snapshot)).toEqual({
      voice: { root: "new-runtime", reference: "", enabled: true }, folders: ["two"], locale: "ja",
    });
    expect(baseline.voice.root).toBe("old-runtime");
    expect(snapshot.voice.reference).toBe("saved");
  });
});
