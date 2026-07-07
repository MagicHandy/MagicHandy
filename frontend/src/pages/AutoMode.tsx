import { useTranslation } from "react-i18next";
import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { StatusSnapshot } from "../api/types";
import { useToast } from "../hooks/useToast";

export function AutoMode() {
  const { t } = useTranslation();
  const { toast, notify } = useToast();
  const [snap, setSnap] = useState<StatusSnapshot | null>(null);

  useEffect(() => {
    let alive = true;
    const load = async () => {
      try {
        const s = await api.getStatus();
        if (alive) setSnap(s);
      } catch {
        /* */
      }
    };
    load();
    const id = setInterval(load, 1000);
    return () => {
      alive = false;
      clearInterval(id);
    };
  }, []);

  const wrap = async (fn: () => Promise<unknown>, ok: string) => {
    try {
      await fn();
      notify(ok, "ok");
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  return (
    <div>
      <h2>{t("autoMode.title")}</h2>
      <p className="muted">{t("autoMode.hint")}</p>

      <div className="panel">
        <div style={{ fontSize: "2rem", fontWeight: 600 }}>
          {snap?.phase ?? "—"}
        </div>
        <p className="muted">
          {t("autoMode.status", {
            auto: snap?.auto_running ? "ON" : "OFF",
            playback: snap?.playback_active ? "ON" : "OFF",
            refill: snap?.planner_refill_busy
              ? t("dashboard.refillBusy")
              : t("dashboard.refillFree"),
          })}
        </p>
        <p>{t("autoMode.buffer", { sec: snap?.buffer_sec ?? 0 })}</p>
      </div>

      <div className="row">
        <button
          type="button"
          className="btn"
          onClick={() => wrap(() => api.startAuto(), t("autoMode.started"))}
        >
          {t("autoMode.startAuto")}
        </button>
        <button
          type="button"
          className="btn warning"
          onClick={() => wrap(() => api.stopAuto(), t("autoMode.stopping"))}
        >
          {t("autoMode.stopAuto")}
        </button>
        <button
          type="button"
          className="btn secondary"
          onClick={() => wrap(() => api.startPlayback(), t("autoMode.playbackActive"))}
        >
          {t("autoMode.playbackOnly")}
        </button>
        <button
          type="button"
          className="btn secondary"
          onClick={() => wrap(() => api.stopPlayback(), t("autoMode.playbackStopped"))}
        >
          {t("autoMode.stopPlayback")}
        </button>
        <button
          type="button"
          className="btn secondary"
          onClick={() =>
            wrap(() => api.refillPlayback(), t("autoMode.refillRequested"))
          }
        >
          {t("autoMode.refillBuffer")}
        </button>
      </div>

      {toast && (
        <div className={`toast ${toast.kind === "error" ? "error" : "ok"}`}>
          {toast.text}
        </div>
      )}
    </div>
  );
}
