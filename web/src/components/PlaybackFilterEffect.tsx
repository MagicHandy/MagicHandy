import { formatNumber, t } from "../i18n";
import type { MediaSyncStatus } from "../api/types";

interface Props {
  sync: MediaSyncStatus;
  smoothing: number;
  rounding: number;
  speedLimit: boolean;
  speedLimitPercent: number;
  pending: boolean;
}

export function PlaybackFilterEffect({ sync, smoothing, rounding, speedLimit, speedLimitPercent, pending }: Props) {
  const effect = sync.filter_effect;
  const measured = !pending && sync.active && effect
    && effect.smoothing_percent === smoothing && effect.rounding_ms === rounding
    && effect.speed_limit_enabled === speedLimit;
  const filtered = smoothing > 0 || rounding > 0 || speedLimit;
  const parts: string[] = [];
  if (pending) parts.push(t("Filter changes are being applied."));
  else if (!filtered) parts.push(t("Filters off; authored motion is preserved."));
  else if (!measured) parts.push(t("Filters on; effect is measured when motion re-arms."));
  else {
    if (effect.actions_removed) parts.push(t("{count} actions removed", {count:formatNumber(effect.actions_removed)}));
    if (effect.rounded_corners) parts.push(t("{count} corners rounded", {count:formatNumber(effect.rounded_corners)}));
    if (effect.peak_reduction_percent) parts.push(t("Peaks up to {percent}% lower", {percent:effect.peak_reduction_percent}));
    if (effect.peak_shift_ms) parts.push(t("Peak timing shifts up to {milliseconds} ms", {milliseconds:effect.peak_shift_ms}));
    if (effect.rounding_limited_corners) parts.push(t("{count} corners use shorter windows", {count:formatNumber(effect.rounding_limited_corners)}));
    if (effect.rounding_skipped_corners) parts.push(t("{count} corners too short to round", {count:formatNumber(effect.rounding_skipped_corners)}));
    if (speedLimit) parts.push(t("Travel is capped at {percent}% without changing the video clock.", {percent:sync.motion_speed_limit_percent ?? speedLimitPercent}));
    if (parts.length === 0) parts.push(t("No eligible script changes at these settings."));
  }
  return <p className="playback-panel-effect" role="status">{parts.join(" · ")}</p>;
}
