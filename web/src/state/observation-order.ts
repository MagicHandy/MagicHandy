import type { BackendObservation, ControllerSnapshot } from "../api/types";

export function validController(value: unknown): value is ControllerSnapshot {
  if (!value || typeof value !== "object") return false;
  const snapshot = value as ControllerSnapshot;
  if (typeof snapshot.active !== "boolean" || typeof snapshot.read_only !== "boolean" || snapshot.active === snapshot.read_only) return false;
  if (snapshot.heartbeat_required !== undefined && typeof snapshot.heartbeat_required !== "boolean") return false;
  return !snapshot.heartbeat_required || (typeof snapshot.epoch === "string" && snapshot.epoch.length > 0 &&
    Number.isSafeInteger(snapshot.generation) && snapshot.generation! >= 0 &&
    Number.isSafeInteger(snapshot.revision) && snapshot.revision! > 0);
}

export function validObservation(value: BackendObservation | undefined): value is BackendObservation {
  return !!value && typeof value.epoch === "string" && value.epoch.length > 0 &&
    Number.isSafeInteger(value.revision) && value.revision > 0;
}

export function controllerIsNewer(candidate: ControllerSnapshot, previous?: ControllerSnapshot | null): boolean {
  if (!previous) return true;
  if (candidate.epoch && previous.epoch && candidate.epoch !== previous.epoch) return false;
  if (candidate.generation !== undefined && previous.generation !== undefined && candidate.generation !== previous.generation) {
    return candidate.generation > previous.generation;
  }
  if (candidate.revision !== undefined && previous.revision !== undefined) return candidate.revision >= previous.revision;
  return true;
}
