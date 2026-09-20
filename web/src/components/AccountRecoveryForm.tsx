import { useEffect, useRef, useState, type FormEvent } from "react";
import { api } from "../api/client";
import { t, translateKnown } from "../i18n";
import { passwordMeetsMinimum } from "../util/password";
import { PasswordConfirmationField } from "./PasswordConfirmationField";

export function AccountRecoveryForm({ initialUsername, onBack }: { initialUsername: string; onBack: (username: string) => void }) {
  const [username, setUsername] = useState(initialUsername);
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [busy, setBusy] = useState(false);
  const [recovered, setRecovered] = useState(false);
  const [error, setError] = useState("");
  const active = useRef<{ controller: AbortController; timer: number } | null>(null);
  useEffect(() => () => {
    const current = active.current;
    active.current = null;
    if (current) { window.clearTimeout(current.timer); current.controller.abort(); }
  }, []);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (busy) return;
    if (!passwordMeetsMinimum(password)) { setError(t("Use a password or passphrase of at least 8 characters.")); return; }
    if (password !== confirmation) { setError(t("The passwords do not match.")); return; }
    const controller = new AbortController();
    const request = { controller, timer: 0 };
    request.timer = window.setTimeout(() => {
      if (active.current !== request) return;
      active.current = null;
      controller.abort();
      setBusy(false);
      setError(t("The recovery result is unknown. Try signing in with the new password before retrying recovery."));
    }, 10_000);
    active.current = request;
    setBusy(true); setError("");
    try {
      const result = await api.recoverPassword(username.trim(), code, password, controller.signal);
      if (active.current !== request) return;
      if (!result.recovered) throw new Error(t("Invalid recovery response."));
      setCode(""); setPassword(""); setConfirmation(""); setRecovered(true);
    } catch (reason) {
      if (active.current !== request) return;
      setError(controller.signal.aborted ? t("The recovery result is unknown. Try signing in with the new password before retrying recovery.")
        : reason instanceof Error ? translateKnown(reason.message) : t("Request failed"));
    } finally {
      window.clearTimeout(request.timer);
      if (active.current === request) { active.current = null; setBusy(false); }
    }
  };

  return <>
    {recovered ? <p role="status">{t("Password reset. Sign in with your new password, then generate a new recovery-code set. All earlier sessions and codes have ended.")}</p> :
      <form className="auth-form" onSubmit={event => void submit(event)}>
        <p className="hint-block">{t("Use a code you saved earlier. Recovery resets your password and signs out every browser for this account.")}</p>
        <label className="field"><span className="label">{t("Username")}</span><input type="text" name="username" autoComplete="username" value={username} disabled={busy} onChange={event => setUsername(event.target.value)} /></label>
        <label className="field"><span className="label">{t("Recovery code")}</span><input type="password" autoComplete="off" spellCheck={false} maxLength={80} value={code} disabled={busy} onChange={event => setCode(event.target.value)} /></label>
        <label className="field"><span className="label">{t("New password")}</span><input type="password" autoComplete="new-password" value={password} disabled={busy} onChange={event => setPassword(event.target.value)} /></label>
        <PasswordConfirmationField password={password} confirmation={confirmation} disabled={busy} onChange={setConfirmation} />
        {error && <p className="form-status auth-error" role="alert">{error}</p>}
        <button className="btn btn-primary" type="submit" disabled={busy || !username.trim() || !code || !password}>{busy ? t("Checking…") : t("Reset password with code")}</button>
      </form>}
    <button className="btn btn-secondary" type="button" onClick={() => onBack(username.trim())}>{t("Back to sign in")}</button>
  </>;
}
