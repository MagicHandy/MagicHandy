// The Handy's descriptor: how its row reads and acts for each connection
// method, and the sections of its device page.
import { api } from "../api/client";
import { handyModelDetail, handyModelLabel } from "../components/QuickSettings";
import { t } from "../i18n";
import { HandyGlyph } from "./glyphs";
import { handyMethod } from "./handy-methods";
import { HandyCalibrationSection, HandyConnectionSection, HandyTestSection } from "./HandyPage";
import { actionFor, lockReason, liveStatus } from "./status";
import type { DeviceContext, DeviceDescriptor, DeviceModel, DeviceStatusWord } from "./types";

interface HandyLink {
  connected: boolean;
  connecting: boolean;
  /** The link dropped or failed after connecting. */
  lost?: boolean;
  /** What only the device page can supply. */
  setup?: string;
  detail?: string;
  /** Connecting opens hardware on the host, which needs host permission. */
  hostOnly: boolean;
  connect?: () => void;
  disconnect?: () => void;
}

function handyLink(ctx: DeviceContext): HandyLink {
  const busy = ctx.busy.handy;
  switch (ctx.settings?.device?.hsp_dispatch_owner) {
    case "cloud_rest": {
      const cloud = ctx.state?.cloud_transport;
      const connected = Boolean(cloud?.connected);
      return {
        connected,
        connecting: busy === "connect",
        setup: ctx.settings?.device?.connection_key_set ? undefined : t("connection key"),
        detail: connected && cloud?.last_latency_ms ? t("{millis} ms", { millis: cloud.last_latency_ms }) : undefined,
        hostOnly: false,
        connect: () => void ctx.run("handy", "connect", async () => {
          const result = await api.cloudConnect();
          if (!result.ok) throw new Error(result.message?.trim() || t("The Handy responded, but HSP is unavailable for its current device state."));
          return t("The Handy is reachable ({latency} ms).", { latency: result.latency_ms });
        }),
        disconnect: () => void ctx.run("handy", "disconnect", async () => {
          const result = await api.cloudDisconnect();
          if (result.warning) throw new Error(result.warning);
          return t("MagicHandy released control of The Handy. It can now be used by another app.");
        }),
      };
    }
    case "native_bluetooth": {
      const native = ctx.native;
      return {
        connected: Boolean(native?.status.connected),
        connecting: busy === "connect" || Boolean(native?.status.connecting),
        setup: native?.supported === false ? t("not available on this system") : native?.available === false ? t("Bluetooth helper missing") : undefined,
        hostOnly: true,
        connect: () => void ctx.run("handy", "connect", async () => { await api.nativeBluetoothConnect(); }),
        disconnect: () => void ctx.run("handy", "disconnect", async () => { await api.nativeBluetoothDisconnect(); }),
      };
    }
    case "browser_bluetooth":
      return {
        connected: ctx.bluetooth.connected,
        connecting: ctx.bluetooth.connecting,
        lost: !ctx.bluetooth.connected && ctx.bluetooth.status === "error",
        setup: ctx.bluetooth.connected || ctx.bluetooth.supported ? undefined : t("this browser has no Bluetooth"),
        hostOnly: true,
        connect: ctx.browserBluetooth.connect,
        disconnect: ctx.browserBluetooth.disconnect,
      };
    case "intiface": {
      // The shared session can be open for the vibrator alone: the Handy is
      // connected only while it holds its own lease.
      const leased = Boolean(ctx.intiface.status.connected && (ctx.intiface.leases?.includes("handy") ?? true));
      const selected = ctx.intiface.status.selected_device_index !== undefined;
      return {
        connected: leased && selected,
        connecting: busy === "connect",
        setup: leased && !selected ? t("choose a device") : undefined,
        hostOnly: false,
        connect: () => void ctx.run("handy", "connect", async () => {
          ctx.setIntiface(await api.intifaceConnect("handy"));
          return t("Intiface connected.");
        }),
        disconnect: () => void ctx.run("handy", "disconnect", async () => { ctx.setIntiface(await api.intifaceDisconnect("handy")); }),
      };
    }
    default:
      return { connected: false, connecting: false, setup: t("choose how it connects"), hostOnly: false };
  }
}

function handyModel(ctx: DeviceContext): DeviceModel {
  const owner = ctx.settings?.device?.hsp_dispatch_owner ?? "";
  const method = handyMethod(owner);
  const link = handyLink(ctx);
  const status: DeviceStatusWord = link.connecting
    ? "connecting"
    : link.lost ? "lost" : link.setup && !link.connected ? "needs_setup" : liveStatus(ctx, "handy", link.connected);
  const details = status === "needs_setup" && link.setup
    ? [link.setup]
    : status === "lost" ? [link.lost ? t("dropped") : t("left this run")] : [method?.short ?? t("Not configured"), ...(link.detail ? [link.detail] : [])];
  if (status === "not_connected" && ctx.failed.handy) details.push(t("last attempt failed"));
  const model = ctx.settings?.motion?.handy_model ?? "handy_original";
  const travel = handyModelDetail(model).split(" · ")[0];
  return {
    id: "handy",
    name: t("The Handy"),
    status,
    details,
    action: actionFor(status, { connect: link.connect, disconnect: link.disconnect, reason: lockReason(ctx, link.hostOnly) }),
    inPlayback: ctx.playing.includes("handy"),
    summary: [method?.label ?? t("Not configured"), handyModelLabel(model), travel].join(" · "),
  };
}

export const handyDevice: DeviceDescriptor = {
  id: "handy",
  name: () => t("The Handy"),
  Glyph: HandyGlyph,
  addDetail: () => t("Stroker · Cloud, Bluetooth or Intiface Central"),
  model: handyModel,
  sections: [
    { id: "connection", title: () => t("Connection"), Body: HandyConnectionSection },
    { id: "calibration", title: () => t("Calibration"), Body: HandyCalibrationSection },
    { id: "test", title: () => t("Test"), Body: HandyTestSection },
  ],
  removeNote: () => t("Removing a device stops motion."),
  connection: (settings) => {
    const owner = settings?.device?.hsp_dispatch_owner;
    return owner === "intiface" ? "intiface" : owner === "native_bluetooth" ? "host_bluetooth" : null;
  },
};
