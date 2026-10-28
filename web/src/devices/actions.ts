// Device commands that both a device's row and its page send.
import { api } from "../api/client";
import { t } from "../i18n";
import type { DeviceContext } from "./types";

/** Connects the Coyote: a remembered or nearby one, or a chosen candidate. */
export function coyoteConnect(ctx: DeviceContext, candidateID?: string) {
  return ctx.run("coyote", "connect", async () => {
    ctx.setCoyote(await api.coyoteConnect(candidateID));
    return t("Coyote connected. Check the strength readout before starting.");
  });
}

/** The vibrator's Test: one second while motion is paused, never at zero. */
export function vibratorTestAvailable(ctx: DeviceContext): boolean {
  const vibrator = ctx.intiface.vibrator;
  return !ctx.locked && ctx.canConfigureHost && Boolean(vibrator?.selected) && (vibrator?.ceiling_percent ?? 0) > 0
    && vibrator?.test_available !== false && vibrator?.playback_state !== "stale" && !ctx.engineActive && !vibrator?.testing && ctx.stopSequence !== undefined;
}

export function vibratorTest(ctx: DeviceContext) {
  const sequence = ctx.stopSequence;
  if (sequence !== undefined) ctx.send(async () => ctx.setIntiface(await api.vibratorTest(sequence)));
}

/** Resume now: lift the pause Stop left on the vibrator. */
export function vibratorResume(ctx: DeviceContext) {
  const sequence = ctx.stopSequence;
  if (sequence !== undefined) ctx.send(async () => ctx.setIntiface(await api.vibratorResume(sequence)));
}

/** Whether the chat model has any vibration vocabulary (ADR 0061). */
export function chatModelSetsVibration(ctx: DeviceContext): boolean {
  const features = ctx.state?.settings?.model_routing?.chat?.features;
  return features?.vibration_level === true || features?.independent_vibration === true;
}
