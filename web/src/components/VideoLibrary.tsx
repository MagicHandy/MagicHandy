import { t } from "../i18n";
import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import type { VideoPlayerHandle } from "../media/playbackController";
import { ArrowLeftIcon } from "../shell/icons";
import { tagCounts } from "../videos/curation";
import { TagManagerDialog } from "../videos/TagManagerDialog";
import { useMediaCatalog } from "../videos/useMediaCatalog";
import { useMediaMaintenance } from "../videos/useMediaMaintenance";
import { VideoDetailsDialog } from "../videos/VideoDetailsDialog";
import { VideoGrid } from "../videos/VideoGrid";
import { VideoPlayerPage } from "../videos/VideoPlayerPage";

interface Props {
  locked: boolean;
  hostAdministration?: boolean;
  stopSequence?: number;
  /**
   * The account may edit titles, ratings, notes and tags. Curation is library
   * content, so it does not need this tab to hold the controller lease.
   */
  canCurate?: boolean;
  /** The open video when the route owns selection. */
  selectedID?: string;
  /** Present when the route owns selection; otherwise the library keeps its own. */
  onSelect?: (id: string) => void;
  onPlayerHandleChange?: (handle: VideoPlayerHandle | null) => void;
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

// The Videos workspace: the catalog grid, or the player page for the open
// video. Catalog data, library maintenance and playback each have their own
// owner; this component only decides which view is showing.
export function VideoLibrary({ locked, hostAdministration = true, stopSequence, canCurate = !locked, selectedID: routedSelection, onSelect, onPlayerHandleChange }: Props) {
  const hostLocked = locked || !hostAdministration;
  const catalog = useMediaCatalog();
  const reloadCatalog = catalog.reload;
  const onCatalogChanged = useCallback(() => void reloadCatalog(), [reloadCatalog]);
  const maintenance = useMediaMaintenance({ hostAdministration, hostLocked, onCatalogChanged });
  const [localSelection, setLocalSelection] = useState("");
  const selectedID = onSelect ? routedSelection ?? "" : localSelection;
  const select = useCallback((id: string) => {
    if (onSelect) onSelect(id);
    else setLocalSelection(id);
  }, [onSelect]);
  // The output ID is derived from a server-only relative path. Remember enough
  // catalog identity to follow only the new sibling produced by this run,
  // without selecting an older same-name video from another folder.
  const [conversionTarget, setConversionTarget] = useState<ConversionFollowTarget | null>(null);
  const { videos, loading } = catalog;
  const { job, setConversionError } = maintenance;
  const selected = videos.find((video) => video.id === selectedID);
  const [editingID, setEditingID] = useState("");
  const [managingTags, setManagingTags] = useState(false);
  const tags = useMemo(() => tagCounts(videos), [videos]);
  const tagSuggestions = useMemo(() => tags.map((entry) => entry.tag), [tags]);
  const editing = videos.find((video) => video.id === editingID);

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
      select(replacements[0].id);
      setConversionTarget(null);
    } else if (replacements.length > 1 || followedJobSucceeded) {
      select("");
      setConversionTarget(null);
      setConversionError(t("Conversion completed, but the repaired file must be opened from the library."));
    }
  }, [conversionTarget, job, loading, select, selected, setConversionError, videos]);

  useEffect(() => {
    if (!conversionTarget || job?.running || !job?.completed_at) return;
    if (conversionTarget.jobStartedAt && job.started_at !== conversionTarget.jobStartedAt) return;
    if (job.cancelled || job.failed > 0 || job.error) setConversionTarget(null);
  }, [conversionTarget, job]);

  const conversionBusy = job?.running === true && job.kind === "conversion";

  function leaveVideo(): void {
    select("");
    setConversionTarget(null);
  }

  async function startConversion(ids: string[]) {
    if (hostLocked) return;
    // Only follow along when the open video is the one being repaired.
    const followTarget = ids.length === 1 && ids[0] === selectedID && selected ? {
      convertedName: `${selected.display_name}${CONVERTED_SUFFIX}`,
      locationPath: selected.location_path,
      knownVideoIDs: videos.map((video) => video.id),
    } : null;
    const started = await maintenance.requestConversion(ids);
    if (started) setConversionTarget(followTarget ? { ...followTarget, jobStartedAt: started.started_at } : null);
  }

  let view: ReactNode;
  if (selectedID && !selected && (conversionTarget || loading)) {
    view = (
      <section className="library-view video-player-view" aria-label={t("Video playback")} aria-busy="true">
        <button type="button" className="btn btn-secondary compact-command" onClick={leaveVideo}><ArrowLeftIcon />{t("Videos")}</button>
        <div className="empty-state compact-empty" role="status"><h2>{conversionTarget ? t("Refreshing catalog") : t("Loading videos")}</h2></div>
      </section>
    );
  } else if (selectedID && !loading && (!selected || selected.missing)) {
    view = (
      <section className="library-view video-player-view" aria-label={t("Video playback")}>
        <button type="button" className="btn btn-secondary compact-command" onClick={leaveVideo}><ArrowLeftIcon />{t("Videos")}</button>
        <div className="empty-state compact-empty" role="alert">
          <h2>{t("Video unavailable")}</h2>
          <p>{t("The catalog entry is missing or no longer available.")}</p>
          <button type="button" className="btn btn-secondary" onClick={() => { leaveVideo(); void catalog.reload(); }}>{t("Return to videos")}</button>
        </div>
      </section>
    );
  } else if (selected && !selected.missing) {
    view = (
      <VideoPlayerPage
        video={selected}
        locked={locked}
        stopSequence={stopSequence}
        hostLocked={hostLocked}
        hostAdministration={hostAdministration}
        canCurate={canCurate}
        toolsAvailable={Boolean(maintenance.tools?.available)}
        conversionBusy={conversionBusy}
        onBack={leaveVideo}
        onVideoUpdate={catalog.replaceVideo}
        onRequestConversion={() => void startConversion([selected.id])}
        onEditDetails={() => setEditingID(selected.id)}
        onHandleChange={onPlayerHandleChange}
      />
    );
  } else {
    view = (
      <VideoGrid
        catalog={catalog}
        maintenance={maintenance}
        hostLocked={hostLocked}
        hostAdministration={hostAdministration}
        canCurate={canCurate}
        onOpen={select}
        onConvertAll={() => void startConversion([])}
        onEditDetails={(video) => setEditingID(video.id)}
        onManageTags={() => setManagingTags(true)}
      />
    );
  }

  return (
    <>
      {view}
      {editing && canCurate && (
        <VideoDetailsDialog
          key={editing.id}
          video={editing}
          tagSuggestions={tagSuggestions}
          onClose={() => setEditingID("")}
          onSaved={catalog.replaceVideo}
        />
      )}
      {managingTags && canCurate && (
        <TagManagerDialog
          tags={tags}
          locked={!canCurate}
          onClose={() => setManagingTags(false)}
          onChanged={() => void catalog.reload()}
        />
      )}
    </>
  );
}
