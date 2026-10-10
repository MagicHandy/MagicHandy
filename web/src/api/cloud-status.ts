import type { CloudPlanningStatus } from "./cloud-types";

// A missing or incompatible cloud endpoint must not break local model settings.
export function checkedCloudStatus(payload: CloudPlanningStatus): CloudPlanningStatus {
  if (!payload?.connection || !Array.isArray(payload.connection.profiles)
    || typeof payload.connection.generation !== "number" || !Array.isArray(payload.models)
    || !payload.readiness || !payload.motion_planner) {
    throw new Error("Cloud connection status is unavailable.");
  }
  return payload;
}
