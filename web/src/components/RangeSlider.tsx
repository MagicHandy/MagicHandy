// The shared track handles pointer input; native range inputs remain available
// to keyboard and assistive-technology users for each bound.
import { useId, useRef, type ChangeEvent, type CSSProperties, type PointerEvent as ReactPointerEvent } from "react";

type RangeBound = "min" | "max";

interface RangeSliderProps {
  label: string;
  minValue: number;
  maxValue: number;
  floor: number;
  ceil?: number;
  minGap?: number;
  disabled?: boolean;
  formatValue?: (min: number, max: number) => string;
  onChange: (next: { min: number; max: number }, changed: RangeBound) => void;
}

export function RangeSlider({ label, minValue, maxValue, floor, ceil = 100, minGap = 0, disabled, formatValue, onChange }: RangeSliderProps) {
  const id = useId();
  const valueId = useId();
  const lowRef = useRef<HTMLInputElement>(null);
  const highRef = useRef<HTMLInputElement>(null);
  const activeBound = useRef<RangeBound | null>(null);
  const span = ceil - floor || 1;
  const lowPct = ((minValue - floor) / span) * 100;
  const highPct = ((maxValue - floor) / span) * 100;

  function changeBound(bound: RangeBound, candidate: number) {
    if (!Number.isFinite(candidate)) return;
    const value = Math.round(Math.max(floor, Math.min(ceil, candidate)));
    if (bound === "min") {
      const next = Math.min(value, maxValue - minGap);
      if (next !== minValue) onChange({ min: next, max: maxValue }, "min");
      return;
    }
    const next = Math.max(value, minValue + minGap);
    if (next !== maxValue) onChange({ min: minValue, max: next }, "max");
  }

  function valueAtPointer(event: ReactPointerEvent<HTMLDivElement>) {
    const bounds = event.currentTarget.getBoundingClientRect();
    if (bounds.width <= 0) return floor;
    const ratio = Math.max(0, Math.min(1, (event.clientX - bounds.left) / bounds.width));
    return floor + ratio * span;
  }

  function boundAtValue(value: number): RangeBound {
    if (value < minValue) return "min";
    if (value > maxValue) return "max";
    return Math.abs(value - minValue) < Math.abs(value - maxValue) ? "min" : "max";
  }

  function startPointerChange(event: ReactPointerEvent<HTMLDivElement>) {
    if (disabled || event.button > 0) return;
    const value = valueAtPointer(event);
    const bound = boundAtValue(value);
    activeBound.current = bound;
    event.currentTarget.setPointerCapture?.(event.pointerId);
    (bound === "min" ? lowRef : highRef).current?.focus();
    changeBound(bound, value);
    event.preventDefault();
  }

  function continuePointerChange(event: ReactPointerEvent<HTMLDivElement>) {
    if (activeBound.current) changeBound(activeBound.current, valueAtPointer(event));
  }

  function finishPointerChange(event: ReactPointerEvent<HTMLDivElement>) {
    activeBound.current = null;
    if (event.currentTarget.hasPointerCapture?.(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
  }

  return (
    <div className="range-slider" role="group" aria-labelledby={id} data-disabled={disabled || undefined}>
      <div className="range-slider-head">
        <span className="label" id={id}>{label}</span>
        <output id={valueId} className="range-slider-value">{formatValue ? formatValue(minValue, maxValue) : `${minValue}–${maxValue}%`}</output>
      </div>
      <div
        className="range-slider-track"
        style={{ "--low": `${lowPct}%`, "--high": `${highPct}%` } as CSSProperties}
        onPointerDown={startPointerChange}
        onPointerMove={continuePointerChange}
        onPointerUp={finishPointerChange}
        onPointerCancel={finishPointerChange}
      >
        <span className="range-slider-fill" aria-hidden="true" />
        <input
          type="range"
          ref={lowRef}
          className="range-slider-input range-slider-low"
          aria-label={`${label} minimum`}
          aria-describedby={valueId}
          aria-valuemax={maxValue - minGap}
          min={floor}
          max={ceil}
          step={1}
          value={minValue}
          disabled={disabled}
          onChange={(event: ChangeEvent<HTMLInputElement>) => changeBound("min", Number(event.target.value))}
        />
        <input
          type="range"
          ref={highRef}
          className="range-slider-input range-slider-high"
          aria-label={`${label} maximum`}
          aria-describedby={valueId}
          aria-valuemin={minValue + minGap}
          min={floor}
          max={ceil}
          step={1}
          value={maxValue}
          disabled={disabled}
          onChange={(event: ChangeEvent<HTMLInputElement>) => changeBound("max", Number(event.target.value))}
        />
      </div>
    </div>
  );
}
