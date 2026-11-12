// Autoblow's cloud devices (ADRs 0069 and 0071). The Ultra plays positions
// like The Handy; the VacuGlide plays a tempo and makes its own strokes, with
// valve steps the person controls. The backend owns both
// connections; these descriptors render its snapshot.
import { api } from "../api/client";
import type { AutoblowDevice, AutoblowStatus } from "../api/types";
import { t } from "../i18n";
import {
  AutoblowConnectionSection, AutoblowTestSection, autoblowStatus, autoblowTest, autoblowTestAvailable,
  VacuGlideConnectionSection, VacuGlideStrokeSection, VacuGlideTempoSection, VacuGlideTestSection, vacuglideStroke,
} from "./AutoblowPage";
import { AutoblowGlyph, VacuGlideGlyph } from "./glyphs";
import { actionFor, liveStatus, lockReason } from "./status";
import type { DeviceContext, DeviceDescriptor, DeviceLevelGroup, DeviceModel, DeviceStatusWord } from "./types";

function deviceName(device: AutoblowDevice): string {
  return device === "vacuglide" ? t("VacuGlide") : t("Autoblow Ultra");
}

function strokeLevels(ctx: DeviceContext, status: AutoblowStatus): DeviceLevelGroup {
  const host = !ctx.locked && ctx.canConfigureHost;
  return {
    label: t("Stroke"),
    name: t("VacuGlide stroke"),
    levels: [{
      id: "stroke",
      label: t("Stroke"),
      value: status.stroke_steps,
      unit: "",
      canLower: host && ctx.stopSequence !== undefined,
      canRaise: host && ctx.stopSequence !== undefined,
      canTest: autoblowTestAvailable(ctx, "vacuglide"),
      lower: () => vacuglideStroke(ctx, false),
      raise: () => vacuglideStroke(ctx, true),
      test: () => autoblowTest(ctx, "vacuglide"),
      lowerLabel: t("Shorter, looser stroke"),
      raiseLabel: t("Longer, tighter stroke"),
      testLabel: t("Test the VacuGlide"),
    }],
    note: status.motion_air ? t("Chat and Autopilot may adjust the stroke. Your steps add to theirs.") : undefined,
  };
}

function autoblowModel(ctx: DeviceContext, device: AutoblowDevice): DeviceModel {
  const status = autoblowStatus(ctx, device);
  const settings = device === "vacuglide" ? ctx.settings?.vacuglide : ctx.settings?.autoblow;
  const connected = Boolean(status?.connected);
  const tokenSet = Boolean(settings?.token_set);
  const dropped = !connected && status?.playback_state === "stale";
  const word: DeviceStatusWord = ctx.busy[device] === "connect" || status?.connecting
    ? "connecting"
    : !tokenSet && !connected ? "needs_setup" : dropped ? "lost" : liveStatus(ctx, device, connected);
  const details = word === "needs_setup"
    ? [t("device token")]
    : word === "lost" ? [dropped ? t("dropped") : t("left this run")]
      : connected ? [t("Autoblow cloud"), ...(status?.round_trip_ms ? [t("{millis} ms", { millis: status.round_trip_ms })] : [])]
        : [t("Autoblow cloud"), ...(ctx.failed[device] ? [t("last attempt failed")] : [])];
  const name = deviceName(device);
  const connect = () => void ctx.run(device, "connect", async () => {
    if (connected) ctx.setAutoblow(await api.autoblowDisconnect(device));
    ctx.setAutoblow(await api.autoblowConnect(device));
    return t("{device} connected through Autoblow's cloud.", { device: name });
  });
  const disconnect = () => void ctx.run(device, "disconnect", async () => {
    ctx.setAutoblow(await api.autoblowDisconnect(device));
    return t("{device} disconnected. It was stopped before closing.", { device: name });
  });
  const commanded = status?.commanded_percent;
  return {
    id: device,
    name,
    status: word,
    details,
    action: actionFor(word, { connect, disconnect, reason: lockReason(ctx, true) }),
    inPlayback: ctx.playing.includes(device),
    levels: device === "vacuglide" && connected && status ? strokeLevels(ctx, status) : undefined,
    readout: connected && commanded !== undefined
      ? device === "vacuglide" ? t("Motor {percent}%", { percent: Math.round(commanded) }) : t("Position {percent}%", { percent: Math.round(commanded) })
      : undefined,
    summary: [t("Autoblow cloud"), tokenSet ? t("token saved") : t("no token")].join(" · "),
  };
}

export const autoblowDevice: DeviceDescriptor = {
  id: "autoblow",
  name: () => t("Autoblow Ultra"),
  Glyph: AutoblowGlyph,
  addDetail: () => `${t("Experimental")} · ${t("Stroker · Autoblow's cloud")}`,
  addNotice: () => t("Plays positions like The Handy. Its commands, Stop included, travel through Autoblow's servers."),
  model: (ctx) => autoblowModel(ctx, "autoblow"),
  sections: [
    { id: "connection", title: () => t("Connection"), Body: AutoblowConnectionSection },
    { id: "test", title: () => t("Test"), Body: AutoblowTestSection },
  ],
  removeNote: () => t("Removing a device stops motion and disconnects it."),
  timing: {
    value: (settings) => settings.autoblow?.timing_offset_ms ?? 0,
    patch: (ms) => ({ autoblow: { timing_offset_ms: ms } }),
  },
  connection: () => null,
};

export const vacuglideDevice: DeviceDescriptor = {
  id: "vacuglide",
  name: () => t("VacuGlide"),
  Glyph: VacuGlideGlyph,
  addDetail: () => `${t("Experimental")} · ${t("Suction stroker · Autoblow's cloud")}`,
  addNotice: () => t("Follows the pace of the motion and makes its own strokes; you set how long and tight they are with its stroke valves. Its commands, Stop included, travel through Autoblow's servers."),
  model: (ctx) => autoblowModel(ctx, "vacuglide"),
  sections: [
    { id: "connection", title: () => t("Connection"), Body: VacuGlideConnectionSection },
    { id: "tempo", title: () => t("Tempo"), Body: VacuGlideTempoSection },
    { id: "stroke", title: () => t("Stroke"), Body: VacuGlideStrokeSection },
    { id: "test", title: () => t("Test"), Body: VacuGlideTestSection },
  ],
  removeNote: () => t("Removing a device stops motion and disconnects it."),
  timing: {
    value: (settings) => settings.vacuglide?.timing_offset_ms ?? 0,
    patch: (ms) => ({ vacuglide: { timing_offset_ms: ms } }),
  },
  connection: () => null,
};
