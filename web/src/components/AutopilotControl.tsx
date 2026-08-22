import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { AutopilotSettings, SessionArc } from "../api/types";
import { t, translateKnown, type MessageKey } from "../i18n";
import { PauseIcon, PlayIcon } from "../shell/icons";
import { SegmentedChoice, SetpointSlider } from "./SetpointControls";
import { useAppState, useToast } from "../state/app-state";
import { ownsActiveMotion } from "../util/motion";

const decisionSourceCopy: Partial<Record<string, MessageKey>> = {
  model: "Assistant selected",
  fallback: "Planner fallback",
  hold: "Continuing current pattern",
  speech: "Spoken check-in changed motion",
};

const speechOptions = [
  ["off", "Off"],
  ["quiet", "Quiet"],
  ["natural", "Natural"],
  ["talkative", "Talkative"],
  ["custom", "Custom"],
] as const satisfies ReadonlyArray<readonly [string, MessageKey]>;

const motionChangeOptions = Array.from({ length: 8 }, (_, index) => {
  const value = String(index + 1);
  return { value, label: value };
});

const authorityOptions = [
  ["chat_only", "Chat only"],
  ["style_only", "Style only"],
  ["full_motion", "Full motion"],
] as const satisfies ReadonlyArray<readonly [string, MessageKey]>;

const minimumBuildupMinutes = 1;
// This only prevents time.Duration overflow in the Go scheduler. It is not a
// product-level session limit and is intentionally absent from the input UI.
const maximumBuildupMinutes = 153_722_867;

// SessionBuildup renders the visible session progression. A model encouraged to
// build intensity through a hidden counter is the escalation pattern the goals
// doc rules out; visible progress with a reset beside it is the difference.
function SessionBuildup({
  arc,
  disabled,
  onReset,
}: {
  arc: SessionArc;
  disabled: boolean;
  onReset: () => void;
}) {
  return (
    <div className="autopilot-buildup">
      <div className="autopilot-buildup-head">
        <span className="label">{t("Session buildup")}</span>
        <span className="hint-inline">{t("{percent}% of {minutes} min", { percent: arc.percent, minutes: arc.minutes })}</span>
      </div>
      <div
        className="autopilot-buildup-track"
        role="meter"
        aria-label={t("Session buildup")}
        aria-valuenow={arc.percent}
        aria-valuemin={0}
        aria-valuemax={100}
      >
        <span className="autopilot-buildup-fill" style={{ width: `${arc.percent}%` }} />
      </div>
      <div className="autopilot-buildup-actions">
        <span className="hint">{t("The assistant aims higher in your speed range as this fills. Your limits never move.")}</span>
        <button type="button" className="btn btn-secondary" disabled={disabled} onClick={onReset}>
          {t("Reset buildup")}
        </button>
      </div>
    </div>
  );
}

const errorMessage = (error: unknown) =>
  error instanceof Error ? translateKnown(error.message) : t("Autopilot request failed.");

function formatClock(milliseconds: number | undefined): string {
  if (!milliseconds || milliseconds <= 0) return t("Due");
  const totalSeconds = Math.max(1, Math.ceil(milliseconds / 1000));
  if (totalSeconds < 60) return t("{seconds} s", { seconds: totalSeconds });
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return seconds === 0 ? `${minutes}m` : `${minutes}m ${seconds}s`;
}

export function AutopilotControl() {
  const { state, backendOnline, readOnly, motion, refresh } = useAppState();
  const { show } = useToast();
  const modes = state?.modes;
  const active = modes?.mode === "autopilot" || modes?.active_mode === "autopilot";
  const engine = motion?.engine;
  const autopilotMotionActive = ownsActiveMotion(engine, "autopilot");
  const autopilotPaused = autopilotMotionActive && engine?.paused === true;
  const canPause = active && autopilotMotionActive && Boolean(engine?.running || engine?.paused);
  const locked = !backendOnline || !state || readOnly;
  const [pending, setPending] = useState<"start" | "stop" | "pause" | "resume" | "">("");
  const pendingRef = useRef(false);

  async function toggle() {
    if (pendingRef.current || locked) return;
    pendingRef.current = true;
    setPending(active ? "stop" : "start");
    try {
      if (active) {
        await api.stopMode();
        show(t("Autopilot stopped."));
      } else {
        await api.startMode("autopilot");
        show(t("Autopilot started."));
      }
    } catch (error) {
      show(errorMessage(error), "error");
    } finally {
      pendingRef.current = false;
      setPending("");
      refresh();
    }
  }

  async function togglePause() {
    if (pendingRef.current || locked || !canPause) return;
    const paused = autopilotPaused;
    pendingRef.current = true;
    setPending(paused ? "resume" : "pause");
    try {
      if (paused) {
        await api.resumeMotion();
        show(t("Motion resumed."));
      } else {
        await api.pauseMotion();
        show(t("Motion paused."));
      }
    } catch (error) {
      show(errorMessage(error), "error");
    } finally {
      pendingRef.current = false;
      setPending("");
      refresh();
    }
  }

  const segment = modes?.segment_index ?? 0;
  let status = t("Off");
  if (pending) {
    status = { start: t("Starting"), stop: t("Stopping"), pause: t("Pausing"), resume: t("Resuming") }[pending];
  } else if (active && autopilotPaused) {
    status = t("Paused");
  } else if (active && segment === 0) {
    status = t("Choosing first segment");
  } else if (active) {
    const source = modes?.decision_source
      ? decisionSourceCopy[modes.decision_source] ?? modes.decision_source
      : t("Active");
    status = t("Segment {segment} · {source}", { segment, source: translateKnown(source) });
  }

  const preferences = state?.settings?.autopilot;
  const resetBuildup = async () => {
    try {
      await api.resetAutopilotArc();
      refresh();
    } catch (error) {
      show(errorMessage(error), "error");
    }
  };
  let clockStatus = "";
  if (active && preferences) {
    const speech = preferences.speech_cadence === "off"
      ? t("Off")
      : modes?.speech_waiting_playback
        ? t("after audio")
        : formatClock(modes?.speech_in_ms);
    const motionDue = modes?.motion_planned
      ? t("planned")
      : formatClock(modes?.motion_change_in_ms);
    clockStatus = t("Motion {motion} · Speech {speech}", { motion: motionDue, speech });
  }

  return (
    <div className="autopilot-control" data-active={active || undefined} aria-busy={Boolean(pending) || undefined}>
      <span className="autopilot-control-dot" aria-hidden="true" />
      <div className="autopilot-control-copy">
        <strong>{t("Autopilot")}</strong>
        <span role="status">{status}</span>
        {clockStatus && <span className="autopilot-clock-status">{clockStatus}</span>}
      </div>
      <div className="autopilot-control-actions">
        {active && (
          <button
            type="button"
            className="icon-button"
            aria-label={autopilotPaused ? t("Resume Autopilot") : t("Pause Autopilot")}
            title={!canPause ? t("Autopilot motion has not started") : autopilotPaused ? t("Resume Autopilot") : t("Pause Autopilot")}
            disabled={locked || Boolean(pending) || !canPause}
            onClick={() => void togglePause()}
          >
            {autopilotPaused ? <PlayIcon /> : <PauseIcon />}
          </button>
        )}
        <button
          type="button"
          className={`btn ${active ? "btn-secondary" : "btn-start"} autopilot-control-action`}
          disabled={locked || Boolean(pending)}
          onClick={() => void toggle()}
        >
          {pending === "start" ? t("Starting…") : pending === "stop" ? t("Stopping…") : active ? t("Stop Autopilot") : t("Start Autopilot")}
        </button>
      </div>
      {active && modes?.session_arc?.enabled && (
        <SessionBuildup
          arc={modes.session_arc}
          disabled={locked}
          onReset={() => void resetBuildup()}
        />
      )}
      {preferences && (
        <AutopilotPreferences
          value={preferences}
          disabled={locked}
          onSaved={() => refresh()}
          onError={(error) => show(errorMessage(error), "error")}
        />
      )}
    </div>
  );
}

function AutopilotPreferences({
  value,
  disabled,
  onSaved,
  onError,
}: {
  value: AutopilotSettings;
  disabled: boolean;
  onSaved: () => void;
  onError: (error: unknown) => void;
}) {
  const [draft, setDraft] = useState(value);
  const [saving, setSaving] = useState(false);
  const draftRef = useRef(value);
  const editingRef = useRef(false);
  const savingRef = useRef(false);
  const queuedSaveRef = useRef<AutopilotSettings | null>(null);

  useEffect(() => {
    if (savingRef.current || editingRef.current) return;
    draftRef.current = value;
    setDraft(value);
  }, [value]);

  function edit(next: AutopilotSettings) {
    editingRef.current = true;
    draftRef.current = next;
    setDraft(next);
  }

  async function save(next: AutopilotSettings) {
    editingRef.current = false;
    draftRef.current = next;
    setDraft(next);
    if (savingRef.current) {
      queuedSaveRef.current = next;
      return;
    }
    savingRef.current = true;
    setSaving(true);
    try {
      let requested: AutopilotSettings | null = next;
      while (requested) {
        queuedSaveRef.current = null;
        const response = await api.saveAutopilotPreferences(requested);
        const queued = queuedSaveRef.current;
        if (!queued) {
          draftRef.current = response.autopilot;
          setDraft(response.autopilot);
        }
        onSaved();
        requested = queued;
      }
    } catch (error) {
      queuedSaveRef.current = null;
      draftRef.current = value;
      setDraft(value);
      onError(error);
    } finally {
      savingRef.current = false;
      setSaving(false);
    }
  }

  // Buildup duration is a single positive whole-minute value, not one end of a
  // cadence window, so it gets its own saver.
  function saveBuildupMinutes(raw: string) {
    const parsed = Number.parseInt(raw, 10);
    const minutes = Number.isFinite(parsed)
      ? Math.min(maximumBuildupMinutes, Math.max(minimumBuildupMinutes, parsed))
      : 30;
    void save({ ...draftRef.current, session_arc_minutes: minutes });
  }

  function saveNumber(
    key: "speech_min_seconds" | "speech_max_seconds",
    raw: string,
  ) {
    const parsed = Number.parseInt(raw, 10);
    const number = Number.isFinite(parsed) ? Math.min(600, Math.max(8, parsed)) : 8;
    const next = { ...draftRef.current, [key]: number };
    if (key.endsWith("min_seconds")) {
      const maxKey = "speech_max_seconds";
      next[maxKey] = Math.max(next[maxKey], number);
    } else {
      const minKey = "speech_min_seconds";
      next[minKey] = Math.min(next[minKey], number);
    }
    void save(next);
  }

  const controlsDisabled = disabled;
  return (
    <fieldset className="autopilot-preferences" disabled={controlsDisabled} aria-busy={saving || undefined}>
      <legend className="visually-hidden">{t("Autopilot timing")}</legend>
      <SetpointSlider
        className="autopilot-setpoints"
        label={t("Motion change rate")}
        value={String(Math.min(8, Math.max(1, draft.motion_change_level || 4)))}
        options={motionChangeOptions}
        disabled={controlsDisabled}
        onChange={(level) => void save({
          ...draftRef.current,
          motion_cadence: "scaled",
          motion_change_level: Number(level),
        })}
      />
      <SetpointSlider
        className="autopilot-setpoints"
        label={t("Spoken check-ins")}
        value={draft.speech_cadence}
        options={speechOptions.map(([option, label]) => ({ value: option, label: translateKnown(label) }))}
        disabled={controlsDisabled}
        onChange={(speech_cadence) => void save({ ...draftRef.current, speech_cadence })}
      />
      {draft.speech_cadence === "custom" && (
        <div className="autopilot-window autopilot-window-range">
          <span>{t("Speech range")}</span>
          <input
            type="number"
            min={8}
            max={600}
            value={draft.speech_min_seconds}
            aria-label={t("Speech minimum seconds")}
            onChange={(event) => edit({ ...draftRef.current, speech_min_seconds: Number(event.target.value) })}
            onBlur={(event) => saveNumber("speech_min_seconds", event.target.value)}
          />
          <span aria-hidden="true">-</span>
          <input
            type="number"
            min={8}
            max={600}
            value={draft.speech_max_seconds}
            aria-label={t("Speech maximum seconds")}
            onChange={(event) => edit({ ...draftRef.current, speech_max_seconds: Number(event.target.value) })}
            onBlur={(event) => saveNumber("speech_max_seconds", event.target.value)}
          />
          <span>{t("seconds")}</span>
        </div>
      )}
      <div className="autopilot-buildup-setting">
        <label className="toggle-line">
          <span className="toggle">
            <input
              type="checkbox"
              checked={draft.session_arc}
              onChange={(event) => {
                const enabled = event.target.checked;
                void save({
                  ...draft,
                  session_arc: enabled,
                  session_tracking: enabled || draft.session_tracking,
                });
              }}
            />
            <span className="track" aria-hidden="true" />
          </span>
          <span>{t("Session buildup")}</span>
        </label>
        {draft.session_arc && (
          <div className="autopilot-window">
            <span>{t("Buildup duration")}</span>
            <input
              type="number"
              min={minimumBuildupMinutes}
              value={draft.session_arc_minutes}
              aria-label={t("Session buildup minutes")}
              onChange={(event) => edit({ ...draftRef.current, session_arc_minutes: Number(event.target.value) })}
              onBlur={(event) => saveBuildupMinutes(event.target.value)}
            />
            <span>{t("minutes")}</span>
          </div>
        )}
      </div>
      <details className="autopilot-advanced">
        <summary>{t("Advanced")}</summary>
        <SegmentedChoice
          className="autopilot-authority"
          label={t("Speech motion")}
          value={draft.speech_motion_authority}
          options={authorityOptions.map(([option, label]) => ({ value: option, label: translateKnown(label) }))}
          disabled={controlsDisabled}
          onChange={(speech_motion_authority) => void save({ ...draftRef.current, speech_motion_authority })}
        />
        <label className="toggle-line">
          <span className="toggle">
            <input
              type="checkbox"
              checked={draft.adaptive_motion_timing}
              onChange={(event) => void save({ ...draft, adaptive_motion_timing: event.target.checked })}
            />
            <span className="track" aria-hidden="true" />
          </span>
          <span>{t("Adaptive motion timing")}</span>
        </label>
        <label className="toggle-line">
          <span className="toggle">
            <input
              type="checkbox"
              checked={draft.adaptive_speech_timing}
              disabled={draft.speech_cadence === "off"}
              onChange={(event) => void save({ ...draft, adaptive_speech_timing: event.target.checked })}
            />
            <span className="track" aria-hidden="true" />
          </span>
          <span>{t("Adaptive speech timing")}</span>
        </label>
        <label className="toggle-line">
          <span className="toggle">
            <input
              type="checkbox"
              checked={draft.session_tracking}
              onChange={(event) => {
                const enabled = event.target.checked;
                // The arc is a reading of session progress, so it cannot outlive
                // the tracking that produces it. Clearing both together keeps the
                // UI from offering a combination the backend rejects.
                void save({
                  ...draft,
                  session_tracking: enabled,
                  session_arc: enabled ? draft.session_arc : false,
                });
              }}
            />
            <span className="track" aria-hidden="true" />
          </span>
          <span>{t("Session tracking")}</span>
        </label>
        <p className="autopilot-session-hint">{t("Shares timing and pace; limits stay unchanged.")}</p>
      </details>
    </fieldset>
  );
}
