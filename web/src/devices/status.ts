import { t } from "../i18n";
import type { DeviceId } from "../api/types";
import type { DeviceAction, DeviceContext, DeviceModel, DeviceStatusWord } from "./types";

export function statusWord(status: DeviceStatusWord): string {
  switch (status) {
    case "playing": return t("Playing");
    case "connected": return t("Connected");
    case "connecting": return t("Connecting…");
    case "needs_setup": return t("Needs setup");
    case "lost": return t("Connection lost");
    default: return t("Not connected");
  }
}

/** The status word and its details, as one line: "Connected · Cloud · 64 ms". */
export function statusLine(model: Pick<DeviceModel, "status" | "details">): string {
  return [statusWord(model.status), ...model.details].join(" · ");
}

/**
 * The shared rule for a reachable device: Playing while it is connected and
 * part of the current run, Connection lost once a degraded run left it.
 */
export function liveStatus(ctx: DeviceContext, device: DeviceId, connected: boolean): DeviceStatusWord {
  if (ctx.retired.includes(device) && ctx.engineActive) return "lost";
  if (!connected) return "not_connected";
  return ctx.engineActive && ctx.playing.includes(device) ? "playing" : "connected";
}

/** Why device commands are unavailable here, if they are. */
export function lockReason(ctx: DeviceContext, hostOnly = false): string | undefined {
  if (!ctx.backendOnline) return t("Core offline.");
  if (ctx.readOnly) return t("Read-only client.");
  if (hostOnly && !ctx.canConfigureHost) return t("Host settings and diagnostics are managed by an administrator.");
  return undefined;
}

/** The one action that fits a status (Device kit, Device row). */
export function actionFor(status: DeviceStatusWord, options: { connect?: () => void; disconnect?: () => void; reason?: string }): DeviceAction {
  const unavailable = (run?: () => void) => Boolean(options.reason) || !run;
  switch (status) {
    case "needs_setup":
      return { kind: "setup" };
    case "connecting":
      return { kind: "connect", busy: true, disabled: true };
    case "lost":
      return { kind: "reconnect", run: options.connect, disabled: unavailable(options.connect), reason: options.reason };
    case "connected":
    case "playing":
      return { kind: "disconnect", run: options.disconnect, disabled: unavailable(options.disconnect), reason: options.reason };
    default:
      return { kind: "connect", run: options.connect, disabled: unavailable(options.connect), reason: options.reason };
  }
}

/** The device chip's accessible status for one device. */
export function chipPhrase(model: DeviceModel): string {
  switch (model.status) {
    case "playing": return t("{device} connected and playing", { device: model.name });
    case "connected": return t("{device} connected", { device: model.name });
    case "connecting": return t("{device} connecting", { device: model.name });
    case "needs_setup": return t("{device} needs setup", { device: model.name });
    case "lost": return t("{device} connection lost", { device: model.name });
    default: return t("{device} not connected", { device: model.name });
  }
}

/**
 * The chip names what plays, then the one problem that needs attention:
 * a lost device first, then setup, then a playing device that is not
 * connected yet. With one device it shows that device's own status.
 */
export function chipSummary(models: DeviceModel[]): { title: string; status: string } {
  if (!models.length) return { title: t("Devices"), status: t("Chat only") };
  if (models.length === 1) {
    const [only] = models;
    return { title: only.name, status: [statusWord(only.status), ...only.details.slice(0, 1)].join(" · ") };
  }
  const playing = models.filter((model) => model.inPlayback);
  const title = playing.length ? playing.map((model) => model.name).join(" + ") : t("No device in playback");
  const problem = models.find((model) => model.status === "lost")
    ?? models.find((model) => model.status === "needs_setup")
    ?? playing.find((model) => model.status === "not_connected" || model.status === "connecting");
  if (problem) return { title, status: chipPhrase(problem) };
  return { title, status: playing.some((model) => model.status === "playing") ? t("Playing") : t("Connected") };
}
