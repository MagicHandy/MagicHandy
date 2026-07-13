// Dual-thumb range control: one slider with a low and a high point, replacing a
// pair of separate min/max sliders. Two overlaid native range inputs keep full
// keyboard and screen-reader support (each thumb is a real slider); the thumbs
// clamp so low never crosses high. Colours come from the design system.
import { useId, type ChangeEvent, type CSSProperties } from "react";

interface RangeSliderProps {
  label: string;
  minValue: number;
  maxValue: number;
  floor: number;
  ceil?: number;
  minGap?: number;
  disabled?: boolean;
  onChange: (next: { min: number; max: number }) => void;
}

export function RangeSlider({ label, minValue, maxValue, floor, ceil = 100, minGap = 0, disabled, onChange }: RangeSliderProps) {
  const id = useId();
  const span = ceil - floor || 1;
  const lowPct = ((minValue - floor) / span) * 100;
  const highPct = ((maxValue - floor) / span) * 100;

  const changeLow = (event: ChangeEvent<HTMLInputElement>) => {
    const next = Math.min(Number(event.target.value), maxValue - minGap);
    onChange({ min: next, max: maxValue });
  };
  const changeHigh = (event: ChangeEvent<HTMLInputElement>) => {
    const next = Math.max(Number(event.target.value), minValue + minGap);
    onChange({ min: minValue, max: next });
  };

  return (
    <div className="range-slider" data-disabled={disabled || undefined}>
      <div className="range-slider-head">
        <span className="label" id={id}>{label}</span>
        <output className="range-slider-value">{minValue}–{maxValue}%</output>
      </div>
      <div className="range-slider-track" style={{ "--low": `${lowPct}%`, "--high": `${highPct}%` } as CSSProperties}>
        <span className="range-slider-fill" aria-hidden="true" />
        <input
          type="range"
          className="range-slider-input range-slider-low"
          aria-label={`${label} minimum`}
          aria-describedby={id}
          min={floor}
          max={ceil}
          value={minValue}
          disabled={disabled}
          onChange={changeLow}
        />
        <input
          type="range"
          className="range-slider-input range-slider-high"
          aria-label={`${label} maximum`}
          aria-describedby={id}
          min={floor}
          max={ceil}
          value={maxValue}
          disabled={disabled}
          onChange={changeHigh}
        />
      </div>
    </div>
  );
}
