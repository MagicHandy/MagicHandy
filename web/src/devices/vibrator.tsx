// The vibrator's descriptor (ADRs 0047 and 0061). The backend owns the shared
// Intiface connection, the device choice, the model's level and the pause
// Emergency Stop leaves. The person owns Max and whether the model sets the
// level; both are saved settings, shown here and on the vibrator's page.
import { api } from "../api/client";
import type { VibrationSettings } from "../api/types";
import { t } from "../i18n";
import { chatModelSetsVibration, vibratorResume, vibratorTest, vibratorTestAvailable } from "./actions";
import { VibratorGlyph } from "./glyphs";
import { actionFor, liveStatus, lockReason } from "./status";
import type { DeviceContext, DeviceDescriptor, DeviceModel, DeviceStatusWord } from "./types";
import { DEFAULT_VIBRATION_SETTINGS, modelReadout, VibratorConnectionSection, VibratorOutputSection, VibratorTestSection } from "./VibratorPage";

/** Each step of Max in the device panel, in percent. */
export const VIBRATOR_STEP_PERCENT = 5;

function vibratorModel(ctx: DeviceContext): DeviceModel {
  const leased = Boolean(ctx.intiface.status.connected && ctx.intiface.leases?.includes("vibrator"));
  const vibrator = ctx.intiface.vibrator;
  const selected = leased && Boolean(vibrator?.selected);
  const dropped = vibrator?.playback_state === "stale";
  const silent = vibrator?.silent_reason === "stopped" || vibrator?.silent_reason === "max_zero" ||
    vibrator?.silent_reason === "below_motor_step" || vibrator?.silent_reason === "level_zero";
  const word: DeviceStatusWord = ctx.busy.vibrator === "connect"
    ? "connecting"
    : leased && !selected && !dropped ? "needs_setup" : dropped ? "lost"
      : selected && ((ctx.settings?.vibration?.level_control !== "manual" && ctx.engineActive && ctx.state?.motion?.engine?.target?.vibration?.mode === "off") || silent) ? "silent"
        : liveStatus(ctx, "vibrator", selected);
  const output = vibrator?.level_percent ?? 0;
  const details = word === "needs_setup"
    ? [t("choose a device")]
    : word === "lost" ? [dropped ? t("dropped") : t("left this run")]
      : [...(word === "playing" && output > 0 ? [t("{percent}% now", { percent: output })] : []), t("Intiface Central"),
        ...(word === "not_connected" && ctx.failed.vibrator ? [t("last attempt failed")] : [])];
  const settings: VibrationSettings = { ...DEFAULT_VIBRATION_SETTINGS, ...ctx.settings?.vibration };
  const max = settings.ceiling_percent;
  const effective = vibrator?.effective_ceiling_percent ?? max;
  const capable = chatModelSetsVibration(ctx);
  const modelSets = settings.level_control !== "manual";
  const level = vibrator?.model_level_percent ?? 100;
  const followsHandy = vibrator?.level_source !== "model";
  const suspended = Boolean(vibrator?.suspended);
  const envelope = ctx.engineActive && modelSets ? ctx.state?.motion?.engine?.target?.vibration : undefined;
  const hostLocked = ctx.locked || !ctx.canConfigureHost;
  const save = (action: string, patch: Partial<VibrationSettings>) =>
    void ctx.run("vibrator", action, async () => { await api.saveDeviceOptions({ vibration: patch }); });
  // A lower Max takes the minimum down with it, so the pair stays valid.
  const saveMax = (next: number) => save("max", { ceiling_percent: next, minimum_percent: Math.min(settings.minimum_percent, next) });
  const connect = () => void ctx.run("vibrator", "connect", async () => { ctx.setIntiface(await api.intifaceConnect("vibrator")); });
  const disconnect = () => void ctx.run("vibrator", "disconnect", async () => {
    ctx.setIntiface(await api.intifaceDisconnect("vibrator"));
    return t("Vibrator disconnected. It was stopped before closing.");
  });
  return {
    id: "vibrator",
    name: t("Vibrator"),
    status: word,
    details,
    // A device that dropped out of a live Intiface session is chosen again on
    // its page; a dropped session reconnects in one step.
    action: word === "lost" && leased ? { kind: "setup" } : actionFor(word, { connect, disconnect, reason: lockReason(ctx, true) }),
    inPlayback: ctx.playing.includes("vibrator"),
    levels: selected ? {
      label: suspended ? t("Paused by Stop") : t("Vibration"),
      name: t("Vibrator strength"),
      levels: [{
        id: "max",
        label: t("Max"),
        value: max,
        unit: "%",
        canLower: !hostLocked && max > 0,
        canRaise: !hostLocked && max < 100,
        canTest: vibratorTestAvailable(ctx),
        lower: () => saveMax(Math.max(0, max - VIBRATOR_STEP_PERCENT)),
        raise: () => saveMax(Math.min(100, max + VIBRATOR_STEP_PERCENT)),
        test: () => vibratorTest(ctx),
        lowerLabel: t("Lower the vibrator max"),
        raiseLabel: t("Raise the vibrator max"),
        testLabel: t("Test the vibrator"),
      }],
      toggle: capable ? {
        label: t("Model sets level"),
        checked: modelSets,
        readout: !modelSets ? t("Off") : envelope ? modelReadout(envelope, level, followsHandy)
          : followsHandy ? t("Follows The Handy") : t("{percent}% of max", { percent: level }),
        disabled: hostLocked,
        onChange: (checked) => save("model_control", { level_control: checked ? "model" : "manual" }),
      } : undefined,
      action: suspended ? { label: t("Resume now"), run: () => vibratorResume(ctx), disabled: hostLocked || ctx.stopSequence === undefined } : undefined,
      note: suspended
        ? t("Your next motion request can resume it. To keep it off, turn off In playback.")
        : max === 0 ? t("Raise the max to allow vibration.")
          : vibrator?.silent_reason === "below_motor_step" ? t("The max is below this device’s smallest motor step.")
            : effective < max ? t("With these motors' steps, the highest level is {percent}%.", { percent: effective })
              : capable && modelSets ? !envelope && followsHandy ? t("Until the model chooses a level, vibration follows The Handy.") : t("The model picks the level up to your max.")
                : t("Vibration follows movement at your max."),
    } : undefined,
    readout: selected
      ? capable && modelSets && !followsHandy ? t("Model level {level}% of max {max}%", { level, max }) : t("Max {max}%", { max })
      : undefined,
    summary: [t("Intiface Central"), selected && vibrator?.device_name ? vibrator.device_name : t("no device chosen")].join(" · "),
  };
}

export const vibratorDevice: DeviceDescriptor = {
  id: "vibrator",
  name: () => t("Vibrator"),
  Glyph: VibratorGlyph,
  addDetail: () => t("Any vibrator Intiface Central supports"),
  addNotice: () => t("Your chat model sets the level under the max you choose. Advanced models, such as hosted ones, can also play their own patterns in Layered or Simple."),
  model: vibratorModel,
  sections: [
    { id: "connection", title: () => t("Connection"), Body: VibratorConnectionSection },
    { id: "output", title: () => t("Output"), Body: VibratorOutputSection },
    { id: "test", title: () => t("Test"), Body: VibratorTestSection },
  ],
  removeNote: () => t("Removing a device stops motion and disconnects it."),
  timing: {
    value: (settings) => settings.vibration?.timing_offset_ms ?? 0,
    patch: (ms) => ({ vibration: { timing_offset_ms: ms } }),
  },
  connection: () => "intiface",
};
