import { t } from "../i18n";
import type { VibrationSettings } from "../api/types";

/** Mirrors config.DefaultVibrationSettings: follow speed, start low. */
export const DEFAULT_VIBRATION_SETTINGS: VibrationSettings = {
  follow: "speed",
  depth_end: "base",
  minimum_percent: 0,
  ceiling_percent: 40,
  timing_offset_ms: 0,
};

interface VibratorSettingsProps {
  vibration: VibrationSettings;
  locked: boolean;
  patch: (next: Partial<VibrationSettings>) => void;
}

/**
 * Settings > Device > Vibrator. The profile shapes the level beneath the
 * user's live intensity; it never raises the intensity itself.
 */
export function VibratorSettings({ vibration, locked, patch }: VibratorSettingsProps) {
  const ceiling = vibration.ceiling_percent;
  return (
    <div className="group">
      <h3 className="group-title">{t("Vibrator")}</h3>
      <label className="field"><span className="label">{t("Follows")}</span>
        <select value={vibration.follow} disabled={locked} onChange={(event) => patch({ follow: event.target.value as VibrationSettings["follow"] })}>
          <option value="speed">{t("Stroke speed")}</option>
          <option value="depth">{t("Stroke depth")}</option>
          <option value="steady">{t("Steady while playing")}</option>
        </select>
      </label>
      <p className="hint-block">{vibration.follow === "depth"
        ? t("The vibration is strongest at the chosen end of each stroke.")
        : vibration.follow === "steady"
          ? t("A constant level while motion plays.")
          : t("Faster strokes vibrate harder.")}</p>
      {vibration.follow === "depth" && (
        <label className="field"><span className="label">{t("Strongest at")}</span>
          <select value={vibration.depth_end} disabled={locked} onChange={(event) => patch({ depth_end: event.target.value as VibrationSettings["depth_end"] })}>
            <option value="base">{t("Base")}</option>
            <option value="tip">{t("Tip")}</option>
          </select>
        </label>
      )}
      <div className="vibrator-fields">
        <label className="field"><span className="label">{t("Minimum intensity (%)")}</span>
          <input type="number" min={0} max={ceiling} step={1} value={vibration.minimum_percent} disabled={locked}
            onChange={(event) => patch({ minimum_percent: clampPercent(event.target.valueAsNumber, ceiling) })} />
        </label>
        <label className="field"><span className="label">{t("Intensity ceiling (%)")}</span>
          <input type="number" min={0} max={100} step={1} value={ceiling} disabled={locked}
            onChange={(event) => {
              const next = clampPercent(event.target.valueAsNumber, 100);
              patch({ ceiling_percent: next, minimum_percent: Math.min(vibration.minimum_percent, next) });
            }} />
        </label>
      </div>
      <p className="hint-block">{t("The live intensity starts at 0 on every connection, and you raise it in the connection manager. It never goes above this ceiling, and the model never changes it.")}</p>
      <p className="hint-block">{t("The minimum is the lowest level while motion drives the vibrator. Raise it if the motors do not start at low levels.")}</p>
      <label className="field"><span className="label">{t("Timing offset (ms)")}</span>
        <input type="number" min={-300} max={300} step={10} value={vibration.timing_offset_ms} disabled={locked}
          onChange={(event) => patch({ timing_offset_ms: Math.max(-300, Math.min(300, Math.round(event.target.valueAsNumber || 0))) })} />
      </label>
      <p className="hint-block">{t("Positive values delay the vibrator relative to motion and video.")}</p>
    </div>
  );
}

function clampPercent(value: number, high: number): number {
  return Number.isFinite(value) ? Math.max(0, Math.min(high, Math.round(value))) : 0;
}
