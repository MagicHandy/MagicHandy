import { formatNumber, t, translateKnown } from "../i18n";
import { useMemo, useState } from "react";
import { api } from "../api/client";
import type { MediaVideo } from "../api/types";
import { needsConversion } from "../api/types";
import { CheckIcon, CloseIcon, PencilIcon, PlayIcon, RefreshIcon, TagIcon, VideoIcon } from "../shell/icons";
import { RatingStars, TagChips } from "./CurationControls";
import { filterVideos, filtersActive, noVideoFilters, sortVideos, splitTagInput, tagCounts, videoTitle, type ScriptFilter, type VideoFilters, type VideoSort } from "./curation";
import { formatDuration, formatFileSize, formatLocation } from "./format";
import type { MediaCatalog } from "./useMediaCatalog";
import type { MediaMaintenance } from "./useMediaMaintenance";

interface Props {
  catalog: MediaCatalog;
  maintenance: MediaMaintenance;
  hostLocked: boolean;
  hostAdministration: boolean;
  /** The account may edit titles, ratings, notes and tags. */
  canCurate: boolean;
  onOpen: (id: string) => void;
  onConvertAll: () => void;
  onEditDetails: (video: MediaVideo) => void;
  onManageTags: () => void;
}

export function VideoGrid({ catalog, maintenance, hostLocked, hostAdministration, canCurate, onOpen, onConvertAll, onEditDetails, onManageTags }: Props) {
  const { videos, loading, error: catalogError } = catalog;
  const { scan, scanError, scanAction, job, tools, conversionError } = maintenance;
  const [filters, setFilters] = useState<VideoFilters>(noVideoFilters);
  const [sort, setSort] = useState<VideoSort>("name");
  const [selecting, setSelecting] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [bulkTag, setBulkTag] = useState("");
  const [bulkBusy, setBulkBusy] = useState(false);
  const [bulkError, setBulkError] = useState("");

  const tags = useMemo(() => tagCounts(videos), [videos]);
  const visible = useMemo(() => sortVideos(filterVideos(videos, filters), sort), [filters, sort, videos]);
  const pairedCount = videos.filter((video) => video.has_funscript).length;
  const brokenCount = videos.filter(needsConversion).length;
  const narrowed = filtersActive(filters);
  const selectedCount = videos.filter((video) => selected.has(video.id)).length;

  function updateFilters(patch: Partial<VideoFilters>) {
    setFilters((current) => ({ ...current, ...patch }));
  }

  function toggleSelecting() {
    setSelecting((current) => !current);
    setSelected(new Set());
    setBulkError("");
  }

  function toggleSelected(id: string) {
    setSelected((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  async function applyBulk(action: "add" | "remove") {
    const tagsToApply = splitTagInput(bulkTag);
    const ids = videos.filter((video) => selected.has(video.id)).map((video) => video.id);
    if (tagsToApply.length === 0 || ids.length === 0) return;
    setBulkBusy(true);
    setBulkError("");
    try {
      const response = await api.tagMediaVideos(ids, action === "add" ? tagsToApply : [], action === "remove" ? tagsToApply : []);
      catalog.replaceVideos(response.videos);
      setBulkTag("");
    } catch (reason) {
      setBulkError(reason instanceof Error ? translateKnown(reason.message) : t("Tags could not be saved."));
    } finally {
      setBulkBusy(false);
    }
  }

  const countLabel = narrowed
    ? t("{visible} of {total} videos", { visible: visible.length, total: videos.length })
    : pairedCount > 0
      ? t("{total} videos / {paired} with scripts", { total: videos.length, paired: pairedCount })
      : videos.length === 1 ? t("1 video") : t("{count} videos", { count: videos.length });

  return (
    <section className="library-view video-library" aria-label={t("Video library")} aria-busy={loading || scan?.running || undefined}>
      <div className="library-toolbar media-library-toolbar">
        <label className="compact-field"><span className="visually-hidden">{t("Search videos")}</span><input type="search" value={filters.query} placeholder={t("Search videos")} onChange={(event) => updateFilters({ query: event.target.value })} /></label>
        <div className="media-toolbar-actions">
          <span className="media-catalog-count">{countLabel}</span>
          <label className="media-sort"><span>{t("Sort")}</span><select value={sort} onChange={(event) => setSort(event.target.value as VideoSort)}><option value="name">{t("Name")}</option><option value="recent">{t("Most recent")}</option><option value="rating">{t("Highest rated")}</option></select></label>
          <button type="button" className="icon-button" aria-label={t("Reload video catalog")} title={t("Reload catalog")} disabled={loading} onClick={() => void catalog.reload()}><RefreshIcon /></button>
          {scan?.running ? (
            <button type="button" className="btn btn-secondary compact-command" disabled={hostLocked || !scan.cancellable || scanAction !== ""} onClick={() => void maintenance.cancelScan()}><CloseIcon />{t("Cancel scan")}</button>
          ) : (
            <button type="button" className="btn btn-secondary compact-command" disabled={hostLocked || scanAction !== ""} onClick={() => void maintenance.startScan()}><RefreshIcon />{t("Scan library")}</button>
          )}
        </div>
      </div>
      {videos.length > 0 && (
        <div className="media-filter-bar" role="group" aria-label={t("Filters")}>
          <label className="media-filter">
            <span>{t("Tag")}</span>
            <select value="" disabled={tags.length === 0} onChange={(event) => { if (event.target.value) updateFilters({ tags: [...filters.tags, event.target.value] }); }}>
              <option value="">{tags.length === 0 ? t("No tags yet") : t("Any tag")}</option>
              {tags.filter((entry) => !filters.tags.includes(entry.tag)).map((entry) => (
                <option key={entry.tag} value={entry.tag}>{t("{tag} ({count})", { tag: entry.tag, count: formatNumber(entry.count) })}</option>
              ))}
            </select>
          </label>
          {filters.tags.map((tag) => (
            <button key={tag} type="button" className="media-filter-chip" aria-label={t("Stop filtering by {tag}", { tag })} title={t("Stop filtering by {tag}", { tag })} onClick={() => updateFilters({ tags: filters.tags.filter((entry) => entry !== tag) })}>
              <span>{tag}</span><CloseIcon size={12} />
            </button>
          ))}
          <label className="media-filter">
            <span>{t("Rating")}</span>
            <select value={filters.minimumRating} onChange={(event) => updateFilters({ minimumRating: Number(event.target.value) })}>
              <option value={0}>{t("Any rating")}</option>
              {[1, 2, 3, 4, 5].map((stars) => <option key={stars} value={stars}>{t("At least {count} of 5", { count: stars })}</option>)}
            </select>
          </label>
          <label className="media-filter">
            <span>{t("Script")}</span>
            <select value={filters.script} onChange={(event) => updateFilters({ script: event.target.value as ScriptFilter })}>
              <option value="any">{t("Any")}</option>
              <option value="paired">{t("With a script")}</option>
              <option value="unpaired">{t("Without a script")}</option>
            </select>
          </label>
          {narrowed && <button type="button" className="btn btn-secondary compact-command" onClick={() => setFilters(noVideoFilters)}>{t("Clear filters")}</button>}
          <span className="media-filter-spacer" />
          {canCurate && <button type="button" className="btn btn-secondary compact-command" onClick={onManageTags}><TagIcon size={16} />{t("Manage tags")}</button>}
          {canCurate && <button type="button" className="btn btn-secondary compact-command" aria-pressed={selecting} onClick={toggleSelecting}>{selecting ? t("Done selecting") : t("Select")}</button>}
        </div>
      )}
      {selecting && (
        <div className="media-selection-bar" role="group" aria-label={t("Selected videos")}>
          <span className="media-selection-count" role="status">{t("{count} selected", { count: formatNumber(selectedCount) })}</span>
          <button type="button" className="btn btn-secondary compact-command" disabled={visible.length === 0} onClick={() => setSelected(new Set(visible.map((video) => video.id)))}>{t("Select all shown")}</button>
          <button type="button" className="btn btn-secondary compact-command" disabled={selectedCount === 0} onClick={() => setSelected(new Set())}>{t("Clear selection")}</button>
          <label className="visually-hidden" htmlFor="media-bulk-tag">{t("Tag to add or remove")}</label>
          <input
            id="media-bulk-tag"
            list="media-bulk-tag-suggestions"
            value={bulkTag}
            maxLength={200}
            placeholder={t("Tag to add or remove")}
            disabled={bulkBusy}
            onChange={(event) => setBulkTag(event.target.value)}
            onKeyDown={(event) => { if (event.key === "Enter") { event.preventDefault(); void applyBulk("add"); } }}
          />
          <datalist id="media-bulk-tag-suggestions">{tags.map((entry) => <option key={entry.tag} value={entry.tag} />)}</datalist>
          <button type="button" className="btn btn-primary compact-command" disabled={bulkBusy || selectedCount === 0 || !bulkTag.trim()} onClick={() => void applyBulk("add")}>{t("Add tag")}</button>
          <button type="button" className="btn btn-secondary compact-command" disabled={bulkBusy || selectedCount === 0 || !bulkTag.trim()} onClick={() => void applyBulk("remove")}>{t("Remove tag")}</button>
        </div>
      )}
      {bulkError && <p className="form-status media-playback-error" role="alert">{bulkError}</p>}
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
            <VideoCard
              key={video.id}
              video={video}
              selecting={selecting}
              selected={selected.has(video.id)}
              canCurate={canCurate}
              onOpen={() => onOpen(video.id)}
              onToggleSelected={() => toggleSelected(video.id)}
              onEditDetails={() => onEditDetails(video)}
            />
          ))}
        </div>
      )}
    </section>
  );
}

interface CardProps {
  video: MediaVideo;
  selecting: boolean;
  selected: boolean;
  canCurate: boolean;
  onOpen: () => void;
  onToggleSelected: () => void;
  onEditDetails: () => void;
}

function VideoCard({ video, selecting, selected, canCurate, onOpen, onToggleSelected, onEditDetails }: CardProps) {
  const title = videoTitle(video);
  const titled = title !== video.display_name;
  const label = selecting
    ? t("Select {name}", { name: title })
    : video.missing ? t("Unavailable {name}", { name: title }) : t("Play {name}", { name: title });
  return (
    <div className="media-card" data-missing={video.missing || undefined} data-selected={(selecting && selected) || undefined}>
      <button
        type="button"
        className="media-card-open"
        aria-disabled={!selecting && video.missing ? true : undefined}
        aria-pressed={selecting ? selected : undefined}
        title={!selecting && video.missing ? t("File unavailable. Reconnect the location and scan again.") : !selecting && needsConversion(video) ? t("This browser cannot play this file. Open it to convert.") : undefined}
        aria-label={label}
        onClick={() => {
          if (selecting) onToggleSelected();
          else if (!video.missing) onOpen();
        }}
      >
        <span className="media-card-visual" aria-hidden="true">
          {video.thumbnail_generated_at
            ? <img className="media-card-thumbnail" src={api.mediaThumbnailURL(video)} alt="" loading="lazy" decoding="async" />
            : <VideoIcon size={30} />}
          {selecting ? <span className="media-card-check">{selected && <CheckIcon size={16} />}</span> : <PlayIcon size={18} />}
        </span>
        <span className="media-card-copy">
          <strong>{title}</strong>
          {titled && <span className="media-card-file" title={video.display_name}>{video.display_name}</span>}
          <span>{formatDuration(video.duration_ms)} / {formatFileSize(video.size_bytes)}</span>
          <span className="media-card-location" title={video.location_path}>{formatLocation(video.location_path)}</span>
        </span>
        <span className="media-card-badges">
          {video.has_funscript && <span className="badge">{t("script")}</span>}
          {video.missing && <span className="badge media-missing-badge">{t("missing")}</span>}
          {needsConversion(video) && <span className="badge media-convert-badge">{t("needs conversion")}</span>}
          <RatingStars rating={video.rating} />
        </span>
        <TagChips tags={video.tags} />
      </button>
      {canCurate && !selecting && (
        <button type="button" className="icon-button media-card-edit" aria-label={t("Edit details for {name}", { name: title })} title={t("Edit details for {name}", { name: title })} onClick={onEditDetails}>
          <PencilIcon size={15} />
        </button>
      )}
    </div>
  );
}
