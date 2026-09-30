import { t, translateKnown } from "../i18n";
import { useCallback, useLayoutEffect, useMemo, useRef, useState } from "react";
import { api } from "../api/client";
import type { MediaVideo, MediaVideoUpdate } from "../api/types";
import { needsConversion } from "../api/types";
import { SegmentedChoice } from "../components/SetpointControls";
import { SyncedVideoPlayer } from "../components/SyncedVideoPlayer";
import type { VideoPlayerHandle } from "../media/playbackController";
import { useRemoteVideoSurface } from "../remote/RemoteExecutorProvider";
import { ArrowLeftIcon, ChatIcon, PencilIcon } from "../shell/icons";
import { useAppState, useToast } from "../state/app-state";
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
  onVideoUpdate: (video: MediaVideoUpdate) => void;
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
  const { state } = useAppState();
  const [chatOpen, setChatOpen] = useState(readChatPreference);
  const [handle, setHandle] = useState<VideoPlayerHandle | null>(null);
  const [choice, setChoice] = useState<{ videoID: string; source: MotionSource } | null>(null);
  const [switching, setSwitching] = useState(false);
  const transition = useRef(false);
  const generation = useRef(0);
  // Opening a video must not call an existing Autopilot/chat run "Off".
  // Seed from the backend once per video; selecting Script or Off then drains
  // that run through the ordinary mode stop before changing the choice.
  const initialSource = useMemo<MotionSource>(() => {
    const engine = state?.motion?.engine;
    const backgroundMotion = !engine?.target?.media_id && (engine?.running || engine?.starting || engine?.paused);
    const existing = state?.modes?.running || backgroundMotion;
    return existing ? "chat" : defaultMotionSource(video.has_funscript);
  }, [video.id, video.has_funscript]);
  const source = choice?.videoID === video.id ? choice.source : initialSource;
  const latest = useRef({ source, handle, locked, stopSequence, videoID: video.id });
  useLayoutEffect(() => {
    latest.current = { source, handle, locked, stopSequence, videoID: video.id };
  });
  useLayoutEffect(() => {
    generation.current += 1;
    return () => { generation.current += 1; };
  }, [video.id, locked, stopSequence]);

  const showChat = useCallback((open: boolean) => {
    setChatOpen(open);
    try {
      localStorage.setItem(CHAT_OPEN_KEY, String(open));
    } catch {
      // The choice still applies to this page when storage is blocked.
    }
  }, []);
  const openChat = useCallback(() => showChat(true), [showChat]);

  // Keep the old choice until Stop is acknowledged. Starting the new script
  // requires Play; a delayed load can never turn a previous choice into motion.
  const chooseSource = useCallback(async (next: MotionSource, expectedStop = latest.current.stopSequence): Promise<boolean> => {
    const { source: current, handle: player, locked: unavailable, stopSequence: sequence, videoID } = latest.current;
    if (transition.current || unavailable || sequence === undefined || expectedStop !== sequence ||
      (next === "script" && !video.has_funscript) || videoID !== video.id) return false;
    if (next === current) return true;
    if (!player) return false;
    const admitted = generation.current;
    transition.current = true;
    setSwitching(true);
    try {
      if (current === "script") {
        if (!await player.releaseMotion(sequence)) throw new Error(t("Motion could not be stopped."));
      } else {
        player.commands.pause();
        if (current === "chat") await api.stopMode();
      }
      if (generation.current !== admitted || latest.current.handle !== player || latest.current.stopSequence !== sequence) return false;
      setChoice({ videoID, source: next });
      return true;
    } catch (reason) {
      if (generation.current === admitted) show(reason instanceof Error ? translateKnown(reason.message) : t("Motion could not be stopped."), "error");
      return false;
    } finally {
      transition.current = false;
      setSwitching(false);
    }
  }, [show, video.has_funscript, video.id]);

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
                disabled={locked || switching}
                onChange={(next) => void chooseSource(next)}
              />
            </div>
            <button type="button" className="btn btn-secondary compact-command video-chat-toggle" aria-pressed={chatOpen} onClick={() => showChat(!chatOpen)}><ChatIcon size={16} />{t("Chat")}</button>
            {canCurate && (
              <button type="button" className="icon-button media-player-edit" aria-label={t("Edit details")} title={t("Edit details")} onClick={onEditDetails}><PencilIcon size={16} /></button>
            )}
          </div>
        </div>
        {chatOpen && <VideoChatSide source={source} switching={switching} onClose={() => showChat(false)} />}
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
