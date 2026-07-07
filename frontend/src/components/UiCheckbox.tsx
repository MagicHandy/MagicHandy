import { useId, type InputHTMLAttributes, type ReactNode } from "react";

interface UiCheckboxProps
  extends Omit<InputHTMLAttributes<HTMLInputElement>, "type"> {
  label?: ReactNode;
  compact?: boolean;
}

export function UiCheckbox({
  label,
  compact = false,
  className = "",
  id,
  ...inputProps
}: UiCheckboxProps) {
  const autoId = useId();
  const inputId = id ?? autoId;

  return (
    <label
      className={`ui-checkbox${compact ? " ui-checkbox--compact" : ""}${className ? ` ${className}` : ""}`}
      htmlFor={inputId}
    >
      <input type="checkbox" id={inputId} {...inputProps} />
      <span className="ui-checkbox-box" aria-hidden="true">
        <svg className="ui-checkbox-icon" viewBox="0 0 12 10" fill="none">
          <path
            d="M1 5.2L4.4 8.6L11 1.4"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      </span>
      {label != null && label !== "" && (
        <span className="ui-checkbox-label">{label}</span>
      )}
    </label>
  );
}
