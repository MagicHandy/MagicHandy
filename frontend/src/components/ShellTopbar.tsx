import { useTranslation } from "react-i18next";
import type { StatusSnapshot } from "../api/types";
import { HandyConnectionMenu } from "./HandyConnectionMenu";
import { SessionControls } from "./SessionControls";
import { StatusChip } from "./StatusChip";
import { TopbarMenu } from "./TopbarMenu";

export function ShellTopbar({
  snap,
  emergency,
  onStop,
  onRecheckOllama,
  onRefresh,
}: {
  snap: StatusSnapshot;
  emergency?: boolean;
  onStop: () => void;
  onRecheckOllama: () => void;
  onRefresh: () => Promise<unknown>;
}) {
  const { t } = useTranslation();
  const buffer = snap.buffer_remaining_sec ?? snap.buffer_sec ?? 0;
  const lowBuffer = buffer < 10;

  return (
    <header className="topbar topbar--v2">
      <div className="topbar-zone topbar-zone--scene">
        <StatusChip label={snap.phase} variant="accent" />
        {snap.active_pose && snap.active_pose !== "none" && (
          <StatusChip
            label={snap.pose_label ?? snap.active_pose}
            title={snap.pose_detail ?? undefined}
          />
        )}
        {snap.phase_locked && (
          <StatusChip label={t("layout.topbar.phaseLocked")} variant="warn" />
        )}
        {snap.phase_ready_to_advance && (
          <StatusChip label={t("layout.topbar.advancePhase")} variant="success" pulse />
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

      <div className="topbar-zone topbar-zone--metrics">
        <div className={`topbar-kpi${lowBuffer ? " topbar-kpi--warn" : ""}`}>
          <span className="topbar-kpi-label">{t("layout.topbar.buffer")}</span>
          <span className="topbar-kpi-value mono">{buffer.toFixed(0)}s</span>
        </div>
        <div className="topbar-kpi">
          <span className="topbar-kpi-label">{t("layout.topbar.queue")}</span>
          <span className="topbar-kpi-value mono">{snap.queue_blocks ?? 0}</span>
        </div>
        <div className="topbar-kpi" title={t("layout.topbar.intensityTitle")}>
          <span className="topbar-kpi-label">{t("layout.topbar.intensity")}</span>
          <span className="topbar-kpi-value mono">{Math.round(snap.intensity)}%</span>
        </div>
      </div>

      <div className="topbar-zone topbar-zone--actions">
        <div className="topbar-menus">
          <TopbarMenu
            label={t("layout.topbar.ollama")}
            connected={Boolean(snap.ollama_connected)}
            detail={snap.ollama_error ?? snap.ollama_model}
            align="left"
          >
            <div className="menu-panel-section">
              <span className="section-label">{t("layout.topbar.localLlm")}</span>
              <p className="hint menu-hint">
                <span className="mono">{snap.ollama_model}</span>
              </p>
              <button
                type="button"
                className="btn btn-sm btn-primary"
                onClick={onRecheckOllama}
              >
                {t("layout.topbar.testConnection")}
              </button>
            </div>
          </TopbarMenu>
          <HandyConnectionMenu snap={snap} onRefresh={onRefresh} />
          <SessionControls snap={snap} />
        </div>
        <button
          type="button"
          className={`btn-stop topbar-stop${emergency ? " active" : ""}`}
          onClick={onStop}
          title={t("layout.emergencyStop")}
        >
          {t("common.stop")}
        </button>
      </div>
    </header>
  );
}
