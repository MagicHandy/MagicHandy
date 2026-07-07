import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { AnimatePresence, motion } from "motion/react";
import { api } from "../api/client";
import { useStatus } from "../contexts/StatusContext";
import { useToast } from "../contexts/ToastContext";
import { MouseControlCylinder } from "../components/MouseControlCylinder";
import { MouseControlGestureHints } from "../components/MouseControlGestureHints";
import { MouseControlResponseSlider } from "../components/MouseControlResponseSlider";
import { useMouseControlGestures } from "../lib/useMouseControlGestures";
import { useMouseDirectControl } from "../lib/useMouseDirectControl";

function formatDuration(ms: number): string {
  const totalSec = Math.max(0, Math.floor(ms / 1000));
  const min = Math.floor(totalSec / 60);
  const sec = totalSec % 60;
  return `${min}:${sec.toString().padStart(2, "0")}`;
}

const MAX_SMOOTHING_MS = 500;

export function MouseControl() {
  const { t } = useTranslation();
  const { notify } = useToast();
  const { snap, refresh } = useStatus();
  const [active, setActive] = useState(false);
  const [starting, setStarting] = useState(false);
  const [recording, setRecording] = useState(false);
  const [recordingBusy, setRecordingBusy] = useState(false);
  const [recordActions, setRecordActions] = useState(0);
  const [recordDurationMs, setRecordDurationMs] = useState(0);
  const [durationMs, setDurationMs] = useState(66);
  const [minSendMs, setMinSendMs] = useState(16);

  const deviceOk = Boolean(snap?.device_connected);
  const emergency = Boolean(snap?.emergency_stop);
  const limitsEnabled = snap?.safety_limits_enabled !== false;

  const onError = useCallback(
    (message: string) => notify(message, "error"),
    [notify],
  );

  const {
    padRef,
    targetNorm,
    sentPct,
    handlePointerMove,
    handlePointerLeave,
  } = useMouseDirectControl({
    active,
    durationMs,
    limitsEnabled,
    minSendIntervalMs: minSendMs,
    onError,
  });

  const targetPct = targetNorm * 100;
  const displayPct = sentPct ?? snap?.motion_position_pct ?? targetPct;

  const pollRecordingStatus = useCallback(async () => {
    try {
      const res = await api.getDirectControlStatus();
      setRecording(Boolean(res.recording));
      setRecordActions(res.recording_action_count ?? 0);
      setRecordDurationMs(res.recording_duration_ms ?? 0);
    } catch {
      /* */
    }
  }, []);

  const start = async () => {
    if (!deviceOk) {
      notify(t("mouse.connectFirst"), "error");
      return;
    }
    setStarting(true);
    try {
      const res = await api.startDirectControl();
      if (res.limits_enabled === false) {
        setDurationMs((d) => Math.max(1, Math.min(d, MAX_SMOOTHING_MS)));
      }
      setActive(true);
      notify(t("mouse.controlActive"), "ok");
      await refresh();
    } catch (e) {
      notify(e instanceof Error ? e.message : t("mouse.startError"), "error");
    } finally {
      setStarting(false);
    }
  };

  const stop = useCallback(async () => {
    try {
      const res = await api.stopDirectControl();
      if (res.saved_recording) {
        const s = res.saved_recording;
        notify(
          t("mouse.recordingSaved", {
            actions: s.action_count,
            duration: formatDuration(s.duration_ms),
          }),
          "ok",
        );
      }
    } catch {
      /* */
    }
    setActive(false);
    setRecording(false);
    setRecordActions(0);
    setRecordDurationMs(0);
    await refresh();
  }, [notify, refresh, t]);

  const minDuration = limitsEnabled ? 20 : 1;
  const maxDuration = MAX_SMOOTHING_MS;
  const durationStep = limitsEnabled ? 8 : 25;

  const startRecording = useCallback(async () => {
    if (!active) return;
    setRecordingBusy(true);
    try {
      const res = await api.startDirectRecording();
      setRecording(res.recording);
      setRecordActions(res.action_count);
      notify(t("mouse.recordingStarted"), "ok");
    } catch (e) {
      notify(e instanceof Error ? e.message : t("mouse.recordError"), "error");
    } finally {
      setRecordingBusy(false);
    }
  }, [active, notify, t]);

  const stopRecording = useCallback(async () => {
    setRecordingBusy(true);
    try {
      const res = await api.stopDirectRecording();
      setRecording(false);
      setRecordActions(0);
      setRecordDurationMs(0);
      notify(t("mouse.recordingSavedLib", { actions: res.action_count }), "ok");
    } catch (e) {
      notify(e instanceof Error ? e.message : t("mouse.saveRecordingError"), "error");
    } finally {
      setRecordingBusy(false);
    }
  }, [notify, t]);

  const toggleRecording = useCallback(() => {
    if (recordingBusy) return;
    if (recording) void stopRecording();
    else void startRecording();
  }, [recording, recordingBusy, startRecording, stopRecording]);

  const {
    fastResponse,
    onPadPointerDown,
    onPadPointerUp,
    onPadPointerCancel,
    onPadContextMenu,
    onPadAuxClick,
  } = useMouseControlGestures({
    active,
    durationMs,
    minDuration,
    maxDuration,
    durationStep,
    recordingBusy,
    onToggleRecording: toggleRecording,
    setDurationMs,
  });

  useEffect(() => {
    if (limitsEnabled) {
      setMinSendMs(16);
      setDurationMs((d) => Math.max(20, Math.min(MAX_SMOOTHING_MS, d)));
    } else {
      setMinSendMs(1);
      setDurationMs((d) => Math.max(1, Math.min(MAX_SMOOTHING_MS, d)));
    }
  }, [limitsEnabled]);

  useEffect(() => {
    if (snap?.direct_control_active && !active) setActive(true);
    if (!snap?.direct_control_active && active && !starting) {
      setActive(false);
      setRecording(false);
    }
  }, [snap?.direct_control_active, active, starting]);

  useEffect(() => {
    return () => {
      if (active) void api.stopDirectControl().catch(() => {});
    };
  }, [active]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && active) void stop();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [active, stop]);

  useEffect(() => {
    if (!recording) return;
    void pollRecordingStatus();
    const timer = window.setInterval(() => void pollRecordingStatus(), 400);
    return () => window.clearInterval(timer);
  }, [recording, pollRecordingStatus]);

  return (
    <div className="page page--fill mouse-control-page">
      <header className="mouse-control-head">
        <div className="mouse-control-head-text">
          <h1 className="mouse-control-title">{t("mouse.title")}</h1>
          <p className="hint mouse-control-sub">{t("mouse.subtitle")}</p>
        </div>
        <div className="mouse-control-actions">
          <AnimatePresence>
            {active && (
              <motion.span
                key="status-chip"
                className={`mouse-control-status-chip${recording ? " mouse-control-status-chip--rec" : ""}`}
                initial={{ opacity: 0, scale: 0.88, x: 10 }}
                animate={{ opacity: 1, scale: 1, x: 0 }}
                exit={{ opacity: 0, scale: 0.88, x: 10 }}
                transition={{ type: "spring", stiffness: 420, damping: 30 }}
              >
                {recording ? t("mouse.recording") : t("mouse.active")}
              </motion.span>
            )}
          </AnimatePresence>
          {!active ? (
            <button
              type="button"
              className="btn btn-primary"
              disabled={starting || !deviceOk || emergency}
              onClick={start}
            >
              {starting ? t("mouse.starting") : t("mouse.startControl")}
            </button>
          ) : (
            <button type="button" className="btn btn-ghost" onClick={stop}>
              {t("mouse.stopControl")}
            </button>
          )}
        </div>
      </header>

      <AnimatePresence>
        {(recording || !deviceOk || emergency) && (
          <motion.div
            key="mouse-alerts"
            className="mouse-control-alerts"
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: "auto" }}
            exit={{ opacity: 0, height: 0 }}
            transition={{ duration: 0.22 }}
          >
            <AnimatePresence mode="popLayout">
              {recording && (
                <motion.div
                  key="rec-banner"
                  className="mouse-control-rec-banner"
                  role="status"
                  initial={{ opacity: 0, y: -6 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, y: -6 }}
                  transition={{ duration: 0.18 }}
                >
                  <motion.span
                    className="mouse-control-rec-dot"
                    aria-hidden
                    animate={{ scale: [1, 0.82, 1], opacity: [1, 0.5, 1] }}
                    transition={{ repeat: Infinity, duration: 1.2, ease: "easeInOut" }}
                  />
                  {formatDuration(recordDurationMs)} · {recordActions} {t("mouse.keyframes")}
                </motion.div>
              )}
              {!deviceOk && (
                <motion.div
                  key="device-offline"
                  className="alert alert-warn mouse-control-alert"
                  initial={{ opacity: 0, x: -8 }}
                  animate={{ opacity: 1, x: 0 }}
                  exit={{ opacity: 0, x: -8 }}
                >
                  {t("mouse.deviceOffline")}
                </motion.div>
              )}
              {emergency && (
                <motion.div
                  key="emergency"
                  className="alert alert-warn mouse-control-alert"
                  initial={{ opacity: 0, x: -8 }}
                  animate={{ opacity: 1, x: 0 }}
                  exit={{ opacity: 0, x: -8 }}
                >
                  {t("mouse.emergencyActive")}
                </motion.div>
              )}
            </AnimatePresence>
          </motion.div>
        )}
      </AnimatePresence>

      <div className="mouse-control-layout">
        <aside className="glass mouse-control-panel">
          <span className="section-label">{t("mouse.smoothing")}</span>
          <MouseControlResponseSlider
            value={durationMs}
            min={minDuration}
            max={maxDuration}
            step={limitsEnabled ? 2 : 1}
            disabled={!active || fastResponse}
            turbo={fastResponse}
            onChange={setDurationMs}
          />

          <MouseControlGestureHints dimmed={!active} />

          <div className="mouse-control-mini-stats">
            <div>
              <span className="mouse-control-stat-label">{t("mouse.target")}</span>
              <motion.span
                key={`target-${targetPct.toFixed(0)}`}
                className="mouse-control-stat-value mono"
                initial={{ opacity: 0.5, y: 4 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ duration: 0.12 }}
              >
                {targetPct.toFixed(0)}%
              </motion.span>
            </div>
            <div>
              <span className="mouse-control-stat-label">{t("mouse.sent")}</span>
              <motion.span
                key={`sent-${displayPct.toFixed(0)}`}
                className="mouse-control-stat-value mono"
                initial={{ opacity: 0.5, y: 4 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ duration: 0.12 }}
              >
                {displayPct.toFixed(0)}%
              </motion.span>
            </div>
          </div>

          <p className="hint mouse-control-hint">
            {active ? t("mouse.hintActive") : t("mouse.hintIdle")}
          </p>
        </aside>

        <motion.div
          className={`mouse-control-stage${active ? " mouse-control-stage--live" : ""}${recording ? " mouse-control-stage--recording" : ""}`}
          initial={false}
          animate={{
            boxShadow: active
              ? recording
                ? "inset 0 0 0 1px rgba(248,113,113,0.22), inset 0 0 48px rgba(248,113,113,0.04)"
                : "inset 0 0 0 1px rgba(74,222,128,0.18), inset 0 0 40px rgba(99,102,241,0.06)"
              : "inset 0 0 0 1px rgba(148,163,184,0.08)",
          }}
          transition={{ duration: 0.3 }}
        >
          <MouseControlCylinder
            active={active}
            recording={recording}
            fastResponse={fastResponse}
            targetPct={targetPct}
            sentPct={displayPct}
            padRef={padRef}
            onPointerMove={handlePointerMove}
            onPointerLeave={handlePointerLeave}
            onPadPointerDown={onPadPointerDown}
            onPadPointerUp={onPadPointerUp}
            onPadPointerCancel={onPadPointerCancel}
            onPadContextMenu={onPadContextMenu}
            onPadAuxClick={onPadAuxClick}
          />

          <AnimatePresence>
            {!active && (
              <motion.div
                key="stage-idle"
                className="mouse-control-stage-idle"
                initial={{ opacity: 0, backdropFilter: "blur(0px)" }}
                animate={{ opacity: 1, backdropFilter: "blur(1px)" }}
                exit={{ opacity: 0, backdropFilter: "blur(0px)" }}
                transition={{ duration: 0.28 }}
              >
                <motion.span
                  className="mouse-control-stage-idle-icon"
                  aria-hidden
                  animate={{ y: [0, -5, 0] }}
                  transition={{ repeat: Infinity, duration: 2.4, ease: "easeInOut" }}
                >
                  ↕
                </motion.span>
                <span>{t("mouse.startIdle")}</span>
                <span className="hint">{t("mouse.startIdleHint")}</span>
              </motion.div>
            )}
          </AnimatePresence>
        </motion.div>
      </div>
    </div>
  );
}
