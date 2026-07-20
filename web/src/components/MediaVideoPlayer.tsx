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

  useEffect(() => {
    setPlaybackError("");
    reported.current = "";
  }, [video.id]);

  async function loadedMetadata(event: SyntheticEvent<HTMLVideoElement>) {
    const durationMillis = Math.round(event.currentTarget.duration * 1000);
    if (!Number.isFinite(durationMillis) || durationMillis <= 0) return;
    onDuration?.(durationMillis);
    const reportKey = `${video.id}:${durationMillis}`;
    if (!allowMetadataWrite || video.duration_ms === durationMillis || reported.current === reportKey) return;
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

  return (
    <div className="media-player" aria-label={`Video player for ${video.display_name}`}>
      <div className="media-video-frame">
        <video
          key={video.id}
          controls
          playsInline
          preload="metadata"
          src={api.mediaStreamURL(video.id)}
          aria-label={video.display_name}
          onLoadedMetadata={(event) => void loadedMetadata(event)}
          onTimeUpdate={(event) => onTimeChange?.(Math.round(event.currentTarget.currentTime * 1000))}
          onSeeking={(event) => onTimeChange?.(Math.round(event.currentTarget.currentTime * 1000))}
          onError={() => setPlaybackError("This video could not be loaded. Verify that the file still exists and uses a browser-supported codec.")}
        />
      </div>
      {playbackError && <p className="form-status media-playback-error" role="alert">{playbackError}</p>}
      {children}
    </div>
  );
}
