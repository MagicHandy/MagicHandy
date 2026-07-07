import type { TFunction } from "i18next";

export type RangeBucket = {
  id: string;
  label: string;
  min?: number;
  max?: number;
};

const BPM_BUCKET_DEFS: Omit<RangeBucket, "label">[] = [
  { id: "" },
  { id: "0-10", min: 0, max: 10 },
  { id: "11-30", min: 11, max: 30 },
  { id: "31-60", min: 31, max: 60 },
  { id: "61-100", min: 61, max: 100 },
  { id: "101-150", min: 101, max: 150 },
  { id: "151+", min: 151 },
];

const DURATION_BUCKET_DEFS: Omit<RangeBucket, "label">[] = [
  { id: "" },
  { id: "0-10", min: 0, max: 10 },
  { id: "11-30", min: 11, max: 30 },
  { id: "31-60", min: 31, max: 60 },
  { id: "61-120", min: 61, max: 120 },
  { id: "121-240", min: 121, max: 240 },
  { id: "241-420", min: 241, max: 420 },
  { id: "421-600", min: 421, max: 600 },
  { id: "600+", min: 601 },
];

const SPEED_IDS = ["", "slow", "medium", "fast", "very_fast"] as const;

export function getBpmRangeBuckets(t: TFunction): RangeBucket[] {
  return BPM_BUCKET_DEFS.map((b) => ({
    ...b,
    label: b.id === "" ? t("patterns.bpm.any") : b.id === "151+" ? t("patterns.bpm.151plus") : b.id,
  }));
}

export function getDurationRangeBuckets(t: TFunction): RangeBucket[] {
  const labels: Record<string, string> = {
    "": t("patterns.duration.any"),
    "0-10": t("patterns.duration.0-10"),
    "11-30": t("patterns.duration.11-30"),
    "31-60": t("patterns.duration.31-60"),
    "61-120": t("patterns.duration.61-120"),
    "121-240": t("patterns.duration.121-240"),
    "241-420": t("patterns.duration.241-420"),
    "421-600": t("patterns.duration.421-600"),
    "600+": t("patterns.duration.600plus"),
  };
  return DURATION_BUCKET_DEFS.map((b) => ({
    ...b,
    label: labels[b.id] ?? b.id,
  }));
}

export function getSpeedFilterOptions(t: TFunction) {
  const labels: Record<string, string> = {
    "": t("patterns.speed.any"),
    slow: t("patterns.speed.slow"),
    medium: t("patterns.speed.medium"),
    fast: t("patterns.speed.fast"),
    very_fast: t("patterns.speed.veryFast"),
  };
  return SPEED_IDS.map((id) => ({ id, label: labels[id] }));
}

export type PlayerBlockFilters = {
  speed: string;
  bpmRange: string;
  durationRange: string;
};

export const EMPTY_PLAYER_BLOCK_FILTERS: PlayerBlockFilters = {
  speed: "",
  bpmRange: "",
  durationRange: "",
};

export function findBucket(
  buckets: RangeBucket[],
  id: string,
): RangeBucket | undefined {
  return buckets.find((b) => b.id === id);
}

export function bucketToBpmParams(bucketId: string): {
  min_bpm?: number;
  max_bpm?: number;
} {
  const b = findBucket(
    BPM_BUCKET_DEFS.map((d) => ({ ...d, label: d.id })),
    bucketId,
  );
  if (!b?.id) return {};
  return {
    ...(b.min != null ? { min_bpm: b.min } : {}),
    ...(b.max != null ? { max_bpm: b.max } : {}),
  };
}

export function bucketToDurationParams(bucketId: string): {
  min_duration_ms?: number;
  max_duration_ms?: number;
} {
  const b = findBucket(
    DURATION_BUCKET_DEFS.map((d) => ({ ...d, label: d.id })),
    bucketId,
  );
  if (!b?.id) return {};
  return {
    ...(b.min != null ? { min_duration_ms: Math.round(b.min * 1000) } : {}),
    ...(b.max != null ? { max_duration_ms: Math.round(b.max * 1000) } : {}),
  };
}

export function formatBlockDuration(sec: number): string {
  if (sec < 60) return `${Math.round(sec)}s`;
  const m = Math.floor(sec / 60);
  const s = Math.round(sec % 60);
  return s > 0 ? `${m}m ${s}s` : `${m}m`;
}

export function speedDisplayLabel(
  speed: string | null | undefined,
  t: TFunction,
): string {
  if (!speed) return "—";
  const key = `patterns.speed.${speed}` as const;
  const translated = t(key, { defaultValue: "" });
  if (translated) return translated;
  return speed;
}

/** @deprecated Use getBpmRangeBuckets(t) */
export const BPM_RANGE_BUCKETS: RangeBucket[] = BPM_BUCKET_DEFS.map((b) => ({
  ...b,
  label: b.id,
}));
/** @deprecated Use getDurationRangeBuckets(t) */
export const DURATION_RANGE_BUCKETS: RangeBucket[] = DURATION_BUCKET_DEFS.map((b) => ({
  ...b,
  label: b.id,
}));
/** @deprecated Use getSpeedFilterOptions(t) */
export const SPEED_FILTER_OPTIONS = SPEED_IDS.map((id) => ({ id, label: id }));
