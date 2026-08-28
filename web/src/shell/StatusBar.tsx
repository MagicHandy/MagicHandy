import { useState } from "react";
import { t, translateKnown } from "../i18n";
// Compact status readouts, run timer, mini visualizer, and the shell-level
// disclosures. Motion controls remain in their routed workspaces.
import { api, ApiError } from "../api/client";
import { MotionVisualizer } from "../components/MotionVisualizer";
import { useAppState, useToast } from "../state/app-state";
import { stopAllAudioPlayback } from "../util/audio";
import { formatClock } from "../util/format";
import { ConnectionManager } from "./ConnectionManager";
import { NotificationCenter } from "./NotificationCenter";
import { ClockIcon, TakeControlIcon } from "./icons";
import type { AuthenticationStatus } from "../api/types";
import { ControlIdentitySelector } from "./ControlIdentitySelector";

type ShellMenu = "connection" | "notifications" | "identity" | null;

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
  const { backendOnline, motion, readOnly, refresh, state } = useAppState();
  const { show } = useToast();
  const [openMenu, setOpenMenu] = useState<ShellMenu>(null);
  const [takingControl, setTakingControl] = useState(false);
  const engine = motion?.engine;
  const awaitingState = state == null;
  const handoffInProgress = Boolean(state?.controller?.takeover_in_progress);

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

  async function takeControl() {
    if (takingControl || handoffInProgress || !backendOnline) return;
    if (!window.confirm(t("Take control of MagicHandy? Active motion, Autopilot, video synchronization, speech playback, and queued voice work will stop before this tab becomes the controller."))) {
      return;
    }

    setTakingControl(true);
    stopAllAudioPlayback();
    window.dispatchEvent(new Event("magichandy:emergency-stop"));
    try {
      const response = await api.takeControl();
      await refresh();
      show(
        response.warning
          ? t("Control transferred, but physical Stop could not be confirmed.")
          : t("This tab now controls MagicHandy."),
        response.warning ? "warning" : "success",
      );
    } catch (error) {
      show(error instanceof ApiError ? translateKnown(error.message) : t("Control transfer failed."), "error");
    } finally {
      setTakingControl(false);
    }
  }

  return (
    <div className="status-bar" role="region" aria-label={t("Status")}>
      <span className="status-readout">
        <span className="status-dot" data-state={phaseState} />
        <span className="status-text">{phaseLabelIsUserAuthored ? phaseLabel : translateKnown(phaseLabel)}</span>
      </span>
      <span className="status-divider" aria-hidden="true" />
      <span className="status-readout">
        <span className="status-dot" data-state={coreState} />
        <span className="status-text">{translateKnown(coreLabel)}</span>
      </span>
      {state && (readOnly ? (
        <button
          type="button"
          className="status-readout status-readout-controller status-controller-action"
          title={handoffInProgress || takingControl ? t("Controller handoff in progress") : t("Read-only client. Take control.")}
          aria-label={handoffInProgress || takingControl ? t("Controller handoff in progress") : t("Take control")}
          aria-busy={takingControl}
          disabled={!backendOnline || handoffInProgress || takingControl}
          onClick={() => void takeControl()}
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
          <span className="status-text">{t("controller: you")}</span>
        </span>
      ))}
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
      <span className="status-spacer" />
      <MotionVisualizer motion={motion} mini />
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
      <ConnectionManager
        open={openMenu === "connection"}
        restoreFocusOnClose={openMenu === null}
        onOpenChange={(open) => setOpenMenu(open ? "connection" : null)}
      />
    </div>
  );
}
