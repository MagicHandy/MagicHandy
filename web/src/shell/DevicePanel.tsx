// The top-bar device chip and the device panel it opens: live jobs only, on
// every route. Status, Connect, In playback, the live levels with Test and
// the Speed and Stroke limits. Setup and configuration live in Settings >
// Devices, which Device settings opens (docs/ui-design-guidelines.md).
import { useEffect, useId, useRef } from "react";
import { QuickSettings } from "../components/QuickSettings";
import { DeviceRow } from "../devices/DeviceRow";
import { DevicesGlyph } from "../devices/glyphs";
import { useDevices } from "../devices/registry";
import { useDeviceWatch } from "../devices/runtime";
import { chipPhrase, chipSummary } from "../devices/status";
import type { DeviceContext } from "../devices/types";
import { t } from "../i18n";
import { ArrowRightIcon, ChevronUpIcon, CloseIcon, PlusIcon, SettingsIcon } from "./icons";
import { useTakeControl } from "./useTakeControl";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  restoreFocusOnClose?: boolean;
}

export function DevicePanel({ open, onOpenChange, restoreFocusOnClose = true }: Props) {
  const { ctx, devices, initializing } = useDevices();
  const triggerRef = useRef<HTMLButtonElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);
  const wasOpen = useRef(false);
  const restoreFocus = useRef(true);
  const titleId = useId();
  const limitsId = useId();
  useDeviceWatch(open);

  useEffect(() => {
    if (open) closeRef.current?.focus();
    else if (wasOpen.current && restoreFocus.current && restoreFocusOnClose) triggerRef.current?.focus();
    wasOpen.current = open;
    restoreFocus.current = true;
  }, [open, restoreFocusOnClose]);

  useEffect(() => {
    const show = () => onOpenChange(true);
    window.addEventListener("magichandy:open-devices", show);
    return () => window.removeEventListener("magichandy:open-devices", show);
  }, [onOpenChange]);

  // A link that leaves for Settings hands focus to the routed page.
  const navigate = () => {
    restoreFocus.current = false;
    onOpenChange(false);
  };
  const models = devices.map((device) => device.model);
  const summary = initializing ? { title: t("Devices"), status: t("Checking…") } : chipSummary(models);
  const statuses = initializing ? [t("Checking…")] : models.length ? models.map(chipPhrase) : [t("Chat only")];
  const chipLabel = [t("Devices"), ...statuses, open ? t("Close the device panel") : t("Open the device panel")].join(". ");

  return (
    <div className="device-panel-anchor" data-open={open}>
      {open && <div className="device-sheet-scrim" aria-hidden="true" onClick={() => onOpenChange(false)} />}
      <section id="device-panel" className="device-panel" aria-labelledby={titleId} hidden={!open}>
        <header className="device-panel-head">
          <h2 id={titleId}>{t("Devices")}</h2>
          {devices.length > 0 && (
            <a className="btn btn-secondary device-panel-settings-head" href="#/settings/devices" onClick={navigate}>
              <SettingsIcon size={16} />{t("Device settings")}
            </a>
          )}
          <button ref={closeRef} type="button" className="icon-button device-panel-close" aria-label={t("Close the device panel")} onClick={() => onOpenChange(false)}>
            <CloseIcon />
          </button>
        </header>
        <PanelNotices ctx={ctx} />
        {initializing ? (
          <p className="form-status device-panel-reason" role="status">{t("Checking devices…")}</p>
        ) : devices.length === 0 ? (
          <div className="device-panel-empty">
            <div>
              <strong>{t("No devices yet")}</strong>
              <p>{t("Chat works without one. Add a device to play motion.")}</p>
            </div>
            {ctx.canConfigureHost && (
              <a className="btn btn-primary" href="#/settings/devices/add" onClick={navigate}><PlusIcon size={14} />{t("Add a device")}</a>
            )}
          </div>
        ) : (
          <>
            <div className="device-panel-columns" aria-hidden="true"><span>{t("Device")}</span><span>{t("In playback")}</span></div>
            {devices.map(({ descriptor, model }) => (
              <DeviceRow key={descriptor.id} descriptor={descriptor} model={model} ctx={ctx} onNavigate={navigate} />
            ))}
            <div className="device-panel-limits" role="group" aria-labelledby={limitsId}>
              <div className="device-panel-section-head">
                <h3 id={limitsId}>{t("Limits")}</h3>
                {!ctx.readOnly && <span>{t("Apply immediately")}</span>}
              </div>
              {ctx.readOnly ? <LimitsReadout ctx={ctx} /> : <QuickSettings section="limits" />}
            </div>
            <div className="device-panel-foot">
              <a className="btn btn-secondary device-panel-settings" href="#/settings/devices" onClick={navigate}>
                <SettingsIcon size={15} />{t("Device settings")}<ArrowRightIcon size={12} className="device-panel-settings-chevron" />
              </a>
            </div>
          </>
        )}
      </section>

      <button
        ref={triggerRef}
        type="button"
        className="device-chip"
        aria-label={chipLabel}
        aria-controls="device-panel"
        aria-expanded={open}
        onClick={() => onOpenChange(!open)}
      >
        <span className="device-chip-glyphs" aria-hidden="true">
          {devices.length ? devices.map(({ descriptor, model }) => (
            <span key={descriptor.id} className="device-chip-glyph" data-status={model.status}>
              <descriptor.Glyph />
              <span className="device-dot device-chip-dot" data-status={model.status} />
            </span>
          )) : <span className="device-chip-glyph"><DevicesGlyph /></span>}
        </span>
        <span className="device-chip-copy">
          <strong>{summary.title}</strong>
          <small>{summary.status}</small>
        </span>
        <ChevronUpIcon size={14} className="device-chip-chevron" />
      </button>
    </div>
  );
}

/** One reason and one way out: offline, a tab that watches, or a run that
 * lost a device. */
function PanelNotices({ ctx }: { ctx: DeviceContext }) {
  const { takeControl, takingControl, handoffInProgress } = useTakeControl();
  const linked = ctx.state?.motion?.engine?.linked_session;
  const names = ctx.retired.map((id) => id === "handy" ? t("The Handy") : id === "coyote" ? t("Coyote") : t("Vibrator"));
  return (
    <>
      {!ctx.backendOnline && <p className="form-status device-panel-reason">{t("Core offline.")}</p>}
      {ctx.backendOnline && ctx.readOnly && (
        <div className="device-panel-watch">
          <p>{ctx.state?.capabilities?.control === false
            ? t("You can watch here. Controlling devices needs a control permission.")
            : t("Another tab controls MagicHandy. You can watch here.")}</p>
          {ctx.state?.capabilities?.control !== false && (
            <button type="button" className="btn btn-secondary btn-sm" disabled={takingControl || handoffInProgress} onClick={takeControl}>{t("Take control")}</button>
          )}
        </div>
      )}
      {ctx.backendOnline && !ctx.readOnly && !ctx.canConfigureHost && (
        <p className="form-status device-panel-reason">{t("Host settings and diagnostics are managed by an administrator.")}</p>
      )}
      {linked?.status === "degraded" && names.length > 0 && (
        <div className="device-panel-notice" role="status">
          <strong>{t("Continuing without {devices}", { devices: names.join(", ") })}</strong>
          <span>{t("Stop and start again to rejoin.")}</span>
        </div>
      )}
      {linked?.status === "failed" && (
        <div className="device-panel-notice" role="status">
          <strong>{t("Playback stopped after a device failure.")}</strong>
          <span>{t("Reconnect the device, then start again.")}</span>
        </div>
      )}
    </>
  );
}

/** The limits as text, for a tab that only watches. */
function LimitsReadout({ ctx }: { ctx: DeviceContext }) {
  const motion = ctx.settings?.motion;
  if (!motion) return null;
  return (
    <dl className="device-limits-readout">
      <div><dt>{t("Speed")}</dt><dd>{t("{a}–{b}%", { a: motion.speed_min_percent, b: motion.speed_max_percent })}</dd></div>
      {ctx.playing.includes("handy") && <div><dt>{t("Stroke")}</dt><dd>{t("{a}–{b}%", { a: motion.stroke_min_percent, b: motion.stroke_max_percent })}</dd></div>}
    </dl>
  );
}
