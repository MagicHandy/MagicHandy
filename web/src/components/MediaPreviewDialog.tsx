import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { api } from "../api/client";
import type { MediaVideo } from "../api/types";
import { CloseIcon, RefreshIcon } from "../shell/icons";
import { trapModalTab } from "../util/modal";
import { ImportTimeline, formatTimelineTime, type TimelinePoint, type TimeWindow } from "./ImportTimeline";
import { MediaVideoPlayer } from "./MediaVideoPlayer";

interface Props {
  funscriptName: string;
  points: TimelinePoint[];
  duration: number;
  trim: TimeWindow;
  viewport: TimeWindow;
  disabled: boolean;
  onTrimChange: Dispatch<SetStateAction<TimeWindow>>;
  onViewportChange: Dispatch<SetStateAction<TimeWindow>>;
  onClose: () => void;
}

export function MediaPreviewDialog({
  funscriptName,
  points,
  duration,
  trim,
  viewport,
  disabled,
  onTrimChange,
  onViewportChange,
  onClose,
}: Props) {
  const [videos, setVideos] = useState<MediaVideo[]>([]);
  const [selectedID, setSelectedID] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [playhead, setPlayhead] = useState(0);
  const [videoDuration, setVideoDuration] = useState<number | null>(null);
  const dialogRef = useRef<HTMLElement>(null);
  const mounted = useRef(true);
  const loadGeneration = useRef(0);

  const loadVideos = useCallback(async (signal?: AbortSignal) => {
    const generation = ++loadGeneration.current;
    setLoading(true);
    setError("");
    try {
      const response = await api.mediaVideos(signal);
      if (!mounted.current || signal?.aborted || generation !== loadGeneration.current) return;
      const available = (response.videos ?? []).filter((video) => !video.missing);
      setVideos(available);
      setSelectedID((current) => {
        if (available.some((video) => video.id === current)) return current;
        return available.find((video) => video.display_name.localeCompare(funscriptName, undefined, { sensitivity: "base" }) === 0)?.id ?? available[0]?.id ?? "";
      });
    } catch (reason) {
      if (mounted.current && !signal?.aborted && generation === loadGeneration.current) {
        setError(reason instanceof Error ? reason.message : "Video catalog could not be loaded.");
      }
    } finally {
      if (mounted.current && !signal?.aborted && generation === loadGeneration.current) setLoading(false);
    }
  }, [funscriptName]);

  useEffect(() => {
    mounted.current = true;
    const controller = new AbortController();
    void loadVideos(controller.signal);
    return () => {
      mounted.current = false;
      loadGeneration.current += 1;
      controller.abort();
    };
  }, [loadVideos]);

  useEffect(() => {
    const returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    dialogRef.current?.focus();
    const keyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onClose();
        return;
      }
      if (dialogRef.current) trapModalTab(event, dialogRef.current);
    };
    document.addEventListener("keydown", keyDown);
    return () => {
      document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", keyDown);
      returnFocus?.focus();
    };
  }, [onClose]);

  useEffect(() => {
    setPlayhead(0);
    setVideoDuration(null);
  }, [selectedID]);

  const selected = videos.find((video) => video.id === selectedID);
  const durationDelta = videoDuration === null ? null : Math.abs(videoDuration - duration);

  return (
    <div className="modal-scrim" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <section ref={dialogRef} className="media-preview-dialog" role="dialog" aria-labelledby="media-preview-title" tabIndex={-1}>
        <header className="media-dialog-header">
          <div><p className="eyebrow">Import preview</p><h2 id="media-preview-title">Match video and timeline</h2></div>
          <button type="button" className="icon-button" aria-label="Close video preview" title="Close" onClick={onClose}><CloseIcon /></button>
        </header>
        <div className="media-dialog-body">
          <div className="media-preview-picker">
            <label><span>Video</span><select value={selectedID} disabled={loading || videos.length === 0} onChange={(event) => setSelectedID(event.target.value)}>{videos.map((video) => <option key={video.id} value={video.id}>{video.display_name}</option>)}</select></label>
            <button type="button" className="icon-button" aria-label="Refresh video choices" title="Refresh" disabled={loading} onClick={() => void loadVideos()}><RefreshIcon /></button>
          </div>
          {loading && <p className="form-status" role="status">Loading video catalog</p>}
          {error && <p className="form-status media-playback-error" role="alert">{error}</p>}
          {!loading && !error && !selected && <div className="empty-state compact-empty"><h2>No scanned videos</h2><a className="btn btn-secondary" href="#/settings/media" onClick={onClose}>Library locations</a></div>}
          {selected && (
            <MediaVideoPlayer video={selected} allowMetadataWrite={!disabled} onTimeChange={setPlayhead} onDuration={setVideoDuration}>
              <div className="media-preview-timeline">
                <div className="media-preview-readout"><strong>{funscriptName}</strong><span>Playhead {formatTimelineTime(playhead)}</span>{durationDelta !== null && durationDelta > 1000 && <span data-warning>Lengths differ by {formatTimelineTime(durationDelta)}</span>}</div>
                <ImportTimeline points={points} duration={duration} start={trim.start} end={trim.end} viewport={viewport} disabled={disabled} playhead={playhead} onTrimChange={onTrimChange} onViewportChange={onViewportChange} />
              </div>
            </MediaVideoPlayer>
          )}
        </div>
      </section>
    </div>
  );
}
