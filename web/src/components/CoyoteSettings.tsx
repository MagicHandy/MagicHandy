import { t } from "../i18n";
import type { EstimSettings } from "../api/types";

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

interface Props {
  estim: EstimSettings;
  rememberedDevice: boolean;
  forgetDevice: boolean;
  locked: boolean;
  patch: (next: Partial<EstimSettings>) => void;
  setForgetDevice: (forget: boolean) => void;
}

const bounded = (value: string, low: number, high: number) => Math.max(low, Math.min(high, Math.round(Number(value) || 0)));

/** The Coyote's saved settings in Settings > Device. Save applies them. */
export function CoyoteSettings({ estim, rememberedDevice, forgetDevice, locked, patch, setForgetDevice }: Props) {
  return (
    <div className="group coyote-settings">
      <h3 className="group-title">{t("Coyote")}</h3>
      <div className="coyote-ceiling-fields">
        <label className="field"><span className="label">{t("Channel A ceiling")}</span>
          <input type="number" min={0} max={200} value={estim.max_strength_a} disabled={locked} onChange={(e) => patch({ max_strength_a: bounded(e.target.value, 0, 200) })} /></label>
        <label className="field"><span className="label">{t("Channel B ceiling")}</span>
          <input type="number" min={0} max={200} value={estim.max_strength_b} disabled={locked} onChange={(e) => patch({ max_strength_b: bounded(e.target.value, 0, 200) })} /></label>
      </div>
      <p className="hint-block">{t("Strength runs from 0 to 200. The Coyote enforces these ceilings itself, including against its own switches, and keeps them after you disconnect. MagicHandy never raises strength on its own.")}</p>
      <label className="field"><span className="label">{t("Follows")}</span>
        <select value={estim.follow} disabled={locked} onChange={(e) => patch({ follow: e.target.value })}>
          <option value="stroke">{t("Stroke and speed")}</option>
          <option value="speed">{t("Speed only")}</option>
          <option value="position">{t("Position only")}</option>
        </select>
      </label>
      <p className="hint-block">{estim.follow === "speed"
        ? t("Faster movement widens the pulses on both channels.")
        : estim.follow === "position"
          ? t("The stroke position moves a steady sensation between channel A (base) and channel B (tip).")
          : t("The stroke position moves the sensation between channel A (base) and channel B (tip), and faster movement widens the pulses.")}</p>
      <div className="coyote-ceiling-fields">
        <label className="field"><span className="label">{t("Lowest pulse rate (Hz)")}</span>
          <input type="number" min={1} max={100} value={estim.frequency_min_hz} disabled={locked} onChange={(e) => patch({ frequency_min_hz: bounded(e.target.value, 1, 100) })} /></label>
        <label className="field"><span className="label">{t("Highest pulse rate (Hz)")}</span>
          <input type="number" min={1} max={100} value={estim.frequency_max_hz} disabled={locked} onChange={(e) => patch({ frequency_max_hz: bounded(e.target.value, 1, 100) })} /></label>
      </div>
      <p className="hint-block">{t("The pulse rate follows the pace within this range. It changes in steps and holds each one briefly, which the Coyote reproduces faithfully.")}</p>
      <label className="field"><span className="label">{t("Channel balance")}<small>{estim.balance === 0 ? t("Even") : estim.balance < 0 ? t("More channel A") : t("More channel B")}</small></span>
        <input type="range" min={-100} max={100} step={5} value={estim.balance} disabled={locked} onChange={(e) => patch({ balance: bounded(e.target.value, -100, 100) })} /></label>
      <label className="toggle-line">
        <span className="toggle"><input type="checkbox" checked={estim.swap_channels} disabled={locked} onChange={(e) => patch({ swap_channels: e.target.checked })} /><span className="track" aria-hidden="true" /></span>
        <span>{t("Swap channels")}<small>{t("Use this when channel B sits at the base.")}</small></span>
      </label>
      <label className="field"><span className="label">{t("Timing offset (ms)")}</span>
        <input type="number" min={-300} max={300} step={10} value={estim.timing_offset_ms} disabled={locked} onChange={(e) => patch({ timing_offset_ms: bounded(e.target.value, -300, 300) })} /></label>
      <p className="hint-block">{t("Positive values delay the Coyote relative to motion and video.")}</p>
      <label className="toggle-line">
        <span className="toggle"><input type="checkbox" checked={estim.safety_acknowledged} disabled={locked} onChange={(e) => patch({ safety_acknowledged: e.target.checked })} /><span className="track" aria-hidden="true" /></span>
        <span>{t("I have read the Coyote safety notes")}<small>{t("Turning this off shows the notes again before the next connection.")}</small></span>
      </label>
      {rememberedDevice && (
        <label className="toggle-line">
          <span className="toggle"><input type="checkbox" checked={forgetDevice} disabled={locked} onChange={(e) => setForgetDevice(e.target.checked)} /><span className="track" aria-hidden="true" /></span>
          <span>{t("Forget the remembered Coyote on save")}<small>{t("The next connection then needs pairing mode.")}</small></span>
        </label>
      )}
      <details className="advanced-fields">
        <summary>{t("Coyote balance parameters")}</summary>
        <p className="hint-block">{t("Written to the Coyote with the ceilings, so they also apply in other apps. Higher frequency balance strengthens low pulse rates.")}</p>
        <div className="coyote-ceiling-fields">
          <label className="field"><span className="label">{t("Frequency balance A")}</span>
            <input type="number" min={0} max={255} value={estim.frequency_balance_a} disabled={locked} onChange={(e) => patch({ frequency_balance_a: bounded(e.target.value, 0, 255) })} /></label>
          <label className="field"><span className="label">{t("Frequency balance B")}</span>
            <input type="number" min={0} max={255} value={estim.frequency_balance_b} disabled={locked} onChange={(e) => patch({ frequency_balance_b: bounded(e.target.value, 0, 255) })} /></label>
          <label className="field"><span className="label">{t("Intensity balance A")}</span>
            <input type="number" min={0} max={255} value={estim.intensity_balance_a} disabled={locked} onChange={(e) => patch({ intensity_balance_a: bounded(e.target.value, 0, 255) })} /></label>
          <label className="field"><span className="label">{t("Intensity balance B")}</span>
            <input type="number" min={0} max={255} value={estim.intensity_balance_b} disabled={locked} onChange={(e) => patch({ intensity_balance_b: bounded(e.target.value, 0, 255) })} /></label>
        </div>
      </details>
    </div>
  );
}
