// The Handy's device page: Connection, Calibration and Test. An additional
// Handy's page reuses the method choice, the key, the native Bluetooth rows
// and the notices below with its own ID.
import { useEffect, useId, useState, type FormEvent } from "react";
import { api } from "../api/client";
import { isHandy } from "../api/devices";
import type { DeviceId, HandyInstanceId } from "../api/types";
import { DismissibleNotice } from "../components/DismissibleNotice";
import { ManualMotionTest } from "../components/ManualMotionTest";
import { QuickSettings } from "../components/QuickSettings";
import { SetupChoice } from "../components/SetupChoice";
import { t } from "../i18n";
import { confirmThen } from "../util/confirm";
import { CommitInput, OptionRow, useLiveSetting, useSaveDeviceOptions } from "./fields";
import { IntifaceGlyph } from "./glyphs";
import { handyConnectionMethods, type ConnectionMethod } from "./handy-methods";
import { IntifaceDeviceChoice } from "./IntifaceDeviceChoice";
import { NearbyHandys } from "./NearbyHandys";
import { useBluetoothDetailsTarget } from "./runtime";
import type { DeviceContext, DeviceSectionProps } from "./types";

export function HandyConnectionSection({ ctx }: DeviceSectionProps) {
  const owner = ctx.settings?.device?.hsp_dispatch_owner ?? "";
  const save = useSaveDeviceOptions();
  const advertised = ctx.settings?.options?.hsp_dispatch_owners;
  const native = ctx.native;

  function forgetKey() {
    confirmThen(t("Forget the saved connection key? The Handy cannot connect through Cloud until you save a key again."), { confirmLabel: t("Forget key"), destructive: true },
      () => void save({ clear_connection_key: true }).catch(() => undefined));
  }

  return (
    <div className="form-rows device-rows">
      <MethodChoice ctx={ctx} device="handy" current={owner}
        methods={handyConnectionMethods().filter((method) => !advertised?.length || advertised.includes(method.value))}
        hint={t("Changing the method stops motion and disconnects The Handy.")}
        confirm={(method) => t("Switch The Handy to {method}? Motion stops and The Handy disconnects.", { method })}
        apply={async (method) => {
          const result = await api.saveSetupPreferences({ device_owner: method });
          if ("warning" in result && result.warning) throw new Error(t("the setting was saved, but the device runtime could not be updated"));
        }} />
      {owner === "cloud_rest" && <>
        <ConnectionKeyRow ctx={ctx} device="handy" keySet={Boolean(ctx.settings?.device?.connection_key_set)}
          save={(key) => api.saveConnectionKey(key)} forget={forgetKey} />
        <CloudCheckRow ctx={ctx} />
      </>}
      {owner === "native_bluetooth" && (
        <NativeBluetoothRows ctx={ctx} device="handy" supported={native?.supported} available={native?.available}
          remembered={Boolean(native?.remembered_device)} connected={Boolean(native?.status.connected)}
          forget={() => void ctx.run("handy", "forget", async () => { await api.nativeBluetoothForget(); })} />
      )}
      {owner === "browser_bluetooth" && <BrowserBluetoothRow />}
      {owner === "intiface" && <>
        <IntifaceStopNotice />
        <IntifaceConnectionRow ctx={ctx} />
        <div className="form-row form-row-plain"><IntifaceDeviceChoice ctx={ctx} device="handy" /></div>
      </>}
      {owner === "cloud_rest" && <ApplicationIdDetails ctx={ctx} />}
    </div>
  );
}

// Browsing the methods changes nothing; Use this method applies the choice,
// because the backend stops motion and closes the old session to switch.
export function MethodChoice({ ctx, device, current, methods, hint, note, confirm, apply }: {
  ctx: DeviceContext;
  device: DeviceId;
  current: string;
  methods: ConnectionMethod[];
  /** What applying does, beside the button. */
  hint: string;
  /** Why a method is not offered, under the choices. */
  note?: string;
  /** The question while motion runs, for the chosen method's label. */
  confirm: (method: string) => string;
  apply: (method: string) => Promise<unknown>;
}) {
  const labelId = useId();
  const [chosen, setChosen] = useState(current);
  useEffect(() => setChosen(current), [current]);
  const locked = ctx.locked || !ctx.canConfigureHost;
  const busy = Boolean(ctx.busy[device]);
  const chosenLabel = methods.find((method) => method.value === chosen)?.label ?? chosen;

  function use() {
    const change = () => void ctx.run(device, "method", async () => { await apply(chosen); });
    if (ctx.engineActive) confirmThen(confirm(chosenLabel), { confirmLabel: t("Use this method") }, change);
    else change();
  }

  return (
    <div className="form-row form-row-stack device-method">
      <span className="form-row-label"><strong id={labelId}>{t("Method")}</strong></span>
      <div className="setup-choices" role="radiogroup" aria-labelledby={labelId}>
        {methods.map((method) => (
          <SetupChoice key={method.value} selected={chosen === method.value} title={method.label} detail={method.detail} disabled={locked || busy} onSelect={() => setChosen(method.value)} />
        ))}
      </div>
      {note && <span className="hint">{note}</span>}
      <div className="row-actions">
        {chosen !== current && <button type="button" className="btn btn-primary" disabled={locked || busy} onClick={use}>{t("Use this method")}</button>}
        <span className="hint">{hint}</span>
      </div>
    </div>
  );
}

// The one device option with an explicit Save: a key is pasted whole.
export function ConnectionKeyRow({ ctx, device, keySet, save, forget }: {
  ctx: DeviceContext;
  device: DeviceId;
  keySet: boolean;
  save: (key: string) => Promise<unknown>;
  /** Asks first, then forgets the saved key. */
  forget: () => void;
}) {
  const id = useId();
  const hintId = useId();
  const [key, setKey] = useState("");
  const busy = ctx.busy[device] === "key";
  const locked = ctx.locked || !ctx.canConfigureHost;

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const next = key.trim();
    if (!next) return;
    void ctx.run(device, "key", async () => {
      await save(next);
      setKey("");
      return t("Handy connection key saved.");
    });
  }

  return (
    <form className="form-row device-key-row" onSubmit={submit}>
      <span className="form-row-label">
        <label htmlFor={id}><strong>{t("Connection key")}</strong></label>
        <small id={hintId}>{t("From the Handy app. Stored on this computer and never shown again.")}</small>
      </span>
      <div className="form-input-action">
        <input id={id} type="password" autoComplete="off" spellCheck={false} aria-describedby={hintId}
          placeholder={keySet ? t("Saved · paste to replace") : t("Paste connection key")}
          value={key} disabled={locked || busy} onChange={(event) => setKey(event.target.value)} />
        <button type="submit" className="btn btn-secondary" disabled={locked || busy || !key.trim()}>{busy ? t("Saving") : t("Save")}</button>
      </div>
      {keySet && <button type="button" className="btn btn-quiet device-key-forget" disabled={locked || Boolean(ctx.busy[device])} onClick={forget}>{t("Forget key")}</button>}
    </form>
  );
}

export function CloudFirmwareNotice() {
  return (
    <DismissibleNotice id="device-firmware" className="device-requirement">
      <span className="label">{t("Firmware / API requirement")}</span>
      <p>{t("Cloud REST requires Handy firmware v4 with API v3 access.")}</p>
    </DismissibleNotice>
  );
}

export function IntifaceStopNotice() {
  return (
    <DismissibleNotice id="intiface-handy-stop-322" className="device-requirement">
      <span className="label">{t("Intiface Stop limitation")}</span>
      <p>{t("Intiface Central 3.2.2 may report Stop without stopping The Handy. Native Bluetooth or Handy Cloud sends the device's Stop command directly.")}</p>
    </DismissibleNotice>
  );
}

// Check asks the Handy for its state without moving it or reacquiring a
// released session; Connect in the device header does that.
function CloudCheckRow({ ctx }: { ctx: DeviceContext }) {
  const keySet = Boolean(ctx.settings?.device?.connection_key_set);
  const busy = ctx.busy.handy === "check";
  function check() {
    void ctx.run("handy", "check", async () => {
      const result = await api.connectionCheck("cloud");
      if (!result.ok) throw new Error(result.message?.trim() || t("The Handy responded, but HSP is unavailable for its current device state."));
      return t("The Handy is reachable ({latency} ms).", { latency: result.latency_ms });
    });
  }
  return (
    <>
      <CloudFirmwareNotice />
      <OptionRow label={t("Check connection")} hint={t("Asks The Handy for its state. It never moves the device or reconnects a released one.")}>
        <button type="button" className="btn btn-secondary" disabled={ctx.locked || !keySet || Boolean(ctx.busy.handy)} onClick={check}>{busy ? t("Checking…") : t("Check")}</button>
      </OptionRow>
    </>
  );
}

export function NativeBluetoothRows({ ctx, device, supported, available, remembered, connected, forget }: {
  ctx: DeviceContext;
  device: "handy" | HandyInstanceId;
  supported?: boolean;
  available?: boolean;
  remembered: boolean;
  connected: boolean;
  forget: () => void;
}) {
  const unavailable = supported === false
    ? t("Native Bluetooth is available on Windows and Linux.")
    : available === false ? t("The native Bluetooth helper is missing. Update MagicHandy to install it.") : "";
  const locked = ctx.locked || !ctx.canConfigureHost;
  return (
    <>
      <div className="form-row form-row-plain">
        <p className="hint-block">{t("Uses Bluetooth on the computer running MagicHandy. Put the Handy in firmware 4 Bluetooth mode before connecting.")}</p>
        {unavailable && <p className="form-status" role="status">{unavailable}</p>}
      </div>
      {remembered && (
        <OptionRow label={t("Remembered device")} hint={t("Reconnects without pairing again. Forget it to use a different Handy.")}>
          <button type="button" className="btn btn-secondary" disabled={locked || Boolean(ctx.busy[device]) || connected} onClick={forget}>{t("Forget")}</button>
        </OptionRow>
      )}
      {!unavailable && <NearbyHandys ctx={ctx} device={device} connected={connected} />}
    </>
  );
}

// The browser's session lives in the shell, so leaving this page keeps it;
// only its details render here.
function BrowserBluetoothRow() {
  const setTarget = useBluetoothDetailsTarget();
  return (
    <div className="form-row form-row-stack">
      <span className="form-row-label">
        <strong>{t("Browser Bluetooth")}</strong>
        <small>{t("The browser that pairs keeps the connection, so keep its tab open while The Handy plays.")}</small>
      </span>
      <div ref={setTarget} className="device-bluetooth-details" />
    </div>
  );
}

export function IntifaceConnectionRow({ ctx }: { ctx: DeviceContext }) {
  const connected = ctx.intiface.status.connected;
  const address = ctx.intiface.address || ctx.settings?.device?.intiface_server_address || "";
  return (
    <div className="form-row device-connection-row">
      <span className="device-connection-name">
        <IntifaceGlyph className="device-connection-glyph" />
        <span className="form-row-label">
          <strong>{t("Intiface Central")}</strong>
          <small className="device-inline-status">
            <span className="device-dot" data-status={connected ? "connected" : "not_connected"} aria-hidden="true" />
            {connected ? t("Connected · {address}", { address }) : t("Not connected")}
          </small>
        </span>
      </span>
      <a href="#/settings/devices">{t("Change in Connections")}</a>
    </div>
  );
}

function ApplicationIdDetails({ ctx }: { ctx: DeviceContext }) {
  const device = ctx.settings?.device;
  const save = useSaveDeviceOptions();
  const sources = ctx.settings?.options?.api_application_id_sources?.length
    ? ctx.settings.options.api_application_id_sources
    : ["bundled_app_id", "developer_override"];
  const [source, setSource] = useLiveSetting(device?.api_application_id_source ?? "bundled_app_id", (value) => save({ api_application_id_source: value }));
  const locked = ctx.locked || !ctx.canConfigureHost;
  const labelId = useId();
  return (
    <details className="form-row-details device-advanced" open={source === "developer_override" || undefined}>
      <summary>{t("Advanced")}</summary>
      <OptionRow label={t("API application ID")} labelId={labelId} hint={t("Cloud uses MagicHandy's built-in ID unless you develop against your own.")}>
        <select aria-labelledby={labelId} value={source} disabled={locked} onChange={(event) => setSource(event.target.value)}>
          {sources.map((value) => <option key={value} value={value}>{value === "developer_override" ? t("Developer override") : value === "bundled_app_id" ? t("Built-in") : value}</option>)}
        </select>
      </OptionRow>
      {source === "developer_override" && (
        <OptionRow label={t("Developer application ID")}>
          <CommitInput aria-label={t("Developer application ID")} value={device?.api_application_id_override ?? ""} disabled={locked} spellCheck={false}
            onCommit={(value) => void save({ api_application_id_override: value }).catch(() => undefined)} />
        </OptionRow>
      )}
    </details>
  );
}

/** With several Handys, each keeps its own model and the shared pace suits the slowest. */
export function severalHandys(ctx: DeviceContext): boolean {
  return ctx.configured.filter(isHandy).length > 1;
}

export function HandyCalibrationSection({ ctx }: DeviceSectionProps) {
  return (
    <>
      <QuickSettings section="calibration" />
      {severalHandys(ctx) && <p className="hint-block">{t("Each Handy has its own model on its page. With several Handys in playback, the shared pace follows the slowest model.")}</p>}
    </>
  );
}

export function HandyTestSection({ ctx }: DeviceSectionProps) {
  return (
    <>
      <ManualMotionTest />
      {severalHandys(ctx) && <p className="hint-block">{t("The test plays on every device in playback, not only The Handy.")}</p>}
    </>
  );
}
