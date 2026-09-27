import { t } from "../i18n";
import { useCallback, useMemo, useState } from "react";
import type { MediaVideo } from "../api/types";
import { needsConversion } from "../api/types";
import { SyncedVideoPlayer } from "../components/SyncedVideoPlayer";
import type { VideoPlayerHandle } from "../media/playbackController";
import { useRemoteVideoSurface } from "../remote/RemoteExecutorProvider";
import { ArrowLeftIcon, ChatIcon, PencilIcon } from "../shell/icons";
import { videoTitle } from "./curation";
import { VideoChatSide } from "./VideoChatSide";
import { VideoDescription } from "./VideoDescription";

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
}

export function VideoPlayerPage({
  video, locked, stopSequence, hostLocked, hostAdministration, canCurate, toolsAvailable, conversionBusy,
  onBack, onVideoUpdate, onRequestConversion, onEditDetails,
}: Props) {
  const title = videoTitle(video);
  const [chatOpen, setChatOpen] = useState(readChatPreference);
  const [handle, setHandle] = useState<VideoPlayerHandle | null>(null);

  const showChat = useCallback((open: boolean) => {
    setChatOpen(open);
    try {
      localStorage.setItem(CHAT_OPEN_KEY, String(open));
    } catch {
      // The choice still applies to this page when storage is blocked.
    }
  }, []);
  const openChat = useCallback(() => showChat(true), [showChat]);
  // The phone remote drives this player through the same commands as its controls.
  const remoteSurface = useMemo(() => handle ? { handle, title, openChat } : null, [handle, title, openChat]);
  useRemoteVideoSurface(remoteSurface);

  return (
    <section className="library-view video-player-view" aria-label={t("Video playback")}>
      <div className="media-player-heading">
        <button type="button" className="btn btn-secondary compact-command" onClick={onBack}><ArrowLeftIcon />{t("Videos")}</button>
        <div className="media-player-title">
          <h2>{title}</h2>
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
            onHandleChange={setHandle}
          />
          {!hostAdministration && needsConversion(video) && <p className="form-status media-playback-error" role="alert">{t("Host settings and diagnostics are managed by an administrator.")}</p>}
          {hostAdministration && !toolsAvailable && needsConversion(video) && (
            <p className="form-status media-playback-error" role="alert">
              {t("Set an FFmpeg location in Settings > Media to enable conversion.")}
              {" "}
              <a href="#/settings/media">{t("Set up FFmpeg")}</a>
            </p>
          )}
          <VideoDescription key={video.id} video={video} title={title} />
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
