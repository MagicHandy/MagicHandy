// The Autoblow devices' pages (ADRs 0069 and 0071): Connection for both, Tempo
// and Stroke for the VacuGlide, and Test. Every command, Stop included,
// reaches these devices only through Autoblow's cloud.
import { api } from "../api/client";
import type { AutoblowDevice, AutoblowStatus } from "../api/types";
import { RangeSlider } from "../components/RangeSlider";
import { t } from "../i18n";
import { confirmThen } from "../util/confirm";
import { OptionRow, SLIDER_SAVE_MS, SwitchRow, useLiveSetting, useSaveDeviceOptions } from "./fields";
import { ConnectionKeyRow, type SecretRowText } from "./HandyPage";
import type { DeviceContext, DeviceSectionProps } from "./types";

export function autoblowStatus(ctx: DeviceContext, device: AutoblowDevice): AutoblowStatus | undefined {
  return ctx.autoblow?.[device];
}

/** Test runs while playback is stopped or paused, from the host. */
export function autoblowTestAvailable(ctx: DeviceContext, device: AutoblowDevice): boolean {
  const status = autoblowStatus(ctx, device);
  return !ctx.locked && ctx.canConfigureHost && Boolean(status?.connected) && !status?.testing && !ctx.engineActive && ctx.stopSequence !== undefined;
}

export function autoblowTest(ctx: DeviceContext, device: AutoblowDevice) {
  const sequence = ctx.stopSequence;
  if (sequence !== undefined) ctx.send(async () => ctx.setAutoblow(await api.autoblowTest(device, sequence)));
}

/** Both valve steps use the current Stop sequence and host permission. */
export function vacuglideStroke(ctx: DeviceContext, longer: boolean) {
  const sequence = ctx.stopSequence;
  if (sequence !== undefined && !ctx.locked && ctx.canConfigureHost) ctx.send(async () => ctx.setAutoblow(await api.vacuglideStroke(longer ? "longer" : "shorter", sequence)));
}

function tokenText(device: AutoblowDevice): SecretRowText {
  return {
    label: t("Device token"),
    hint: device === "vacuglide"
      ? t("From Autoblow's online setup for the VacuGlide. Stored on this computer and never shown again.")
      : t("From Autoblow's online setup for the Ultra. Stored on this computer and never shown again."),
    placeholder: t("Paste device token"),
    saved: t("Device token saved."),
    forget: t("Forget token"),
  };
}

function connectionText(status: AutoblowStatus | undefined): string {
  if (!status?.connected) return t("Not connected");
  if (status.simulated) return t("Simulated cloud");
  return status.cluster ? t("Online through {cluster}", { cluster: status.cluster }) : t("Online");
}

function AutoblowConnectionRows({ ctx, device }: { ctx: DeviceContext; device: AutoblowDevice }) {
  const save = useSaveDeviceOptions();
  const status = autoblowStatus(ctx, device);
  const tokenSet = Boolean(device === "vacuglide" ? ctx.settings?.vacuglide?.token_set : ctx.settings?.autoblow?.token_set);
  function forget() {
    confirmThen(t("Forget the saved device token? The device disconnects and cannot connect until you save a token again."), { confirmLabel: t("Forget token"), destructive: true },
      () => void save(device === "vacuglide" ? { vacuglide: { clear_token: true } } : { autoblow: { clear_token: true } }).catch(() => undefined));
  }
  return (
    <div className="form-rows device-rows">
      <div className="form-row form-row-plain"><p className="hint-block">{t("Experimental support; tested in simulation. Device timing and physical behavior still need verification.")}</p></div>
      <ConnectionKeyRow ctx={ctx} device={device} keySet={tokenSet} save={(token) => api.saveAutoblowToken(device, token)} forget={forget} text={tokenText(device)} />
      <OptionRow label={t("Online mode")} hint={device === "vacuglide"
        ? t("Hold the VacuGlide's mode button for about 2.5 seconds to turn online mode on or off, then choose Connect.")
        : t("Turn the Ultra on after setting up its Wi-Fi in Autoblow's app, then choose Connect.")}>
        <output>{connectionText(status)}</output>
      </OptionRow>
      {status?.connected && (
        <OptionRow label={t("Round trip")} hint={t("How long a command takes to reach Autoblow's cloud and come back. Commands leave half of it early.")}>
          <output>{t("{millis} ms", { millis: status.round_trip_ms })}</output>
        </OptionRow>
      )}
      {status?.connected && status.firmware_status && status.firmware_status !== "UP_TO_DATE" && (
        <OptionRow label={t("Device firmware")}>
          <output>{status.firmware_status === "UPDATE_REQUIRED" ? t("Update required in Autoblow's app") : t("Update available in Autoblow's app")}</output>
        </OptionRow>
      )}
      <OptionRow label={t("Device buttons")} hint={device === "vacuglide"
        ? t("While connected, the mode button stops all motion like Esc, and the speed buttons move the top of the Tempo range. Holding the mode button leaves online mode.")
        : t("While connected, the pause button stops all motion like Esc, and the speed buttons raise or lower the Speed limit.")}>
        <output>{status?.button_events ? t("Listening") : t("Not listening")}</output>
      </OptionRow>
      <div className="form-row form-row-plain">
        <p className="hint-block">{t("Commands, Stop included, reach this device only through Autoblow's servers. If your internet connection or Autoblow's cloud is down, MagicHandy cannot stop it: switch it off on the device.")}</p>
      </div>
    </div>
  );
}

export function AutoblowConnectionSection({ ctx }: DeviceSectionProps) {
  return <AutoblowConnectionRows ctx={ctx} device="autoblow" />;
}

export function VacuGlideConnectionSection({ ctx }: DeviceSectionProps) {
  return <AutoblowConnectionRows ctx={ctx} device="vacuglide" />;
}

/** The lowest motor speed the Tempo range may start at: the motor stalls
 * below about 4%. */
export const VACUGLIDE_TEMPO_FLOOR = 5;

export function VacuGlideTempoSection({ ctx, compact = false }: Pick<DeviceSectionProps, "ctx"> & { compact?: boolean }) {
  const save = useSaveDeviceOptions();
  const status = autoblowStatus(ctx, "vacuglide");
  const locked = ctx.locked || !ctx.canConfigureHost;
  const vacuglide = ctx.settings?.vacuglide;
  const [range, setRange] = useLiveSetting(
    { min: vacuglide?.tempo_min_percent ?? VACUGLIDE_TEMPO_FLOOR, max: vacuglide?.tempo_max_percent ?? 100 },
    (value) => save({ vacuglide: { tempo_min_percent: value.min, tempo_max_percent: value.max } }), SLIDER_SAVE_MS);
  if (compact) {
    return (
      <div className="form-rows device-rows">
        <RangeSlider label={t("Tempo range")} floor={VACUGLIDE_TEMPO_FLOOR} ceil={100} minValue={range.min} maxValue={range.max} disabled={locked} onChange={setRange} />
        <SwitchRow label={t("Chat and Autopilot may adjust the stroke")} hint={t("Up to three valve steps either way a session.")}
          checked={Boolean(vacuglide?.motion_air)} disabled={locked}
          onChange={(checked) => void save({ vacuglide: { motion_air: checked } }).catch(() => undefined)} />
      </div>
    );
  }
  return (
    <div className="form-rows device-rows">
      <OptionRow label={t("Tempo range")} hint={t("The motor speeds the slowest and the fastest tempo play at. Chat, Autopilot and routines choose a tempo inside this range and never raise it. The device's speed buttons move its top by 10.")}>
        <RangeSlider label={t("Tempo range")} floor={VACUGLIDE_TEMPO_FLOOR} ceil={100} minValue={range.min} maxValue={range.max} disabled={locked} onChange={setRange} />
      </OptionRow>
      {status?.connected && (
        <OptionRow label={t("Motor speed")} hint={t("The last speed MagicHandy sent, not a measurement.")}>
          <output>{t("{n}%", { n: Math.round(status.commanded_percent ?? 0) })}</output>
        </OptionRow>
      )}
      <div className="form-row form-row-plain">
        <p className="hint-block">{t("The VacuGlide makes its own strokes, so it plays a tempo: how fast it strokes. With only the VacuGlide playing, chat, Autopilot and its routines choose the tempo. When a Handy or an Ultra plays too, the VacuGlide follows the pace of their movement.")}</p>
      </div>
    </div>
  );
}

export function VacuGlideStrokeSection({ ctx }: DeviceSectionProps) {
  const save = useSaveDeviceOptions();
  const status = autoblowStatus(ctx, "vacuglide");
  const connected = Boolean(status?.connected);
  const hostLocked = ctx.locked || !ctx.canConfigureHost;
  return (
    <div className="form-rows device-rows">
      <OptionRow label={t("Stroke")} hint={t("Each step opens a valve for a moment. Longer draws air out of the sleeve for a longer, tighter stroke; shorter lets air in. The Allen-key setting on the device caps the longest stroke.")}>
        <div className="row-actions">
          <button type="button" className="btn btn-secondary" disabled={!connected || hostLocked || ctx.stopSequence === undefined} onClick={() => vacuglideStroke(ctx, false)}>{t("Shorter")}</button>
          <button type="button" className="btn btn-secondary" disabled={!connected || hostLocked || ctx.stopSequence === undefined} onClick={() => vacuglideStroke(ctx, true)}>{t("Longer")}</button>
          {connected && <output>{t("Steps this connection: {steps}", { steps: status?.stroke_steps ?? 0 })}</output>}
        </div>
      </OptionRow>
      <SwitchRow label={t("Chat and Autopilot may adjust the stroke")}
        hint={t("Lets them take up to three valve steps either way in one playback session, only while it plays. Nothing measures the air, so each step is relative to how you set the device.")}
        checked={Boolean(ctx.settings?.vacuglide?.motion_air)} disabled={hostLocked}
        onChange={(checked) => void save({ vacuglide: { motion_air: checked } }).catch(() => undefined)} />
      {connected && Boolean(status?.motion_air_steps) && (
        <OptionRow label={t("Steps by chat and Autopilot")} hint={t("In this playback session.")}>
          <output>{status?.motion_air_steps ?? 0}</output>
        </OptionRow>
      )}
    </div>
  );
}

function TestRow({ ctx, device, label, hint }: { ctx: DeviceContext; device: AutoblowDevice; label: string; hint: string }) {
  return (
    <div className="form-rows device-rows">
      <OptionRow label={label} hint={hint}>
        <button type="button" className="btn btn-start" disabled={!autoblowTestAvailable(ctx, device)} onClick={() => autoblowTest(ctx, device)}>{t("Test")}</button>
      </OptionRow>
    </div>
  );
}

export function AutoblowTestSection({ ctx }: DeviceSectionProps) {
  return <TestRow ctx={ctx} device="autoblow" label={t("Test movement")} hint={t("Strokes gently in the middle of the range for three seconds, while playback is stopped or paused.")} />;
}

export function VacuGlideTestSection({ ctx }: DeviceSectionProps) {
  return <TestRow ctx={ctx} device="vacuglide" label={t("Test motor")} hint={t("Runs the motor slowly for three seconds, while playback is stopped or paused.")} />;
}
