import { formatNumber, t } from "../i18n";
import { useMemo, useState } from "react";
import { api } from "../api/client";
import { needsConversion } from "../api/types";
import { CloseIcon, PlayIcon, RefreshIcon, VideoIcon } from "../shell/icons";
import { formatDuration, formatFileSize, formatLocation } from "./format";
import type { MediaCatalog } from "./useMediaCatalog";
import type { MediaMaintenance } from "./useMediaMaintenance";

interface Props {
  catalog: MediaCatalog;
  maintenance: MediaMaintenance;
  hostLocked: boolean;
  hostAdministration: boolean;
  onOpen: (id: string) => void;
  onConvertAll: () => void;
}

export function VideoGrid({ catalog, maintenance, hostLocked, hostAdministration, onOpen, onConvertAll }: Props) {
  const { videos, loading, error: catalogError } = catalog;
  const { scan, scanError, scanAction, job, tools, conversionError } = maintenance;
  const [query, setQuery] = useState("");
  const [sort, setSort] = useState<"name" | "recent">("name");

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

  const pairedCount = videos.filter((video) => video.has_funscript).length;
  const brokenCount = videos.filter(needsConversion).length;

  return (
    <section className="library-view video-library" aria-label={t("Video library")} aria-busy={loading || scan?.running || undefined}>
      <div className="library-toolbar media-library-toolbar">
        <label className="compact-field"><span className="visually-hidden">{t("Search videos")}</span><input type="search" value={query} placeholder={t("Search videos")} onChange={(event) => setQuery(event.target.value)} /></label>
        <div className="media-toolbar-actions">
          <span className="media-catalog-count">{query.trim() ? t("{visible} of {total} videos", { visible: visible.length, total: videos.length }) : pairedCount > 0 ? t("{total} videos / {paired} with scripts", { total: videos.length, paired: pairedCount }) : videos.length === 1 ? t("1 video") : t("{count} videos", { count: videos.length })}</span>
          <label className="media-sort"><span>{t("Sort")}</span><select value={sort} onChange={(event) => setSort(event.target.value as "name" | "recent")}><option value="name">{t("Name")}</option><option value="recent">{t("Most recent")}</option></select></label>
          <button type="button" className="icon-button" aria-label={t("Reload video catalog")} title={t("Reload catalog")} disabled={loading} onClick={() => void catalog.reload()}><RefreshIcon /></button>
          {scan?.running ? (
            <button type="button" className="btn btn-secondary compact-command" disabled={hostLocked || !scan.cancellable || scanAction !== ""} onClick={() => void maintenance.cancelScan()}><CloseIcon />{t("Cancel scan")}</button>
          ) : (
            <button type="button" className="btn btn-secondary compact-command" disabled={hostLocked || scanAction !== ""} onClick={() => void maintenance.startScan()}><RefreshIcon />{t("Scan library")}</button>
          )}
        </div>
      </div>
      {brokenCount > 0 && !job?.running && (
        <div className="form-status media-convert-banner" role="status">
          <span>{t("{count} files cannot be played by this browser.", { count: formatNumber(brokenCount) })}</span>
          {!hostAdministration ? <span>{t("Host settings and diagnostics are managed by an administrator.")}</span> : tools?.available
            ? <button type="button" className="btn btn-secondary compact-command" disabled={hostLocked} onClick={onConvertAll}>{t("Convert all")}</button>
            : <a className="btn btn-secondary compact-command" href="#/settings/media">{t("Set up FFmpeg")}</a>}
        </div>
      )}
      {job?.running && (
        <div className="form-status media-job-status" role="status">
          <span>{job.kind === "conversion"
            ? t("Converting {name} ({done} of {total}, {percent}%)", { name: job.current_name ?? "", done: formatNumber(job.processed + 1), total: formatNumber(job.total), percent: formatNumber(job.item_percent) })
            : t("Generating thumbnails ({done} of {total})", { done: formatNumber(job.processed), total: formatNumber(job.total) })}</span>
          <button type="button" className="btn btn-secondary compact-command" disabled={hostLocked || !job.cancellable} onClick={() => void maintenance.cancelJob()}>{t("Cancel")}</button>
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
      {catalogError && videos.length === 0 && <div className="empty-state compact-empty" role="alert"><h2>{t("Video library unavailable")}</h2><p>{catalogError}</p><button type="button" className="btn btn-secondary" onClick={() => void catalog.reload()}>{t("Retry")}</button></div>}
      {!catalogError && loading && videos.length === 0 && <div className="empty-state compact-empty" role="status"><h2>{t("Loading videos")}</h2></div>}
      {!catalogError && !loading && videos.length === 0 && (
        <div className="empty-state compact-empty">
          <VideoIcon size={28} />
          <h2>{t("No videos scanned")}</h2>
          <p>{hostAdministration ? t("Add or review library locations before scanning the catalog.") : t("Host settings and diagnostics are managed by an administrator.")}</p>
          {hostAdministration && <a className="btn btn-secondary" href="#/settings/media">{t("Library locations")}</a>}
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
              onClick={() => { if (!video.missing) onOpen(video.id); }}
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
