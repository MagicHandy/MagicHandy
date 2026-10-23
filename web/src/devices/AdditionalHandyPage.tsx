// An additional Handy's page: Connection (its name, its method and what the
// method needs) and Calibration (its model). The motion, its limits and the
// motion test are shared by every Handy in playback, so they stay in the
// device panel and the shared test section.
import { useId } from "react";
import { api } from "../api/client";
import { isHandyInstance } from "../api/devices";
import type { HandyInstanceId } from "../api/types";
import { DEFAULT_HANDY_MODELS, handyModelDetail, handyModelLabel } from "../components/QuickSettings";
import { SegmentedChoice } from "../components/SetpointControls";
import { t, translateKnown } from "../i18n";
import { confirmThen } from "../util/confirm";
import { CommitInput, OptionRow, useLiveSetting, useSaveHandy } from "./fields";
import { additionalHandyMethods, handyConnection, handyInstanceLink, handyInstanceName } from "./handy-instances";
import { CloudFirmwareNotice, ConnectionKeyRow, IntifaceConnectionRow, IntifaceStopNotice, MethodChoice, NativeBluetoothRows } from "./HandyPage";
import { IntifaceDeviceChoice } from "./IntifaceDeviceChoice";
import type { DeviceContext, DeviceSectionProps } from "./types";

export function AdditionalHandyConnectionSection({ ctx, model }: DeviceSectionProps) {
  return isHandyInstance(model.id) ? <ConnectionRows ctx={ctx} id={model.id} /> : null;
}

export function AdditionalHandyCalibrationSection({ ctx, model }: DeviceSectionProps) {
  return isHandyInstance(model.id) ? <ModelRow ctx={ctx} id={model.id} /> : null;
}

function ConnectionRows({ ctx, id }: { ctx: DeviceContext; id: HandyInstanceId }) {
  const connection = handyConnection(ctx.settings, id);
  const link = handyInstanceLink(ctx, id);
  const name = handyInstanceName(connection);
  const save = useSaveHandy(id);
  const connected = Boolean(link?.connected);

  function forgetKey() {
    confirmThen(t("Forget the saved connection key? {device} cannot connect through Cloud until you save a key again.", { device: name }), { confirmLabel: t("Forget key"), destructive: true },
      () => void save({ clear_connection_key: true }).catch(() => undefined));
  }

  return (
    <div className="form-rows device-rows">
      <NameRow ctx={ctx} name={connection.name} save={(value) => void save({ name: value }).catch(() => undefined)} />
      <MethodChoice ctx={ctx} device={id} current={connection.method}
        methods={additionalHandyMethods(ctx.settings?.options?.hsp_dispatch_owners)}
        hint={t("Changing the method stops motion and disconnects {device}.", { device: name })}
        note={t("Browser Bluetooth is only for The Handy. Connect more Handys with Bluetooth on this computer or Intiface Central.")}
        confirm={(method) => t("Switch {device} to {method}? Motion stops and {device} disconnects.", { device: name, method })}
        apply={async (method) => {
          await api.updateHandy(id, { method });
          // A failed attempt belongs to the method it was made with.
          ctx.setFailed(id, false);
        }} />
      {connection.method === "cloud_rest" && <>
        <ConnectionKeyRow ctx={ctx} device={id} keySet={connection.connection_key_set}
          save={(key) => api.updateHandy(id, { connection_key: key })} forget={forgetKey} />
        <CloudFirmwareNotice />
      </>}
      {connection.method === "native_bluetooth" && (
        <NativeBluetoothRows ctx={ctx} device={id}
          supported={link?.native_supported ?? ctx.native?.supported} available={link?.native_available ?? ctx.native?.available}
          remembered={connection.remembered_device} connected={connected}
          forget={() => void ctx.run(id, "forget", async () => { ctx.setHandy(await api.handyForget(id)); })} />
      )}
      {connection.method === "intiface" && <>
        <IntifaceStopNotice />
        <IntifaceConnectionRow ctx={ctx} />
        <div className="form-row form-row-plain"><IntifaceDeviceChoice ctx={ctx} device={id} /></div>
      </>}
      {link?.status === "lost" && link.message && <p className="form-status" role="status">{translateKnown(link.message)}</p>}
    </div>
  );
}

// The name tells several Handys apart; it saves when it is committed.
function NameRow({ ctx, name, save }: { ctx: DeviceContext; name: string; save: (value: string) => void }) {
  const labelId = useId();
  const locked = ctx.locked || !ctx.canConfigureHost;
  return (
    <OptionRow label={t("Name")} labelId={labelId} hint={t("Tells your Handys apart in the device panel and the top bar.")}>
      <CommitInput aria-labelledby={labelId} value={name} disabled={locked} spellCheck={false}
        onCommit={(value) => { if (value) save(value); }} />
    </OptionRow>
  );
}

function ModelRow({ ctx, id }: { ctx: DeviceContext; id: HandyInstanceId }) {
  const save = useSaveHandy(id);
  const models = ctx.settings?.options?.handy_models ?? DEFAULT_HANDY_MODELS;
  const [model, setModel] = useLiveSetting(handyConnection(ctx.settings, id).handy_model || "handy_original", (handy_model) => save({ handy_model }));
  return (
    <div className="form-rows device-rows">
      <OptionRow label={t("Model")} hint={t("Sets this Handy's travel. With several Handys in playback, the shared pace follows the slowest model.")}>
        <SegmentedChoice className="device-segmented" label={t("Handy model")} value={model} disabled={ctx.locked || !ctx.canConfigureHost}
          options={models.map((value) => ({ value, label: handyModelLabel(value) }))} onChange={setModel} />
        <small className="quick-handy-spec">{handyModelDetail(model)}</small>
      </OptionRow>
    </div>
  );
}
