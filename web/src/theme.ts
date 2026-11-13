export const DEFAULT_THEME = "steel-azure";

export interface ThemeOption {
  id: string;
  label: string;
  swatches: readonly [string, string, string];
  default?: boolean;
}

// One palette per brand colourway (docs/decisions/0073-brand-mark-and-palettes.md).
// IDs map one-to-one to selectors in themes.css; swatches are ground, surface
// and accent. Steel Azure and Warm Titanium keep their earlier values.
export const UI_THEME_OPTIONS = [
  { id: "steel-azure", label: "Steel Azure", swatches: ["#0e0f11", "#16181b", "#5b9dd9"], default: true },
  { id: "warm-titanium", label: "Warm Titanium", swatches: ["#151312", "#201d1b", "#e69d45"] },
  { id: "ember", label: "Ember", swatches: ["#100f0e", "#191716", "#f0906c"] },
  { id: "midnight-gold", label: "Midnight Gold", swatches: ["#0a101c", "#111a2b", "#d7a545"] },
  { id: "forest-brass", label: "Forest Brass", swatches: ["#0a1512", "#11211c", "#c9a04a"] },
  { id: "harbor", label: "Harbor", swatches: ["#0b131c", "#131e2b", "#8fc3ee"] },
  { id: "carbon", label: "Carbon", swatches: ["#0d0d0e", "#161617", "#e3e1dc"] },
  { id: "high-contrast", label: "High Contrast", swatches: ["#000000", "#0c0d0f", "#6cc4ff"] },
] as const satisfies readonly ThemeOption[];

export type UIThemeID = typeof UI_THEME_OPTIONS[number]["id"];

export function normalizeTheme(value: string | null | undefined): UIThemeID {
  return UI_THEME_OPTIONS.some((theme) => theme.id === value)
    ? value as UIThemeID
    : DEFAULT_THEME;
}

export function availableThemes(allowed: readonly string[] | undefined): readonly ThemeOption[] {
  if (!allowed?.length) return UI_THEME_OPTIONS;
  const allowedIDs = new Set(allowed);
  const available = UI_THEME_OPTIONS.filter((theme) => allowedIDs.has(theme.id));
  return available.length ? available : UI_THEME_OPTIONS;
}
