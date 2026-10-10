import { forwardRef, type CSSProperties, type InputHTMLAttributes } from "react";

type RangeInputProps = Omit<InputHTMLAttributes<HTMLInputElement>, "type" | "value" | "min" | "max"> & {
  value: number;
  min?: number;
  max?: number;
};

// A native range in the shared slider recipe (setpoint-controls.css). It only
// publishes --range-fill, the value's position from min to max (0–1), because
// Chromium cannot paint a native range's filled track by itself. Keyboard,
// pointer and assistive behavior stay the browser's own.
export const RangeInput = forwardRef<HTMLInputElement, RangeInputProps>(function RangeInput(
  { value, min = 0, max = 100, style, ...props },
  ref,
) {
  const span = max - min;
  const ratio = span > 0 ? (value - min) / span : 0;
  // An invalid value must not invalidate the track's paint, so it reads empty.
  const fill = Number.isFinite(ratio) ? Math.min(1, Math.max(0, ratio)) : 0;
  return (
    <input
      {...props}
      ref={ref}
      type="range"
      min={min}
      max={max}
      value={value}
      style={{ ...style, "--range-fill": fill } as CSSProperties}
    />
  );
});
