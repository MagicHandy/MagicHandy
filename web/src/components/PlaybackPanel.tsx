import { t } from "../i18n";
// Playback panel for the video currently open in the player. Its whole purpose
// is to be adjusted while watching: calibration you cannot see the effect of is
// just a settings form in a worse place. So wider screens dock it as a rail
// beside the picture (or over the chat column) and never over the picture, its
// transport, the script plot or the motion-source switch; phones float it as a
// sheet above the Stop footer (media.css).
import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { MediaPlaybackSettings, MediaSyncStatus, MediaVideo, MediaVideoUpdate } from "../api/types";
import { CloseIcon } from "../shell/icons";
import { PlaybackFilterEffect } from "./PlaybackFilterEffect";
import { RangeInput } from "./RangeInput";

// Mirror config.MaxScriptOffsetMillis / MaxScriptSmoothingPercent / MaxPeakRoundingMillis.
const MAX_OFFSET_MILLIS = 2000;
const MAX_SMOOTHING_PERCENT = 5;
const MAX_ROUNDING_MILLIS = 200;
const WRITE_DEBOUNCE_MILLIS = 180;

export interface MediaPlaybackPatch {
  script_smoothing_percent?: number;
  peak_rounding_ms?: number;
  apply_video_speed_limit?: boolean;
}

interface Props {
  /** The id its trigger names in aria-controls. */
  id?: string;
  video: MediaVideo;
  sync: MediaSyncStatus;
  locked: boolean;
  setupOffsetMillis: number;
  smoothingPercent: number;
  roundingMillis: number;
  limitSpeed: boolean;
  speedLimitPercent: number;
  onClose: () => void;
  onVideoUpdate?: (video: MediaVideoUpdate) => void;
  onFiltersChanging?: () => void;
  onFiltersChanged?: (patch: MediaPlaybackPatch) => Promise<MediaPlaybackSettings>;
}

export function PlaybackPanel({
  id,
  video,
  sync,
  locked,
  setupOffsetMillis,
  smoothingPercent,
  roundingMillis,
  limitSpeed,
  speedLimitPercent,
  onClose,
  onVideoUpdate,
  onFiltersChanging,
  onFiltersChanged,
}: Props) {
  const [offset, setOffset] = useState(video.script_offset_ms ?? 0);
  const [smoothing, setSmoothing] = useState(smoothingPercent);
  const [rounding, setRounding] = useState(roundingMillis);
  const [speedLimit, setSpeedLimit] = useState(limitSpeed);
  const [error, setError] = useState("");
  const layerRef = useRef<HTMLDivElement | null>(null);
  const panelRef = useRef<HTMLDivElement | null>(null);
  const offsetTimer = useRef<number>();
  const filterTimer = useRef<number>();
  const pendingFilters = useRef<MediaPlaybackPatch>({});
  const mounted = useRef(true);
  const filterRevision = useRef(0);
  const filterRequests = useRef<Promise<unknown>>(Promise.resolve());
  const filtersPending = useRef(false);
  const [savingFilters, setSavingFilters] = useState(false);
  const backendFilters = useRef({ smoothingPercent, roundingMillis, limitSpeed });
  backendFilters.current = { smoothingPercent, roundingMillis, limitSpeed };

  useEffect(() => {
    if (filtersPending.current) return;
    setSmoothing(smoothingPercent);
    setRounding(roundingMillis);
    setSpeedLimit(limitSpeed);
  }, [smoothingPercent, roundingMillis, limitSpeed]);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      // Debounced writes intentionally survive the panel closing. Dismissing a
      // floating panel immediately after a drag must not discard the value.
    };
  }, []);

  useEffect(() => {
    panelRef.current?.querySelector<HTMLElement>("input:not(:disabled), button:not(:disabled)")?.focus();
  }, []);

  useEffect(() => {
    // Escape is also Emergency Stop's shortcut; closing never consumes it.
    function onKey(event: KeyboardEvent) {
      if (event.key === "Escape") onClose();
    }
    // Only the floating sheet closes on a press elsewhere. The docked rail stays
    // open while the picture, its transport and the script plot are used, and
    // the trigger toggles it either way.
    function onPointer(event: MouseEvent) {
      const target = event.target as Element | null;
      if (panelRef.current?.contains(target) || (id && target?.closest?.(`[aria-controls="${id}"]`))) return;
      if (layerRef.current && getComputedStyle(layerRef.current).position !== "fixed") return;
      onClose();
    }
    window.addEventListener("keydown", onKey);
    // Deferred so the click that opened the panel does not immediately close it.
    const timer = window.setTimeout(() => window.addEventListener("mousedown", onPointer), 0);
    return () => {
      window.clearTimeout(timer);
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("mousedown", onPointer);
    };
  }, [id, onClose]);

  // The offset writes on a short debounce: dragging is one gesture, not thirty
  // requests, and the backend applies each one to the live run without stopping.
  const writeOffset = useCallback((millis: number) => {
    window.clearTimeout(offsetTimer.current);
    offsetTimer.current = window.setTimeout(() => {
      void api.saveMediaScriptOffset(video.id, millis)
        .then(() => {
          if (!mounted.current) return;
          setError("");
          onVideoUpdate?.({ id: video.id, script_offset_ms: millis });
        })
        .catch((reason: unknown) => {
          if (mounted.current) setError(reason instanceof Error ? reason.message : "Offset could not be saved.");
        });
    }, WRITE_DEBOUNCE_MILLIS);
  }, [onVideoUpdate, video]);

  const writeFilters = useCallback((patch: MediaPlaybackPatch) => {
    const revision = ++filterRevision.current;
    filtersPending.current = true;
    setSavingFilters(true);
    onFiltersChanging?.();
    pendingFilters.current = { ...pendingFilters.current, ...patch };
    window.clearTimeout(filterTimer.current);
    filterTimer.current = window.setTimeout(() => {
      const next = pendingFilters.current;
      pendingFilters.current = {};
      const write = filterRequests.current.then(() => onFiltersChanged ? onFiltersChanged(next) : api.saveMediaPlayback(next));
      filterRequests.current = write.catch(() => undefined);
      void write
        .then((saved) => {
          if (!mounted.current || filterRevision.current !== revision) return;
          setSmoothing(saved.media.script_smoothing_percent ?? 0);
          setRounding(saved.media.peak_rounding_ms ?? 0);
          setSpeedLimit(saved.motion.apply_video_speed_limit ?? false);
          setError("");
        })
        .catch((reason: unknown) => {
          if (!mounted.current || filterRevision.current !== revision) return;
          setSmoothing(backendFilters.current.smoothingPercent);
          setRounding(backendFilters.current.roundingMillis);
          setSpeedLimit(backendFilters.current.limitSpeed);
          setError(reason instanceof Error ? reason.message : "Filters could not be saved.");
        }).finally(() => {
          if (filterRevision.current !== revision) return;
          filtersPending.current = false;
          if (mounted.current) setSavingFilters(false);
        });
    }, WRITE_DEBOUNCE_MILLIS);
  }, [onFiltersChanged, onFiltersChanging]);

  function changeOffset(millis: number) {
    const next = clamp(millis, -MAX_OFFSET_MILLIS, MAX_OFFSET_MILLIS);
    setOffset(next);
    writeOffset(next);
  }

  function reset() {
    changeOffset(0);
    setSmoothing(0);
    setRounding(0);
    setSpeedLimit(false);
    writeFilters({ script_smoothing_percent: 0, peak_rounding_ms: 0, apply_video_speed_limit: false });
  }

  const effective = clamp(setupOffsetMillis + offset, -MAX_OFFSET_MILLIS, MAX_OFFSET_MILLIS);

  return (
    <div className="playback-panel-layer" ref={layerRef}>
      <section className="playback-panel" id={id} ref={panelRef} aria-label={t("Playback settings for {display_name}", { display_name: video.display_name })}>
        <header className="playback-panel-head">
          <h2>{t("Playback")}</h2>
          <span title={video.display_name}>{video.display_name}</span>
          <button type="button" className="icon-button" aria-label={t("Close playback settings")} onClick={onClose}>
            <CloseIcon />
          </button>
        </header>

        <fieldset className="playback-panel-group" disabled={locked}>
          <legend className="visually-hidden">{t("Sync offset")}</legend>
          <div className="playback-panel-row">
            <span className="playback-panel-label">{t("Offset")}</span>
            <output className="playback-panel-value">{formatMillis(effective)}</output>
          </div>
          <RangeInput
            aria-label={t("Sync offset for this video")}
            min={-MAX_OFFSET_MILLIS}
            max={MAX_OFFSET_MILLIS}
            step={10}
            value={offset}
            disabled={locked}
            onChange={(event) => changeOffset(Number(event.target.value))}
          />
          <p className="playback-panel-hint">{t("this video {video} · setup {setup}", { video: formatMillis(offset), setup: formatMillis(setupOffsetMillis) })}
            <br />{t("Positive delays the device against the picture. Applies while playing.")}</p>
        </fieldset>

        <fieldset className="playback-panel-group" disabled={locked}>
          <legend className="playback-panel-legend">{t("Script filters")}<span>{t("restarts motion")}</span>
          </legend>

          <label className="playback-panel-toggle">
            <span>{t("Smoothing")}</span>
            <span className="toggle">
              <input
                type="checkbox"
                checked={smoothing > 0}
                disabled={locked}
                onChange={(event) => {
                  const next = event.target.checked ? 3 : 0;
                  setSmoothing(next);
                  writeFilters({ script_smoothing_percent: next });
                }}
              />
              <span className="track" aria-hidden="true" />
            </span>
            <output>{smoothing > 0 ? <>{smoothing}%</> : t("off")}</output>
          </label>
          {smoothing > 0 && (
            <RangeInput
              aria-label={t("Smoothing threshold")}
              min={1}
              max={MAX_SMOOTHING_PERCENT}
              step={1}
              value={smoothing}
              disabled={locked}
              onChange={(event) => {
                const next = Number(event.target.value);
                setSmoothing(next);
                writeFilters({ script_smoothing_percent: next });
              }}
            />
          )}

          <label className="playback-panel-toggle">
            <span>{t("Round peaks")}</span>
            <span className="toggle">
              <input
                type="checkbox"
                checked={rounding > 0}
                disabled={locked}
                onChange={(event) => {
                  const next = event.target.checked ? 60 : 0;
                  setRounding(next);
                  writeFilters({ peak_rounding_ms: next });
                }}
              />
              <span className="track" aria-hidden="true" />
            </span>
            <output>{rounding > 0 ? t("{rounding} ms", { rounding: rounding }) : t("off")}</output>
          </label>
          {rounding > 0 && (
            <RangeInput
              aria-label={t("Peak rounding window")}
              min={10}
              max={MAX_ROUNDING_MILLIS}
              step={10}
              value={rounding}
              disabled={locked}
              onChange={(event) => {
                const next = Number(event.target.value);
                setRounding(next);
                writeFilters({ peak_rounding_ms: next });
              }}
            />
          )}

          <label className="playback-panel-toggle">
            <span>{t("Limit speed")}</span>
            <span className="toggle">
              <input
                type="checkbox"
                checked={speedLimit}
                disabled={locked}
                onChange={(event) => {
                  setSpeedLimit(event.target.checked);
                  writeFilters({ apply_video_speed_limit: event.target.checked });
                }}
              />
              <span className="track" aria-hidden="true" />
            </span>
            <output>{speedLimit ? t("{percent}% max", { percent: speedLimitPercent }) : t("off")}</output>
          </label>
        </fieldset>

        <footer className="playback-panel-foot">
          <PlaybackFilterEffect sync={sync} smoothing={smoothing} rounding={rounding} speedLimit={speedLimit} speedLimitPercent={speedLimitPercent} pending={savingFilters} />
          <button type="button" className="btn btn-secondary compact-command" disabled={locked} onClick={reset}>{t("Reset")}</button>
        </footer>
        {error && <p className="form-status media-playback-error" role="alert">{error}</p>}
        {locked && <p className="form-status">{t("Read-only tab — playback settings are visible only.")}</p>}
      </section>
    </div>
  );
}

function clamp(value: number, minimum: number, maximum: number): number {
  if (!Number.isFinite(value)) return 0;
  return Math.max(minimum, Math.min(maximum, Math.round(value)));
}

export function formatMillis(millis: number): string {
  if (millis === 0) return "0 ms";
  return `${millis > 0 ? "+" : "−"}${Math.abs(millis)} ms`;
}
