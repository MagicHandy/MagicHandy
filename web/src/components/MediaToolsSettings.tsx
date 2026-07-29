import { formatNumber, t } from "../i18n";
import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { MediaJobState, MediaSettingsPayload, MediaToolStatus } from "../api/types";
import { HostPathField } from "./HostPathField";

// Mirrors internal/config bounds so the form cannot ask for a value the server
// would silently clamp.
const MIN_CRF = 18;
const MAX_CRF = 30;
const PRESETS = ["ultrafast", "superfast", "veryfast", "faster", "fast", "medium", "slow", "slower", "veryslow"] as const;
// Mirrors config.MinReencodeAudioKbps / MaxReencodeAudioKbps.
const MIN_AUDIO_KBPS = 96;
const MAX_AUDIO_KBPS = 576;
// Every standard AAC bitrate is a multiple of 16, so the step lands on them.
const AUDIO_KBPS_STEP = 16;
const JOB_POLL_MILLIS = 700;

interface Props {
  media: MediaSettingsPayload;
  locked: boolean;
  onChange: (patch: Partial<MediaSettingsPayload>) => void;
}

/**
 * MediaToolsSettings owns the optional FFmpeg dependency and everything that
 * needs it. The dependency has three honest states and the panel shows which
 * one is live: absent, configured but unusable, or verified. Nothing here
 * half-works when FFmpeg is missing — the actions stay visible and disabled
 * with the reason, because hiding them would make the feature undiscoverable
 * for exactly the people who need it.
 */
export function MediaToolsSettings({ media, locked, onChange }: Props) {
  const [tools, setTools] = useState<MediaToolStatus | null>(null);
  const [job, setJob] = useState<MediaJobState | null>(null);
  const [status, setStatus] = useState("");
  const [error, setError] = useState("");
  const mounted = useRef(true);

  const refresh = useCallback(async () => {
    try {
      const [toolResponse, jobResponse] = await Promise.all([api.mediaTools(), api.mediaJob()]);
      if (!mounted.current) return;
      setTools(toolResponse.tools);
      setJob(jobResponse.job);
    } catch (reason) {
      if (mounted.current) setError(reason instanceof Error ? reason.message : t("Media tool status could not be loaded."));
    }
  }, []);

  useEffect(() => {
    mounted.current = true;
    void refresh();
    return () => { mounted.current = false; };
  }, [refresh]);

  // Re-validate whenever the saved path changes, so the readout reflects the
  // binary that is actually configured rather than the one that was.
  useEffect(() => { void refresh(); }, [media.ffmpeg_path, refresh]);

  useEffect(() => {
    if (!job?.running) return undefined;
    const timer = window.setTimeout(() => void refresh(), JOB_POLL_MILLIS);
    return () => window.clearTimeout(timer);
  }, [job, refresh]);

  const available = tools?.available === true;
  const busy = job?.running === true;
  const disabled = locked || busy || !available;

  async function run(action: () => Promise<unknown>, done: string) {
    setError("");
    setStatus("");
    try {
      await action();
      if (mounted.current) setStatus(done);
      await refresh();
    } catch (reason) {
      if (mounted.current) setError(reason instanceof Error ? reason.message : t("The media job could not be started."));
    }
  }

  const h265Forced = media.convert_h265_for_compatibility === true;
  const effectiveCodec = h265Forced ? "h264" : (media.reencode_codec ?? "h264");
  const crfField = effectiveCodec === "h265" ? "reencode_crf_h265" : "reencode_crf_h264";
  const crfValue = effectiveCodec === "h265"
    ? (media.reencode_crf_h265 ?? 28)
    : (media.reencode_crf_h264 ?? 23);

  return (
    <div className="media-tools-settings">
      <div className="group">
      <h3 className="group-title">{t("FFmpeg")}</h3>
      <p className="form-status media-tool-hint">{t("Thumbnails for unopened videos, and repairing files this browser cannot play, both need FFmpeg. Nothing else in the app does, and everything else keeps working without it.")}</p>

      <HostPathField
        label={t("FFmpeg location")}
        value={media.ffmpeg_path ?? ""}
        kind="file"
        disabled={locked}
        placeholder={t("Choose the ffmpeg executable")}
        onChange={(value) => onChange({ ffmpeg_path: value })}
      />
      <p className={available ? "form-status media-tool-ready" : "form-status"} role="status">
        {available
          ? t("Ready: {version}", { version: tools?.version ?? "" })
          : tools?.configured
            ? t("Not usable: {reason}", { reason: tools?.error ?? "" })
            : t("Not configured. Save a path above to enable thumbnails and conversion.")}
      </p>
      <p className="form-status media-tool-hint">{t("ffprobe must sit beside ffmpeg; conversion uses it to decide whether a file needs re-encoding at all. A build that can encode H.265 is a GPL build — MagicHandy runs it as a separate program and never links it.")}</p>

      </div>
      <div className="group">
      <h3 className="group-title">{t("Thumbnails")}</h3>
      <p className="form-status media-tool-hint">{t("Videos you play get a cover automatically from the frame the browser already decoded. This fills in the rest.")}</p>
      <div className="media-tool-actions">
        <button type="button" className="btn btn-secondary" disabled={disabled} onClick={() => void run(() => api.generateMediaThumbnails(false), t("Thumbnail generation started."))}>{t("Generate missing")}</button>
        <button type="button" className="btn btn-secondary" disabled={disabled} onClick={() => void run(() => api.generateMediaThumbnails(true), t("Thumbnail generation started."))}>{t("Regenerate all")}</button>
        <button type="button" className="btn btn-secondary" disabled={locked || busy} onClick={() => void run(() => api.clearMediaThumbnails(), t("Thumbnails cleared."))}>{t("Clear thumbnails")}</button>
      </div>

      </div>
      <div className="group">
      <h3 className="group-title">{t("Conversion")}</h3>
      <p className="form-status media-tool-hint">{t("Conversion repairs files that cannot play. A file that already plays is never converted, by any path — not by a button, not by a scan, not to save space. The original is never modified or deleted; the converted copy is written beside it and the original is hidden from the library.")}</p>

      <label className="toggle-line">
        <span className="toggle">
          <input
            type="checkbox"
            checked={h265Forced}
            disabled={locked}
            onChange={(event) => onChange({ convert_h265_for_compatibility: event.target.checked })}
          />
          <span className="track" aria-hidden="true" />
        </span>
        <span>{t("Convert H.265 for wider compatibility")}<small>{t("Off assumes this browser plays HEVC, which most do — Firefox commonly does not. On, H.265 files count as needing repair and every re-encode targets H.264. You do not have to guess: a file that fails to play is detected and offered for conversion regardless of this setting.")}</small></span>
      </label>

      <label className="toggle-line">
        <span className="toggle">
          <input
            type="checkbox"
            checked={media.show_superseded_originals ?? false}
            disabled={locked}
            onChange={(event) => onChange({ show_superseded_originals: event.target.checked })}
          />
          <span className="track" aria-hidden="true" />
        </span>
        <span>{t("Show originals that have been converted")}<small>{t("Off hides a source file once a converted copy sits beside it. Nothing is deleted: delete the converted file and the original returns on the next scan.")}</small></span>
      </label>

      <label className="field">
        <span className="label">{t("Re-encode codec")}</span>
        <select
          value={effectiveCodec}
          disabled={locked || h265Forced}
          onChange={(event) => onChange({ reencode_codec: event.target.value as "h264" | "h265" })}
        >
          <option value="h264">{t("H.264 — plays everywhere")}</option>
          <option value="h265">{t("H.265 — about half the size, less compatible")}</option>
        </select>
        <small>{h265Forced
          ? t("Forced to H.264 while the compatibility option above is on, so a re-encode cannot land back on a codec this browser will not play.")
          : t("Only used when the container alone cannot be fixed. Most repairs copy the streams across untouched and never reach this setting.")}</small>
      </label>

      <label className="field">
        <span className="label">{t("Quality")}<span className="hint-inline">{t("CRF {value}", { value: formatNumber(crfValue) })}</span></span>
        <input
          type="range"
          min={MIN_CRF}
          max={MAX_CRF}
          step={1}
          value={crfValue}
          disabled={locked}
          onChange={(event) => onChange({ [crfField]: Number(event.target.value) } as Partial<MediaSettingsPayload>)}
        />
        <small>{t("Lower is better quality and a larger file. The scales differ between codecs, so each keeps its own value.")}</small>
      </label>

      <label className="field">
        <span className="label">{t("Encoder preset")}</span>
        <select
          value={media.reencode_preset ?? "medium"}
          disabled={locked}
          onChange={(event) => onChange({ reencode_preset: event.target.value })}
        >
          {PRESETS.map((preset) => <option key={preset} value={preset}>{preset}</option>)}
        </select>
        <small>{t("Trades encoding time for file size at the same quality. It does not trade quality.")}</small>
      </label>

      <label className="field">
        <span className="label">{t("Audio bitrate")}<span className="hint-inline">{t("{rate} kbps", { rate: formatNumber(media.reencode_audio_kbps ?? 192) })}</span></span>
        <input
          type="range"
          min={MIN_AUDIO_KBPS}
          max={MAX_AUDIO_KBPS}
          step={AUDIO_KBPS_STEP}
          value={media.reencode_audio_kbps ?? 192}
          disabled={locked}
          onChange={(event) => onChange({ reencode_audio_kbps: Number(event.target.value) })}
        />
        <small>{t("Only used when the source audio is not already AAC; existing AAC is copied without quality loss. 192 kbps suits speech and most soundtracks; raise it for music. This is a target bitrate: FFmpeg may use less or clamp it to the source channel count and sample rate.")}</small>
      </label>

      <div className="media-tool-actions">
        <button type="button" className="btn btn-secondary" disabled={disabled} onClick={() => void run(() => api.convertMedia([]), t("Conversion started."))}>{t("Convert everything that cannot play")}</button>
      </div>

      </div>

      {Boolean(job?.running || job?.completed_at || job?.issues?.length || status || error) && (
        <div className="group">
          <h3 className="group-title">{t("Media task status")}</h3>

          {job?.running && (
            <div className="form-status media-job-status" role="status">
              <span>{job.kind === "conversion"
                ? t("Converting {name} ({done} of {total}, {percent}%)", { name: job.current_name ?? "", done: formatNumber(job.processed + 1), total: formatNumber(job.total), percent: formatNumber(job.item_percent) })
                : t("Generating thumbnails ({done} of {total})", { done: formatNumber(job.processed), total: formatNumber(job.total) })}</span>
              <button type="button" className="btn btn-secondary compact-command" disabled={!job.cancellable} onClick={() => void run(() => api.cancelMediaJob(), t("Cancelling."))}>{t("Cancel")}</button>
            </div>
          )}
          {!job?.running && job?.completed_at && (
            <p className="form-status" role="status">{t("Last job: {succeeded} done, {failed} failed.", { succeeded: formatNumber(job.succeeded), failed: formatNumber(job.failed) })}</p>
          )}
          {(job?.issues ?? []).map((issue) => (
            <p className="form-status media-playback-error" role="alert" key={`${issue.name}:${issue.message}`}>{issue.name}: {issue.message}</p>
          ))}
          {status && <p className="form-status" role="status">{status}</p>}
          {error && <p className="form-status media-playback-error" role="alert">{error}</p>}
        </div>
      )}
    </div>
  );
}
