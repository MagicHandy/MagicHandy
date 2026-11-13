import { describe, expect, it } from "vitest";
import { availableThemes, DEFAULT_THEME, normalizeTheme, UI_THEME_OPTIONS } from "./theme";

describe("theme catalog", () => {
  it("contains Steel Azure, Warm Titanium and the six brand palettes", () => {
    expect(UI_THEME_OPTIONS).toHaveLength(8);
    expect(UI_THEME_OPTIONS[0]).toMatchObject({ id: DEFAULT_THEME, label: "Steel Azure", default: true });
    expect(new Set(UI_THEME_OPTIONS.map((theme) => theme.id))).toHaveProperty("size", 8);
    expect(UI_THEME_OPTIONS.map((theme) => String(theme.id))).toEqual([
      "steel-azure",
      "warm-titanium",
      "ember",
      "midnight-gold",
      "forest-brass",
      "harbor",
      "carbon",
      "high-contrast",
    ]);
  });

  it("keeps the two surviving palettes on their earlier colours", () => {
    const swatches = Object.fromEntries(UI_THEME_OPTIONS.map((theme) => [theme.id, theme.swatches[2]]));
    expect(swatches).toMatchObject({ "steel-azure": "#5b9dd9", "warm-titanium": "#e69d45" });
  });

  it("offers no violet palette", () => {
    expect(UI_THEME_OPTIONS.map((theme) => String(theme.id)).join(" ")).not.toMatch(/violet|lavender/);
  });

  it("falls back to the default and filters against backend option hints", () => {
    expect(normalizeTheme("midnight-gold")).toBe("midnight-gold");
    expect(normalizeTheme("deep-violet")).toBe(DEFAULT_THEME);
    expect(normalizeTheme("deleted-theme")).toBe(DEFAULT_THEME);
    expect(availableThemes(["steel-azure", "harbor"]).map((theme) => theme.id)).toEqual([
      "steel-azure",
      "harbor",
    ]);
    expect(availableThemes([])).toBe(UI_THEME_OPTIONS);
  });
});
