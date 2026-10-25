import { useState } from "react";
import { t, translateKnown } from "../i18n";
// Compact status readouts, run timer, mini visualizer, and the shell-level
// disclosures. Motion controls remain in their routed workspaces.
import { LiveVibrationReadout, showsVibration } from "../components/VibrationReadout";
import { MotionVisualizer } from "../components/MotionVisualizer";
import { useAppState, useMotionState } from "../state/app-state";
import { formatClock } from "../util/format";
import { DevicePanel } from "./DevicePanel";
import { NotificationCenter } from "./NotificationCenter";
import { useTakeControl } from "./useTakeControl";
import { ClockIcon, TakeControlIcon } from "./icons";
import type { AuthenticationStatus } from "../api/types";
import { ControlIdentitySelector } from "./ControlIdentitySelector";

type ShellMenu = "devices" | "notifications" | "identity" | null;

export function StatusBar({
  authenticationLocked = false,
  authenticationStatus = null,
  onLogout,
  onSelectControlIdentity,
}: {
  authenticationLocked?: boolean;
  authenticationStatus?: AuthenticationStatus | null;
  onLogout?: () => Promise<void>;
  onSelectControlIdentity?: (accountID: string) => Promise<void>;
}) {
  const { backendOnline, readOnly, state } = useAppState();
  const motion = useMotionState();
  const [openMenu, setOpenMenu] = useState<ShellMenu>(null);
  const { takeControl, takingControl, handoffInProgress } = useTakeControl();
  const engine = motion?.engine;
  const awaitingState = state == null;

  // Voice earns a readout only when it is enabled and unhealthy: a crashed
  // worker, or speak-replies promised while the TTS worker cannot deliver.
  // Healthy or disabled voice stays out of the bar entirely.
  const voiceSettings = state?.settings?.voice;
  const voiceWorkers = state?.voice?.workers;
  const voiceCrashed = Boolean(voiceSettings?.enabled && (voiceWorkers?.tts?.state === "crashed" || voiceWorkers?.asr?.state === "crashed"));
  const speakNotReady = Boolean(
    voiceSettings?.enabled &&
      voiceSettings.speak_replies &&
      voiceSettings.tts_provider &&
      voiceSettings.tts_provider !== "none" &&
      !(voiceWorkers?.tts?.state === "running" && voiceWorkers?.tts?.model_state === "ready"),
  );
  if (authenticationLocked) {
    return (
      <div className="status-bar" role="region" aria-label={t("Status")}>
        <span className="status-readout">
          <span className="status-dot" data-state="warn" />
          <span className="status-text">{t("sign-in required")}</span>
        </span>
        <span className="status-divider" aria-hidden="true" />
        <span className="status-readout">
          <span className="status-dot" data-state="ok" />
          <span className="status-text">{t("core ok")}</span>
        </span>
        <span className="status-spacer" />
      </div>
    );
  }
  let phaseState = "idle";
  let phaseLabel = motion?.available === false ? "unavailable" : "idle";
  let phaseLabelIsUserAuthored = false;
  if (awaitingState) {
    phaseState = "pending";
    phaseLabel = "state pending";
  } else if (engine?.paused) {
    phaseState = "paused";
    phaseLabel = "paused";
  } else if (engine?.completing) {
    phaseState = "active";
    phaseLabel = "motion stopping";
  } else if (engine?.starting) {
    phaseState = "active";
    phaseLabel = "motion starting";
  } else if (engine?.running) {
    phaseState = "running";
    if (engine.target?.label) {
      phaseLabel = engine.target.label;
      phaseLabelIsUserAuthored = true;
    } else {
      phaseLabel = "running";
    }
  }
  const coreState = awaitingState && backendOnline ? "pending" : backendOnline ? "ok" : "error";
  const coreLabel = awaitingState && backendOnline ? "core starting" : backendOnline ? "core ok" : "core offline";

  return (
    <div className="status-bar" role="region" aria-label={t("Status")}>
      <div className="status-readouts">
        <span className="status-readout">
          <span className="status-dot" data-state={phaseState} />
          <span className="status-text">{phaseLabelIsUserAuthored ? phaseLabel : translateKnown(phaseLabel)}</span>
        </span>
        <span className="status-divider" aria-hidden="true" />
        {state?.motion_simulated && (
          <span className="status-readout" title={t("Simulation is active. Motion commands go to the simulator; connected devices will not move.")}>
            <span className="status-dot" data-state="warn" />
            <span className="status-text">{t("Motion simulator")}</span>
          </span>
        )}
        <span className="status-readout">
          <span className="status-dot" data-state={coreState} />
          <span className="status-text">{translateKnown(coreLabel)}</span>
        </span>
        {(voiceCrashed || speakNotReady) && (
          <span className="status-readout">
            <span className="status-dot" data-state={voiceCrashed ? "error" : "warn"} />
            <span className="status-text">{voiceCrashed ? t("voice crashed") : t("voice not ready")}</span>
          </span>
        )}
        <span className="status-divider" aria-hidden="true" />
        <span className="status-timer">
          <ClockIcon />
          <span className="value">{formatClock(engine?.running_ms)}</span>
        </span>
      </div>
      {state && (state.capabilities?.control === false ? (
        <span className="status-readout status-readout-observer" title={t("Ask the administrator for a control permission.")}><span className="status-dot" data-state="idle" /><span className="status-text">{t("Observer")}</span></span>
      ) : readOnly ? (
        <button
          type="button"
          className="status-readout status-readout-controller status-controller-action"
          title={handoffInProgress || takingControl ? t("Controller handoff in progress") : t("Read-only client. Take control.")}
          aria-label={handoffInProgress || takingControl ? t("Controller handoff in progress") : t("Take control")}
          aria-busy={takingControl}
          disabled={!backendOnline || handoffInProgress || takingControl}
          onClick={takeControl}
        >
          <span className="status-dot" data-state={handoffInProgress || takingControl ? "pending" : "warn"} />
          <TakeControlIcon size={16} className="icon" />
          <span className="status-text">{t("Take control")}</span>
        </button>
      ) : (
        <span
          className="status-readout status-readout-controller"
          title={t("This tab is the controller")}
          aria-label={t("This tab is the controller")}
        >
          <span className="status-dot" data-state="ok" />
          {/* Phones show the compact form: Take control's glyph and "You". */}
          <TakeControlIcon size={16} className="icon status-controller-glyph" />
          <span className="status-text status-controller-label">{t("controller: you")}</span>
          <span className="status-text status-controller-short" aria-hidden="true">{t("You")}</span>
        </span>
      ))}
      {showsVibration(state?.settings, motion) ? <LiveVibrationReadout mini /> : <MotionVisualizer motion={motion} mini device={{ owner: state?.settings?.device?.hsp_dispatch_owner, model: state?.settings?.motion?.handy_model }} />}
      {authenticationStatus?.authenticated && authenticationStatus.control_identities?.length && onLogout && onSelectControlIdentity ? (
        <ControlIdentitySelector
          identities={authenticationStatus.control_identities}
          open={openMenu === "identity"}
          restoreFocusOnClose={openMenu === null}
          onOpenChange={(open) => setOpenMenu(open ? "identity" : null)}
          onSelect={onSelectControlIdentity}
          onLogout={onLogout}
        />
      ) : null}
      <NotificationCenter
        open={openMenu === "notifications"}
        restoreFocusOnClose={openMenu === null}
        onOpenChange={(open) => setOpenMenu(open ? "notifications" : null)}
      />
      <DevicePanel
        open={openMenu === "devices"}
        restoreFocusOnClose={openMenu === null}
        onOpenChange={(open) => setOpenMenu(open ? "devices" : null)}
      />
    </div>
  );
}
