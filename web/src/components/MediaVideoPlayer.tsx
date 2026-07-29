import { t } from "../i18n";
import { useCallback, useEffect, useRef, useState, type MutableRefObject, type ReactNode, type SyntheticEvent } from "react";
import { api } from "../api/client";
import type { MediaVideo } from "../api/types";

interface Props {
  video: MediaVideo;
  allowMetadataWrite: boolean;
  children?: ReactNode;
  controlsEnabled?: boolean;
  busy?: boolean;
  onDuration?: (durationMillis: number) => void;
  onTimeChange?: (timeMillis: number) => void;
  onVideoUpdate?: (video: MediaVideo) => void;
  playerRef?: MutableRefObject<HTMLVideoElement | null>;
  onPlaybackEvent?: (event: MediaPlaybackEvent, player: HTMLVideoElement) => void;
  synchronized?: boolean;
  /** Offered when the browser refuses to decode this file. */
  onRequestConversion?: () => void;
  conversionBusy?: boolean;
}

export type MediaPlaybackEvent = "play" | "playing" | "pause" | "seeking" | "seeked" | "ended" | "ratechange" | "waiting" | "stalled" | "canplay" | "error";

// MediaError codes that mean "this browser cannot play these bytes" rather than
// "these bytes did not arrive". MEDIA_ERR_SRC_NOT_SUPPORTED is what Firefox
// raises for an .mp4 holding HEVC, which is the case no extension check can
// catch: the container is one every browser opens, and the codec inside is not.
const MEDIA_ERR_DECODE = 3;
const MEDIA_ERR_SRC_NOT_SUPPORTED = 4;

// Covers are taken a little way in rather than at frame zero, because the first
// frame of a video is very often black.
const THUMBNAIL_MIN_CAPTURE_SECONDS = 3;
const THUMBNAIL_MAX_EDGE = 640;
const THUMBNAIL_QUALITY = 0.85;

export function MediaVideoPlayer({
  video,
  allowMetadataWrite,
  children,
  controlsEnabled = true,
  busy = false,
  onDuration,
  onTimeChange,
  onVideoUpdate,
  playerRef,
  onPlaybackEvent,
  synchronized = false,
  onRequestConversion,
  conversionBusy = false,
}: Props) {
  const [playbackError, setPlaybackError] = useState("");
  const [incompatible, setIncompatible] = useState(false);
  const reported = useRef("");
  const compatibilityReported = useRef("");
  const captured = useRef("");
  const internalPlayerRef = useRef<HTMLVideoElement | null>(null);
  const setPlayerRef = useCallback((node: HTMLVideoElement | null) => {
    internalPlayerRef.current = node;
    if (playerRef) playerRef.current = node;
  }, [playerRef]);

  useEffect(() => {
    setPlaybackError("");
    setIncompatible(false);
    reported.current = "";
    compatibilityReported.current = "";
    captured.current = "";
  }, [video.id]);

  // reportCompatibility records what playback actually revealed. It is stored
  // so the verdict survives a reload, and it is reversible on purpose: the
  // answer is specific to this browser, so a later success in one that does
  // have the decoder has to be able to clear it.
  const reportCompatibility = useCallback(async (state: "playable" | "unsupported_codec") => {
    if (!allowMetadataWrite || compatibilityReported.current === state) return;
    if (video.compatibility === state && compatibilityReported.current === "") {
      compatibilityReported.current = state;
      return;
    }
    compatibilityReported.current = state;
    try {
      await api.reportMediaCompatibility(video.id, state);
      onVideoUpdate?.({ ...video, compatibility: state });
    } catch {
      // A read-only tab losing this race costs nothing: the next controller
      // playback reports the same result.
      compatibilityReported.current = "";
    }
  }, [allowMetadataWrite, onVideoUpdate, video]);

  // classifyFailure separates "cannot decode" from "cannot fetch". A missing
  // file also surfaces as MEDIA_ERR_SRC_NOT_SUPPORTED in some browsers, and
  // labelling a deleted video as needing conversion would send the user to fix
  // the wrong thing — so the bytes are checked before the codec is blamed.
  const classifyFailure = useCallback(async (player: HTMLVideoElement) => {
    const code = player.error?.code ?? 0;
    if (code !== MEDIA_ERR_DECODE && code !== MEDIA_ERR_SRC_NOT_SUPPORTED) {
      setPlaybackError(t("This video could not be loaded. Check that the file is still available."));
      return;
    }
    let reachable = false;
    try {
      const probe = await fetch(api.mediaStreamURL(video.id), { headers: { Range: "bytes=0-0" } });
      reachable = probe.ok;
      await probe.body?.cancel();
    } catch {
      reachable = false;
    }
    if (!reachable) {
      setPlaybackError(t("This video could not be loaded. Check that the file is still available."));
      return;
    }
    setIncompatible(true);
    setPlaybackError("");
    void reportCompatibility("unsupported_codec");
  }, [reportCompatibility, video.id]);

  // captureCover draws the frame the browser has already decoded. No new
  // dependency and no decision: the decode happened for playback anyway.
  const captureCover = useCallback(async (player: HTMLVideoElement) => {
    if (!allowMetadataWrite || captured.current === video.id || video.thumbnail_generated_at) return;
    if (!player.videoWidth || !player.videoHeight) return;
    captured.current = video.id;
    try {
      const scale = Math.min(1, THUMBNAIL_MAX_EDGE / Math.max(player.videoWidth, player.videoHeight));
      const canvas = document.createElement("canvas");
      canvas.width = Math.max(1, Math.round(player.videoWidth * scale));
      canvas.height = Math.max(1, Math.round(player.videoHeight * scale));
      const context = canvas.getContext("2d");
      if (!context) return;
      context.imageSmoothingEnabled = true;
      context.imageSmoothingQuality = "high";
      context.drawImage(player, 0, 0, canvas.width, canvas.height);
      const blob = await new Promise<Blob | null>((resolve) => {
        canvas.toBlob(resolve, "image/jpeg", THUMBNAIL_QUALITY);
      });
      if (!blob) return;
      const response = await api.saveMediaThumbnail(video.id, blob);
      if (response.status === "saved") {
        onVideoUpdate?.({ ...video, thumbnail_generated_at: new Date().toISOString() });
      }
    } catch {
      // A cover is a nicety. Failing to store one must never disturb playback,
      // so the attempt is simply not retried for this video.
    }
  }, [allowMetadataWrite, onVideoUpdate, video]);

  async function loadedMetadata(event: SyntheticEvent<HTMLVideoElement>) {
    const durationMillis = Math.round(event.currentTarget.duration * 1000);
    if (!Number.isFinite(durationMillis) || durationMillis <= 0) return;
    onDuration?.(durationMillis);
    const reportKey = `${video.id}:${durationMillis}`;
    const savedDurationMatches = video.duration_ms !== null && Math.abs(video.duration_ms - durationMillis) <= 250;
    if (!allowMetadataWrite || savedDurationMatches || reported.current === reportKey) return;
    reported.current = reportKey;
    try {
      await api.saveMediaDuration(video.id, durationMillis);
      onVideoUpdate?.({ ...video, duration_ms: durationMillis });
    } catch {
      // Playback remains useful when a read-only tab wins a metadata race or
      // the catalog write fails. The next controller playback can retry.
      reported.current = "";
    }
  }

  // maybeCapture takes the frame that is already on screen once playback has
  // moved past the opening seconds.
  //
  // It deliberately never seeks to find a nicer frame. This element is the same
  // one the clock-locked run follows, and a seek would emit seeking/seeked into
  // the sync engine — moving the device to chase a thumbnail. Waiting for real
  // playback costs nothing, because Tier 1 only ever claimed to cover videos
  // the user actually opens; anything unopened is the batch job's to do.
  function maybeCapture(player: HTMLVideoElement): void {
    if (captured.current === video.id || video.thumbnail_generated_at || !allowMetadataWrite) return;
    if (player.currentTime < THUMBNAIL_MIN_CAPTURE_SECONDS) return;
    void captureCover(player);
  }

  function retryPlayback() {
    setPlaybackError("");
    setIncompatible(false);
    compatibilityReported.current = "";
    internalPlayerRef.current?.load();
  }

  return (
    <div className="media-player" data-synchronized={synchronized || undefined} aria-label={t("Video player for {display_name}", { display_name: video.display_name })} aria-busy={busy || undefined}>
      <div className="media-video-frame">
        <video
          ref={setPlayerRef}
          key={video.id}
          controls={controlsEnabled}
          tabIndex={controlsEnabled ? undefined : -1}
          playsInline
          preload={synchronized ? "auto" : "metadata"}
          src={api.mediaStreamURL(video.id)}
          aria-label={video.display_name}
          onLoadedMetadata={(event) => void loadedMetadata(event)}
          onTimeUpdate={(event) => {
            onTimeChange?.(Math.round(event.currentTarget.currentTime * 1000));
            maybeCapture(event.currentTarget);
          }}
          onPlay={(event) => onPlaybackEvent?.("play", event.currentTarget)}
          onPlaying={(event) => onPlaybackEvent?.("playing", event.currentTarget)}
          onPause={(event) => onPlaybackEvent?.("pause", event.currentTarget)}
          onSeeking={(event) => {
            onTimeChange?.(Math.round(event.currentTarget.currentTime * 1000));
            onPlaybackEvent?.("seeking", event.currentTarget);
          }}
          onSeeked={(event) => onPlaybackEvent?.("seeked", event.currentTarget)}
          onEnded={(event) => onPlaybackEvent?.("ended", event.currentTarget)}
          onRateChange={(event) => onPlaybackEvent?.("ratechange", event.currentTarget)}
          onWaiting={(event) => onPlaybackEvent?.("waiting", event.currentTarget)}
          onStalled={(event) => onPlaybackEvent?.("stalled", event.currentTarget)}
          onCanPlay={(event) => {
            setPlaybackError("");
            setIncompatible(false);
            // The browser decoded it. That is the only positive playability
            // evidence the app ever gets, so it is worth recording.
            void reportCompatibility("playable");
            maybeCapture(event.currentTarget);
            onPlaybackEvent?.("canplay", event.currentTarget);
          }}
          onError={(event) => {
            onPlaybackEvent?.("error", event.currentTarget);
            void classifyFailure(event.currentTarget);
          }}
        />
      </div>
      {incompatible && (
        <div className="form-status media-incompatible-notice" role="alert">
          <div className="media-incompatible-copy">
            <strong>{t("This browser cannot play this file")}</strong>
            <span>{video.video_codec
              ? t("This browser refused to decode this file ({codec}). Converting it to H.264 in an MP4 makes it playable without touching the original.", { codec: video.video_codec })
              : t("The file was found and downloaded, but this browser has no decoder for it. Converting it to H.264 in an MP4 makes it playable without touching the original.")}</span>
          </div>
          <div className="media-incompatible-actions">
            {onRequestConversion && (
              <button type="button" className="btn btn-primary compact-command" disabled={conversionBusy} onClick={onRequestConversion}>
                {conversionBusy ? t("Converting") : t("Convert this video")}
              </button>
            )}
            <button type="button" className="btn btn-secondary compact-command" onClick={retryPlayback}>{t("Retry video")}</button>
          </div>
        </div>
      )}
      {playbackError && <div className="form-status media-playback-error media-playback-error-row" role="alert"><span>{playbackError}</span><button type="button" className="btn btn-secondary compact-command" onClick={retryPlayback}>{t("Retry video")}</button></div>}
      {children}
    </div>
  );
}
