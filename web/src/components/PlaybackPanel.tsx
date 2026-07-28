import { formatNumber, t } from "../i18n";
// Floating playback panel for the video currently open in the player. It
// overlays the workspace rather than reflowing it, because its whole purpose is
// to be adjusted while watching: calibration you cannot see the effect of is
// just a settings form in a worse place.
import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { MediaSyncStatus, MediaVideo } from "../api/types";
import { CloseIcon } from "../shell/icons";

// Mirror config.MaxScriptOffsetMillis / MaxScriptSmoothingPercent / MaxPeakRoundingMillis.
const MAX_OFFSET_MILLIS = 2000;
const MAX_SMOOTHING_PERCENT = 5;
const MAX_ROUNDING_MILLIS = 200;
const WRITE_DEBOUNCE_MILLIS = 180;

interface Props {
  video: MediaVideo;
  sync: MediaSyncStatus;
  locked: boolean;
  setupOffsetMillis: number;
  smoothingPercent: number;
  roundingMillis: number;
  limitSpeed: boolean;
  onClose: () => void;
  onVideoUpdate?: (video: MediaVideo) => void;
  onFiltersChanged?: () => void;
}

export function PlaybackPanel({
  video,
  sync,
  locked,
  setupOffsetMillis,
  smoothingPercent,
  roundingMillis,
  limitSpeed,
  onClose,
  onVideoUpdate,
  onFiltersChanged,
}: Props) {
  const [offset, setOffset] = useState(video.script_offset_ms ?? 0);
  const [smoothing, setSmoothing] = useState(smoothingPercent);
  const [rounding, setRounding] = useState(roundingMillis);
  const [speedLimit, setSpeedLimit] = useState(limitSpeed);
  const [error, setError] = useState("");
  const panelRef = useRef<HTMLDivElement | null>(null);
  const offsetTimer = useRef<number>();
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      window.clearTimeout(offsetTimer.current);
    };
  }, []);

  useEffect(() => {
    panelRef.current?.querySelector<HTMLElement>("input, button")?.focus();
  }, []);

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if (event.key === "Escape") onClose();
    }
    function onPointer(event: MouseEvent) {
      if (!panelRef.current?.contains(event.target as Node)) onClose();
    }
    window.addEventListener("keydown", onKey);
    // Deferred so the click that opened the panel does not immediately close it.
    const timer = window.setTimeout(() => window.addEventListener("mousedown", onPointer), 0);
    return () => {
      window.clearTimeout(timer);
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("mousedown", onPointer);
    };
  }, [onClose]);

  // The offset writes on a short debounce: dragging is one gesture, not thirty
  // requests, and the backend applies each one to the live run without stopping.
  const writeOffset = useCallback((millis: number) => {
    window.clearTimeout(offsetTimer.current);
    offsetTimer.current = window.setTimeout(() => {
      void api.saveMediaScriptOffset(video.id, millis)
        .then(() => {
          if (!mounted.current) return;
          setError("");
          onVideoUpdate?.({ ...video, script_offset_ms: millis });
        })
        .catch((reason: unknown) => {
          if (mounted.current) setError(reason instanceof Error ? reason.message : "Offset could not be saved.");
        });
    }, WRITE_DEBOUNCE_MILLIS);
  }, [onVideoUpdate, video]);

  const writeFilters = useCallback((patch: Record<string, number | boolean>) => {
    void api.saveMediaPlayback(patch)
      .then(() => {
        if (!mounted.current) return;
        setError("");
        onFiltersChanged?.();
      })
      .catch((reason: unknown) => {
        if (mounted.current) setError(reason instanceof Error ? reason.message : "Filters could not be saved.");
      });
  }, [onFiltersChanged]);

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
  const effect = sync.filter_effect;
  const actionsRemoved = effect?.actions_removed ?? 0;
  const peakReductionPercent = effect?.peak_reduction_percent ?? 0;
  const hasMeasuredEffect = actionsRemoved > 0 || peakReductionPercent > 0;
  const filtered = smoothing > 0 || rounding > 0 || speedLimit;

  return (
    <div className="playback-panel-layer">
      <section className="playback-panel" ref={panelRef} aria-label={t("Playback settings for {display_name}", { display_name: video.display_name })}>
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
          <input
            type="range"
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
            <span>{t("Smoothing")}</span>
            <output>{smoothing > 0 ? <>{smoothing}%</> : t("off")}</output>
          </label>
          {smoothing > 0 && (
            <input
              type="range"
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
            <span>{t("Round peaks")}</span>
            <output>{rounding > 0 ? t("{rounding} ms", { rounding: rounding }) : t("off")}</output>
          </label>
          {rounding > 0 && (
            <input
              type="range"
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
            <input
              type="checkbox"
              checked={speedLimit}
              disabled={locked}
              onChange={(event) => {
                setSpeedLimit(event.target.checked);
                writeFilters({ apply_video_speed_limit: event.target.checked });
              }}
            />
            <span>{t("Limit speed")}</span>
            <output>{speedLimit ? t("on") : t("off")}</output>
          </label>
        </fieldset>

        <footer className="playback-panel-foot">
          <p className="playback-panel-effect" role="status">
            {!filtered && t("Playing the script exactly as authored.")}
            {filtered && !hasMeasuredEffect && t("Filters on; effect is measured when motion re-arms.")}
            {filtered && hasMeasuredEffect && (actionsRemoved > 0 && peakReductionPercent > 0
              ? t("{count} actions removed · peaks up to {percent}% lower", {
                count: formatNumber(actionsRemoved),
                percent: peakReductionPercent,
              })
              : actionsRemoved > 0
                ? t("{count} actions removed", { count: formatNumber(actionsRemoved) })
                : t("Peaks up to {percent}% lower", { percent: peakReductionPercent }))}
          </p>
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
