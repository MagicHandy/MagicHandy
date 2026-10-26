// One user-facing phase per voice worker, derived only from the backend
// snapshot plus this tab's own in-flight request. Keeping the derivation pure
// makes the precedence explicit: a load the backend is tracking is shown as
// loading, and every failure it reports stays visible. The text of an error
// never decides whether it is one.
import type { VoiceWorkerStatus } from "../api/types";

export type VoicePendingAction = "start" | "restart" | "load" | "unload" | "stop" | "test";

export type VoicePhase =
  | "checking"
  | "unavailable"
  | "off"
  | "unsaved"
  | "setup"
  | "installing"
  | "setup_failed"
  | "stopped"
  | "starting"
  | "loading"
  | "busy"
  | "ready"
  | "unloaded"
  | "stopping"
  | "failed";

export type VoicePhaseTone = "ok" | "pending" | "error" | "setup" | "idle";

export interface VoicePhaseInput {
  worker?: VoiceWorkerStatus;
  /** A status snapshot has been received at least once. */
  loaded: boolean;
  /** The status request failed before any snapshot arrived. */
  unavailable?: boolean;
  /** The settings form has unsaved voice changes. */
  dirty?: boolean;
  /** The selected app-managed module is missing or incomplete. */
  moduleBlocked?: boolean;
  repairActive?: boolean;
  repairFailed?: boolean;
  /** This tab's own request that has not answered yet. */
  pending?: VoicePendingAction;
  /** A snapshot taken after the pending request began reports a new failure. */
  pendingFailed?: boolean;
}

export function isLoadAction(action?: VoicePendingAction): boolean {
  return action === "start" || action === "restart" || action === "load";
}

/** True when the backend reports a failure: a crash, or any recorded error. */
export function workerHasFailure(worker?: VoiceWorkerStatus): boolean {
  return worker?.state === "crashed" || Boolean(worker?.last_error?.trim());
}

export function voiceWorkerPhase(input: VoicePhaseInput): VoicePhase {
  const { worker } = input;
  if (input.repairActive) return "installing";
  if (input.pending === "stop" || input.pending === "unload") return "stopping";
  if (!input.loaded) return input.unavailable ? "unavailable" : "checking";

  const state = worker?.state ?? "disabled";
  const notConfigured = state === "not_configured";
  // A missing module needs installing whether or not the form is saved, and
  // its repair does not depend on unsaved settings.
  if (input.moduleBlocked) return input.repairFailed ? "setup_failed" : "setup";
  if (input.dirty && (state === "disabled" || notConfigured)) return "unsaved";
  if (state === "disabled") return "off";
  if (notConfigured) return "setup";

  const failed = workerHasFailure(worker);
  if (state === "running" && !failed && worker?.model_state === "ready") return "ready";
  // This tab's start or load is in progress until it answers, unless a newer
  // snapshot already reports that it failed.
  if (isLoadAction(input.pending) && !input.pendingFailed) return state === "running" ? "loading" : "starting";
  if (failed) return "failed";
  if (state === "starting") return "starting";
  if (state === "running") {
    // The supervisor reports loading only while it tracks the operation.
    if (worker?.model_state === "loading") return "loading";
    if (worker?.model_state === "unloading") return "stopping";
    if (worker?.model_state === "unloaded") return "unloaded";
    return "busy";
  }
  return "stopped";
}

export function voicePhaseTone(phase: VoicePhase): VoicePhaseTone {
  switch (phase) {
    case "ready":
      return "ok";
    case "starting":
    case "loading":
    case "busy":
    case "installing":
    case "stopping":
      return "pending";
    case "failed":
    case "setup_failed":
    case "unavailable":
      return "error";
    case "setup":
    case "unsaved":
      return "setup";
    default:
      return "idle";
  }
}

export function voicePhaseIsBusy(phase: VoicePhase): boolean {
  return voicePhaseTone(phase) === "pending";
}

/** Identifies a worker observation, to tell a new failure from an old one. */
export function workerSignature(worker?: VoiceWorkerStatus): string {
  return [worker?.state ?? "", worker?.started_at ?? "", worker?.last_error ?? ""].join("\u0000");
}

/**
 * A crash appends the stderr tail to last_error. Show the cause by itself and
 * keep the raw worker output for the technical details.
 */
export function splitWorkerError(worker?: VoiceWorkerStatus): { message: string; output: string } {
  const message = worker?.last_error?.trim() ?? "";
  const output = worker?.stderr_tail?.trim() ?? "";
  const suffix = output ? ` — stderr: ${worker?.stderr_tail ?? ""}` : "";
  if (suffix && worker?.last_error?.endsWith(suffix)) {
    return { message: worker.last_error.slice(0, -suffix.length).trim(), output };
  }
  return { message, output };
}
