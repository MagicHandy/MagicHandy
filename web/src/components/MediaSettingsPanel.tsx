import { formatNumber, t } from "../i18n";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api } from "../api/client";
import type { MediaScanState, MediaSettingsPayload, MediaVideo } from "../api/types";
import { RefreshIcon, TrashIcon } from "../shell/icons";
import { HostPathField } from "./HostPathField";
import { MediaToolsSettings } from "./MediaToolsSettings";

// Mirrors config.MaxScriptOffsetMillis.
const MAX_SCRIPT_OFFSET_MILLIS = 2000;

function clampOffset(value: number): number {
  if (!Number.isFinite(value)) return 0;
  return Math.max(-MAX_SCRIPT_OFFSET_MILLIS, Math.min(MAX_SCRIPT_OFFSET_MILLIS, Math.round(value)));
}

function scanSettingsKey(media: MediaSettingsPayload): string {
  return JSON.stringify([
    media.library_paths,
    media.auto_scan_on_startup ?? false,
    media.remove_missing_on_scan ?? true,
    media.generate_thumbnails_on_scan ?? false,
    media.convert_incompatible_on_scan ?? false,
    media.ffmpeg_path ?? "",
    media.convert_h265_for_compatibility ?? false,
    media.reencode_codec ?? "h264",
    media.reencode_crf_h264 ?? 23,
    media.reencode_crf_h265 ?? 28,
    media.reencode_preset ?? "medium",
    media.reencode_audio_kbps ?? 192,
  ]);
}

interface Props {
  media: MediaSettingsPayload;
  savedMedia: MediaSettingsPayload;
  limitVideoScriptSpeed: boolean;
  onLimitVideoScriptSpeedChange: (enabled: boolean) => void;
  locked: boolean;
  onChange: (patch: Partial<MediaSettingsPayload>) => void;
}

export function MediaSettingsPanel({
  media,
  savedMedia,
  limitVideoScriptSpeed,
  onLimitVideoScriptSpeedChange,
  locked,
  onChange,
}: Props) {
  const locations = media.library_paths;
  const scriptOffsetMillis = media.script_offset_ms ?? 0;
  const onScriptOffsetChange = (millis: number) => onChange({ script_offset_ms: millis });
  const [draft, setDraft] = useState("");
  const [videos, setVideos] = useState<MediaVideo[]>([]);
  const [scan, setScan] = useState<MediaScanState | null>(null);
  const [error, setError] = useState("");
  const mounted = useRef(true);
  const refreshGeneration = useRef(0);
  const savedLocations = savedMedia.library_paths;
  const dirty = scanSettingsKey(media) !== scanSettingsKey(savedMedia);
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
    onChange({ library_paths: [...locations, value] });
    setDraft("");
    setError("");
  }

  function removeLocation(location: string) {
    if (!window.confirm(t("Remove {location} from the video library?", { location }))) return;
    onChange({ library_paths: locations.filter((entry) => entry !== location) });
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
  const scanStatus = scan?.running
    ? scan.trigger === "startup"
      ? t("Startup scan: {files} files checked / {videos} videos found", { files: formatNumber(scan.files_visited), videos: formatNumber(scan.videos_found) })
      : t("{files} files checked / {videos} videos found", { files: formatNumber(scan.files_visited), videos: formatNumber(scan.videos_found) })
    : dirty ? t("Save library or scan-option changes before scanning.")
      : t("{count} catalog entries", { count: formatNumber(videos.length) });
  return (
    <div className="media-settings">
      <div className="group">
      <h3 className="group-title">{t("Video script playback")}</h3>
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
      </div>

      <div className="group">
        <h3 className="group-title">{t("Video library")}</h3>
        <p className="form-status media-tool-hint">{t("MagicHandy catalogs videos and same-name funscripts from these folders. Scans run in the background and never change source files.")}</p>

        <h4 className="media-settings-subtitle">{t("Locations")}</h4>
        <div className="media-location-list" aria-label={t("Video library locations")}>
          {locations.length === 0 && <p className="form-status">{t("No locations configured.")}</p>}
          {locations.map((location) => {
            const count = counts.get(location) ?? { total: 0, missing: 0 };
            return (
              <div className="media-location-row" key={location}>
                <span>
                  <strong>{location}</strong>
                  <small>{count.missing > 0
                    ? t("{total} videos · {missing} missing", { total: formatNumber(count.total), missing: formatNumber(count.missing) })
                    : t("{count} videos", { count: formatNumber(count.total) })}</small>
                </span>
                <button
                  type="button"
                  className="icon-button"
                  aria-label={t("Remove {location}", { location })}
                  title={t("Remove location")}
                  disabled={locked || scan?.running}
                  onClick={() => removeLocation(location)}
                >
                  <TrashIcon />
                </button>
              </div>
            );
          })}
        </div>
        <div className="media-location-add">
          <HostPathField label={t("New location")} value={draft} kind="directory" disabled={locked || scan?.running} placeholder={t("Choose a video folder")} onChange={setDraft} />
          <button type="button" className="btn btn-secondary" disabled={locked || scan?.running || !draft.trim()} onClick={addLocation}>{t("Add location")}</button>
        </div>

        <fieldset className="media-scan-options" disabled={locked || scan?.running}>
          <legend>{t("Scan options")}</legend>
          <label className="toggle-line">
            <span className="toggle">
              <input type="checkbox" checked={media.auto_scan_on_startup ?? false} onChange={(event) => onChange({ auto_scan_on_startup: event.target.checked })} />
              <span className="track" aria-hidden="true" />
            </span>
            <span>{t("Scan library when MagicHandy starts")}<small>{t("Runs in the background after the core opens the catalog.")}</small></span>
          </label>
          <label className="toggle-line">
            <span className="toggle">
              <input type="checkbox" checked={media.remove_missing_on_scan ?? true} onChange={(event) => onChange({ remove_missing_on_scan: event.target.checked })} />
              <span className="track" aria-hidden="true" />
            </span>
            <span>{t("Remove missing catalog entries")}<small>{t("Only after a location is read completely. Source files and unavailable or partially read locations are always preserved.")}</small></span>
          </label>
          <label className="toggle-line">
            <span className="toggle">
              <input type="checkbox" checked={media.generate_thumbnails_on_scan ?? false} onChange={(event) => onChange({ generate_thumbnails_on_scan: event.target.checked })} />
              <span className="track" aria-hidden="true" />
            </span>
            <span>{t("Generate missing thumbnails after scanning")}<small>{t("Requires the saved FFmpeg location.")}</small></span>
          </label>
          <label className="toggle-line">
            <span className="toggle">
              <input type="checkbox" checked={media.convert_incompatible_on_scan ?? false} onChange={(event) => onChange({ convert_incompatible_on_scan: event.target.checked })} />
              <span className="track" aria-hidden="true" />
            </span>
            <span>{t("Convert unplayable files after scanning")}<small>{t("Only established unplayable files. This can take hours on a large library.")}</small></span>
          </label>
        </fieldset>

        <div className="media-scan-controls">
          <div>
            <strong>{t("Catalog scan")}</strong>
            <span>{scanStatus}</span>
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

      <MediaToolsSettings media={media} locked={locked} onChange={onChange} />
    </div>
  );
}
