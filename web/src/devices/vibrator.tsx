// The vibrator's descriptor (ADR 0047). The backend owns the shared Intiface
// connection, the device choice and the live intensity, which starts at 0 on
// every connection and never rises on its own.
import { api } from "../api/client";
import { t } from "../i18n";
import { vibratorTest, vibratorTestAvailable } from "./actions";
import { VibratorGlyph } from "./glyphs";
import { actionFor, liveStatus, lockReason } from "./status";
import type { DeviceContext, DeviceDescriptor, DeviceModel, DeviceStatusWord } from "./types";
import { VibratorConnectionSection, VibratorOutputSection, VibratorTestSection } from "./VibratorPage";

/** Each step of the live intensity, in percent. */
export const VIBRATOR_STEP_PERCENT = 5;

function vibratorModel(ctx: DeviceContext): DeviceModel {
  const leased = Boolean(ctx.intiface.status.connected && ctx.intiface.leases?.includes("vibrator"));
  const vibrator = ctx.intiface.vibrator;
  const selected = leased && Boolean(vibrator?.selected);
  const dropped = vibrator?.playback_state === "stale";
  const word: DeviceStatusWord = ctx.busy.vibrator === "connect"
    ? "connecting"
    : leased && !selected && !dropped ? "needs_setup" : dropped ? "lost"
      : selected && ((ctx.engineActive && ctx.state?.motion?.engine?.target?.vibration?.mode === "off") || vibrator?.silent_reason === "emergency_stop" || vibrator?.silent_reason === "intensity_zero" || vibrator?.silent_reason === "below_motor_step") ? "silent"
        : liveStatus(ctx, "vibrator", selected);
  const details = word === "needs_setup"
    ? [t("choose a device")]
    : word === "lost" ? [dropped ? t("dropped") : t("left this run")]
      : [t("Intiface Central"), ...(word === "not_connected" && ctx.failed.vibrator ? [t("last attempt failed")] : [])];
  const intensity = vibrator?.intensity_percent ?? 0;
  const ceiling = vibrator?.ceiling_percent ?? 0;
  const effective = vibrator?.effective_ceiling_percent ?? ceiling;
  const setIntensity = (percent: number) => ctx.send(async () => ctx.setIntiface(await api.vibratorIntensity(percent, percent > intensity ? ctx.stopSequence : undefined)));
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
      label: t("Intensity"),
      name: t("Vibrator intensity"),
      levels: [{
        id: "intensity",
        label: t("Intensity"),
        value: intensity,
        ceiling,
        unit: "%",
        canLower: !ctx.locked && intensity > 0,
        canRaise: !ctx.locked && ctx.canConfigureHost && intensity < ceiling,
        canTest: vibratorTestAvailable(ctx),
        lower: () => setIntensity(Math.max(0, intensity - VIBRATOR_STEP_PERCENT)),
        raise: () => setIntensity(Math.min(ceiling, intensity + VIBRATOR_STEP_PERCENT)),
        test: () => vibratorTest(ctx),
        lowerLabel: t("Lower the vibrator intensity"),
        raiseLabel: t("Raise the vibrator intensity"),
        testLabel: t("Test the vibrator"),
      }],
      note: vibrator?.silent_reason === "emergency_stop"
        ? t("Stop reset the intensity to 0. Raise it to allow vibration again.")
        : vibrator?.silent_reason === "below_motor_step" ? t("The intensity is below this device’s smallest motor step.")
          : intensity === 0 ? t("Raise the intensity to allow vibration. The model cannot raise it.")
          : effective < ceiling ? t("With these motors' steps, the highest level is {percent}%.", { percent: effective }) : undefined,
    } : undefined,
    readout: selected ? t("{intensity}% of {ceiling}%", { intensity, ceiling }) : undefined,
    summary: [t("Intiface Central"), selected && vibrator?.device_name ? vibrator.device_name : t("no device chosen")].join(" · "),
  };
}

export const vibratorDevice: DeviceDescriptor = {
  id: "vibrator",
  name: () => t("Vibrator"),
  Glyph: VibratorGlyph,
  addDetail: () => t("Any vibrator Intiface Central supports"),
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
