import { formatNumber, t } from "../i18n";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api } from "../api/client";
import type { MediaScanState, MediaVideo } from "../api/types";
import { RefreshIcon, TrashIcon } from "../shell/icons";
import { HostPathField } from "./HostPathField";

// Mirrors config.MaxScriptOffsetMillis.
const MAX_SCRIPT_OFFSET_MILLIS = 2000;

function clampOffset(value: number): number {
  if (!Number.isFinite(value)) return 0;
  return Math.max(-MAX_SCRIPT_OFFSET_MILLIS, Math.min(MAX_SCRIPT_OFFSET_MILLIS, Math.round(value)));
}

interface Props {
  locations: string[];
  savedLocations: string[];
  limitVideoScriptSpeed: boolean;
  onLimitVideoScriptSpeedChange: (enabled: boolean) => void;
  scriptOffsetMillis: number;
  onScriptOffsetChange: (millis: number) => void;
  locked: boolean;
  onChange: (locations: string[]) => void;
}

export function MediaSettingsPanel({
  locations,
  savedLocations,
  limitVideoScriptSpeed,
  onLimitVideoScriptSpeedChange,
  scriptOffsetMillis,
  onScriptOffsetChange,
  locked,
  onChange,
}: Props) {
  const [draft, setDraft] = useState("");
  const [videos, setVideos] = useState<MediaVideo[]>([]);
  const [scan, setScan] = useState<MediaScanState | null>(null);
  const [error, setError] = useState("");
  const mounted = useRef(true);
  const refreshGeneration = useRef(0);
  const dirty = JSON.stringify(locations) !== JSON.stringify(savedLocations);
  const savedLocationsKey = JSON.stringify(savedLocations);

  const refresh = useCallback(async () => {
    const generation = ++refreshGeneration.current;
    try {
      const [videoResponse, scanResponse] = await Promise.all([api.mediaVideos(), api.mediaScan()]);
      if (!mounted.current || generation !== refreshGeneration.current) return;
      setVideos(videoResponse.videos ?? []);
      setScan(scanResponse.scan);
      setError("");
    } catch (reason) {
      if (mounted.current && generation === refreshGeneration.current) {
        setError(reason instanceof Error ? reason.message : t("Media library status could not be loaded."));
      }
    }
  }, []);

  useEffect(() => {
    mounted.current = true;
    void refresh();
    return () => {
      mounted.current = false;
      refreshGeneration.current += 1;
    };
  }, [refresh, savedLocationsKey]);

  useEffect(() => {
    if (!scan?.running) return undefined;
    const timer = window.setTimeout(() => void refresh(), 500);
    return () => window.clearTimeout(timer);
  }, [refresh, scan]);

  const counts = useMemo(() => {
    const result = new Map<string, { total: number; missing: number }>();
    for (const video of videos) {
      const current = result.get(video.location_path) ?? { total: 0, missing: 0 };
      current.total++;
      if (video.missing) current.missing++;
      result.set(video.location_path, current);
    }
    return result;
  }, [videos]);

  function addLocation() {
    const value = draft.trim();
    if (!value) return;
    if (locations.some((location) => location.localeCompare(value, undefined, { sensitivity: "base" }) === 0)) {
      setError(t("That library location is already listed."));
      return;
    }
    onChange([...locations, value]);
    setDraft("");
    setError("");
  }

  function removeLocation(location: string) {
    if (!window.confirm(t("Remove {location} from the video library?", { location }))) return;
    onChange(locations.filter((entry) => entry !== location));
  }

  async function startScan() {
    setError("");
    try {
      const response = await api.startMediaScan();
      setScan(response.scan);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("Media scan could not be started."));
    }
  }

  async function cancelScan() {
    try {
      const response = await api.cancelMediaScan();
      setScan(response.scan);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("Media scan could not be cancelled."));
    }
  }

  const summary = scan?.summary;
  return (
    <div className="media-settings">
      <h2 className="section-title">{t("Video script playback")}</h2>
      <label className="toggle-line">
        <span className="toggle">
          <input
            type="checkbox"
            checked={limitVideoScriptSpeed}
            disabled={locked}
            onChange={(event) => onLimitVideoScriptSpeedChange(event.target.checked)}
          />
          <span className="track" aria-hidden="true" />
        </span>
        <span>{t("Apply motion speed limit to video scripts")}<small>{t("Off preserves paired funscript timing and movement. On caps only over-limit segments. A change during playback requires Play again.")}</small>
        </span>
      </label>
      <label className="field">
        <span className="label">{t("Script offset")}<span className="hint-inline">{t("milliseconds")}</span>
        </span>
        <input
          type="number"
          min={-MAX_SCRIPT_OFFSET_MILLIS}
          max={MAX_SCRIPT_OFFSET_MILLIS}
          step={10}
          value={scriptOffsetMillis}
          disabled={locked}
          onChange={(event) => onScriptOffsetChange(clampOffset(Number(event.target.value)))}
        />
        <small>{t("Positive delays the device against the picture, negative advances it. Some offset is normal and is not a fault in the video or the script: scripts are authored to a particular sense of timing, screens add their own display delay, and the device takes real time to move. Start at 0, and adjust only if motion consistently feels early or late. A change during playback requires Play again.")}</small>
      </label>
      <div className="divider" />
      <h2 className="section-title">{t("Library locations")}</h2>
      <div className="media-location-list" aria-label={t("Video library locations")}>
        {locations.length === 0 && <p className="form-status">{t("No locations configured.")}</p>}
        {locations.map((location) => {
          const count = counts.get(location) ?? { total: 0, missing: 0 };
          return (
            <div className="media-location-row" key={location}>
              <span><strong>{location}</strong><small>{count.missing > 0 ? t("{total} videos · {missing} missing", { total: formatNumber(count.total), missing: formatNumber(count.missing) }) : t("{count} videos", { count: formatNumber(count.total) })}</small></span>
              <button type="button" className="icon-button" aria-label={t("Remove {location}", { location: location })} title={t("Remove location")} disabled={locked || scan?.running} onClick={() => removeLocation(location)}><TrashIcon /></button>
            </div>
          );
        })}
      </div>
      <div className="media-location-add">
        <HostPathField label={t("New location")} value={draft} kind="directory" disabled={locked || scan?.running} placeholder={t("Choose a video folder")} onChange={setDraft} />
        <button type="button" className="btn btn-secondary" disabled={locked || scan?.running || !draft.trim()} onClick={addLocation}>{t("Add location")}</button>
      </div>
      <div className="divider" />
      <div className="media-scan-controls">
        <div>
          <strong>{t("Catalog scan")}</strong>
          <span>{dirty ? t("Save location changes before scanning.") : scan?.running ? t("{files} files checked / {videos} videos found", { files: formatNumber(scan.files_visited), videos: formatNumber(scan.videos_found) }) : t("{count} catalog entries", { count: formatNumber(videos.length) })}</span>
        </div>
        {scan?.running
          ? <button type="button" className="btn btn-secondary" disabled={locked || !scan.cancellable} onClick={() => void cancelScan()}>{t("Cancel scan")}</button>
          : <button type="button" className="btn btn-primary" disabled={locked || dirty || savedLocations.length === 0} onClick={() => void startScan()}><RefreshIcon />{t("Scan now")}</button>}
      </div>
      {scan?.running && <progress className="media-scan-progress" aria-label={t("Media scan progress")} />}
      {!scan?.running && scan?.completed_at && summary && (
        <p className="media-scan-summary" role="status">{t("Last scan: {added} added / {updated} updated / {missing} missing / {removed} removed / {skipped} skipped", { added: summary.added, updated: summary.updated, missing: summary.missing, removed: summary.removed, skipped: summary.skipped })}</p>
      )}
      {(summary?.issues ?? []).map((issue) => <p className="form-status media-playback-error" role="alert" key={`${issue.location}:${issue.message}`}>{issue.location}: {issue.message}</p>)}
      {scan?.error && <p className="form-status media-playback-error" role="alert">{scan.error}</p>}
      {error && <p className="form-status media-playback-error" role="alert">{error}</p>}
    </div>
  );
}
