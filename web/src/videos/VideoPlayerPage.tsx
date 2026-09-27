import { t } from "../i18n";
import type { MediaVideo } from "../api/types";
import { needsConversion } from "../api/types";
import { SyncedVideoPlayer } from "../components/SyncedVideoPlayer";
import type { VideoPlayerHandle } from "../media/playbackController";
import { ArrowLeftIcon } from "../shell/icons";
import { formatFileSize, formatLocation } from "./format";

interface Props {
  video: MediaVideo;
  locked: boolean;
  stopSequence?: number;
  hostLocked: boolean;
  hostAdministration: boolean;
  toolsAvailable: boolean;
  conversionBusy: boolean;
  onBack: () => void;
  onVideoUpdate: (video: MediaVideo) => void;
  onRequestConversion: () => void;
  onHandleChange?: (handle: VideoPlayerHandle | null) => void;
}

export function VideoPlayerPage({
  video, locked, stopSequence, hostLocked, hostAdministration, toolsAvailable, conversionBusy,
  onBack, onVideoUpdate, onRequestConversion, onHandleChange,
}: Props) {
  const details = video.has_funscript
    ? t("{size} / {location} / script found", { size: formatFileSize(video.size_bytes), location: formatLocation(video.location_path) })
    : t("{size} / {location}", { size: formatFileSize(video.size_bytes), location: formatLocation(video.location_path) });
  return (
    <section className="library-view video-player-view" aria-label={t("Video playback")}>
      <div className="media-player-heading">
        <button type="button" className="btn btn-secondary compact-command" onClick={onBack}><ArrowLeftIcon />{t("Videos")}</button>
        <div><h2>{video.display_name}</h2><span>{details}</span></div>
      </div>
      <SyncedVideoPlayer
        video={video}
        locked={locked}
        stopSequence={stopSequence}
        onVideoUpdate={onVideoUpdate}
        conversionBusy={conversionBusy}
        onRequestConversion={hostLocked || !toolsAvailable ? undefined : onRequestConversion}
        onHandleChange={onHandleChange}
      />
      {!hostAdministration && needsConversion(video) && <p className="form-status media-playback-error" role="alert">{t("Host settings and diagnostics are managed by an administrator.")}</p>}
      {hostAdministration && !toolsAvailable && needsConversion(video) && (
        <p className="form-status media-playback-error" role="alert">
          {t("Set an FFmpeg location in Settings > Media to enable conversion.")}
          {" "}
          <a href="#/settings/media">{t("Set up FFmpeg")}</a>
        </p>
      )}
    </section>
  );
}
