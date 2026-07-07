import type { FunscriptAction } from "../lib/funscriptHeatmap";

export function blockTimelineSpanMs(actions: FunscriptAction[]): number {
  if (actions.length < 2) return 0;
  const sorted = [...actions].sort((a, b) => a.at - b.at);
  return Math.max(1, sorted[sorted.length - 1].at - sorted[0].at);
}

export function trimBlockActions(
  actions: FunscriptAction[],
  trimStartMs: number,
  trimEndMs: number,
): FunscriptAction[] {
  if (actions.length < 2) return [];
  const sorted = [...actions].sort((a, b) => a.at - b.at);
  const t0 = sorted[0].at;
  const startAbs = t0 + Math.max(0, trimStartMs);
  const endAbs = t0 + Math.max(startAbs, trimEndMs);
  return sorted.filter((item) => item.at >= startAbs && item.at <= endAbs);
}

export function formatEditMs(ms: number): string {
  const totalSec = Math.max(0, Math.round(ms / 1000));
  const minutes = Math.floor(totalSec / 60);
  const seconds = totalSec % 60;
  if (minutes > 0) return `${minutes}:${seconds.toString().padStart(2, "0")}`;
  return `${(ms / 1000).toFixed(1)}s`;
}
