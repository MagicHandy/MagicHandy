// Lookups for additional Handys: the saved connection from settings and the
// live link from the backend snapshot, both by instance ID.
import type { HandyConnection, HandyConnectionState } from "../api/handy-types";
import type { HandyInstanceId, PublicSettings } from "../api/types";
import { t } from "../i18n";
import { handyConnectionMethods, type ConnectionMethod } from "./handy-methods";
import type { DeviceContext } from "./types";

/** The saved connection; a placeholder while the backend has not described it. */
export function handyConnection(settings: PublicSettings | undefined, id: HandyInstanceId): HandyConnection {
  return settings?.device?.handy_connections?.find((connection) => connection.id === id)
    ?? { id, name: "", method: "", handy_model: "handy_original", connection_key_set: false, remembered_device: false };
}

export function handyInstanceName(connection: HandyConnection): string {
  return connection.name.trim() || t("Additional Handy");
}

export function handyInstanceLink(ctx: DeviceContext, id: HandyInstanceId): HandyConnectionState | undefined {
  return ctx.handys.find((link) => link.id === id);
}

/** The methods another Handy can use. Browser Bluetooth pairs one Handy, the
 * primary one, through the shell's bridge. */
export function additionalHandyMethods(advertised: string[] | undefined): ConnectionMethod[] {
  return handyConnectionMethods().filter((method) => method.value !== "browser_bluetooth" && (!advertised?.length || advertised.includes(method.value)));
}

/** The shared connection an additional Handy reaches its hardware through. */
export function handyMethodConnection(method: string): "intiface" | "host_bluetooth" | null {
  return method === "intiface" ? "intiface" : method === "native_bluetooth" ? "host_bluetooth" : null;
}
