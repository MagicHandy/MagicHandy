// The device choice for a device that reaches its hardware through Intiface
// Central. Device indices belong to this discovery session, so the choice is
// runtime state the backend keeps, not a setting. Every Handy and the
// vibrator share the session; a device another one uses is shown, disabled,
// with who uses it. The backend refuses a claimed device either way.
import { useEffect, useId, useState } from "react";
import { api } from "../api/client";
import { isHandyInstance } from "../api/devices";
import type { HandyInstanceId, IntifaceDevice } from "../api/types";
import { SetupChoice } from "../components/SetupChoice";
import { t } from "../i18n";
import { handyConnection, handyInstanceLink, handyInstanceName } from "./handy-instances";
import type { DeviceContext } from "./types";

interface Choice {
  value: string;
  label: string;
  detail: string;
  disabled: boolean;
}

type IntifaceUser = "handy" | "vibrator" | HandyInstanceId;

// Who uses each Intiface device, by device index, apart from this user.
function claimsBesides(ctx: DeviceContext, user: IntifaceUser): Map<number, string> {
  const claims = new Map<number, string>();
  const vibrator = ctx.intiface.vibrator;
  if (user !== "vibrator" && vibrator?.selected && vibrator.device_index !== undefined) claims.set(vibrator.device_index, t("Used as the vibrator"));
  const primary = ctx.intiface.status.selected_device_index;
  if (user !== "handy" && primary !== undefined) claims.set(primary, t("Used as The Handy"));
  for (const link of ctx.handys) {
    if (link.id === user || !isHandyInstance(link.id) || link.selected_device_index === undefined || !ctx.configured.includes(link.id)) continue;
    claims.set(link.selected_device_index, t("Used by {device}", { device: handyInstanceName(handyConnection(ctx.settings, link.id)) }));
  }
  return claims;
}

function linearChoices(devices: IntifaceDevice[], claims: Map<number, string>): Choice[] {
  return devices.flatMap((device) => {
    const actuators = device.linear_actuators ?? [];
    if (!actuators.length) return [{ value: `none:${device.device_index}`, label: device.device_name, detail: t("No linear actuator"), disabled: true }];
    const claim = claims.get(device.device_index);
    return actuators.map((actuator) => ({
      value: `${device.device_index}:${actuator.index}`,
      label: device.device_name,
      detail: claim ?? (actuator.feature_descriptor || actuator.actuator_type || t("Linear {index}", { index: actuator.index + 1 })),
      disabled: claim !== undefined,
    }));
  });
}

function vibratorChoices(devices: IntifaceDevice[], claims: Map<number, string>): Choice[] {
  return devices.map((device) => {
    const motors = (device.scalar_actuators ?? []).filter((actuator) => actuator.actuator_type === "Vibrate" && (actuator.step_count ?? 0) > 0).length;
    const claim = claims.get(device.device_index);
    return {
      value: String(device.device_index),
      label: device.device_name,
      detail: claim ?? (motors ? t("{count} vibration motors, driven together", { count: motors }) : t("No vibration motors")),
      disabled: claim !== undefined || motors === 0,
    };
  });
}

function selection(deviceIndex: number | undefined, actuatorIndex: number | undefined): string {
  return deviceIndex !== undefined && actuatorIndex !== undefined ? `${deviceIndex}:${actuatorIndex}` : "";
}

export function IntifaceDeviceChoice({ ctx, device }: { ctx: DeviceContext; device: IntifaceUser }) {
  const legendId = useId();
  const status = ctx.intiface.status;
  const vibrator = ctx.intiface.vibrator;
  const instance = isHandyInstance(device) ? device : undefined;
  const link = instance ? handyInstanceLink(ctx, instance) : undefined;
  // An older backend reports no leases: its one Intiface session was the Handy's.
  const leased = instance ? Boolean(link?.connected) : Boolean(status.connected && (ctx.intiface.leases?.includes(device) ?? device === "handy"));
  const current = instance
    ? selection(link?.selected_device_index, link?.selected_actuator_index)
    : device === "handy"
      ? selection(status.selected_device_index, status.selected_actuator_index)
      : vibrator?.selected && vibrator.device_index !== undefined ? String(vibrator.device_index) : "";
  const claims = claimsBesides(ctx, device);
  const choices = device === "vibrator" ? vibratorChoices(status.devices ?? [], claims) : linearChoices(status.devices ?? [], claims);
  const eligible = choices.filter((choice) => !choice.disabled);
  const [chosen, setChosen] = useState(current);
  const busy = Boolean(ctx.busy[device]);
  const locked = ctx.locked || !ctx.canConfigureHost;

  useEffect(() => {
    if (current) setChosen(current);
    else if (eligible.length === 1) setChosen(eligible[0].value);
  }, [current, eligible.length === 1 ? eligible[0].value : ""]);

  if (!leased) {
    return <p className="hint-block">{instance
      ? t("Connect {device} to list the devices Intiface Central offers.", { device: handyInstanceName(handyConnection(ctx.settings, instance)) })
      : device === "handy"
        ? t("Connect The Handy to list the devices Intiface Central offers.")
        : t("Connect the vibrator to list the devices Intiface Central offers.")}</p>;
  }

  function use() {
    if (device === "vibrator") {
      const index = Number(chosen);
      if (!Number.isInteger(index)) return;
      void ctx.run("vibrator", "select", async () => {
        ctx.setIntiface(await api.vibratorSelect(index));
        return t("Vibrator ready. Playback stays within your Max.");
      });
      return;
    }
    const [deviceIndex, actuatorIndex] = chosen.split(":").map(Number);
    if (!Number.isInteger(deviceIndex) || !Number.isInteger(actuatorIndex)) return;
    void ctx.run(device, "select", async () => {
      if (instance) ctx.setHandy(await api.handySelect(instance, deviceIndex, actuatorIndex));
      else ctx.setIntiface(await api.intifaceSelect(deviceIndex, actuatorIndex));
      return t("Intiface actuator selected.");
    });
  }

  function scan() {
    void ctx.run(device, "scan", async () => {
      ctx.setIntiface(await (status.scanning ? api.intifaceStopScan() : api.intifaceStartScan()));
    });
  }

  return (
    <div className="device-choice">
      <strong id={legendId} className="device-choice-label">{t("Device")}</strong>
      {choices.length > 0 ? (
        <div className="setup-choices" role="radiogroup" aria-labelledby={legendId}>
          {choices.map((choice) => (
            <SetupChoice key={choice.value} selected={chosen === choice.value} title={choice.label} detail={choice.detail}
              disabled={choice.disabled || locked || busy} onSelect={() => setChosen(choice.value)} />
          ))}
        </div>
      ) : (
        <p className="form-status">{status.scanning
          ? t("Scanning for devices…")
          : device === "vibrator"
            ? t("No vibrators found. Make the vibrator available to Intiface Central, then scan.")
            : t("No linear actuators discovered. Start a scan after the device is available to Intiface Central.")}</p>
      )}
      <div className="row-actions">
        {choices.length > 0 && (
          <button type="button" className="btn btn-primary" disabled={locked || busy || !eligible.some((choice) => choice.value === chosen) || chosen === current} onClick={use}>
            {device === "vibrator" ? t("Use this vibrator") : t("Use this device")}
          </button>
        )}
        <button type="button" className="btn btn-secondary" disabled={locked || busy} onClick={scan}>
          {status.scanning ? t("Stop scan") : t("Scan for devices")}
        </button>
        <span className="hint">{t("Each device can serve one Handy or the vibrator at a time.")}</span>
      </div>
    </div>
  );
}
