import { t } from "../i18n";

// One plain sentence per LLM motion mode, shown beside the selector so the
// names are never the only explanation.
export function llmMotionModeHelp(mode: string): string {
  switch (mode) {
    case "dynamic":
      return t("The AI invents the stroke shape, speed and range each turn.");
    case "pattern":
      return t("The AI picks from your enabled library patterns and sets the speed.");
    case "layered":
      return t("The AI adjusts reach, location and pace separately and keeps the rest.");
    case "creative_v2":
      return t("A flowing phrase that keeps developing over many strokes. The default.");
    case "off":
      return t("The AI only talks and never moves the device.");
    default:
      return "";
  }
}
