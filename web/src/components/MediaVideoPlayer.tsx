import { useEffect, useRef, useState, type ReactNode, type SyntheticEvent } from "react";
import { api } from "../api/client";
import type { MediaVideo } from "../api/types";

interface Props {
  video: MediaVideo;
  allowMetadataWrite: boolean;
  children?: ReactNode;
  onDuration?: (durationMillis: number) => void;
  onTimeChange?: (timeMillis: number) => void;
  onVideoUpdate?: (video: MediaVideo) => void;
}

export function MediaVideoPlayer({
  video,
  allowMetadataWrite,
  children,
  onDuration,
  onTimeChange,
  onVideoUpdate,
}: Props) {
  const [playbackError, setPlaybackError] = useState("");
  const reported = useRef("");
  const playerRef = useRef<HTMLVideoElement>(null);

  useEffect(() => {
    setPlaybackError("");
    reported.current = "";
  }, [video.id]);

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

  function retryPlayback() {
    setPlaybackError("");
    playerRef.current?.load();
  }

  return (
    <div className="media-player" aria-label={`Video player for ${video.display_name}`}>
      <div className="media-video-frame">
        <video
          ref={playerRef}
          key={video.id}
          controls
          playsInline
          preload="metadata"
          src={api.mediaStreamURL(video.id)}
          aria-label={video.display_name}
          onLoadedMetadata={(event) => void loadedMetadata(event)}
          onTimeUpdate={(event) => onTimeChange?.(Math.round(event.currentTarget.currentTime * 1000))}
          onSeeking={(event) => onTimeChange?.(Math.round(event.currentTarget.currentTime * 1000))}
          onCanPlay={() => setPlaybackError("")}
          onError={() => setPlaybackError("This video could not be loaded. Verify that the file still exists and uses a browser-supported codec.")}
        />
      </div>
      {playbackError && <div className="form-status media-playback-error media-playback-error-row" role="alert"><span>{playbackError}</span><button type="button" className="btn btn-secondary compact-command" onClick={retryPlayback}>Retry video</button></div>}
      {children}
    </div>
  );
}
