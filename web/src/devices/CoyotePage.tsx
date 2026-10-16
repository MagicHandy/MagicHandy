// The Coyote's device page: Safety, Connection, Output and Test. The ceilings
// are written to the Coyote, which enforces them itself (ADR 0044).
import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { EstimSettings } from "../api/types";
import { RangeSlider } from "../components/RangeSlider";
import { SegmentedChoice } from "../components/SetpointControls";
import { SetupChoice } from "../components/SetupChoice";
import { t } from "../i18n";
import { coyoteConnect } from "./actions";
import { NumberInput, OptionRow, SLIDER_SAVE_MS, SwitchRow, ValueSlider, useLiveSetting, useSaveDeviceOptions } from "./fields";
import type { DeviceSectionProps } from "./types";

/** Mirrors config.DefaultEstimSettings for a backend that reports none. */
export const DEFAULT_ESTIM_SETTINGS: EstimSettings = {
  safety_acknowledged: false,
  max_strength_a: 30,
  max_strength_b: 30,
  follow: "stroke",
  frequency_min_hz: 40,
  frequency_max_hz: 100,
  balance: 0,
  swap_channels: false,
  timing_offset_ms: 0,
  frequency_balance_a: 160,
  frequency_balance_b: 160,
  intensity_balance_a: 0,
  intensity_balance_b: 0,
};

const MAX_STRENGTH = 200;

export function CoyoteSafetySection({ ctx }: DeviceSectionProps) {
  const acknowledged = Boolean(ctx.settings?.estim?.safety_acknowledged);
  const locked = ctx.locked || !ctx.canConfigureHost;
  const busy = Boolean(ctx.busy.coyote);
  const acknowledge = (value: boolean) => void ctx.run("coyote", "safety", async () => { await api.setEstimSafety(value); });
  if (!acknowledged) {
    return (
      <div className="device-safety-notes" aria-live="polite">
        <strong>{t("Before you connect a Coyote")}</strong>
        <ul>
          <li>{t("Place electrodes below the waist only. Never across the chest or near the heart.")}</li>
          <li>{t("Do not use it with a pacemaker or other implant, a heart condition, epilepsy, or during pregnancy.")}</li>
          <li>{t("Start at zero and raise strength slowly. MagicHandy never raises strength on its own.")}</li>
          <li>{t("Stop and Esc set both channels to zero.")}</li>
        </ul>
        <button type="button" className="btn btn-primary" disabled={locked || busy} onClick={() => acknowledge(true)}>{t("I have read these notes")}</button>
      </div>
    );
  }
  return (
    <div className="form-rows device-rows">
      <OptionRow label={t("Safety notes")} hint={t("Read on this computer. Electrodes below the waist only; never with a pacemaker, a heart condition, epilepsy or during pregnancy.")}>
        <button type="button" className="btn btn-secondary" disabled={locked || busy} onClick={() => acknowledge(false)}>{t("Show before next connection")}</button>
      </OptionRow>
    </div>
  );
}

type Candidate = Awaited<ReturnType<typeof api.coyoteDiscover>>["candidates"][number];

export function CoyoteConnectionSection({ ctx }: DeviceSectionProps) {
  const snapshot = ctx.coyote;
  const status = snapshot?.status;
  const connected = Boolean(status?.connected);
  const acknowledged = Boolean(ctx.settings?.estim?.safety_acknowledged);
  const remembered = Boolean(ctx.settings?.estim?.remembered_device);
  const locked = ctx.locked || !ctx.canConfigureHost;
  const busy = ctx.busy.coyote;
  const [candidates, setCandidates] = useState<Candidate[] | null>(null);
  const [chosen, setChosen] = useState("");

  useEffect(() => {
    if (connected) {
      setCandidates(null);
      setChosen("");
    }
  }, [connected]);

  function discover() {
    setCandidates(null);
    setChosen("");
    void ctx.run("coyote", "discover", async () => setCandidates((await api.coyoteDiscover()).candidates));
  }

  function forget() {
    void ctx.run("coyote", "forget", async () => {
      await api.saveDeviceOptions({ estim: { forget_device: true } });
      return t("The remembered Coyote is forgotten. The next connection needs pairing mode.");
    });
  }

  return (
    <div className="form-rows device-rows">
      {snapshot && !snapshot.available && !status?.simulated && <p className="form-status">{t("The Coyote Bluetooth helper is missing from this installation")}</p>}
      {remembered && (
        <OptionRow label={t("Remembered device")} hint={t("Reconnects without pairing mode. Forget it to use a different Coyote.")}>
          {status?.device_name && <span className="device-option-value">{status.device_name}</span>}
          <button type="button" className="btn btn-secondary" disabled={locked || Boolean(busy)} onClick={forget}>{t("Forget")}</button>
        </OptionRow>
      )}
      {!status?.simulated && (
        <OptionRow label={t("Find nearby devices")} hint={acknowledged ? t("Turn the Coyote on and close its phone app first. Strongest signal first.") : t("Available after the safety notes.")}>
          <button type="button" className="btn btn-secondary" disabled={locked || !acknowledged || connected || Boolean(busy)} onClick={discover}>
            {busy === "discover" ? t("Finding devices…") : t("Find nearby devices")}
          </button>
        </OptionRow>
      )}
      {candidates !== null && !connected && (
        <div className="form-row form-row-plain">
          {candidates.length ? (
            <>
              <div className="setup-choices" role="radiogroup" aria-label={t("Nearby devices")}>
                {candidates.map((candidate, index) => (
                  <SetupChoice key={candidate.id} selected={chosen === candidate.id}
                    title={candidate.identification === "remembered" ? t("Remembered COYOTE") : candidate.identification === "name_match" ? t("Likely COYOTE 3.0") : t("Possible COYOTE {number}", { number: index + 1 })}
                    detail={t("{signal} dBm", { signal: candidate.rssi })}
                    disabled={locked || Boolean(busy)} onSelect={() => setChosen(candidate.id)} />
                ))}
              </div>
              <div className="row-actions">
                <button type="button" className="btn btn-primary" disabled={locked || !chosen || Boolean(busy)} onClick={() => void coyoteConnect(ctx, chosen)}>{t("Connect")}</button>
                <span className="hint">{t("Strongest signal first. An unnamed device is not yet verified as a Coyote; Connect checks it before sending commands.")}</span>
              </div>
            </>
          ) : <p className="hint-block">{t("No nearby candidates found. Wake the Coyote and try again.")}</p>}
        </div>
      )}
      <div className="form-row form-row-plain">
        {!connected && <p className="hint-block">{t("Turn the Coyote on and disconnect the phone app. If the logo eyes are already flashing, leave the controls alone and connect.")}</p>}
        <p className="hint-block">{t("The Coyote uses the Bluetooth of the computer running MagicHandy, not this browser.")}</p>
      </div>
    </div>
  );
}

function followHint(follow: string): string {
  if (follow === "speed") return t("Faster movement widens the pulses on both channels.");
  if (follow === "position") return t("The stroke position moves a steady sensation between channel A (base) and channel B (tip).");
  return t("The stroke position moves the sensation between channel A (base) and channel B (tip), and faster movement widens the pulses.");
}

export function CoyoteOutputSection({ ctx }: DeviceSectionProps) {
  const estim = { ...DEFAULT_ESTIM_SETTINGS, ...ctx.settings?.estim };
  const save = useSaveDeviceOptions();
  const locked = ctx.locked || !ctx.canConfigureHost;
  const [follow, setFollow] = useLiveSetting(estim.follow, (follow) => save({ estim: { follow } }));
  const [ceilingA, setCeilingA] = useLiveSetting(estim.max_strength_a, (value) => save({ estim: { max_strength_a: value } }), SLIDER_SAVE_MS);
  const [ceilingB, setCeilingB] = useLiveSetting(estim.max_strength_b, (value) => save({ estim: { max_strength_b: value } }), SLIDER_SAVE_MS);
  const [rate, setRate] = useLiveSetting({ min: estim.frequency_min_hz, max: estim.frequency_max_hz },
    (value) => save({ estim: { frequency_min_hz: value.min, frequency_max_hz: value.max } }), SLIDER_SAVE_MS);
  const [balance, setBalance] = useLiveSetting(estim.balance, (value) => save({ estim: { balance: value } }), SLIDER_SAVE_MS);
  const [swap, setSwap] = useLiveSetting(estim.swap_channels, (value) => save({ estim: { swap_channels: value } }));
  const balanceText = balance === 0 ? t("Even") : balance < 0 ? t("More channel A") : t("More channel B");

  return (
    <div className="form-rows device-rows">
      <OptionRow label={t("Follows")} hint={followHint(follow)}>
        <SegmentedChoice className="device-segmented" label={t("Follows")} value={follow} disabled={locked}
          options={[
            { value: "stroke", label: t("Stroke + speed") },
            { value: "speed", label: t("Speed") },
            { value: "position", label: t("Position") },
          ]}
          onChange={setFollow} />
      </OptionRow>
      <OptionRow label={t("Ceilings")} hint={t("The Coyote enforces these itself, even against its own switches, and keeps them after you disconnect. MagicHandy never raises strength on its own.")}>
        <ValueSlider label={t("Channel A")} value={ceilingA} min={0} max={MAX_STRENGTH} disabled={locked}
          valueText={t("{strength} of {ceiling}", { strength: ceilingA, ceiling: MAX_STRENGTH })} onChange={setCeilingA} />
        <ValueSlider label={t("Channel B")} value={ceilingB} min={0} max={MAX_STRENGTH} disabled={locked}
          valueText={t("{strength} of {ceiling}", { strength: ceilingB, ceiling: MAX_STRENGTH })} onChange={setCeilingB} />
      </OptionRow>
      <OptionRow label={t("Pulse rate")} hint={t("Follows the pace within this range, in short held steps.")}>
        <RangeSlider label={t("Pulse rate")} floor={1} ceil={100} unit="Hz" minValue={rate.min} maxValue={rate.max} disabled={locked} onChange={setRate} />
      </OptionRow>
      <OptionRow label={t("Channel balance")} hint={t("Shifts strength toward one channel.")}>
        <ValueSlider label={t("Channel balance")} hideLabel value={balance} min={-100} max={100} step={5} disabled={locked} valueText={balanceText}
          ends={[t("More A"), t("More B")]} onChange={setBalance} />
      </OptionRow>
      <SwitchRow label={t("Swap channels")} hint={t("Use this when channel B sits at the base.")} checked={swap} disabled={locked} onChange={setSwap} />
      <details className="form-row-details">
        <summary>{t("Balance parameters, written to the Coyote")}</summary>
        <p className="hint-block">{t("Written to the Coyote with the ceilings, so they also apply in other apps. Higher frequency balance strengthens low pulse rates.")}</p>
        <div className="device-balance-fields">
          {([
            ["frequency_balance_a", t("Frequency balance A")],
            ["frequency_balance_b", t("Frequency balance B")],
            ["intensity_balance_a", t("Intensity balance A")],
            ["intensity_balance_b", t("Intensity balance B")],
          ] as const).map(([field, label]) => (
            <label key={field} className="field">
              <span className="label">{label}</span>
              <NumberInput value={estim[field]} min={0} max={255} disabled={locked}
                onCommit={(value) => void save({ estim: { [field]: value } }).catch(() => undefined)} />
            </label>
          ))}
        </div>
      </details>
    </div>
  );
}

export function CoyoteTestSection({ ctx, model }: DeviceSectionProps) {
  const status = ctx.coyote?.status;
  const levels = model.levels?.levels ?? [];
  const hint = status?.connected && status.strength_known
    ? t("A short pulse at the strength set in the device panel, now A {a} and B {b}. Only while motion is paused.", { a: status.strength_a, b: status.strength_b })
    : t("A short pulse at the strength set in the device panel. Connect the Coyote and raise a channel first.");
  return (
    <div className="form-rows device-rows">
      <OptionRow label={t("Test pulse")} hint={hint}>
        <div className="row-actions">
          {(["a", "b"] as const).map((channel) => {
            const level = levels.find((item) => item.id === channel);
            return (
              <button key={channel} type="button" className="btn btn-start" disabled={!level?.canTest} onClick={level?.test}>
                {channel === "a" ? t("Test A") : t("Test B")}
              </button>
            );
          })}
        </div>
      </OptionRow>
    </div>
  );
}
