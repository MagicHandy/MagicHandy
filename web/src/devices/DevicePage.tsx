// A device's page under Settings > Devices: its name, status and one
// action, then the sections its descriptor lists, then Remove.
import { useId } from "react";
import { api } from "../api/client";
import { t } from "../i18n";
import { ArrowRightIcon } from "../shell/icons";
import { confirmThen } from "../util/confirm";
import { DeviceActionButton, DeviceStatusLine } from "./DeviceRow";
import type { DeviceContext, DeviceDescriptor, DeviceModel } from "./types";

export function DevicePage({ descriptor, ctx, model }: { descriptor: DeviceDescriptor; ctx: DeviceContext; model?: DeviceModel }) {
  const name = descriptor.name();
  const locked = ctx.locked || !ctx.canConfigureHost;
  const breadcrumb = (
    <nav className="device-breadcrumb" aria-label={t("Breadcrumb")}>
      <a href="#/settings/devices">{t("Devices")}</a>
      <ArrowRightIcon size={12} className="device-breadcrumb-separator" />
      <span aria-current="page">{name}</span>
    </nav>
  );

  if (!model) {
    return (
      <div className="device-page">
        {breadcrumb}
        <div className="group device-not-added">
          <p>{t("{device} is not added.", { device: name })}</p>
          {descriptor.addNotice && <p className="hint">{descriptor.addNotice()}</p>}
          <button type="button" className="btn btn-primary" disabled={locked || Boolean(ctx.busy[descriptor.id])}
            onClick={() => void ctx.run(descriptor.id, "add", async () => { await api.changeDeviceConnection(descriptor.id, "add"); })}>
            {t("Add {device}", { device: name })}
          </button>
        </div>
      </div>
    );
  }

  function remove() {
    confirmThen(t("Remove {device}? {consequence}", { device: name, consequence: descriptor.removeNote() }), { confirmLabel: t("Remove {device}", { device: name }), destructive: true }, () => {
      void ctx.run(descriptor.id, "remove", async () => {
        await (descriptor.remove ? descriptor.remove() : api.changeDeviceConnection(descriptor.id, "remove"));
      }).then((removed) => {
        if (removed) window.location.hash = "#/settings/devices";
      });
    });
  }

  // On its own page a device needs no Set up: the sections below are it.
  const action = model.action?.kind === "setup" || ctx.readOnly ? null : model.action;
  return (
    <div className="device-page">
      {breadcrumb}
      <div className="device-page-head">
        <span className="device-glyph-tile device-glyph-tile-page" aria-hidden="true"><descriptor.Glyph /></span>
        <div className="device-page-title">
          <h2>{name}</h2>
          <DeviceStatusLine model={model} extra={model.inPlayback ? undefined : t("not in playback")} />
        </div>
        {action && <DeviceActionButton action={action} model={model} size="md" />}
      </div>
      {descriptor.sections.map((section) => <PageSection key={section.id} title={section.title()} section={section} ctx={ctx} model={model} />)}
      <div className="device-remove">
        <button type="button" className="btn btn-danger-outline" disabled={locked || Boolean(ctx.busy[descriptor.id])} onClick={remove}>
          {t("Remove {device}", { device: name })}
        </button>
        <span className="hint">{descriptor.removeNote()}</span>
      </div>
    </div>
  );
}

function PageSection({ title, section, ctx, model }: { title: string; section: DeviceDescriptor["sections"][number]; ctx: DeviceContext; model: DeviceModel }) {
  const titleId = useId();
  return (
    <section className="group device-section" aria-labelledby={titleId}>
      <h3 className="group-title" id={titleId}>{title}</h3>
      <section.Body ctx={ctx} model={model} />
    </section>
  );
}
