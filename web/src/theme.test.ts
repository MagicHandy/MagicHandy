import { describe, expect, it } from "vitest";
import { availableThemes, DEFAULT_THEME, normalizeTheme, UI_THEME_OPTIONS } from "./theme";

describe("theme catalog", () => {
  it("contains Steel Azure plus the 21 surviving mockup palettes", () => {
    expect(UI_THEME_OPTIONS).toHaveLength(22);
    expect(UI_THEME_OPTIONS[0]).toMatchObject({ id: DEFAULT_THEME, label: "Steel Azure", default: true });
    expect(new Set(UI_THEME_OPTIONS.map((theme) => theme.id))).toHaveProperty("size", 22);
    expect(UI_THEME_OPTIONS.map((theme) => String(theme.id))).not.toContain("velvet-lavender");
  });

  it("uses concise names for the awkward generated labels", () => {
    const labels = Object.fromEntries(UI_THEME_OPTIONS.map((theme) => [theme.id, theme.label]));
    expect(labels).toMatchObject({
      "midnight-graphite": "Midnight Graphite",
      "green-phosphor": "Green Phosphor",
      "paperwhite-crt": "Paperwhite CRT",
      "polar-frost": "Polar Frost",
      "deep-violet": "Deep Violet",
    });
  });

  it("falls back to the default and filters against backend option hints", () => {
    expect(normalizeTheme("deep-violet")).toBe("deep-violet");
    expect(normalizeTheme("deleted-theme")).toBe(DEFAULT_THEME);
    expect(availableThemes(["steel-azure", "paperwhite-crt"]).map((theme) => theme.id)).toEqual([
      "steel-azure",
      "paperwhite-crt",
    ]);
    expect(availableThemes([])).toBe(UI_THEME_OPTIONS);
  });
});
