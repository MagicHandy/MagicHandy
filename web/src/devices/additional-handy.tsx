// An additional Handy's descriptor. Each is a named instance beside the
// primary Handy with its own connection and model; every Handy in playback
// follows the one shared motion, range and speed limits.
import { api } from "../api/client";
import type { HandyConnection, HandyConnectionState } from "../api/handy-types";
import type { HandyInstanceId, PublicSettings } from "../api/types";
import { handyModelDetail, handyModelLabel } from "../components/QuickSettings";
import { t } from "../i18n";
import { AdditionalHandyCalibrationSection, AdditionalHandyConnectionSection } from "./AdditionalHandyPage";
import { HandyGlyph } from "./glyphs";
import { handyConnection, handyInstanceLink, handyInstanceName, handyMethodConnection } from "./handy-instances";
import { handyMethod } from "./handy-methods";
import { actionFor, liveStatus, lockReason } from "./status";
import type { DeviceContext, DeviceDescriptor, DeviceModel, DeviceStatusWord } from "./types";

// What only the page can supply before Connect can work.
function setupNeed(connection: HandyConnection, link: HandyConnectionState | undefined): string | undefined {
  switch (connection.method) {
    case "cloud_rest":
      return connection.connection_key_set ? undefined : t("connection key");
    case "native_bluetooth":
      if (link?.native_supported === false) return t("not available on this system");
      if (link?.native_available === false) return t("Bluetooth helper missing");
      return connection.remembered_device ? undefined : t("choose a nearby Handy");
    case "intiface":
      // The Intiface lease comes first; then this Handy needs its own device.
      return link?.connected && link.selected_device_index === undefined ? t("choose a device") : undefined;
    default:
      return t("choose how it connects");
  }
}

function additionalHandyModel(ctx: DeviceContext, connection: HandyConnection): DeviceModel {
  const { id } = connection;
  const link = handyInstanceLink(ctx, id);
  const method = handyMethod(connection.method);
  const setup = setupNeed(connection, link);
  const connected = Boolean(link?.connected) && (connection.method !== "intiface" || link?.selected_device_index !== undefined);
  const dropped = !connected && link?.status === "lost";
  const status: DeviceStatusWord = ctx.busy[id] === "connect" || link?.connecting
    ? "connecting"
    : dropped ? "lost" : setup && !connected ? "needs_setup" : liveStatus(ctx, id, connected);
  const details = status === "needs_setup" && setup
    ? [setup]
    : status === "lost" ? [dropped ? t("dropped") : t("left this run")]
      : [method?.short ?? t("Not configured"), ...(connected && link?.latency_ms ? [t("{millis} ms", { millis: link.latency_ms })] : [])];
  if (status === "not_connected" && ctx.failed[id]) details.push(t("last attempt failed"));
  const model = connection.handy_model || "handy_original";
  return {
    id,
    name: handyInstanceName(connection),
    status,
    details,
    action: actionFor(status, {
      connect: () => void ctx.run(id, "connect", async () => { ctx.setHandy(await api.handyConnect(id)); }),
      disconnect: () => void ctx.run(id, "disconnect", async () => { ctx.setHandy(await api.handyDisconnect(id)); }),
      reason: lockReason(ctx, true),
    }),
    inPlayback: ctx.playing.includes(id),
    summary: [method?.label ?? t("Not configured"), handyModelLabel(model), handyModelDetail(model).split(" · ")[0]].join(" · "),
  };
}

const SECTIONS: DeviceDescriptor["sections"] = [
  { id: "connection", title: () => t("Connection"), Body: AdditionalHandyConnectionSection },
  { id: "calibration", title: () => t("Calibration"), Body: AdditionalHandyCalibrationSection },
];

/** The descriptor for one saved additional Handy. */
export function additionalHandyDevice(id: HandyInstanceId, settings: PublicSettings | undefined): DeviceDescriptor {
  const connection = handyConnection(settings, id);
  return {
    id,
    name: () => handyInstanceName(connection),
    Glyph: HandyGlyph,
    addDetail: () => t("Follows the same motion · Cloud, Bluetooth or Intiface Central"),
    model: (ctx) => additionalHandyModel(ctx, connection),
    sections: SECTIONS,
    removeNote: () => t("Removing a device stops motion and disconnects it."),
    remove: () => api.removeHandy(id),
    connection: (current) => handyMethodConnection(handyConnection(current, id).method),
  };
}
