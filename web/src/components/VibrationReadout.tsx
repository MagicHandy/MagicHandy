import type { IntifaceTransportSnapshot, MotionInfo, PublicSettings } from "../api/types";
import { isHandy, playbackDevices } from "../api/devices";
import { useDeviceContext, useDeviceWatch } from "../devices/runtime";
import { t } from "../i18n";

export function showsVibration(settings: PublicSettings | undefined, motion: MotionInfo | null): boolean {
  const outputs = playbackDevices(settings?.device);
  return Boolean(motion?.engine?.target?.vibration_only) || (outputs.includes("vibrator") && !outputs.some(isHandy));
}

export function VibrationReadout({ vibrator, mini = false }: { vibrator?: IntifaceTransportSnapshot["vibrator"]; mini?: boolean }) {
  return <div className={`vibration-readout${mini ? " mini" : ""}`}>
    <span>{t("Vibrator")}</span>
    <div className="vibration-motors">
      {(vibrator?.motors ?? []).map((motor, index) => <div className="vibration-motor" key={motor.index}>
        <meter min={0} max={100} value={motor.level_percent ?? 0} aria-label={t("Motor {motor}: {percent}%", { motor: index + 1, percent: motor.level_percent ?? 0 })} />
        <output>{motor.level_percent ?? 0}%</output>
      </div>)}
      {!vibrator?.motors.length && <span>{t("Not connected")}</span>}
    </div>
    {!mini && <small>{t("Last commands sent to Intiface, not measured vibration.")}</small>}
  </div>;
}

export function LiveVibrationReadout({ mini = false }: { mini?: boolean }) {
  useDeviceWatch();
  const ctx = useDeviceContext();
  return <VibrationReadout vibrator={ctx.intiface.vibrator} mini={mini} />;
}
