import { useId, type CSSProperties } from "react";

export interface SetpointOption<Value extends string> {
  value: Value;
  label: string;
}

interface SetpointSliderProps<Value extends string> {
  label: string;
  hint?: string;
  value: Value;
  options: ReadonlyArray<SetpointOption<Value>>;
  disabled?: boolean;
  onChange: (value: Value) => void;
  className?: string;
}

// SetpointSlider is for an ordered, low-cardinality scale. It keeps native
// range keyboard/assistive semantics while exposing every named stop instead
// of hiding the available values in a drop-down list.
export function SetpointSlider<Value extends string>({
  label,
  hint,
  value,
  options,
  disabled = false,
  onChange,
  className = "",
}: SetpointSliderProps<Value>) {
  const id = useId();
  const index = Math.max(0, options.findIndex((option) => option.value === value));
  const current = options[index] ?? options[0];
  if (!current) return null;
  const finalIndex = Math.max(1, options.length - 1);
  const stopPosition = (optionIndex: number) => `${(optionIndex / finalIndex) * 100}%`;

  return (
    <label className={`setpoint-control ${className}`.trim()} htmlFor={id}>
      <span className="setpoint-head">
        <span>{label}{hint && <small>{hint}</small>}</span>
        <output htmlFor={id}>{current.label}</output>
      </span>
      <input
        id={id}
        type="range"
        min={0}
        max={Math.max(0, options.length - 1)}
        step={1}
        value={index}
        disabled={disabled || options.length < 2}
        aria-label={label}
        aria-valuetext={current.label}
        style={{ "--setpoint-progress": stopPosition(index) } as CSSProperties}
        onChange={(event) => {
          const option = options[Number(event.target.value)];
          if (option) onChange(option.value);
        }}
      />
      <span
        className="setpoint-scale"
        aria-hidden="true"
      >
        {options.map((option, optionIndex) => (
          <span
            className="setpoint-stop"
            data-selected={optionIndex === index || undefined}
            key={option.value}
            style={{ "--setpoint-position": stopPosition(optionIndex) } as CSSProperties}
          >
            {option.label}
          </span>
        ))}
      </span>
    </label>
  );
}

interface SegmentedChoiceProps<Value extends string> {
  label: string;
  value: Value;
  options: ReadonlyArray<SetpointOption<Value>>;
  disabled?: boolean;
  onChange: (value: Value) => void;
  className?: string;
  emptySlots?: number;
}

// Categorical choices are intentionally not rendered as a slider: spatial
// order must not imply that Creative, Library, and Off are quantities.
export function SegmentedChoice<Value extends string>({
  label,
  value,
  options,
  disabled = false,
  onChange,
  className = "",
  emptySlots = 0,
}: SegmentedChoiceProps<Value>) {
  const name = useId();

  return (
    <fieldset className={`segmented-field ${className}`.trim()} disabled={disabled}>
      <legend>{label}</legend>
      <div className="segmented-choice" role="radiogroup" aria-label={label}>
        {options.map((option) => (
          <label className="segmented-option" key={option.value}>
            <input
              type="radio"
              name={name}
              value={option.value}
              checked={option.value === value}
              onChange={() => onChange(option.value)}
            />
            <span>{option.label}</span>
          </label>
        ))}
        {Array.from({length:emptySlots},(_,index)=><span className="segmented-option segmented-placeholder" aria-hidden="true" key={`reserved-${index}`}><span /></span>)}
      </div>
    </fieldset>
  );
}
