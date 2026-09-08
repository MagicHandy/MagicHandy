// Rebase an unsaved form onto a new backend snapshot. Unedited fields adopt
// the snapshot (including installer-owned paths); explicit edits stay local.
export function rebaseSettingsDraft<T>(baseline: T, draft: T, snapshot: T): T {
  if (JSON.stringify(baseline) === JSON.stringify(draft)) return snapshot;
  if (!record(baseline) || !record(draft) || !record(snapshot)) return draft;
  const result: Record<string, unknown> = { ...snapshot };
  for (const key of Object.keys(draft)) {
    result[key] = rebaseSettingsDraft(baseline[key], draft[key], snapshot[key]);
  }
  return result as T;
}

function record(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}
