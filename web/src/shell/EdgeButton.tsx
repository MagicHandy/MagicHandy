import { useState } from "react";
import { api } from "../api/client";
import { t, translateKnown } from "../i18n";
import { useAppState, useMotionState, useToast } from "../state/app-state";

/** The backend-owned holding status, as the desktop and the phone receive it. */
export interface EdgeHoldView {
  active: boolean;
  phase: string;
  automatic: boolean;
  resume_in_seconds?: number;
  message?: string;
}

// This control submits semantic intent only. Holding state and return timing
// come from the backend; Emergency Stop is an independent sibling. The desktop
// and the phone remote render the same control with their own admission.
export function EdgeControl({ hold, allowed, unavailable, activate, resume, onSettled, className }: {
  hold?: EdgeHoldView;
  /** Live motion that Edge can hold: running, not paused, not video or a program. */
  allowed: boolean;
  /** Read-only, offline or stale: the press could not be admitted now. */
  unavailable: boolean;
  activate: () => Promise<unknown>;
  resume: () => Promise<unknown>;
  onSettled?: () => unknown;
  className?: string;
}) {
  const active = Boolean(hold?.active);
  const [pending, setPending] = useState(false);
  const { show } = useToast();
  const disabled = pending || unavailable || !allowed;
  const detail = hold?.phase === "failed" && hold.message ? translateKnown(hold.message) : active
    ? hold?.phase === "planning" ? t("Planning the return…")
      : hold?.automatic ? hold.resume_in_seconds ? t("Autopilot returns in {seconds} s", {seconds: hold.resume_in_seconds}) : t("Autopilot will resume") : t("Holding until you resume")
    : "";
  const hint = active
    ? hold?.message ? translateKnown(hold.message) : t("Return to the previous motion. Stop always ends the session.")
    : t("Switch to a gentle hold without waiting for AI. Queued device motion may briefly continue.");
  const invoke = async (returning = false) => {
    setPending(true);
    try {
      if (returning) await resume();
      else await activate();
    } catch (error) {
      show(translateKnown(error instanceof Error ? error.message : "The holding pattern could not be applied."), "error");
    } finally {
      setPending(false);
      await onSettled?.();
    }
  };
  return <div className={className ? `edge-control ${className}` : "edge-control"} data-active={active || undefined}>
    <div className="edge-actions">
    <button type="button" className="edge-button" disabled={disabled} onClick={() => void invoke()}
      aria-label={t("Edge — gentle hold")} title={active ? t("Holding until you resume") : hint}>
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" aria-hidden="true">
        <path d="M2 12c4-10 6-10 10 0s6 10 10 0" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
      <span>{pending ? t("Applying…") : t("Edge")}</span>
    </button>
    {active && <button type="button" className="edge-return" disabled={disabled} onClick={() => void invoke(true)}
      aria-label={t("Resume from gentle hold")} title={hint}>{t("Return")}</button>}
    </div>
    {detail && <span className="edge-caption" role="status" aria-live={hold?.automatic ? "off" : "polite"} title={hint}>{detail}</span>}
  </div>;
}

/** The desktop's Edge, admitted by this tab's control. */
export function EdgeButton() {
  const { state, readOnly, stale, backendOnline, refresh } = useAppState();
  const live = useMotionState();
  const motion = (live ?? state?.motion)?.engine;
  const allowed = Boolean(motion?.running && !motion.paused && !motion.target?.media_id && !motion.target?.program_id);
  return <EdgeControl hold={state?.modes?.edge_hold} allowed={allowed} unavailable={readOnly || stale || !backendOnline}
    activate={api.activateEdge} resume={api.resumeEdge} onSettled={refresh} />;
}
