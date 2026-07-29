import { formatNumber, t } from "../i18n";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api } from "../api/client";
import type { MediaJobState, MediaScanState, MediaToolStatus, MediaVideo } from "../api/types";
import { needsConversion } from "../api/types";
import { ArrowLeftIcon, CloseIcon, PlayIcon, RefreshIcon, VideoIcon } from "../shell/icons";
import { SyncedVideoPlayer } from "./SyncedVideoPlayer";

interface Props {
  locked: boolean;
  stopSequence?: number;
}

// Mirrors media.ConvertedSuffix. Fixed on both sides: changing it would orphan
// every file already carrying it.
const CONVERTED_SUFFIX = "_MHConverted";

interface ConversionFollowTarget {
  convertedName: string;
  locationPath: string;
  knownVideoIDs: string[];
  jobStartedAt?: string;
}

export function VideoLibrary({ locked, stopSequence }: Props) {
  const [videos, setVideos] = useState<MediaVideo[]>([]);
  const [selectedID, setSelectedID] = useState("");
  const [query, setQuery] = useState("");
  const [sort, setSort] = useState<"name" | "recent">("name");
  const [loading, setLoading] = useState(true);
  const [catalogError, setCatalogError] = useState("");
  const [scanError, setScanError] = useState("");
  const [scanAction, setScanAction] = useState<"start" | "cancel" | "">("");
  const [scan, setScan] = useState<MediaScanState | null>(null);
  const [job, setJob] = useState<MediaJobState | null>(null);
  const [tools, setTools] = useState<MediaToolStatus | null>(null);
  const [conversionError, setConversionError] = useState("");
  // The output ID is derived from a server-only relative path. Remember enough
  // catalog identity to follow only the new sibling produced by this run,
  // without selecting an older same-name video from another folder.
  const [conversionTarget, setConversionTarget] = useState<ConversionFollowTarget | null>(null);
  const updateJob = useCallback((next: MediaJobState) => {
    setJob((current) => {
      // A status request issued before a new job started may resolve late.
      // Never let that older snapshot stop polling the job we just accepted.
      if (current?.running && current.started_at && next.started_at !== current.started_at) {
        return current;
      }
      return next;
    });
  }, []);
  const mounted = useRef(true);
  const loadGeneration = useRef(0);

  const load = useCallback(async (signal?: AbortSignal) => {
    const generation = ++loadGeneration.current;
    setLoading(true);
    setCatalogError("");
    try {
      const response = await api.mediaVideos(signal);
      if (!signal?.aborted && mounted.current && generation === loadGeneration.current) {
        setVideos(Array.isArray(response.videos) ? response.videos : []);
      }
    } catch (reason) {
      if (!signal?.aborted && mounted.current && generation === loadGeneration.current) {
        setCatalogError(reason instanceof Error ? reason.message : t("Video catalog could not be loaded."));
      }
    } finally {
      if (!signal?.aborted && mounted.current && generation === loadGeneration.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    mounted.current = true;
    const controller = new AbortController();
    void load(controller.signal);
    void api.mediaScan(controller.signal).then((response) => {
      if (mounted.current && !controller.signal.aborted) {
        setScan(response.scan);
        setScanError("");
      }
    }).catch((reason) => {
      if (mounted.current && !controller.signal.aborted) {
        setScanError(reason instanceof Error ? reason.message : t("Scan status could not be loaded."));
      }
    });
    void api.mediaTools(controller.signal).then((response) => {
      if (mounted.current && !controller.signal.aborted) setTools(response.tools);
    }).catch(() => {
      // The absent state is the honest default here: without a tools answer,
      // conversion stays offered but disabled with a reason.
    });
    void api.mediaJob(controller.signal).then((response) => {
      if (mounted.current && !controller.signal.aborted) updateJob(response.job);
    }).catch(() => {});
    return () => {
      mounted.current = false;
      loadGeneration.current += 1;
      controller.abort();
    };
  }, [load, updateJob]);

  useEffect(() => {
    if (!scan?.running) return undefined;
    let stopped = false;
    let timer: number | undefined;
    const schedule = (delay: number) => {
      timer = window.setTimeout(() => void poll(), delay);
    };
    const poll = async () => {
      try {
        const response = await api.mediaScan();
        if (stopped || !mounted.current) return;
        setScan(response.scan);
        setScanError("");
        if (response.scan.running) schedule(500);
        else await load();
      } catch (reason) {
        if (stopped || !mounted.current) return;
        setScanError(reason instanceof Error ? reason.message : t("Scan status could not be loaded."));
        schedule(1500);
      }
    };
    schedule(500);
    return () => {
      stopped = true;
      window.clearTimeout(timer);
    };
  }, [load, scan?.running]);

  useEffect(() => {
    if (!job?.running) return undefined;
    let stopped = false;
    let timer: number | undefined;
    const poll = async () => {
      try {
        const response = await api.mediaJob();
        if (stopped || !mounted.current) return;
        updateJob(response.job);
        if (response.job.running) timer = window.setTimeout(() => void poll(), 700);
        else await load();
      } catch {
        if (!stopped && mounted.current) timer = window.setTimeout(() => void poll(), 2000);
      }
    };
    timer = window.setTimeout(() => void poll(), 700);
    return () => {
      stopped = true;
      window.clearTimeout(timer);
    };
  }, [job?.running, load, updateJob]);

  const visible = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase();
    const filtered = needle ? videos.filter((video) => (
      `${video.display_name} ${video.location_path}`.toLocaleLowerCase().includes(needle)
    )) : [...videos];
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

  // Converting the open video hides the original and adds a repaired sibling
  // under a new identifier. Match the new row by root/name and exclude every ID
  // that existed when conversion started. Display names alone are not unique.
  useEffect(() => {
    if (!conversionTarget || selected || loading) return;
    const knownIDs = new Set(conversionTarget.knownVideoIDs);
    const followedJobSucceeded = !job?.running
      && Boolean(job?.completed_at)
      && (!conversionTarget.jobStartedAt || job?.started_at === conversionTarget.jobStartedAt)
      && !job?.cancelled
      && (job?.failed ?? 0) === 0
      && !job?.error;
    const replacements = videos.filter((video) => (
      video.location_path === conversionTarget.locationPath
      && video.display_name === conversionTarget.convertedName
      && !knownIDs.has(video.id)
    ));
    if (replacements.length === 1 && followedJobSucceeded) {
      setSelectedID(replacements[0].id);
      setConversionTarget(null);
    } else if (replacements.length > 1 || followedJobSucceeded) {
      setSelectedID("");
      setConversionTarget(null);
      setConversionError(t("Conversion completed, but the repaired file must be opened from the library."));
    }
  }, [conversionTarget, job, loading, selected, videos]);

  useEffect(() => {
    if (!conversionTarget || job?.running || !job?.completed_at) return;
    if (conversionTarget.jobStartedAt && job.started_at !== conversionTarget.jobStartedAt) return;
    if (job.cancelled || job.failed > 0 || job.error) setConversionTarget(null);
  }, [conversionTarget, job]);

  const pairedCount = videos.filter((video) => video.has_funscript).length;
  const brokenCount = videos.filter(needsConversion).length;
  const conversionBusy = job?.running === true && job.kind === "conversion";

  function leaveVideo(): void {
    setSelectedID("");
    setConversionTarget(null);
  }

  // startConversion repairs files that cannot play. Named identifiers or the
  // whole library; either way the server converts only what it has established
  // is broken, so this cannot re-encode a working file.
  async function startConversion(ids: string[]) {
    setConversionError("");
    // Only follow along when the open video is the one being repaired.
    const followTarget = ids.length === 1 && ids[0] === selectedID && selected ? {
      convertedName: `${selected.display_name}${CONVERTED_SUFFIX}`,
      locationPath: selected.location_path,
      knownVideoIDs: videos.map((video) => video.id),
    } : null;
    try {
      const response = await api.convertMedia(ids);
      if (mounted.current) {
        setConversionTarget(followTarget ? { ...followTarget, jobStartedAt: response.job.started_at } : null);
        updateJob(response.job);
        if (!response.job.running) void load();
      }
    } catch (reason) {
      if (mounted.current) {
        setConversionError(reason instanceof Error ? reason.message : t("Conversion could not be started."));
      }
    }
  }

  async function cancelJob() {
    try {
      const response = await api.cancelMediaJob();
      if (mounted.current) updateJob(response.job);
    } catch {
      // Cancellation is advisory; the next poll reports the real state.
    }
  }

  async function startScan() {
    setScanError("");
    setScanAction("start");
    try {
      const response = await api.startMediaScan();
      if (mounted.current) setScan(response.scan);
    } catch (reason) {
      if (mounted.current) setScanError(reason instanceof Error ? reason.message : t("Video scan could not be started."));
    } finally {
      if (mounted.current) setScanAction("");
    }
  }

  async function cancelScan() {
    setScanError("");
    setScanAction("cancel");
    try {
      const response = await api.cancelMediaScan();
      if (mounted.current) setScan(response.scan);
    } catch (reason) {
      if (mounted.current) setScanError(reason instanceof Error ? reason.message : t("Video scan could not be cancelled."));
    } finally {
      if (mounted.current) setScanAction("");
    }
  }

  function updateVideo(video: MediaVideo) {
    setVideos((current) => current.map((entry) => entry.id === video.id ? video : entry));
  }

  if (selectedID && !loading && !selected && conversionTarget) {
    return (
      <section className="library-view video-player-view" aria-label={t("Video playback")} aria-busy="true">
        <button type="button" className="btn btn-secondary compact-command" onClick={leaveVideo}><ArrowLeftIcon />{t("Videos")}</button>
        <div className="empty-state compact-empty" role="status"><h2>{t("Refreshing catalog")}</h2></div>
      </section>
    );
  }

  if (selectedID && !loading && (!selected || selected.missing)) {
    return (
      <section className="library-view video-player-view" aria-label={t("Video playback")}>
        <button type="button" className="btn btn-secondary compact-command" onClick={leaveVideo}><ArrowLeftIcon />{t("Videos")}</button>
        <div className="empty-state compact-empty" role="alert">
          <h2>{t("Video unavailable")}</h2>
          <p>{t("The catalog entry is missing or no longer available.")}</p>
          <button type="button" className="btn btn-secondary" onClick={() => { leaveVideo(); void load(); }}>{t("Return to videos")}</button>
        </div>
      </section>
    );
  }

  if (selected && !selected.missing) {
    return (
      <section className="library-view video-player-view" aria-label={t("Video playback")}>
        <div className="media-player-heading">
          <button type="button" className="btn btn-secondary compact-command" onClick={leaveVideo}><ArrowLeftIcon />{t("Videos")}</button>
          <div><h2>{selected.display_name}</h2><span>{selected.has_funscript ? t("{size} / {location} / script found", { size: formatFileSize(selected.size_bytes), location: formatLocation(selected.location_path) }) : t("{size} / {location}", { size: formatFileSize(selected.size_bytes), location: formatLocation(selected.location_path) })}</span></div>
        </div>
        <SyncedVideoPlayer
          video={selected}
          locked={locked}
          stopSequence={stopSequence}
          onVideoUpdate={updateVideo}
          conversionBusy={conversionBusy}
          onRequestConversion={locked || !tools?.available ? undefined : () => void startConversion([selected.id])}
        />
        {!tools?.available && needsConversion(selected) && (
          <p className="form-status media-playback-error" role="alert">
            {t("Set an FFmpeg location in Settings > Media to enable conversion.")}
            {" "}
            <a href="#/settings/media">{t("Set up FFmpeg")}</a>
          </p>
        )}
      </section>
    );
  }

  return (
    <section className="library-view video-library" aria-label={t("Video library")} aria-busy={loading || scan?.running || undefined}>
      <div className="library-toolbar media-library-toolbar">
        <label className="compact-field"><span className="visually-hidden">{t("Search videos")}</span><input type="search" value={query} placeholder={t("Search videos")} onChange={(event) => setQuery(event.target.value)} /></label>
        <div className="media-toolbar-actions">
          <span className="media-catalog-count">{query.trim() ? t("{visible} of {total} videos", { visible: visible.length, total: videos.length }) : pairedCount > 0 ? t("{total} videos / {paired} with scripts", { total: videos.length, paired: pairedCount }) : videos.length === 1 ? t("1 video") : t("{count} videos", { count: videos.length })}</span>
          <label className="media-sort"><span>{t("Sort")}</span><select value={sort} onChange={(event) => setSort(event.target.value as "name" | "recent")}><option value="name">{t("Name")}</option><option value="recent">{t("Most recent")}</option></select></label>
          <button type="button" className="icon-button" aria-label={t("Reload video catalog")} title={t("Reload catalog")} disabled={loading} onClick={() => void load()}><RefreshIcon /></button>
          {scan?.running ? (
            <button type="button" className="btn btn-secondary compact-command" disabled={locked || !scan.cancellable || scanAction !== ""} onClick={() => void cancelScan()}><CloseIcon />{t("Cancel scan")}</button>
          ) : (
            <button type="button" className="btn btn-secondary compact-command" disabled={locked || scanAction !== ""} onClick={() => void startScan()}><RefreshIcon />{t("Scan library")}</button>
          )}
        </div>
      </div>
      {brokenCount > 0 && !job?.running && (
        <div className="form-status media-convert-banner" role="status">
          <span>{t("{count} files cannot be played by this browser.", { count: formatNumber(brokenCount) })}</span>
          {tools?.available
            ? <button type="button" className="btn btn-secondary compact-command" disabled={locked} onClick={() => void startConversion([])}>{t("Convert all")}</button>
            : <a className="btn btn-secondary compact-command" href="#/settings/media">{t("Set up FFmpeg")}</a>}
        </div>
      )}
      {job?.running && (
        <div className="form-status media-job-status" role="status">
          <span>{job.kind === "conversion"
            ? t("Converting {name} ({done} of {total}, {percent}%)", { name: job.current_name ?? "", done: formatNumber(job.processed + 1), total: formatNumber(job.total), percent: formatNumber(job.item_percent) })
            : t("Generating thumbnails ({done} of {total})", { done: formatNumber(job.processed), total: formatNumber(job.total) })}</span>
          <button type="button" className="btn btn-secondary compact-command" disabled={!job.cancellable} onClick={() => void cancelJob()}>{t("Cancel")}</button>
        </div>
      )}
      {conversionError && <p className="form-status media-playback-error" role="alert">{conversionError}</p>}
      {!job?.running && job?.failed ? <p className="form-status media-playback-error" role="alert">{t("{count} files could not be converted.", { count: formatNumber(job.failed) })}</p> : null}
      {scan?.running && <p className="form-status media-scan-status" role="status">{t("Scanning: {files} files / {videos} videos found", { files: formatNumber(scan.files_visited), videos: formatNumber(scan.videos_found) })}</p>}
      {loading && videos.length > 0 && <p className="form-status" role="status">{t("Refreshing catalog")}</p>}
      {scanError && <p className="form-status media-playback-error" role="alert">{t("Scan status: {message}", { message: scanError })}</p>}
      {!scan?.running && scan?.error && <p className="form-status media-playback-error" role="alert">{t("Scan failed: {message}", { message: scan.error })}</p>}
      {!scan?.running && (scan?.summary.issues?.length ?? 0) > 0 && <p className="form-status media-playback-error" role="alert">{t("{count} locations could not be fully scanned.", { count: scan?.summary.issues?.length ?? 0 })}<a href="#/settings/media">{t("Review locations")}</a></p>}
      {catalogError && videos.length > 0 && <p className="form-status media-playback-error" role="alert">{t("Catalog refresh failed; showing the last loaded results. {message}", { message: catalogError })}</p>}
      {catalogError && videos.length === 0 && <div className="empty-state compact-empty" role="alert"><h2>{t("Video library unavailable")}</h2><p>{catalogError}</p><button type="button" className="btn btn-secondary" onClick={() => void load()}>{t("Retry")}</button></div>}
      {!catalogError && loading && videos.length === 0 && <div className="empty-state compact-empty" role="status"><h2>{t("Loading videos")}</h2></div>}
      {!catalogError && !loading && videos.length === 0 && (
        <div className="empty-state compact-empty">
          <VideoIcon size={28} />
          <h2>{t("No videos scanned")}</h2>
          <p>{t("Add or review library locations before scanning the catalog.")}</p>
          <a className="btn btn-secondary" href="#/settings/media">{t("Library locations")}</a>
        </div>
      )}
      {videos.length > 0 && visible.length === 0 && <div className="empty-state compact-empty"><h2>{t("No matching videos")}</h2></div>}
      {visible.length > 0 && (
        <div className="media-grid">
          {visible.map((video) => (
            <button
              type="button"
              key={video.id}
              className="media-card"
              data-missing={video.missing || undefined}
              aria-disabled={video.missing || undefined}
              title={video.missing ? t("File unavailable. Reconnect the location and scan again.") : needsConversion(video) ? t("This browser cannot play this file. Open it to convert.") : undefined}
              onClick={() => { if (!video.missing) setSelectedID(video.id); }}
              aria-label={video.missing ? t("Unavailable {name}", { name: video.display_name }) : t("Play {name}", { name: video.display_name })}
            >
              <span className="media-card-visual" aria-hidden="true">
                {video.thumbnail_generated_at
                  ? <img className="media-card-thumbnail" src={api.mediaThumbnailURL(video)} alt="" loading="lazy" decoding="async" />
                  : <VideoIcon size={30} />}
                <PlayIcon size={18} />
              </span>
              <span className="media-card-copy"><strong>{video.display_name}</strong><span>{formatDuration(video.duration_ms)} / {formatFileSize(video.size_bytes)}</span><span className="media-card-location" title={video.location_path}>{formatLocation(video.location_path)}</span></span>
              <span className="media-card-badges">{video.has_funscript && <span className="badge">{t("script")}</span>}{video.missing && <span className="badge media-missing-badge">{t("missing")}</span>}{needsConversion(video) && <span className="badge media-convert-badge">{t("needs conversion")}</span>}</span>
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

export function formatLocation(location: string): string {
  const parts = location.trim().replace(/[\\/]+$/, "").split(/[\\/]/).filter(Boolean);
  return parts[parts.length - 1] || location || "Unknown location";
}
