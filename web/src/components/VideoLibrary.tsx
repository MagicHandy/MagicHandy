import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api } from "../api/client";
import type { MediaScanState, MediaVideo } from "../api/types";
import { ArrowLeftIcon, PlayIcon, RefreshIcon, VideoIcon } from "../shell/icons";
import { MediaVideoPlayer } from "./MediaVideoPlayer";

interface Props {
  active: boolean;
  locked: boolean;
}

export function VideoLibrary({ active, locked }: Props) {
  const [videos, setVideos] = useState<MediaVideo[]>([]);
  const [selectedID, setSelectedID] = useState("");
  const [query, setQuery] = useState("");
  const [sort, setSort] = useState<"name" | "recent">("name");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [scan, setScan] = useState<MediaScanState | null>(null);
  const mounted = useRef(true);

  const load = useCallback(async (signal?: AbortSignal) => {
    setLoading(true);
    setError("");
    try {
      const response = await api.mediaVideos(signal);
      if (!signal?.aborted && mounted.current) setVideos(Array.isArray(response.videos) ? response.videos : []);
    } catch (reason) {
      if (!signal?.aborted && mounted.current) setError(reason instanceof Error ? reason.message : "Video catalog could not be loaded.");
    } finally {
      if (!signal?.aborted && mounted.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    mounted.current = true;
    if (!active) return () => { mounted.current = false; };
    const controller = new AbortController();
    void load(controller.signal);
    void api.mediaScan().then((response) => {
      if (mounted.current && !controller.signal.aborted) setScan(response.scan);
    }).catch(() => undefined);
    return () => {
      mounted.current = false;
      controller.abort();
    };
  }, [active, load]);

  useEffect(() => {
    if (!active || !scan?.running) return undefined;
    const timer = window.setTimeout(() => {
      void api.mediaScan().then(async (response) => {
        if (!mounted.current) return;
        setScan(response.scan);
        if (!response.scan.running) await load();
      }).catch((reason) => {
        if (mounted.current) setError(reason instanceof Error ? reason.message : "Scan status could not be loaded.");
      });
    }, 500);
    return () => window.clearTimeout(timer);
  }, [active, load, scan]);

  const visible = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase();
    const filtered = needle ? videos.filter((video) => video.display_name.toLocaleLowerCase().includes(needle)) : [...videos];
    filtered.sort((left, right) => {
      const availability = Number(left.missing) - Number(right.missing);
      if (availability !== 0) return availability;
      return sort === "recent"
        ? Date.parse(right.modified_at) - Date.parse(left.modified_at) || left.display_name.localeCompare(right.display_name)
        : left.display_name.localeCompare(right.display_name, undefined, { sensitivity: "base" });
    });
    return filtered;
  }, [query, sort, videos]);
  const selected = videos.find((video) => video.id === selectedID);

  async function startScan() {
    setError("");
    try {
      const response = await api.startMediaScan();
      setScan(response.scan);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Video scan could not be started.");
    }
  }

  function updateVideo(video: MediaVideo) {
    setVideos((current) => current.map((entry) => entry.id === video.id ? video : entry));
  }

  if (selected && !selected.missing) {
    return (
      <section className="library-view video-player-view" aria-label="Video playback">
        <div className="media-player-heading">
          <button type="button" className="btn btn-secondary compact-command" onClick={() => setSelectedID("")}><ArrowLeftIcon />Videos</button>
          <div><h2>{selected.display_name}</h2><span>{formatFileSize(selected.size_bytes)}{selected.has_funscript ? " / script found" : ""}</span></div>
        </div>
        <MediaVideoPlayer video={selected} allowMetadataWrite={!locked} onVideoUpdate={updateVideo} />
      </section>
    );
  }

  return (
    <section className="library-view video-library" aria-label="Video library">
      <div className="library-toolbar media-library-toolbar">
        <label className="compact-field"><span className="visually-hidden">Search videos</span><input type="search" value={query} placeholder="Search videos" onChange={(event) => setQuery(event.target.value)} /></label>
        <div className="media-toolbar-actions">
          <label className="media-sort"><span>Sort</span><select value={sort} onChange={(event) => setSort(event.target.value as "name" | "recent")}><option value="name">Name</option><option value="recent">Most recent</option></select></label>
          <button type="button" className="icon-button" aria-label="Refresh video catalog" title="Refresh" disabled={loading} onClick={() => void load()}><RefreshIcon /></button>
        </div>
      </div>
      {scan?.running && <p className="form-status" role="status">Scanning {scan.files_visited.toLocaleString()} files / {scan.videos_found.toLocaleString()} videos found</p>}
      {error && <div className="empty-state compact-empty" role="alert"><h2>Video library unavailable</h2><p>{error}</p><button type="button" className="btn btn-secondary" onClick={() => void load()}>Retry</button></div>}
      {!error && loading && videos.length === 0 && <div className="empty-state compact-empty" role="status"><h2>Loading videos</h2></div>}
      {!error && !loading && videos.length === 0 && (
        <div className="empty-state compact-empty">
          <VideoIcon size={28} />
          <h2>No videos scanned</h2>
          <p>Add library locations in Settings, then scan the catalog.</p>
          <div className="row-actions">
            <a className="btn btn-secondary" href="#/settings/media">Library locations</a>
            <button type="button" className="btn btn-primary" disabled={locked || scan?.running} onClick={() => void startScan()}><RefreshIcon />Scan library</button>
          </div>
        </div>
      )}
      {!error && videos.length > 0 && visible.length === 0 && <div className="empty-state compact-empty"><h2>No matching videos</h2></div>}
      {!error && visible.length > 0 && (
        <div className="media-grid">
          {visible.map((video) => (
            <button
              type="button"
              key={video.id}
              className="media-card"
              data-missing={video.missing || undefined}
              disabled={video.missing}
              onClick={() => setSelectedID(video.id)}
              aria-label={`${video.missing ? "Unavailable " : "Play "}${video.display_name}`}
            >
              <span className="media-card-visual" aria-hidden="true"><VideoIcon size={30} /><PlayIcon size={18} /></span>
              <span className="media-card-copy"><strong>{video.display_name}</strong><span>{formatDuration(video.duration_ms)} / {formatFileSize(video.size_bytes)}</span></span>
              <span className="media-card-badges">{video.has_funscript && <span className="badge">script</span>}{video.missing && <span className="badge media-missing-badge">missing</span>}</span>
            </button>
          ))}
        </div>
      )}
    </section>
  );
}

export function formatDuration(durationMillis: number | null): string {
  if (!durationMillis || durationMillis <= 0) return "Duration unknown";
  const total = Math.round(durationMillis / 1000);
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
