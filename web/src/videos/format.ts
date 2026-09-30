import { t } from "../i18n";

export function formatDuration(durationMillis: number | null | undefined): string {
  if (!durationMillis || durationMillis <= 0) return "Duration unknown";
  return formatClock(durationMillis);
}

/** A position or length as m:ss or h:mm:ss; zero reads 0:00. */
export function formatClock(millis: number): string {
  const total = Math.max(0, Math.round((Number.isFinite(millis) ? millis : 0) / 1000));
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const seconds = total % 60;
  return hours > 0 ? `${hours}:${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}` : `${minutes}:${String(seconds).padStart(2, "0")}`;
}

export function formatFileSize(size: number): string {
  if (!Number.isFinite(size) || size < 0) return "Unknown size";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let value = size;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  return `${value >= 10 || unit === 0 ? Math.round(value) : value.toFixed(1)} ${units[unit]}`;
}

export function formatLocation(location: string): string {
  const parts = location.trim().replace(/[\\/]+$/, "").split(/[\\/]/).filter(Boolean);
  return parts[parts.length - 1] || location || t("Shared library");
}
