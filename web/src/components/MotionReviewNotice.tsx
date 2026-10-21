import type { AppState } from "../api/types";
import { t, translateKnown, type MessageKey } from "../i18n";
import { DismissibleNotice } from "./DismissibleNotice";

const messages: Record<string, MessageKey> = {
  checking: "Reviewing motion in the background; local motion continues.",
  kept: "Second opinion: kept the local choice.",
  corrected: "Second opinion: corrected the motion choice.",
  skipped: "Second opinion: an older review was discarded.",
  unavailable: "Second opinion unavailable; local motion continues.",
};

export function MotionReviewNotice({ status }: { status: AppState["motion_review"] }) {
  const text = status && messages[status.state];
  if (!text) return null;
  return <DismissibleNotice id="model-motion-review-result" className="hint">
    <span role="status">{translateKnown(text)} {status?.model && <small>{[status.model, status.elapsed_ms ? t("{seconds} s", { seconds: (status.elapsed_ms / 1000).toFixed(1) }) : ""].filter(Boolean).join(" · ")}</small>}</span>
  </DismissibleNotice>;
}
