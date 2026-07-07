import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api } from "../api/client";
import type { StatusSnapshot } from "../api/types";

export function Dashboard() {
  const { t } = useTranslation();
  const [snap, setSnap] = useState<StatusSnapshot | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let alive = true;
    const load = async () => {
      try {
        const data = await api.getStatus();
        if (alive) {
          setSnap(data);
          setError(null);
        }
      } catch (e) {
        if (alive) setError(e instanceof Error ? e.message : t("common.error"));
      }
    };
    load();
    const id = setInterval(load, 1500);
    return () => {
      alive = false;
      clearInterval(id);
    };
  }, [t]);

  if (error) {
    return (
      <div>
        <h2>{t("dashboard.title")}</h2>
        <p className="muted">{t("dashboard.apiUnavailable", { error })}</p>
      </div>
    );
  }

  if (!snap) return <p className="muted">{t("common.loading")}</p>;

  const cards = [
    {
      label: t("dashboard.card.intiface"),
      value: snap.intiface_connected
        ? t("dashboard.connected")
        : t("dashboard.disconnected", {
            error: snap.intiface_error ? ` (${snap.intiface_error})` : "",
          }),
    },
    {
      label: t("dashboard.card.device"),
      value: `${snap.use_mock ? "Mock" : "Live"} — ${snap.device_label} (${snap.device_connected ? "on" : "off"})`,
    },
    {
      label: t("dashboard.card.persona"),
      value: `${snap.persona_name} (${snap.persona_id})`,
    },
    {
      label: t("dashboard.card.ollama"),
      value: `${snap.ollama_model} @ ${snap.ollama_url}`,
    },
    {
      label: t("dashboard.card.mode"),
      value: `${snap.operation_mode} · ${t("session.scene.phaseTime")} ${snap.phase} · auto ${snap.auto_running ? "on" : "off"}`,
    },
    { label: t("dashboard.card.intensity"), value: `${snap.intensity.toFixed(0)}%` },
    {
      label: t("dashboard.card.range"),
      value: `${snap.min_position} – ${snap.max_position}`,
    },
    {
      label: t("dashboard.card.buffer"),
      value: `${snap.buffer_sec}s${snap.emergency_stop ? t("dashboard.stopActive") : ""}`,
    },
  ];

  return (
    <div>
      <h2>{t("dashboard.title")}</h2>
      {snap.emergency_stop && (
        <p className="badge stop" style={{ marginBottom: "0.75rem" }}>
          {t("dashboard.emergencyStop")}
        </p>
      )}
      <div className="card-grid">
        {cards.map((c) => (
          <div key={c.label} className="card">
            <div className="label">{c.label}</div>
            <div className="value">{c.value}</div>
          </div>
        ))}
      </div>

      <div className="panel">
        <h3 style={{ marginTop: 0, fontSize: "1rem" }}>{t("dashboard.queuePreview")}</h3>
        {(snap.queue_preview?.length ?? 0) === 0 ? (
          <p className="muted">{t("dashboard.queueEmpty")}</p>
        ) : (
          <ul style={{ margin: 0, paddingLeft: "1.2rem" }}>
            {snap.queue_preview.map((item) => (
              <li key={item.block_id} className="muted">
                {item.block_id.slice(0, 24)}… · {item.duration_ms}ms @{" "}
                {item.intensity}%
              </li>
            ))}
          </ul>
        )}
        <p className="muted" style={{ marginTop: "0.75rem" }}>
          {t("dashboard.playbackFooter", {
            playback: snap.playback_active
              ? t("dashboard.playbackActive")
              : t("dashboard.playbackStopped"),
            count: snap.queue_blocks ?? 0,
            refill: snap.planner_refill_busy
              ? t("dashboard.refillBusy")
              : t("dashboard.refillFree"),
          })}
        </p>
      </div>
    </div>
  );
}
