import type { MessageKey } from "../i18n";
import { translateKnown } from "../i18n";

// One source for the persona axis labels, shared by the tile chips, the editor,
// and the switcher. Two copies would drift, and a chip reading "explicit" beside
// an editor reading "Explicit (direct sexual language)" is the kind of small
// inconsistency that makes an app feel assembled rather than designed.
//
// The register labels are the same keys Settings > Model already uses, so the
// two surfaces name the same value identically.
export const VOICE_LABELS: Partial<Record<string, MessageKey>> = {
  utility: "Utility (neutral assistant)",
  warm: "Warm (flirtatious, never explicit)",
  intimate: "Intimate (sensual partner)",
  explicit: "Explicit (direct sexual language)",
};

// The tile has room for one word, so the chip uses the short form of the same
// value the editor spells out.
export const VOICE_CHIP_LABELS: Partial<Record<string, MessageKey>> = {
  utility: "Utility",
  warm: "Warm",
  intimate: "Intimate",
  explicit: "Explicit",
};

export const STYLE_LABELS: Partial<Record<string, MessageKey>> = {
  neutral: "No particular style",
  playful: "Playful",
  tender: "Tender",
  dominant: "Dominant",
  submissive: "Submissive",
  teasing: "Teasing",
};

// Zones are named by where they are on the stroke, not by anatomy: the same
// wording the area-focus contract uses for tip/shaft/base.
export const AREA_LABELS: Partial<Record<string, MessageKey>> = {
  full: "Full range",
  tip: "Shallow end",
  shaft: "Middle",
  base: "Deep end",
};

export const LORE_MODE_LABELS: Partial<Record<string, MessageKey>> = {
  off: "Off",
  relevant: "Relevant only",
  full: "Full",
};

export function personaOptionLabel(
  labels: Partial<Record<string, MessageKey>>,
  value: string,
): string {
  const label = labels[value];
  return label ? translateKnown(label) : value;
}
