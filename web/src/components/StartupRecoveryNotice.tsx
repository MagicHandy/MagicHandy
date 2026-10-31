import { useRef, useState } from "react";
import { api } from "../api/client";
import { t, translateKnown } from "../i18n";
import { useAppState, useMotionState, useToast } from "../state/app-state";
import "../styles/startup-recovery.css";

export function StartupRecoveryNotice({ announce = false }: { announce?: boolean }) {
  const motion = useMotionState();
  const { backendOnline, readOnly, refresh } = useAppState();
  const { show } = useToast();
  const [pending, setPending] = useState(false);
  const inFlight = useRef(false);
  const recovery = motion?.engine?.startup_recovery;
  if (!recovery) return null;
  const busy = pending || recovery.resetting;

  async function reset() {
    if (!recovery || !backendOnline || readOnly || busy || inFlight.current) return;
    inFlight.current = true;
    setPending(true);
    try {
      await api.resetMotionPosition(recovery.id);
      show(t("Device stopped and position readings checked. The next start checks arrival again."));
    } catch (error) {
      show(error instanceof Error ? translateKnown(error.message) : t("Could not recheck the device position."), "error");
    } finally {
      inFlight.current = false;
      setPending(false);
      void refresh();
    }
  }

  return <section className="startup-recovery" aria-label={t("Position needs a recheck")}>
    <strong>{t("Position needs a recheck")}</strong>
    <p role={announce ? "status" : undefined}>{recovery.kind === "invalid_telemetry" ? t("The device returned invalid position or calibration readings.")
      : recovery.kind === "moving_after_stop" ? t("The device was still moving after Stop.")
        : recovery.kind === "unsafe_geometry" ? t("The device reported a position outside its calibrated travel.")
          : t("The device did not settle at the requested starting position.")}</p>
    <button type="button" className="btn btn-secondary btn-sm" disabled={!backendOnline || readOnly || busy} onClick={() => void reset()}>
      {busy ? t("Rechecking…") : t("Reset and recheck")}
    </button>
    <small>{readOnly ? t("Use the controlling tab to reset the position check.")
      : t("Stops the device and refreshes its position. Playback stays stopped.")}</small>
  </section>;
}
