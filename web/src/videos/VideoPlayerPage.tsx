import { t, translateKnown } from "../i18n";
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { api } from "../api/client";
import type { MediaVideo } from "../api/types";
import { needsConversion } from "../api/types";
import { SegmentedChoice } from "../components/SetpointControls";
import { SyncedVideoPlayer } from "../components/SyncedVideoPlayer";
import type { VideoPlayerHandle } from "../media/playbackController";
import { useRemoteVideoSurface } from "../remote/RemoteExecutorProvider";
import { ArrowLeftIcon, ChatIcon, PencilIcon } from "../shell/icons";
import { useToast } from "../state/app-state";
import { videoTitle } from "./curation";
import { defaultMotionSource, motionSourceNote, motionSourceOptions, type MotionSource } from "./motionSource";
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

// The watch page, laid out like a livestream: the picture first and as large
// as the window allows, one compact bar under it, the chat as a column beside
// it, and everything else folded below (ADR 0032).
export function VideoPlayerPage({
  video, locked, stopSequence, hostLocked, hostAdministration, canCurate, toolsAvailable, conversionBusy,
  onBack, onVideoUpdate, onRequestConversion, onEditDetails,
}: Props) {
  const title = videoTitle(video);
  const { show } = useToast();
  const [chatOpen, setChatOpen] = useState(readChatPreference);
  const [handle, setHandle] = useState<VideoPlayerHandle | null>(null);
  const [choice, setChoice] = useState<{ videoID: string; source: MotionSource } | null>(null);
  const source = choice?.videoID === video.id ? choice.source : defaultMotionSource(video.has_funscript);
  const latest = useRef({ source, handle });
  useLayoutEffect(() => {
    latest.current = { source, handle };
  });
  // The video to arm once its script loads, after a switch to Script mid-play.
  const resumeWithScript = useRef("");

  const showChat = useCallback((open: boolean) => {
    setChatOpen(open);
    try {
      localStorage.setItem(CHAT_OPEN_KEY, String(open));
    } catch {
      // The choice still applies to this page when storage is blocked.
    }
  }, []);
  const openChat = useCallback(() => showChat(true), [showChat]);

  // Switching is a Stop and a fresh start. Leaving the script closes its run
  // with its controller; leaving the chat stops Autopilot and chat motion.
  // Arriving at the script mid-play holds the picture and arms a new run
  // from there once the script is ready.
  const chooseSource = useCallback((next: MotionSource): boolean => {
    const { source: current, handle: player } = latest.current;
    if (next === current) return true;
    if (locked || (next === "script" && !video.has_funscript)) return false;
    if (next === "script" && player?.getSnapshot().playbackIntent) {
      player.commands.pause();
      resumeWithScript.current = video.id;
    }
    setChoice({ videoID: video.id, source: next });
    if (current === "chat") {
      void api.stopMode().catch((reason: unknown) => {
        show(reason instanceof Error ? translateKnown(reason.message) : t("Motion could not be stopped."), "error");
      });
    }
    return true;
  }, [locked, show, video.has_funscript, video.id]);

  useEffect(() => {
    if (!handle?.synchronized) return undefined;
    const resume = () => {
      if (resumeWithScript.current !== handle.videoID) return;
      const snapshot = handle.getSnapshot();
      if (snapshot.scriptLoading) return;
      resumeWithScript.current = "";
      if (snapshot.script) handle.commands.play();
    };
    resume();
    return handle.subscribe(resume);
  }, [handle]);

  // The phone remote drives this player through the same commands as its controls.
  const remoteSurface = useMemo(() => handle ? {
    handle, title, openChat, motionSource: source, hasScript: video.has_funscript, setMotionSource: chooseSource,
  } : null, [handle, title, openChat, source, video.has_funscript, chooseSource]);
  useRemoteVideoSurface(remoteSurface);

  return (
    <section className="library-view video-player-view" aria-label={t("Video playback")}>
      {/* With the chat open, fullscreen takes the whole layout so the chat stays beside the picture. */}
      <div className="video-watch" data-chat-open={chatOpen || undefined} data-fullscreen-root={chatOpen || undefined}>
        <div className="video-stage">
          <SyncedVideoPlayer
            video={video}
            synchronized={source === "script"}
            locked={locked}
            stopSequence={stopSequence}
            onVideoUpdate={onVideoUpdate}
            conversionBusy={conversionBusy}
            onRequestConversion={hostLocked || !toolsAvailable ? undefined : onRequestConversion}
            onHandleChange={setHandle}
          />
          <div className="video-watch-bar">
            <button type="button" className="icon-button" aria-label={t("Back to videos")} title={t("Back to videos")} onClick={onBack}><ArrowLeftIcon /></button>
            <h2 className="video-watch-title" title={title}>{title}</h2>
            <div className="video-source" title={motionSourceNote(source)}>
              <span className="video-source-label" aria-hidden="true">{t("Motion")}</span>
              <SegmentedChoice
                className="video-source-choice"
                label={t("Motion source")}
                value={source}
                options={motionSourceOptions(video.has_funscript)}
                disabled={locked}
                onChange={(next) => void chooseSource(next)}
              />
            </div>
            <button type="button" className="btn btn-secondary compact-command" aria-pressed={chatOpen} onClick={() => showChat(!chatOpen)}><ChatIcon size={16} />{t("Chat")}</button>
            {canCurate && (
              <button type="button" className="icon-button media-player-edit" aria-label={t("Edit details")} title={t("Edit details")} onClick={onEditDetails}><PencilIcon size={16} /></button>
            )}
          </div>
        </div>
        {chatOpen && <VideoChatSide source={source} onClose={() => showChat(false)} />}
        <div className="video-watch-below">
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
