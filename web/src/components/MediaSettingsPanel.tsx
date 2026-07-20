import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api } from "../api/client";
import type { MediaScanState, MediaVideo } from "../api/types";
import { RefreshIcon, TrashIcon } from "../shell/icons";
import { HostPathField } from "./HostPathField";

interface Props {
  locations: string[];
  savedLocations: string[];
  locked: boolean;
  onChange: (locations: string[]) => void;
}

export function MediaSettingsPanel({ locations, savedLocations, locked, onChange }: Props) {
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
        setError(reason instanceof Error ? reason.message : "Media library status could not be loaded.");
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
      setError("That library location is already listed.");
      return;
    }
    onChange([...locations, value]);
    setDraft("");
    setError("");
  }

  function removeLocation(location: string) {
    if (!window.confirm(`Remove ${location} from the video library?`)) return;
    onChange(locations.filter((entry) => entry !== location));
  }

  async function startScan() {
    setError("");
    try {
      const response = await api.startMediaScan();
      setScan(response.scan);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Media scan could not be started.");
    }
  }

  async function cancelScan() {
    try {
      const response = await api.cancelMediaScan();
      setScan(response.scan);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Media scan could not be cancelled.");
    }
  }

  const summary = scan?.summary;
  return (
    <div className="media-settings">
      <h2 className="section-title">Library locations</h2>
      <div className="media-location-list" aria-label="Video library locations">
        {locations.length === 0 && <p className="form-status">No locations configured.</p>}
        {locations.map((location) => {
          const count = counts.get(location) ?? { total: 0, missing: 0 };
          return (
            <div className="media-location-row" key={location}>
              <span><strong>{location}</strong><small>{count.total.toLocaleString()} videos{count.missing > 0 ? ` / ${count.missing.toLocaleString()} missing` : ""}</small></span>
              <button type="button" className="icon-button" aria-label={`Remove ${location}`} title="Remove location" disabled={locked || scan?.running} onClick={() => removeLocation(location)}><TrashIcon /></button>
            </div>
          );
        })}
      </div>
      <div className="media-location-add">
        <HostPathField label="New location" value={draft} kind="directory" disabled={locked || scan?.running} placeholder="Choose a video folder" onChange={setDraft} />
        <button type="button" className="btn btn-secondary" disabled={locked || scan?.running || !draft.trim()} onClick={addLocation}>Add location</button>
      </div>
      <div className="divider" />
      <div className="media-scan-controls">
        <div>
          <strong>Catalog scan</strong>
          <span>{dirty ? "Save location changes before scanning." : scan?.running ? `${scan.files_visited.toLocaleString()} files checked / ${scan.videos_found.toLocaleString()} videos found` : `${videos.length.toLocaleString()} catalog entries`}</span>
        </div>
        {scan?.running
          ? <button type="button" className="btn btn-secondary" disabled={locked || !scan.cancellable} onClick={() => void cancelScan()}>Cancel scan</button>
          : <button type="button" className="btn btn-primary" disabled={locked || dirty || savedLocations.length === 0} onClick={() => void startScan()}><RefreshIcon />Scan now</button>}
      </div>
      {scan?.running && <progress className="media-scan-progress" aria-label="Media scan progress" />}
      {!scan?.running && scan?.completed_at && summary && (
        <p className="media-scan-summary" role="status">Last scan: {summary.added} added / {summary.updated} updated / {summary.missing} missing / {summary.removed} removed / {summary.skipped} skipped</p>
      )}
      {(summary?.issues ?? []).map((issue) => <p className="form-status media-playback-error" role="alert" key={`${issue.location}:${issue.message}`}>{issue.location}: {issue.message}</p>)}
      {scan?.error && <p className="form-status media-playback-error" role="alert">{scan.error}</p>}
      {error && <p className="form-status media-playback-error" role="alert">{error}</p>}
    </div>
  );
}
