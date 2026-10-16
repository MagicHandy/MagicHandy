// Settings > Devices: your devices, how they play together, and the shared
// connections they use. Each device opens its own page. Every option saves
// as it changes; only the Handy's connection key keeps an explicit Save.
import { useEffect, useId, useRef, useState } from "react";
import { api } from "../api/client";
import type { DeviceId } from "../api/types";
import { t } from "../i18n";
import { ArrowRightIcon, CheckIcon, PlusIcon } from "../shell/icons";
import { confirmThen } from "../util/confirm";
import { DevicePage } from "./DevicePage";
import { deviceRoute } from "./DeviceRow";
import { CommitInput, NumberInput, OptionRow, useLiveSetting, useSaveDeviceOptions } from "./fields";
import { HostBluetoothGlyph, IntifaceGlyph } from "./glyphs";
import { deviceDescriptor, useDevices, type DeviceEntry } from "./registry";
import { useDeviceWatch } from "./runtime";
import { statusWord } from "./status";
import type { DeviceContext, DeviceDescriptor } from "./types";

/** `page` is the route after #/settings/devices: empty, add, or a device id. */
export function DevicesSettings({ page }: { page: string }) {
  const { ctx, devices, addable } = useDevices();
  useDeviceWatch();
  const descriptor = deviceDescriptor(page);
  if (descriptor) return <DevicePage descriptor={descriptor} ctx={ctx} model={devices.find((entry) => entry.descriptor.id === descriptor.id)?.model} />;
  return (
    <>
      <h2 className="section-title">{t("Devices")}</h2>
      <p className="devices-saves-note"><CheckIcon size={12} />{t("Changes save as you make them")}</p>
      <YourDevices ctx={ctx} devices={devices} addable={addable} adding={page === "add"} />
      {devices.length > 0 && <Playback ctx={ctx} devices={devices} />}
      <Connections ctx={ctx} devices={devices} />
    </>
  );
}

function YourDevices({ ctx, devices, addable, adding }: { ctx: DeviceContext; devices: DeviceEntry[]; addable: DeviceDescriptor[]; adding: boolean }) {
  const titleId = useId();
  return (
    <section className="group device-list-group" aria-labelledby={titleId}>
      <h3 className="group-title" id={titleId}>{t("Your devices")}</h3>
      {devices.length > 0 && (
        <ul className="device-list">
          {devices.map(({ descriptor, model }) => (
            <li key={descriptor.id}>
              <a className="device-list-row" href={deviceRoute(descriptor.id)}>
                <span className="device-glyph-tile" aria-hidden="true"><descriptor.Glyph /></span>
                <span className="device-list-copy"><strong>{model.name}</strong><small>{model.summary}</small></span>
                <span className="device-list-status"><span className="device-dot" data-status={model.status} aria-hidden="true" />{statusWord(model.status)}</span>
                <ArrowRightIcon size={12} className="device-list-chevron" />
              </a>
            </li>
          ))}
        </ul>
      )}
      <AddDevice ctx={ctx} addable={addable} adding={adding || devices.length === 0} />
    </section>
  );
}

function AddDevice({ ctx, addable, adding }: { ctx: DeviceContext; addable: DeviceDescriptor[]; adding: boolean }) {
  const listId = useId();
  const [open, setOpen] = useState(adding);
  const first = useRef<HTMLButtonElement>(null);
  const locked = ctx.locked || !ctx.canConfigureHost;
  useEffect(() => {
    if (!adding) return;
    setOpen(true);
    first.current?.focus();
  }, [adding]);

  if (!addable.length) {
    return (
      <p className="device-add-full">
        <span className="device-glyph-tile device-glyph-tile-add" aria-hidden="true"><PlusIcon size={14} /></span>
        {t("Every device is added. Each can be added once.")}
      </p>
    );
  }

  async function add(id: DeviceId) {
    const added = await ctx.run(id, "add", async () => {
      const result = await api.changeDeviceConnection(id, "add");
      if ("warning" in result && result.warning) throw new Error(t("the setting was saved, but the device runtime could not be updated"));
    });
    if (added) window.location.hash = deviceRoute(id);
  }

  return (
    <div className="device-add">
      <button type="button" className="device-add-toggle" aria-expanded={open} aria-controls={listId} disabled={locked} onClick={() => setOpen(!open)}>
        <span className="device-glyph-tile device-glyph-tile-add" aria-hidden="true"><PlusIcon size={14} /></span>
        <span>{t("Add device")}</span>
      </button>
      {open && (
        <div id={listId} className="device-add-list">
          {addable.map((descriptor, index) => (
            <button key={descriptor.id} ref={index === 0 ? first : undefined} type="button" className="device-add-choice"
              disabled={locked || Boolean(ctx.busy[descriptor.id])} onClick={() => void add(descriptor.id)}>
              <span className="device-glyph-tile" aria-hidden="true"><descriptor.Glyph /></span>
              <span className="device-list-copy"><strong>{descriptor.name()}</strong><small>{descriptor.addDetail()}</small></span>
              <PlusIcon size={14} className="device-list-chevron" />
            </button>
          ))}
          <p className="hint">{t("Adding saves the device, ready to set up. It does not connect it or add it to playback.")}</p>
        </div>
      )}
    </div>
  );
}

function Playback({ ctx, devices }: { ctx: DeviceContext; devices: DeviceEntry[] }) {
  const titleId = useId();
  const policyId = useId();
  const save = useSaveDeviceOptions();
  const locked = ctx.locked || !ctx.canConfigureHost;
  const [policy, setPolicy] = useLiveSetting(ctx.settings?.device?.linked_failure_policy ?? "stop_all", (value) => save({ linked_failure_policy: value }));
  const timed = devices.filter((entry) => entry.descriptor.timing);
  return (
    <section className="group" aria-labelledby={titleId}>
      <h3 className="group-title" id={titleId}>{t("Playback")}</h3>
      <div className="form-rows device-rows">
        <OptionRow label={t("On device failure")} labelId={policyId} hint={t("Applies to unexpected disconnections and playback errors during a linked run. Reconnecting never rejoins a run automatically. Emergency Stop requests Stop for every device.")}>
          <select aria-labelledby={policyId} value={policy} disabled={locked} onChange={(event) => setPolicy(event.target.value as "stop_all" | "continue_healthy")}>
            <option value="stop_all">{t("Stop every device (recommended)")}</option>
            <option value="continue_healthy">{t("Keep going on healthy devices")}</option>
          </select>
        </OptionRow>
        {timed.length > 0 && (
          <div className="form-row form-row-stack device-timing">
            <span className="form-row-label">
              <strong>{t("Timing")}</strong>
              <small>{t("Shift a device against motion and video. Positive values delay it.")}</small>
            </span>
            <div className="device-timing-grid">
              {timed.map((entry) => <TimingField key={entry.descriptor.id} ctx={ctx} entry={entry} locked={locked} />)}
            </div>
          </div>
        )}
      </div>
      <p className="hint-block">{t("Changing playback devices or the failure policy stops the current run.")}</p>
    </section>
  );
}

function TimingField({ ctx, entry, locked }: { ctx: DeviceContext; entry: DeviceEntry; locked: boolean }) {
  const id = useId();
  const save = useSaveDeviceOptions();
  const timing = entry.descriptor.timing;
  if (!timing || !ctx.settings) return null;
  return (
    <div className="device-timing-field">
      <label htmlFor={id} className="device-timing-name"><entry.descriptor.Glyph className="device-timing-glyph" />{entry.model.name}</label>
      <span className="device-timing-input">
        <NumberInput id={id} value={timing.value(ctx.settings)} min={-300} max={300} step={10} disabled={locked}
          onCommit={(value) => void save(timing.patch(value)).catch(() => undefined)} />
        <span>{t("ms")}</span>
      </span>
    </div>
  );
}

function Connections({ ctx, devices }: { ctx: DeviceContext; devices: DeviceEntry[] }) {
  const titleId = useId();
  return (
    <section className="group" aria-labelledby={titleId}>
      <h3 className="group-title" id={titleId}>{t("Connections")}</h3>
      <div className="form-rows device-rows">
        <IntifaceConnection ctx={ctx} devices={devices} />
        <HostBluetoothConnection ctx={ctx} devices={devices} />
      </div>
    </section>
  );
}

function usersOf(ctx: DeviceContext, devices: DeviceEntry[], connection: "intiface" | "host_bluetooth"): string {
  return devices.filter((entry) => entry.descriptor.connection(ctx.settings) === connection).map((entry) => entry.model.name).join(", ");
}

// One Intiface Central connection serves every device that uses it: one
// address, one status, one Disconnect.
function IntifaceConnection({ ctx, devices }: { ctx: DeviceContext; devices: DeviceEntry[] }) {
  const addressId = useId();
  const save = useSaveDeviceOptions();
  const status = ctx.intiface.status;
  const leases = (ctx.intiface.leases ?? []).filter((lease): lease is "handy" | "vibrator" => lease === "handy" || lease === "vibrator");
  const users = usersOf(ctx, devices, "intiface");
  const holders = devices.filter((entry) => leases.includes(entry.descriptor.id as "handy" | "vibrator")).map((entry) => entry.model.name).join(", ");
  const address = ctx.settings?.device?.intiface_server_address ?? "";
  const busy = leases.some((lease) => ctx.busy[lease]);
  const locked = ctx.locked || !ctx.canConfigureHost;

  async function disconnect() {
    for (const lease of leases) {
      await ctx.run(lease, "disconnect", async () => { ctx.setIntiface(await api.intifaceDisconnect(lease)); });
    }
  }

  function changeAddress(value: string) {
    const apply = () => void save({ intiface_server_address: value }).catch(() => undefined);
    if (status.connected) confirmThen(t("Use the new address? Intiface Central disconnects every device that uses it."), { confirmLabel: t("Use this address") }, apply);
    else apply();
  }

  return (
    <>
      <div className="form-row device-connection-row">
        <span className="device-connection-name">
          <span className="device-glyph-tile" aria-hidden="true"><IntifaceGlyph /></span>
          <span className="form-row-label">
            <strong>{t("Intiface Central")}</strong>
            <small>{t("Shared by every device that uses it")}</small>
          </span>
        </span>
        <span className="device-connection-state">
          <span className="device-dot" data-status={status.connected ? "connected" : "not_connected"} aria-hidden="true" />
          {status.connected
            ? holders ? t("Connected · used by {devices}", { devices: holders }) : t("Connected")
            : users ? t("{state} · used by {devices}", { state: t("Not connected"), devices: users }) : t("Not connected")}
        </span>
        {status.connected && leases.length > 0 && (
          <button type="button" className="btn btn-secondary" disabled={ctx.locked || busy} onClick={() => void disconnect()}>{t("Disconnect")}</button>
        )}
      </div>
      <OptionRow label={t("Server address")} labelId={addressId}
        hint={status.connected
          ? ctx.intiface.ping_watchdog === false
            ? t("The keepalive timeout is off: if MagicHandy stops answering, only a closed connection stops its devices.")
            : t("Keepalive is on: Intiface Central stops its devices if MagicHandy stops answering.")
          : t("A device's Connect opens this connection when it needs it.")}>
        <CommitInput type="url" className="device-address-input" aria-labelledby={addressId} spellCheck={false}
          value={address} disabled={locked} onCommit={changeAddress} />
      </OptionRow>
    </>
  );
}

function HostBluetoothConnection({ ctx, devices }: { ctx: DeviceContext; devices: DeviceEntry[] }) {
  const users = usersOf(ctx, devices, "host_bluetooth");
  const unsupported = ctx.native?.supported === false;
  const missing = ctx.native?.available === false || (ctx.coyote ? !ctx.coyote.available && !ctx.coyote.status.simulated : false);
  const state = unsupported ? t("Not available on this system") : missing ? t("Helper missing") : t("Ready");
  return (
    <div className="form-row device-connection-row">
      <span className="device-connection-name">
        <span className="device-glyph-tile" aria-hidden="true"><HostBluetoothGlyph /></span>
        <span className="form-row-label">
          <strong>{t("Bluetooth on this computer")}</strong>
          <small>{t("MagicHandy's Bluetooth helper")}</small>
        </span>
      </span>
      <span className="device-connection-state">
        <span className="device-dot" data-status={unsupported || missing ? "lost" : "connected"} aria-hidden="true" />
        {users ? t("{state} · used by {devices}", { state, devices: users }) : state}
      </span>
    </div>
  );
}
