import type { PublicSettings } from "../api/types";

const editableSections = ["ui", "media", "motion", "llm", "voice", "chat", "autopilot", "diagnostics"] as const;
type PatchFields = Partial<Record<typeof editableSections[number], Record<string, unknown>>>;
export interface SettingsPatch {
  credential_revision?: string;
  changes: PatchFields;
  expected: PatchFields;
}

export function settingsPatch(baseline: PublicSettings, draft: PublicSettings, keys: { elevenlabs?: string; openai_tts?: string }): SettingsPatch {
  const changes: PatchFields = {};
  const expected: PatchFields = {};
  for (const section of editableSections) {
    const before = baseline[section] as Record<string, unknown> | undefined;
    const after = draft[section] as Record<string, unknown> | undefined;
    for (const [field, value] of Object.entries(after ?? {})) {
      if (JSON.stringify(value) === JSON.stringify(before?.[field])) continue;
      (changes[section] ??= {})[field] = value;
      (expected[section] ??= {})[field] = before?.[field] ?? null;
    }
  }
  for (const name of ["elevenlabs", "openai_tts"] as const) {
    if (keys[name] === undefined) continue;
    const field = name + "_api_key";
    (changes.voice ??= {})[field] = keys[name];
    (expected.voice ??= {})[field] = baseline.voice[name === "elevenlabs" ? "elevenlabs_key_set" : "openai_tts_key_set"];
  }
  return { changes, expected };
}

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
