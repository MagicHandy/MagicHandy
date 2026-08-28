import { useId } from "react";
import { t } from "../i18n";
import { passwordConfirmationState } from "../util/password";

export function PasswordConfirmationField({
  password,
  confirmation,
  disabled = false,
  label,
  name,
  onChange,
}: {
  password: string;
  confirmation: string;
  disabled?: boolean;
  label?: string;
  name?: string;
  onChange: (value: string) => void;
}) {
  const feedbackID = useId();
  const state = passwordConfirmationState(password, confirmation);
  const feedback = state === "match"
    ? t("Passwords match.")
    : state === "mismatch"
      ? t("The passwords do not match.")
      : "";

  return (
    <label className="field">
      <span className="label">{label ?? t("Confirm password")}</span>
      <input
        type="password"
        name={name}
        autoComplete="new-password"
        value={confirmation}
        disabled={disabled}
        aria-describedby={feedbackID}
        aria-invalid={state === "mismatch"}
        data-match-state={state}
        onChange={(event) => onChange(event.target.value)}
      />
      <span
        id={feedbackID}
        className="password-match-feedback"
        data-state={state}
        aria-live="polite"
        aria-atomic="true"
      >
        {feedback}
      </span>
    </label>
  );
}
