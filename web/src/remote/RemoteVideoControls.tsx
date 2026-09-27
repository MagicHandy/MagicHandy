import { t, translateKnown } from "../i18n";
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import type { RemoteVideoPresence } from "../api/remote-types";
import type { MediaSyncStatus } from "../api/types";
import { syncStatusLabel } from "../components/SyncedVideoPlayer";
import { ArrowLeftIcon, ArrowRightIcon, PauseIcon, PlayIcon, VolumeIcon, VolumeMutedIcon } from "../shell/icons";
import { formatClock } from "../videos/format";
import type { RemoteSend } from "./useRemoteCommands";

// The same speeds as the desktop player's own control.
const RATES = [0.25, 0.5, 0.75, 1, 1.25, 1.5, 2];
const SKIP_MS = 10_000;
// Dragging sends one seek when the thumb rests, not one per pixel.
const SETTLE_MS = 300;
// A dragged value stays on screen until the desktop reports near it.
const SETTLED_WITHIN_MS = 1_500;
const DRAFT_TIMEOUT_MS = 3_000;

interface Props {
  video: RemoteVideoPresence;
  /** Where the video is now, placed from the last report. */
  position: number;
  send: RemoteSend;
}

// Transport for the desktop's open video. Every control sends a command; the
// desktop's own player decides, so a paired script still arms and stops there.
export function RemoteVideoControls({ video, position, send }: Props) {
  const seek = useSettledValue(position, SETTLED_WITHIN_MS, (value) => void send({ target: "video", action: "seek", ms: Math.round(value) }));
  const volume = useSettledValue(Math.round(video.volume * 100), 1, (value) => void send({ target: "video", action: "volume", value: value / 100 }));
  const duration = Math.max(video.duration_ms, 0);
  const shownPosition = seek.draft ?? position;
  const syncLabel = video.synchronized
    ? translateKnown(syncStatusLabel({ active: false, state: (video.sync_state ?? "idle") as MediaSyncStatus["state"] }, false, null))
    : t("Video only; the device is not following a script");

  return (
    <section className="remote-video" aria-label={t("Desktop video")}>
      <div className="remote-now-playing">
        <h2>{video.title}</h2>
        <p className="remote-video-state">
          {!video.ready ? t("Loading the paired script…") : syncLabel}
        </p>
      </div>

      <div className="remote-scrubber">
        <label className="visually-hidden" htmlFor="remote-position">{t("Position")}</label>
        <input
          id="remote-position"
          type="range"
          min={0}
          max={Math.max(duration, 1)}
          step={1000}
          value={Math.min(shownPosition, Math.max(duration, 1))}
          disabled={!video.ready || duration <= 0}
          aria-valuetext={t("{position} of {duration}", { position: formatClock(shownPosition), duration: formatClock(duration) })}
          onChange={(event) => seek.change(Number(event.target.value))}
        />
        <div className="remote-clock" aria-hidden="true">
          <span>{formatClock(shownPosition)}</span>
          <span>{formatClock(duration)}</span>
        </div>
      </div>

      <div className="remote-transport">
        <button
          type="button"
          className="btn btn-secondary remote-skip"
          disabled={!video.ready}
          aria-label={t("Back 10 s")}
          title={t("Back 10 s")}
          onClick={() => void send({ target: "video", action: "seek_by", ms: -SKIP_MS })}
        >
          <ArrowLeftIcon size={16} />{t("{seconds} s", { seconds: SKIP_MS / 1000 })}
        </button>
        <button
          type="button"
          className="btn btn-primary remote-play"
          disabled={!video.ready}
          aria-label={video.playing ? t("Pause") : t("Play")}
          onClick={() => void send({ target: "video", action: video.playing ? "pause" : "play" })}
        >
          {video.playing ? <PauseIcon size={28} /> : <PlayIcon size={28} />}
        </button>
        <button
          type="button"
          className="btn btn-secondary remote-skip"
          disabled={!video.ready}
          aria-label={t("Forward 10 s")}
          title={t("Forward 10 s")}
          onClick={() => void send({ target: "video", action: "seek_by", ms: SKIP_MS })}
        >
          {t("{seconds} s", { seconds: SKIP_MS / 1000 })}<ArrowRightIcon size={16} />
        </button>
      </div>

      <div className="remote-audio">
        <button
          type="button"
          className="icon-button remote-mute"
          aria-pressed={video.muted}
          aria-label={video.muted ? t("Unmute video") : t("Mute video")}
          title={video.muted ? t("Unmute video") : t("Mute video")}
          onClick={() => void send({ target: "video", action: "mute", flag: !video.muted })}
        >
          {video.muted ? <VolumeMutedIcon /> : <VolumeIcon />}
        </button>
        <label className="visually-hidden" htmlFor="remote-volume">{t("Video volume")}</label>
        <input
          id="remote-volume"
          type="range"
          min={0}
          max={100}
          step={5}
          value={volume.draft ?? Math.round(video.volume * 100)}
          aria-valuetext={t("{n}%", { n: volume.draft ?? Math.round(video.volume * 100) })}
          onChange={(event) => volume.change(Number(event.target.value))}
        />
        <label className="remote-rate">
          <span>{t("Speed")}</span>
          <select value={String(video.rate)} aria-label={t("Video playback speed")} disabled={!video.ready} onChange={(event) => void send({ target: "video", action: "rate", value: Number(event.target.value) })}>
            {!RATES.includes(video.rate) && <option value={String(video.rate)}>{t("{rate}x", { rate: video.rate })}</option>}
            {RATES.map((rate) => <option key={rate} value={String(rate)}>{t("{rate}x", { rate })}</option>)}
          </select>
        </label>
      </div>

      <button type="button" className="btn btn-secondary remote-close" onClick={() => void send({ target: "video", action: "close" })}>
        {t("Close video")}
      </button>
    </section>
  );
}

// A draft value that commits once it stops changing, then yields to the
// desktop's report when one arrives near it, or after a timeout if none does.
function useSettledValue(reported: number, tolerance: number, commit: (value: number) => void) {
  const [draft, setDraft] = useState<number | null>(null);
  const settling = useRef(false);
  const settle = useRef<ReturnType<typeof setTimeout>>(undefined);
  const release = useRef<ReturnType<typeof setTimeout>>(undefined);
  const latestCommit = useRef(commit);
  useLayoutEffect(() => {
    latestCommit.current = commit;
  });
  useEffect(() => () => {
    clearTimeout(settle.current);
    clearTimeout(release.current);
  }, []);
  useEffect(() => {
    if (draft !== null && !settling.current && Math.abs(reported - draft) <= tolerance) setDraft(null);
  }, [draft, reported, tolerance]);

  const change = useCallback((value: number) => {
    setDraft(value);
    settling.current = true;
    clearTimeout(settle.current);
    clearTimeout(release.current);
    settle.current = setTimeout(() => {
      settling.current = false;
      latestCommit.current(value);
      release.current = setTimeout(() => setDraft(null), DRAFT_TIMEOUT_MS);
    }, SETTLE_MS);
  }, []);
  return { draft, change };
}
