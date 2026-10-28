// The vibrator's device page: Connection, Output and Test. Max is the one
// strength the person sets, the same setting as the device panel's; the model
// chooses the level under it when Model control is on (ADR 0061).
import { useId } from "react";
import { api } from "../api/client";
import type { VibrationEnvelope, VibrationSettings } from "../api/types";
import { SegmentedChoice } from "../components/SetpointControls";
import { t } from "../i18n";
import { OptionRow, SLIDER_SAVE_MS, useLiveSetting, useSaveDeviceOptions, ValueSlider } from "./fields";
import { IntifaceConnectionRow } from "./HandyPage";
import { IntifaceDeviceChoice } from "./IntifaceDeviceChoice";
import { chatModelSetsVibration, vibratorTest, vibratorTestAvailable } from "./actions";
import type { DeviceSectionProps } from "./types";

/** Mirrors config.DefaultVibrationSettings: follow speed, start low, model sets the level. */
export const DEFAULT_VIBRATION_SETTINGS: VibrationSettings = {
  follow: "speed",
  depth_end: "base",
  minimum_percent: 0,
  ceiling_percent: 40,
  timing_offset_ms: 0,
  level_control: "model",
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
  return t("Faster strokes vibrate harder, across your Handy speed range.");
}

/** What the model is doing with the vibrator now, in one short line. */
export function modelReadout(envelope: VibrationEnvelope | undefined, level: number, followsHandy: boolean): string {
  if (!envelope || envelope.mode === "follow") {
    if (envelope && envelope.max_percent > 0) return t("Following movement, up to {percent}% of max", { percent: envelope.max_percent });
    return followsHandy ? t("Following The Handy until the model chooses a level") : t("Following movement at {percent}% of max", { percent: level });
  }
  if (envelope.mode === "off") return t("Off");
  if (envelope.mode === "steady") return t("Steady at {percent}% of max", { percent: envelope.max_percent });
  const range = { min: envelope.min_percent, max: envelope.max_percent, seconds: envelope.period_seconds };
  return envelope.mode === "wave"
    ? t("Wave, {min}–{max}% of max, every {seconds} s", range)
    : t("Pulse, {min}–{max}% of max, every {seconds} s", range);
}

export function VibratorOutputSection({ ctx }: DeviceSectionProps) {
  const vibration = { ...DEFAULT_VIBRATION_SETTINGS, ...ctx.settings?.vibration };
  const save = useSaveDeviceOptions();
  const controlId = useId();
  const envelope = ctx.engineActive && vibration.level_control !== "manual" ? ctx.state?.motion?.engine?.target?.vibration : undefined;
  const independent = ctx.state?.settings?.model_routing?.chat?.features?.independent_vibration === true;
  const capable = chatModelSetsVibration(ctx);
  const modelSets = vibration.level_control !== "manual";
  const locked = ctx.locked || !ctx.canConfigureHost;
  const [follow, setFollow] = useLiveSetting(vibration.follow, (follow) => save({ vibration: { follow } }));
  const [depthEnd, setDepthEnd] = useLiveSetting(vibration.depth_end, (depth_end) => save({ vibration: { depth_end } }));
  // A lower Max takes the minimum down with it, so the pair stays valid.
  const [max, setMax] = useLiveSetting(vibration.ceiling_percent,
    (ceiling_percent) => save({ vibration: { ceiling_percent, minimum_percent: Math.min(vibration.minimum_percent, ceiling_percent) } }), SLIDER_SAVE_MS);
  const [minimum, setMinimum] = useLiveSetting(vibration.minimum_percent, (minimum_percent) => save({ vibration: { minimum_percent } }), SLIDER_SAVE_MS);
  const controlHint = !capable
    ? t("Your chat model cannot set vibration, so it follows movement at your max.")
    : independent
      ? t("Your chat model sets the level and can play its own patterns, as written. Off: vibration follows movement at your max.")
      : t("Your chat model sets the level as vibration follows movement. Off: vibration follows movement at your max.");
  return (
    <div className="form-rows device-rows">
      <OptionRow label={t("Max")} hint={t("The strongest the vibrator goes. The model and Test stay at or below it. Also in the device panel.")}>
        <ValueSlider label={t("Max")} hideLabel value={max} min={0} max={100} step={5} valueText={t("{n}%", { n: max })}
          disabled={locked} onChange={setMax} />
      </OptionRow>
      <OptionRow label={t("Model control")} labelId={controlId} hint={controlHint}>
        <span className="toggle">
          <input type="checkbox" role="switch" aria-labelledby={controlId} checked={capable && modelSets} disabled={locked || !capable}
            onChange={(event) => void save({ vibration: { level_control: event.target.checked ? "model" : "manual" } })} />
          <span className="track" aria-hidden="true" />
        </span>
        <span>{!capable || !modelSets ? t("Following movement at your max")
          : ctx.intiface.vibrator?.selected ? modelReadout(envelope, ctx.intiface.vibrator.model_level_percent, ctx.intiface.vibrator.level_source !== "model") : t("Following movement")}</span>
        {envelope && envelope.mode !== "follow" && (
          <button type="button" className="btn" disabled={ctx.locked} onClick={() => ctx.send(() => api.followVibration())}>{t("Follow movement")}</button>
        )}
      </OptionRow>
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
      <OptionRow label={t("Minimum while moving")} hint={t("Keeps a gentle level whenever movement plays. The model's own patterns set their own minimum.")}>
        <ValueSlider label={t("Minimum while moving")} hideLabel value={Math.min(minimum, max)} min={0} max={max} step={5}
          valueText={t("{n}%", { n: Math.min(minimum, max) })} disabled={locked || max === 0} onChange={setMinimum} />
      </OptionRow>
      {ctx.intiface.vibrator?.selected && (
        <OptionRow label={t("Motor output")} hint={t("Last commands sent to Intiface, not measured vibration.")}>
          <output>{ctx.intiface.vibrator.motors.map((motor, index) => t("Motor {motor}: {percent}%", { motor: index + 1, percent: motor.level_percent ?? 0 })).join(" · ")}</output>
        </OptionRow>
      )}
      <div className="form-row form-row-plain">
        <p className="hint-block">{t("Stop pauses the vibrator until you request motion or choose Resume now in the device panel. Your max and model control stay as they are.")}</p>
      </div>
    </div>
  );
}

export function VibratorTestSection({ ctx }: DeviceSectionProps) {
  return (
    <div className="form-rows device-rows">
      <OptionRow label={t("Test vibration")} hint={t("One second at the lower of your max and 25%, while playback is stopped or paused. Choose a device first.")}>
        <button type="button" className="btn btn-start" disabled={!vibratorTestAvailable(ctx)} onClick={() => vibratorTest(ctx)}>{t("Test")}</button>
      </OptionRow>
    </div>
  );
}
