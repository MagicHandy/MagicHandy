import {
  useEffect,
  useRef,
  useState,
  type FocusEvent as ReactFocusEvent,
  type PointerEvent as ReactPointerEvent,
} from "react";
import { t } from "../i18n";
import {
  FullscreenIcon,
  PauseIcon,
  PlayIcon,
  VolumeIcon,
  VolumeMutedIcon,
} from "../shell/icons";
import { formatTimelineTime } from "./ImportTimeline";

interface Props {
  autoHide: boolean;
  currentTimeMillis: number;
  durationMillis: number;
  muted: boolean;
  playbackIntent: boolean;
  playbackRate: number;
  volume: number;
  onFullscreen: () => void;
  onMuteChange: (muted: boolean) => void;
  onPlaybackRateChange: (rate: number) => void;
  onSeekCancel: () => void;
  onSeekCommit: (milliseconds: number) => void;
  onSeekStart: () => void;
  onTogglePlayback: () => void;
  onVolumeChange: (volume: number) => void;
}

const AUTO_HIDE_DELAY_MS = 2_000;

export function SynchronizedVideoControls({
  autoHide,
  currentTimeMillis,
  durationMillis,
  muted,
  playbackIntent,
  playbackRate,
  volume,
  onFullscreen,
  onMuteChange,
  onPlaybackRateChange,
  onSeekCancel,
  onSeekCommit,
  onSeekStart,
  onTogglePlayback,
  onVolumeChange,
}: Props) {
  const dragging = useRef(false);
  const keyboardSeeking = useRef(false);
  const focusWithin = useRef(false);
  const hideTimer = useRef<number | null>(null);
  const previewTimeRef = useRef<number | null>(null);
  const [controlsVisible, setControlsVisible] = useState(true);
  const [previewTime, setPreviewTime] = useState<number | null>(null);
  const duration = Math.max(1, Math.round(durationMillis));
  const displayedTime = Math.max(0, Math.min(duration, previewTime ?? currentTimeMillis));

  useEffect(() => {
    if (!dragging.current && !keyboardSeeking.current) setPreviewTime(null);
  }, [currentTimeMillis]);

  useEffect(() => {
    clearHideTimer();
    setControlsVisible(true);
    if (autoHide) scheduleHide();
    return clearHideTimer;
  }, [autoHide]);

  function clearHideTimer() {
    if (hideTimer.current === null) return;
    window.clearTimeout(hideTimer.current);
    hideTimer.current = null;
  }

  function scheduleHide() {
    clearHideTimer();
    if (!autoHide || focusWithin.current || dragging.current || keyboardSeeking.current) return;
    hideTimer.current = window.setTimeout(() => {
      hideTimer.current = null;
      setControlsVisible(false);
    }, AUTO_HIDE_DELAY_MS);
  }

  function revealControls() {
    setControlsVisible(true);
    scheduleHide();
  }

  function handleFocus(event: ReactFocusEvent<HTMLDivElement>) {
    focusWithin.current = true;
    clearHideTimer();
    setControlsVisible(true);
    if (event.currentTarget.contains(event.relatedTarget as Node | null)) return;
  }

  function handleBlur(event: ReactFocusEvent<HTMLDivElement>) {
    if (event.currentTarget.contains(event.relatedTarget as Node | null)) return;
    focusWithin.current = false;
    scheduleHide();
  }

  function setSeekPreview(milliseconds: number | null) {
    previewTimeRef.current = milliseconds;
    setPreviewTime(milliseconds);
  }

  function beginPointerSeek(event: ReactPointerEvent<HTMLInputElement>) {
    if (dragging.current) return;
    dragging.current = true;
    setSeekPreview(currentTimeMillis);
    event.currentTarget.setPointerCapture?.(event.pointerId);
    onSeekStart();
  }

  function finishPointerSeek(event: ReactPointerEvent<HTMLInputElement>) {
    if (!dragging.current) return;
    dragging.current = false;
    const target = previewTimeRef.current ?? currentTimeMillis;
    setSeekPreview(null);
    if (event.currentTarget.hasPointerCapture?.(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
    onSeekCommit(target);
    revealControls();
  }

  function cancelPointerSeek(event?: ReactPointerEvent<HTMLInputElement>) {
    if (!dragging.current) return;
    dragging.current = false;
    setSeekPreview(null);
    if (event?.currentTarget.hasPointerCapture?.(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
    onSeekCancel();
    revealControls();
  }

  function beginKeyboardSeek() {
    if (keyboardSeeking.current) return;
    keyboardSeeking.current = true;
    setSeekPreview(currentTimeMillis);
    onSeekStart();
  }

  function finishKeyboardSeek() {
    if (!keyboardSeeking.current) return;
    keyboardSeeking.current = false;
    const target = previewTimeRef.current ?? currentTimeMillis;
    setSeekPreview(null);
    onSeekCommit(target);
    revealControls();
  }

  return (
    <div
      className="media-transport-overlay"
      data-visible={controlsVisible}
      data-auto-hide={autoHide || undefined}
      onPointerDown={revealControls}
      onPointerMove={revealControls}
      onPointerLeave={scheduleHide}
      onFocusCapture={handleFocus}
      onBlurCapture={handleBlur}
    >
      <div className="media-transport-controls" role="group" aria-label={t("Synchronized video controls")}>
        <button
          type="button"
          className="icon-button media-transport-play"
          title={playbackIntent ? t("Pause video and motion") : t("Play video with paired motion")}
          aria-label={playbackIntent ? t("Pause video and motion") : t("Play video with paired motion")}
          onClick={onTogglePlayback}
        >
          {playbackIntent ? <PauseIcon /> : <PlayIcon />}
        </button>
        <span className="media-transport-time" aria-hidden="true">{formatTimelineTime(displayedTime)}</span>
        <input
          type="range"
          className="media-transport-seek"
          aria-label={t("Video position")}
          aria-valuetext={t("{current} of {duration}", {
            current: formatTimelineTime(displayedTime),
            duration: formatTimelineTime(duration),
          })}
          min={0}
          max={duration}
          step={50}
          value={displayedTime}
          onPointerDown={beginPointerSeek}
          onPointerUp={finishPointerSeek}
          onPointerCancel={cancelPointerSeek}
          onLostPointerCapture={cancelPointerSeek}
          onChange={(event) => {
            if (!dragging.current && !keyboardSeeking.current) beginKeyboardSeek();
            setSeekPreview(Number(event.target.value));
          }}
          onKeyUp={(event) => {
            if (["ArrowLeft", "ArrowRight", "ArrowDown", "ArrowUp", "Home", "End", "PageDown", "PageUp"].includes(event.key)) {
              finishKeyboardSeek();
            }
          }}
          onBlur={() => {
            if (keyboardSeeking.current) finishKeyboardSeek();
            else if (dragging.current) cancelPointerSeek();
          }}
        />
        <span className="media-transport-time" aria-hidden="true">{formatTimelineTime(duration)}</span>
        <button
          type="button"
          className="icon-button media-transport-mute"
          title={muted ? t("Unmute video") : t("Mute video")}
          aria-label={muted ? t("Unmute video") : t("Mute video")}
          onClick={() => onMuteChange(!muted)}
        >
          {muted || volume === 0 ? <VolumeMutedIcon /> : <VolumeIcon />}
        </button>
        <input
          type="range"
          className="media-transport-volume"
          aria-label={t("Video volume")}
          min={0}
          max={1}
          step={0.05}
          value={volume}
          onChange={(event) => onVolumeChange(Number(event.target.value))}
        />
        <select
          className="media-transport-rate"
          aria-label={t("Video playback speed")}
          value={playbackRate}
          onChange={(event) => onPlaybackRateChange(Number(event.target.value))}
        >
          {[0.25, 0.5, 0.75, 1, 1.25, 1.5, 2].map((rate) => (
            <option key={rate} value={rate}>{t("{rate}x", { rate })}</option>
          ))}
        </select>
        <button
          type="button"
          className="icon-button media-transport-fullscreen"
          title={t("Enter fullscreen")}
          aria-label={t("Enter fullscreen")}
          onClick={onFullscreen}
        >
          <FullscreenIcon />
        </button>
      </div>
    </div>
  );
}
