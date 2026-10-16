import { useState } from "react";
import { api, ApiError } from "../api/client";
import { t, translateKnown } from "../i18n";
import { useAppState, useToast } from "../state/app-state";
import { stopAllAudioPlayback } from "../util/audio";
import { confirmThen } from "../util/confirm";

/**
 * Take control from a read-only tab. The backend runs global Emergency Stop
 * before transferring ownership, so the person confirms first.
 */
export function useTakeControl() {
  const { backendOnline, refresh, state } = useAppState();
  const { show } = useToast();
  const [takingControl, setTakingControl] = useState(false);
  const handoffInProgress = Boolean(state?.controller?.takeover_in_progress);

  async function transferControl() {
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

  function takeControl() {
    if (takingControl || handoffInProgress || !backendOnline) return;
    confirmThen(
      t("Take control of MagicHandy? Active motion, Autopilot, video synchronization, speech playback, and queued voice work will stop before this tab becomes the controller."),
      { confirmLabel: t("Take control") },
      () => void transferControl(),
    );
  }

  return { takeControl, takingControl, handoffInProgress };
}
