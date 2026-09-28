import { t } from "../i18n";

/**
 * What moves the device while a video plays (ADR 0032): its paired script,
 * the chat (not synced to the picture), or nothing. One source at a time.
 */
export type MotionSource = "script" | "chat" | "off";

/** Unless the viewer picks, a paired script drives and anything else stays still. */
export function defaultMotionSource(hasScript: boolean): MotionSource {
  return hasScript ? "script" : "off";
}

/** What the chat is told owns the device when it does not. */
export function chatMotionOwner(source: MotionSource): "script" | "off" | undefined {
  return source === "chat" ? undefined : source;
}

export function motionSourceNote(source: MotionSource): string {
  switch (source) {
    case "script": return t("The script moves the device. The chat talks but cannot change the motion.");
    case "chat": return t("The chat moves the device. It is not synced to the picture.");
    default: return t("Nothing moves the device while this video plays. The chat only talks.");
  }
}

export function motionSourceOptions(hasScript: boolean) {
  return [
    { value: "script" as const, label: t("Script"), disabled: !hasScript, title: hasScript ? undefined : t("This video has no script.") },
    { value: "chat" as const, label: t("Chat") },
    { value: "off" as const, label: t("Off") },
  ];
}
