// The vibrator's device page: Connection, Output and Test. The level range
// shapes motion beneath the live intensity; it never raises the intensity.
import type { VibrationSettings } from "../api/types";
import { RangeSlider } from "../components/RangeSlider";
import { SegmentedChoice } from "../components/SetpointControls";
import { t } from "../i18n";
import { OptionRow, SLIDER_SAVE_MS, useLiveSetting, useSaveDeviceOptions } from "./fields";
import { IntifaceConnectionRow } from "./HandyPage";
import { IntifaceDeviceChoice } from "./IntifaceDeviceChoice";
import { vibratorTest, vibratorTestAvailable } from "./actions";
import type { DeviceSectionProps } from "./types";

/** Mirrors config.DefaultVibrationSettings: follow speed, start low. */
export const DEFAULT_VIBRATION_SETTINGS: VibrationSettings = {
  follow: "speed",
  depth_end: "base",
  minimum_percent: 0,
  ceiling_percent: 40,
  timing_offset_ms: 0,
};

export function VibratorConnectionSection({ ctx }: DeviceSectionProps) {
  const leased = Boolean(ctx.intiface.status.connected && ctx.intiface.leases?.includes("vibrator"));
  return (
    <div className="form-rows device-rows">
      <IntifaceConnectionRow ctx={ctx} />
      <div className="form-row form-row-plain">
        <IntifaceDeviceChoice ctx={ctx} device="vibrator" />
        {leased && ctx.intiface.ping_watchdog === false && (
          <p className="hint-block">{t("The Intiface keepalive timeout is off, so if MagicHandy stops responding only a closed connection stops the vibrator.")}</p>
        )}
      </div>
    </div>
  );
}

function followHint(follow: VibrationSettings["follow"]): string {
  if (follow === "depth") return t("The vibration is strongest at the chosen end of each stroke.");
  if (follow === "steady") return t("A constant level while motion plays.");
  return t("Faster strokes vibrate harder.");
}

export function VibratorOutputSection({ ctx }: DeviceSectionProps) {
  const vibration = { ...DEFAULT_VIBRATION_SETTINGS, ...ctx.settings?.vibration };
  const save = useSaveDeviceOptions();
  const locked = ctx.locked || !ctx.canConfigureHost;
  const [follow, setFollow] = useLiveSetting(vibration.follow, (follow) => save({ vibration: { follow } }));
  const [depthEnd, setDepthEnd] = useLiveSetting(vibration.depth_end, (depth_end) => save({ vibration: { depth_end } }));
  const [range, setRange] = useLiveSetting({ min: vibration.minimum_percent, max: vibration.ceiling_percent },
    (value) => save({ vibration: { minimum_percent: value.min, ceiling_percent: value.max } }), SLIDER_SAVE_MS);
  return (
    <div className="form-rows device-rows">
      <OptionRow label={t("Follows")} hint={followHint(follow)}>
        <SegmentedChoice className="device-segmented" label={t("Follows")} value={follow} disabled={locked}
          options={[
            { value: "speed", label: t("Speed") },
            { value: "depth", label: t("Depth") },
            { value: "steady", label: t("Steady") },
          ]}
          onChange={setFollow} />
      </OptionRow>
      {follow === "depth" && (
        <OptionRow label={t("Strongest at")}>
          <SegmentedChoice className="device-segmented" label={t("Strongest at")} value={depthEnd} disabled={locked}
            options={[{ value: "base", label: t("Base") }, { value: "tip", label: t("Tip") }]}
            onChange={setDepthEnd} />
        </OptionRow>
      )}
      <OptionRow label={t("Level range")} hint={t("Motion drives the vibrator between these. The live intensity starts at 0 on every connection, you raise it in the device panel, and it never passes the ceiling. The model never changes it.")}>
        <RangeSlider label={t("Minimum to ceiling")} floor={0} ceil={100} minValue={range.min} maxValue={range.max} disabled={locked} onChange={setRange} />
      </OptionRow>
    </div>
  );
}

export function VibratorTestSection({ ctx }: DeviceSectionProps) {
  return (
    <div className="form-rows device-rows">
      <OptionRow label={t("Test vibration")} hint={t("One second at the lower of the intensity and 25%, only while motion is paused. Choose a device and raise its intensity first.")}>
        <button type="button" className="btn btn-start" disabled={!vibratorTestAvailable(ctx)} onClick={() => vibratorTest(ctx)}>{t("Test")}</button>
      </OptionRow>
    </div>
  );
}
