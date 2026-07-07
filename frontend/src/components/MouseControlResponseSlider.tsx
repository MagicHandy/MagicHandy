import { useTranslation } from "react-i18next";

type Props = {
  value: number;
  min: number;
  max: number;
  step: number;
  disabled?: boolean;
  turbo?: boolean;
  onChange: (value: number) => void;
};

export function MouseControlResponseSlider({
  value,
  min,
  max,
  step,
  disabled = false,
  turbo = false,
  onChange,
}: Props) {
  const { t } = useTranslation();
  const span = Math.max(1, max - min);
  const pct = Math.max(0, Math.min(100, ((value - min) / span) * 100));

  return (
    <div
      className={`mc-response-slider${disabled ? " mc-response-slider--disabled" : ""}${turbo ? " mc-response-slider--turbo" : ""}`}
    >
      <div className="mc-response-slider-head">
        <span className="mc-response-slider-label">{t("mouse.response.label")}</span>
        <strong className="mc-response-slider-value mono">
          {value}
          <span className="mc-response-slider-unit">ms</span>
        </strong>
        {turbo && <span className="mouse-control-fast-badge">{t("mouse.response.turbo")}</span>}
      </div>

      <div className="mc-response-slider-track-wrap">
        <div className="mc-response-slider-track" aria-hidden>
          <div className="mc-response-slider-fill" style={{ width: `${pct}%` }} />
          <div className="mc-response-slider-thumb" style={{ left: `${pct}%` }} />
        </div>
        <input
          type="range"
          className="mc-response-slider-input"
          min={min}
          max={max}
          step={step}
          value={value}
          disabled={disabled}
          aria-valuemin={min}
          aria-valuemax={max}
          aria-valuenow={value}
          aria-label={t("mouse.response.aria")}
          onChange={(e) => onChange(Number(e.target.value))}
        />
      </div>

      <div className="mc-response-slider-bounds mono">
        <span>{min} ms</span>
        <span className="mc-response-slider-bounds-mid">{t("mouse.response.bounds")}</span>
        <span>{max} ms</span>
      </div>
    </div>
  );
}
