import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api } from "../api/client";
import type { StatusSnapshot } from "../api/types";
import { useToast } from "../contexts/ToastContext";
import { TopbarMenu } from "./TopbarMenu";

interface SessionControlsProps {
  snap: StatusSnapshot;
}

type IntensityPreset = "leve" | "medio" | "rapido" | "intenso";

export function SessionControls({ snap }: SessionControlsProps) {
  const { t } = useTranslation();
  const { notify } = useToast();
  const [waitFirstMsg, setWaitFirstMsg] = useState(true);
  const [rosterBusy, setRosterBusy] = useState(false);
  const [durationMin, setDurationMin] = useState(15);
  const [intensity, setIntensity] = useState<IntensityPreset>("medio");
  const [includeBuildup, setIncludeBuildup] = useState(true);
  const [rosterNote, setRosterNote] = useState("");

  const intensityOptions: { id: IntensityPreset; label: string }[] = [
    { id: "leve", label: t("session.intensity.leve") },
    { id: "medio", label: t("session.intensity.medio") },
    { id: "rapido", label: t("session.intensity.rapido") },
    { id: "intenso", label: t("session.intensity.intenso") },
  ];

  useEffect(() => {
    api
      .getSettings()
      .then((s) => {
        const autodom = (s.autodom ?? {}) as Record<string, unknown>;
        setWaitFirstMsg(autodom.wait_for_user_message !== false);
      })
      .catch(() => {});
  }, []);

  const wrap = async (fn: () => Promise<unknown>, ok: string) => {
    try {
      await fn();
      notify(ok, "ok");
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  const sessionActive = snap.auto_running || snap.playback_active;
  const rosterActive = Boolean(snap.roster_chat_active);

  const startRosterSession = async () => {
    setRosterBusy(true);
    try {
      const res = await api.planRosterSession({
        message: rosterNote.trim(),
        duration_min: durationMin,
        intensity_preset: intensity,
        include_buildup: includeBuildup,
      });
      notify(
        res.enqueued
          ? t("session.rosterStarted", { count: res.enqueued })
          : t("session.rosterPlanned"),
        "ok",
      );
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    } finally {
      setRosterBusy(false);
    }
  };

  const detail = rosterActive
    ? t("session.detail.roster")
    : snap.auto_running
      ? snap.playback_active
        ? t("session.detail.autoPlayback")
        : t("session.detail.auto")
      : snap.playback_active
        ? t("session.detail.playback")
        : t("session.detail.stopped");

  return (
    <TopbarMenu
      label={t("session.menuLabel")}
      connected={sessionActive}
      detail={detail}
      badge={
        sessionActive ? (
          <span className="menu-badge">
            {rosterActive
              ? t("session.badge.roster")
              : snap.auto_running && snap.playback_active
                ? t("session.badge.autoPlayback")
                : snap.auto_running
                  ? t("session.badge.auto")
                  : t("session.badge.playback")}
          </span>
        ) : null
      }
      align="right"
    >
      <div className="menu-panel-section">
        <span className="section-label">{t("session.roster.title")}</span>
        <p className="hint menu-hint">{t("session.roster.hint")}</p>
        {snap.emergency_stop && (
          <p className="hint roster-estop-hint">{t("session.roster.estopHint")}</p>
        )}
        <label className="field menu-field">
          <span>{t("session.roster.duration")}</span>
          <input
            type="number"
            min={5}
            max={120}
            step={5}
            value={durationMin}
            onChange={(e) => setDurationMin(Number(e.target.value) || 15)}
          />
        </label>
        <span className="field-label">{t("session.roster.intensity")}</span>
        <div className="segment-row" role="group" aria-label={t("session.roster.intensity")}>
          {intensityOptions.map((opt) => (
            <button
              key={opt.id}
              type="button"
              className={`segment-btn${intensity === opt.id ? " active" : ""}`}
              onClick={() => setIntensity(opt.id)}
            >
              {opt.label}
            </button>
          ))}
        </div>
        <label className="check-label session-wait-msg">
          <input
            type="checkbox"
            checked={includeBuildup}
            onChange={(e) => setIncludeBuildup(e.target.checked)}
          />
          {t("session.roster.buildup")}
        </label>
        <label className="field menu-field">
          <span>{t("session.roster.optionalNote")}</span>
          <input
            type="text"
            placeholder={t("session.roster.notePlaceholder")}
            value={rosterNote}
            onChange={(e) => setRosterNote(e.target.value)}
          />
        </label>
        <button
          type="button"
          className="btn btn-sm btn-primary roster-start-btn"
          disabled={rosterBusy}
          onClick={startRosterSession}
        >
          {rosterBusy ? t("session.roster.generating") : t("session.roster.start")}
        </button>
        {rosterActive && snap.roster_session && (
          <p className="hint roster-active-hint">
            {t("session.roster.playing", {
              min: snap.roster_session.duration_min ?? "?",
              intensity: String(snap.roster_session.intensity_preset ?? intensity),
            })}
          </p>
        )}
      </div>

      <div className="menu-panel-section">
        <span className="section-label">{t("session.autoPlayback.title")}</span>
        <p className="hint menu-hint">{t("session.autoPlayback.hint")}</p>
        <div className="menu-btn-grid">
          <button
            type="button"
            className="btn btn-sm btn-ghost"
            onClick={() => wrap(() => api.startAuto(), t("session.autoPlayback.autoOn"))}
          >
            {t("session.autoPlayback.auto")}
          </button>
          <button
            type="button"
            className="btn btn-sm btn-ghost"
            onClick={() => wrap(() => api.stopAuto(), t("session.autoPlayback.autoOff"))}
          >
            {t("session.autoPlayback.stopAuto")}
          </button>
          <button
            type="button"
            className="btn btn-sm btn-ghost"
            onClick={() => wrap(() => api.startPlayback(), t("session.autoPlayback.playbackOn"))}
          >
            {t("session.autoPlayback.playback")}
          </button>
          <button
            type="button"
            className="btn btn-sm btn-ghost"
            onClick={() => wrap(() => api.refillPlayback(), t("session.autoPlayback.refillDone"))}
          >
            {t("session.autoPlayback.refill")}
          </button>
          <button
            type="button"
            className="btn btn-sm btn-ghost"
            onClick={() =>
              wrap(() => api.clearQueueAndRefill(), t("session.autoPlayback.clearDone"))
            }
          >
            {t("session.autoPlayback.clearQueue")}
          </button>
        </div>
        <div className="status-chips session-chips">
          <span className={`chip${snap.auto_running ? " on" : ""}`}>
            {t("session.autoPlayback.auto")}
          </span>
          <span className={`chip${snap.playback_active ? " on" : ""}`}>
            {t("session.autoPlayback.playback")}
          </span>
          {rosterActive && <span className="chip on">{t("session.badge.roster")}</span>}
        </div>
        <label className="check-label session-wait-msg">
          <input
            type="checkbox"
            checked={waitFirstMsg}
            onChange={async (e) => {
              const on = e.target.checked;
              setWaitFirstMsg(on);
              try {
                const s = await api.getSettings();
                const autodom = (s.autodom ?? {}) as Record<string, unknown>;
                await api.saveSettings({
                  autodom: {
                    ...autodom,
                    wait_for_user_message: on,
                  },
                });
                notify(
                  on ? t("session.waitFirstOn") : t("session.waitFirstOff"),
                  "ok",
                );
              } catch (err) {
                notify(err instanceof Error ? err.message : t("common.error"), "error");
              }
            }}
          />
          {t("session.waitFirst")}
        </label>
      </div>
    </TopbarMenu>
  );
}
