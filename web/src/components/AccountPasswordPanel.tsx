import { useState, type FormEvent } from "react";
import { api } from "../api/client";
import { t, translateKnown } from "../i18n";
import { useToast } from "../state/app-state";
import { PasswordConfirmationField, newPasswordError } from "./PasswordConfirmationField";

const errorMessage = (reason: unknown) => reason instanceof Error ? translateKnown(reason.message) : t("Request failed");

export function AccountPasswordPanel({ disabled, onChanged }: { disabled: boolean; onChanged: () => Promise<unknown> }) {
  const { show } = useToast();
  const [current, setCurrent] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const validation = newPasswordError(password, confirmation);
    if (validation) {
      setError(validation);
      return;
    }
    setBusy(true);
    setError("");
    try {
      await api.authChangePassword(current, password);
      show(t("Password changed. Sign in again with the new password."), "success");
      await onChanged();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
      setCurrent("");
      setPassword("");
      setConfirmation("");
    }
  };
  return (
    <section className="group">
      <h3 className="group-title">{t("Change your password")}</h3>
      <form className="account-form" onSubmit={(event) => void submit(event)}>
        <div className="settings-grid two">
          <label className="field"><span className="label">{t("Current password")}</span><input type="password" autoComplete="current-password" value={current} disabled={disabled || busy} onChange={(event) => setCurrent(event.target.value)} /></label>
          <label className="field"><span className="label">{t("New password")}</span><input type="password" autoComplete="new-password" value={password} disabled={disabled || busy} onChange={(event) => setPassword(event.target.value)} /></label>
          <PasswordConfirmationField password={password} confirmation={confirmation} disabled={disabled || busy} onChange={setConfirmation} />
        </div>
        <p className="hint-block">{t("Changing your password signs out every browser session for this account.")}</p>
        {error && <p className="form-status auth-error" role="alert">{error}</p>}
        <button className="btn btn-secondary" type="submit" disabled={disabled || busy || !current || !password}>{busy ? t("Changing…") : t("Change password")}</button>
      </form>
    </section>
  );
}
