import { useTranslation } from "react-i18next";
import type { StatusSnapshot } from "../api/types";
import { StatusChip } from "./StatusChip";

export function SceneStatusCard({
  snap,
  dense,
}: {
  snap: StatusSnapshot | null;
  dense?: boolean;
}) {
  const { t } = useTranslation();
  if (!snap) return null;

  const phase = snap.phase ?? "—";
  const pose =
    snap.active_pose && snap.active_pose !== "none"
      ? (snap.pose_label ?? snap.active_pose)
      : null;
  const planned = snap.phase_planned_duration_sec;
  const elapsed = snap.phase_elapsed_sec ?? 0;
  const max = planned ?? snap.phase_max_sec ?? 0;
  const remaining = snap.phase_remaining_sec ?? 0;
  const pct =
    snap.phase_progress_pct ??
    (max > 0 ? Math.min(100, (elapsed / max) * 100) : 0);
  const buffer = snap.buffer_remaining_sec ?? snap.buffer_sec ?? 0;

  return (
    <section
      className={`glass scene-status-card${dense ? " scene-status-card--dense" : ""}`}
      aria-label={t("session.scene.aria")}
    >
      <div className="scene-status-head">
        <div>
          <span className="section-label">
            {dense ? t("session.scene.label") : t("session.scene.live")}
          </span>
          <div className="scene-status-phase">{phase}</div>
          {pose && (
            <p className="scene-status-pose">
              <strong>{pose}</strong>
              {snap.pose_detail ? ` · ${snap.pose_detail}` : ""}
            </p>
          )}
        </div>
        {!dense && (
          <div className="scene-status-chips">
            {snap.phase_locked && (
              <StatusChip
                label={t("session.scene.locked")}
                variant="accent"
                title={t("session.scene.lockedTitle")}
              />
            )}
            {snap.phase_ready_to_advance && (
              <StatusChip label={t("session.scene.advance")} variant="success" pulse />
            )}
            {snap.autospeak_enabled && (
              <StatusChip
                label={
                  snap.autospeak_scheduled
                    ? t("layout.topbar.autospeakScheduled")
                    : t("layout.topbar.autospeak")
                }
                variant="muted"
                pulse={snap.autospeak_scheduled}
              />
            )}
          </div>
        )}
      </div>

      <div className="scene-status-meters">
        <div className="scene-meter">
          <div className="scene-meter-label">
            <span>{t("session.scene.phaseTime")}</span>
            <span className="mono">
              {max > 0
                ? `${Math.min(elapsed, max).toFixed(0)}s / ${max}s`
                : "—"}
            </span>
          </div>
          <div className="buffer-bar" role="progressbar" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100}>
            <div
              className={`buffer-fill${remaining <= 5 && max > 0 ? " buffer-fill--urgent" : ""}`}
              style={{ width: `${pct}%` }}
            />
          </div>
        </div>
        <div className="scene-meter">
          <div className="scene-meter-label">
            <span>{t("session.scene.motionBuffer")}</span>
            <span className="mono">{buffer.toFixed(0)}s</span>
          </div>
          <div className="buffer-bar buffer-bar--thin">
            <div
              className="buffer-fill buffer-fill--buffer"
              style={{
                width: `${Math.min(100, (buffer / Math.max(buffer, 30)) * 100)}%`,
              }}
            />
          </div>
        </div>
      </div>

      {!dense && (
        <div className="scene-status-stats">
          <Stat label={t("layout.topbar.queue")} value={`${snap.queue_blocks ?? 0} bl`} />
          <Stat
            label={t("session.scene.refill")}
            value={snap.planner_refill_busy ? t("session.scene.active") : t("session.scene.idle")}
            warn={snap.planner_refill_busy}
          />
          <Stat
            label={t("session.scene.ai")}
            value={
              snap.chat_pending || snap.planner_busy
                ? snap.planner_busy_source ?? t("session.scene.busy")
                : t("session.scene.idle")
            }
            accent={snap.chat_pending || snap.planner_busy}
          />
          <Stat label={t("layout.topbar.intensity")} value={`${Math.round(snap.intensity)}%`} />
        </div>
      )}
    </section>
  );
}

function Stat({
  label,
  value,
  accent,
  warn,
}: {
  label: string;
  value: string;
  accent?: boolean;
  warn?: boolean;
}) {
  return (
    <div
      className={`scene-stat${accent ? " scene-stat--accent" : ""}${warn ? " scene-stat--warn" : ""}`}
    >
      <span className="scene-stat-label">{label}</span>
      <span className="scene-stat-value">{value}</span>
    </div>
  );
}
