// The device kit's row and Level (docs/ui-design-guidelines.md, Device kit).
// One anatomy for every device: glyph, name, In playback switch, then the
// status dot and word with the one action that fits it. Connect is the only
// filled action in a row; level steps are neutral and never start output.
import { Fragment, useId } from "react";
import { api } from "../api/client";
import { ArrowRightIcon } from "../shell/icons";
import { t } from "../i18n";
import { statusWord } from "./status";
import type { DeviceAction, DeviceContext, DeviceDescriptor, DeviceLevelGroup, DeviceLevelModel, DeviceModel } from "./types";

export function deviceRoute(id: string): string {
  return `#/settings/devices/${id}`;
}

/** The status word in the text colour, then its details. */
export function DeviceStatusLine({ model, extra }: { model: DeviceModel; extra?: string }) {
  const details = extra ? [...model.details, extra] : model.details;
  return (
    <span className="device-status-line">
      <span className="device-dot" data-status={model.status} aria-hidden="true" />
      <span className="device-status-text">
        <strong>{statusWord(model.status)}</strong>
        {details.map((detail, index) => <Fragment key={index}>{" · "}{detail}</Fragment>)}
      </span>
    </span>
  );
}

export function DeviceActionButton({ action, model, onNavigate, size = "sm" }: {
  action: DeviceAction;
  model: DeviceModel;
  onNavigate?: () => void;
  size?: "sm" | "md";
}) {
  const small = size === "sm" ? " btn-sm" : "";
  if (action.kind === "setup") {
    return (
      <a className={`btn btn-secondary${small} device-action`} href={deviceRoute(model.id)} onClick={onNavigate}
        aria-label={t("Set up {device}", { device: model.name })}>
        {t("Set up")}<ArrowRightIcon size={12} />
      </a>
    );
  }
  // In a row Disconnect is quiet; on a device page it is an ordinary button.
  const style = action.kind === "connect" ? "btn-primary" : action.kind === "reconnect" || size === "md" ? "btn-secondary" : "btn-quiet";
  const label = action.kind === "connect"
    ? action.busy ? t("Connecting…") : t("Connect")
    : action.kind === "reconnect" ? t("Reconnect") : t("Disconnect");
  const name = action.kind === "connect"
    ? t("Connect {device}", { device: model.name })
    : action.kind === "reconnect" ? t("Reconnect {device}", { device: model.name }) : t("Disconnect {device}", { device: model.name });
  return (
    <button type="button" className={`btn ${style}${small} device-action`} aria-label={name} title={action.reason}
      aria-busy={action.busy || undefined} disabled={action.disabled} onClick={action.run}>
      {label}
    </button>
  );
}

export function DeviceRow({ descriptor, model, ctx, onNavigate }: {
  descriptor: DeviceDescriptor;
  model: DeviceModel;
  ctx: DeviceContext;
  onNavigate: () => void;
}) {
  const nameId = useId();
  const { Glyph } = descriptor;
  const watching = ctx.readOnly;
  const switchLocked = ctx.locked || !ctx.canConfigureHost || Boolean(ctx.busy[model.id]);
  function setPlayback(included: boolean) {
    void ctx.run(model.id, "playback", async () => {
      const result = await api.changeDeviceConnection(model.id, included ? "include" : "exclude");
      if ("warning" in result && result.warning) throw new Error(t("the setting was saved, but the device runtime could not be updated"));
    });
  }
  return (
    <div className="device-row" role="group" aria-labelledby={nameId} data-status={model.status}>
      <span className="device-glyph-tile" aria-hidden="true"><Glyph /></span>
      <span className="device-row-name" id={nameId}>{model.name}</span>
      <span className="toggle device-row-switch">
        <input type="checkbox" role="switch" aria-label={t("Use {device} in playback", { device: model.name })}
          checked={model.inPlayback} disabled={switchLocked} onChange={(event) => setPlayback(event.target.checked)} />
        <span className="track" aria-hidden="true" />
      </span>
      <div className="device-row-status">
        <DeviceStatusLine model={model} extra={watching ? model.readout : undefined} />
        {!watching && model.action && <DeviceActionButton action={model.action} model={model} onNavigate={onNavigate} />}
      </div>
      {!watching && model.levels && <DeviceLevels group={model.levels} />}
    </div>
  );
}

export function DeviceLevels({ group }: { group: DeviceLevelGroup }) {
  const several = group.levels.length > 1;
  return (
    <div className="device-levels" role="group" aria-label={group.name}>
      {(several || group.zero) && (
        <div className="device-levels-head">
          <span>{group.label}</span>
          {group.zero && <button type="button" className="btn btn-quiet btn-sm" disabled={group.zero.disabled} onClick={group.zero.run}>{group.zero.label}</button>}
        </div>
      )}
      {group.levels.map((level) => <DeviceLevel key={level.id} level={level} />)}
      {group.note && <p className="hint device-levels-note">{group.note}</p>}
    </div>
  );
}

/** A live level: "N of ceiling" with neutral steps and the one green Test. */
export function DeviceLevel({ level }: { level: DeviceLevelModel }) {
  const value = level.value === null
    ? t("Confirming strength…")
    : t("{value} of {ceiling}", { value: level.unit === "%" ? t("{n}%", { n: level.value }) : level.value, ceiling: level.ceiling });
  return (
    <div className="device-level">
      <span className="device-level-label">{level.label}</span>
      <output className="device-level-value" aria-label={t("{state}: {message}", { state: level.label, message: value })}>{value}</output>
      <button type="button" className="device-step" aria-label={level.lowerLabel} disabled={!level.canLower} onClick={level.lower}>−</button>
      <button type="button" className="device-step" aria-label={level.raiseLabel} disabled={!level.canRaise} onClick={level.raise}>+</button>
      <button type="button" className="btn btn-start btn-sm device-level-test" aria-label={level.testLabel} title={t("Test plays while motion is paused.")}
        disabled={!level.canTest} onClick={level.test}>{t("Test")}</button>
    </div>
  );
}
