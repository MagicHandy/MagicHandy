import { t } from "../i18n";
import { useState } from "react";
import type { MediaVideo } from "../api/types";
import { needsConversion } from "../api/types";
import { SyncedVideoPlayer } from "../components/SyncedVideoPlayer";
import type { VideoPlayerHandle } from "../media/playbackController";
import { ArrowLeftIcon, ChatIcon, PencilIcon } from "../shell/icons";
import { RatingStars, TagChips } from "./CurationControls";
import { videoTitle } from "./curation";
import { formatFileSize, formatLocation } from "./format";
import { VideoChatSide } from "./VideoChatSide";

const CHAT_OPEN_KEY = "magichandy-video-chat-open";

interface Props {
  video: MediaVideo;
  locked: boolean;
  stopSequence?: number;
  hostLocked: boolean;
  hostAdministration: boolean;
  canCurate: boolean;
  toolsAvailable: boolean;
  conversionBusy: boolean;
  onBack: () => void;
  onVideoUpdate: (video: MediaVideo) => void;
  onRequestConversion: () => void;
  onEditDetails: () => void;
  onHandleChange?: (handle: VideoPlayerHandle | null) => void;
}

export function VideoPlayerPage({
  video, locked, stopSequence, hostLocked, hostAdministration, canCurate, toolsAvailable, conversionBusy,
  onBack, onVideoUpdate, onRequestConversion, onEditDetails, onHandleChange,
}: Props) {
  const title = videoTitle(video);
  const [chatOpen, setChatOpen] = useState(readChatPreference);

  function showChat(open: boolean) {
    setChatOpen(open);
    try {
      localStorage.setItem(CHAT_OPEN_KEY, String(open));
    } catch {
      // The choice still applies to this page when storage is blocked.
    }
  }

  const details = video.has_funscript
    ? t("{size} / {location} / script found", { size: formatFileSize(video.size_bytes), location: formatLocation(video.location_path) })
    : t("{size} / {location}", { size: formatFileSize(video.size_bytes), location: formatLocation(video.location_path) });
  return (
    <section className="library-view video-player-view" aria-label={t("Video playback")}>
      <div className="media-player-heading">
        <button type="button" className="btn btn-secondary compact-command" onClick={onBack}><ArrowLeftIcon />{t("Videos")}</button>
        <div className="media-player-title">
          <h2>{title}</h2>
          {title !== video.display_name && <span className="media-player-file">{video.display_name}</span>}
          <span>{details}</span>
          {(video.rating || (video.tags?.length ?? 0) > 0) && (
            <span className="media-player-curation"><RatingStars rating={video.rating} /><TagChips tags={video.tags} limit={8} /></span>
          )}
          {video.notes && <p className="media-player-notes">{video.notes}</p>}
        </div>
        <div className="media-player-actions">
          {canCurate && (
            <button type="button" className="btn btn-secondary compact-command media-player-edit" onClick={onEditDetails}><PencilIcon size={16} />{t("Edit details")}</button>
          )}
          <button type="button" className="btn btn-secondary compact-command" aria-pressed={chatOpen} onClick={() => showChat(!chatOpen)}><ChatIcon size={16} />{t("Chat")}</button>
        </div>
      </div>
      {/* With the chat open, fullscreen takes the whole layout so the chat stays beside the picture. */}
      <div className="video-watch" data-chat-open={chatOpen || undefined} data-fullscreen-root={chatOpen || undefined}>
        <div className="video-watch-main">
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
        </div>
        {chatOpen && <VideoChatSide synchronized={video.has_funscript} onClose={() => showChat(false)} />}
      </div>
    </section>
  );
}

function readChatPreference(): boolean {
  try {
    return localStorage.getItem(CHAT_OPEN_KEY) === "true";
  } catch {
    return false;
  }
}
