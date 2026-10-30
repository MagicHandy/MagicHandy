// The Coyote's descriptor. Strength steps never initiate output; Test is the
// separate action that needs fresh control authority (ADR 0044).
import { api } from "../api/client";
import type { CoyoteStatus } from "../api/types";
import { t } from "../i18n";
import { coyoteConnect } from "./actions";
import { CoyoteConnectionSection, CoyoteOutputSection, CoyoteSafetySection, CoyoteTestSection } from "./CoyotePage";
import { CoyoteGlyph } from "./glyphs";
import { actionFor, liveStatus, lockReason } from "./status";
import type { DeviceContext, DeviceDescriptor, DeviceLevelModel, DeviceModel, DeviceStatusWord } from "./types";

function coyoteLevel(ctx: DeviceContext, status: CoyoteStatus, channel: "a" | "b"): DeviceLevelModel {
  const label = channel === "a" ? t("Channel A") : t("Channel B");
  const strength = channel === "a" ? status.strength_a : status.strength_b;
  const ceiling = channel === "a" ? status.ceiling_a : status.ceiling_b;
  const ready = status.limits_applied && status.strength_known && !status.strength_pending && status.playback_state !== "stopping";
  const host = !ctx.locked && ctx.canConfigureHost;
  const step = (delta: number) => ctx.send(async () => ctx.setCoyote(await api.coyoteStrength(channel, delta, ctx.stopSequence)));
  return {
    id: channel,
    label,
    value: status.strength_known ? strength : null,
    ceiling,
    unit: "",
    canLower: !ctx.locked && !(status.strength_known && strength <= 0),
    canRaise: host && ready && strength < ceiling,
    canTest: host && ready && ctx.stopSequence !== undefined && !ctx.engineActive && !status.reference_pulse && status.playback_state === "idle" && strength > 0,
    lower: () => step(-1),
    raise: () => step(1),
    test: () => {
      const sequence = ctx.stopSequence;
      if (sequence !== undefined) ctx.send(async () => ctx.setCoyote(await api.coyoteTest(channel, sequence)));
    },
    lowerLabel: t("Lower {channel}", { channel: label }),
    raiseLabel: t("Raise {channel}", { channel: label }),
    testLabel: t("Test {channel}", { channel: label }),
  };
}

function coyoteModel(ctx: DeviceContext): DeviceModel {
  const snapshot = ctx.coyote;
  const status = snapshot?.status;
  const estim = ctx.settings?.estim;
  const connected = Boolean(status?.connected);
  const helperMissing = Boolean(snapshot && !snapshot.available && !status?.simulated);
  const setup = !estim?.safety_acknowledged ? t("safety notes first") : helperMissing ? t("Bluetooth helper missing") : undefined;
  const dropped = !connected && status?.playback_state === "stale";
  // Motion can play while nothing is felt: strength is the user's alone.
  const silent = connected && (status?.silent_reason === "strength_zero" || status?.silent_reason === "ceiling_zero");
  const word: DeviceStatusWord = ctx.busy.coyote === "connect" || status?.connecting
    ? "connecting"
    : setup && !connected ? "needs_setup" : dropped ? "lost"
      : silent && ctx.engineActive && ctx.playing.includes("coyote") ? "silent" : liveStatus(ctx, "coyote", connected);
  const details = word === "needs_setup" && setup
    ? [setup]
    : word === "lost" ? [dropped ? t("dropped") : t("left this run")]
      : connected ? (status?.battery_percent !== undefined ? [t("Battery {percent}%", { percent: status.battery_percent })] : [])
        : [t("Bluetooth on this computer"), ...(ctx.failed.coyote ? [t("last attempt failed")] : [])];
  const connect = () => void coyoteConnect(ctx);
  const disconnect = () => void ctx.run("coyote", "disconnect", async () => {
    ctx.setCoyote(await api.coyoteDisconnect());
    return t("Coyote disconnected. A zero command was sent before closing.");
  });
  return {
    id: "coyote",
    name: t("Coyote"),
    status: word,
    details,
    action: actionFor(word, { connect, disconnect, reason: lockReason(ctx, true) }),
    inPlayback: ctx.playing.includes("coyote"),
    levels: connected && status ? {
      label: t("Strength"),
      name: t("Coyote strength"),
      levels: [coyoteLevel(ctx, status, "a"), coyoteLevel(ctx, status, "b")],
      action: { label: t("Zero both"), disabled: ctx.locked, run: () => ctx.send(async () => ctx.setCoyote(await api.coyoteZero())) },
      note: status.silent_reason === "ceiling_zero" ? t("Both ceilings are 0. Raise them in the Coyote's Output settings to allow strength.")
        : status.strength_source === "zero_command" ? t("Zero command accepted. The device has not reported a strength change.")
          : status.silent_reason === "strength_zero" ? t("Both channels are at 0, so nothing is felt. Raise a channel when you are ready.") : undefined,
    } : undefined,
    readout: connected && status?.strength_known ? t("A {a} · B {b}", { a: status.strength_a, b: status.strength_b }) : undefined,
    summary: [t("Bluetooth on this computer"), t("ceilings {a} and {b}", { a: estim?.max_strength_a ?? 0, b: estim?.max_strength_b ?? 0 })].join(" · "),
  };
}

export const coyoteDevice: DeviceDescriptor = {
  id: "coyote",
  name: () => t("Coyote"),
  Glyph: CoyoteGlyph,
  addDetail: () => t("Two-channel e-stim · this computer's Bluetooth"),
  model: coyoteModel,
  sections: [
    { id: "safety", title: () => t("Safety"), Body: CoyoteSafetySection },
    { id: "connection", title: () => t("Connection"), Body: CoyoteConnectionSection },
    { id: "output", title: () => t("Output"), Body: CoyoteOutputSection },
    { id: "test", title: () => t("Test"), Body: CoyoteTestSection },
  ],
  removeNote: () => t("Removing a device stops motion and disconnects it."),
  timing: {
    value: (settings) => settings.estim?.timing_offset_ms ?? 0,
    patch: (ms) => ({ estim: { timing_offset_ms: ms } }),
  },
  connection: () => "host_bluetooth",
};
