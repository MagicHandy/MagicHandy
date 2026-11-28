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
export function EdgeControl({ hold, allowed, phase, still, unavailable, activate, resume, holdStill, resumeStill, onSettled, className }: {
  hold?: EdgeHoldView;
  /** Live motion that Edge can hold: running, not paused, not video or a program. */
  allowed: boolean;
  phase?: string;
  still?: boolean;
  /** Read-only, offline or stale: the press could not be admitted now. */
  unavailable: boolean;
  activate: () => Promise<unknown>;
  resume: () => Promise<unknown>;
  holdStill: () => Promise<unknown>;
  resumeStill: () => Promise<unknown>;
  onSettled?: () => unknown;
  className?: string;
}) {
  const active = Boolean(hold?.active || phase);
  const [pending, setPending] = useState(false);
  const { show } = useToast();
  const busy = pending || unavailable;
  const disabled = busy || !allowed;
  const detail = still ? t("Holding still until you resume") : hold?.phase === "failed" && hold.message ? translateKnown(hold.message) : pending && !active ? t("Edge requested") : active
    ? phase === "queued" || hold?.phase === "applying" ? t("Edge requested")
      : phase === "slowing" ? t("Slowing")
      : hold?.phase === "planning" ? t("Planning the return…")
      : hold?.automatic ? hold.resume_in_seconds ? t("Autopilot returns in {seconds} s", {seconds: hold.resume_in_seconds}) : t("Autopilot will resume") : t("Holding until you resume")
    : "";
  const hint = active
    ? hold?.message ? translateKnown(hold.message) : t("Return to the previous motion. Stop always ends the session.")
    : t("Switch to a gentle hold without waiting for AI. Queued device motion may briefly continue.");
  const invoke = async (action: () => Promise<unknown>) => {
    setPending(true);
    try {
      await action();
    } catch (error) {
      show(translateKnown(error instanceof Error ? error.message : "The holding pattern could not be applied."), "error");
    } finally {
      setPending(false);
      await onSettled?.();
    }
  };
  return <div className={className ? `edge-control ${className}` : "edge-control"} data-active={active || undefined}>
    <div className="edge-actions">
    <button type="button" className="edge-button" disabled={disabled} onClick={() => void invoke(activate)}
      aria-label={t("Edge — gentle hold")} title={active ? t("Holding until you resume") : hint}>
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" aria-hidden="true">
        <path d="M2 12c4-10 6-10 10 0s6 10 10 0" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
      <span>{pending ? t("Applying…") : t("Edge")}</span>
    </button>
    {allowed && <button type="button" className="edge-still" disabled={disabled} onClick={() => void invoke(holdStill)}
      aria-label={t("Hold still")} title={t("Hold still")}>
      <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M4 2h3v12H4zm5 0h3v12H9z"/></svg>
      {!active && <span>{t("Hold still")}</span>}
    </button>}
    {still ? <button type="button" className="edge-return" disabled={busy} onClick={() => void invoke(resumeStill)}
      aria-label={t("Resume from still hold")}>{t("Return")}</button> : active && <button type="button" className="edge-return" disabled={disabled} onClick={() => void invoke(resume)}
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
  return <EdgeControl hold={state?.modes?.edge_hold} allowed={Boolean(motion?.edge_allowed)} phase={motion?.edge_phase}
    still={motion?.still_hold} unavailable={readOnly || stale || !backendOnline}
    activate={api.activateEdge} resume={api.resumeEdge} holdStill={api.holdStill} resumeStill={api.resumeStillHold} onSettled={refresh} />;
}
