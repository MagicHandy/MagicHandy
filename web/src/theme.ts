export const DEFAULT_THEME = "steel-azure";

export interface ThemeOption {
  id: string;
  label: string;
  swatches: readonly [string, string, string];
  default?: boolean;
}

// User-facing names are intentionally concise. IDs stay descriptive and map
// one-to-one to selectors in themes.css.
export const UI_THEME_OPTIONS = [
  { id: "steel-azure", label: "Steel Azure", swatches: ["#0e0f11", "#17191d", "#5b9dd9"], default: true },
  { id: "high-contrast", label: "High Contrast", swatches: ["#000000", "#101216", "#38b6ff"] },
  { id: "carbon-stealth", label: "Carbon Stealth", swatches: ["#0c0c0e", "#151518", "#e2e8f0"] },
  { id: "platinum", label: "Platinum", swatches: ["#161719", "#222428", "#94a3b8"] },
  { id: "zenith-monochrome", label: "Zenith Monochrome", swatches: ["#121417", "#1a1d22", "#f8fafc"] },
  { id: "sage", label: "Sage", swatches: ["#181a1b", "#242729", "#84a98c"] },
  { id: "polar-frost", label: "Polar Frost", swatches: ["#14181c", "#1f252c", "#81a1c1"] },
  { id: "iceberg-slate", label: "Iceberg Slate", swatches: ["#0e141b", "#161e28", "#38bdf8"] },
  { id: "nordic-dusk", label: "Nordic Dusk", swatches: ["#11131c", "#191c28", "#93c5fd"] },
  { id: "cyber-midnight", label: "Cyber Midnight", swatches: ["#050608", "#0f1116", "#00b4d8"] },
  { id: "midnight-graphite", label: "Midnight Graphite", swatches: ["#030406", "#0b0d12", "#22d3ee"] },
  { id: "ocean-trench", label: "Ocean Trench", swatches: ["#04090e", "#0b1520", "#06b6d4"] },
  { id: "obsidian-teal", label: "Obsidian Teal", swatches: ["#04070a", "#0a1017", "#00f5d4"] },
  { id: "carbon-emerald", label: "Carbon Emerald", swatches: ["#080a08", "#101511", "#10b981"] },
  { id: "moonlight-lavender", label: "Moonlight Lavender", swatches: ["#0b0b12", "#13131f", "#ddd6fe"] },
  { id: "deep-violet", label: "Deep Violet", swatches: ["#07060a", "#100d17", "#c084fc"] },
  { id: "obsidian-violet", label: "Obsidian Violet", swatches: ["#0a080d", "#14111c", "#a855f7"] },
  { id: "warm-titanium", label: "Warm Titanium", swatches: ["#151312", "#201d1b", "#e69d45"] },
  { id: "solaris-amber", label: "Solaris Amber", swatches: ["#080502", "#140d05", "#f59e0b"] },
  { id: "amber-phosphor", label: "Amber Phosphor", swatches: ["#060402", "#120c05", "#ffb000"] },
  { id: "green-phosphor", label: "Green Phosphor", swatches: ["#010602", "#061208", "#22c55e"] },
  { id: "paperwhite-crt", label: "Paperwhite CRT", swatches: ["#05070a", "#0d1118", "#7dd3fc"] },
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
