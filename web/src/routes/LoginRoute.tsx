import { useEffect, useRef, useState, type FormEvent } from "react";
import { t, translateKnown } from "../i18n";
import { useAuth } from "../state/auth";

const passwordBytes = (value: string) => new TextEncoder().encode(value).byteLength;

export function LoginRoute() {
  const auth = useAuth();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const usernameInput = useRef<HTMLInputElement>(null);
  const bootstrap = auth.status?.initialized === false;

  useEffect(() => usernameInput.current?.focus(), [bootstrap]);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (busy) return;
    const normalized = username.trim();
    if (bootstrap && passwordBytes(password) < 12) {
      setError(t("Use a password or passphrase of at least 12 bytes."));
      return;
    }
    if (bootstrap && password !== confirmation) {
      setError(t("The passwords do not match."));
      return;
    }
    setBusy(true);
    setError("");
    try {
      if (bootstrap) await auth.bootstrap(normalized, password);
      else await auth.login(normalized, password);
      setPassword("");
      setConfirmation("");
    } catch (reason) {
      setError(reason instanceof Error ? translateKnown(reason.message) : t("Sign in failed."));
    } finally {
      setBusy(false);
    }
  };

  const bootstrapUnavailable = bootstrap && auth.status?.bootstrap_available === false;
  return (
    <section className="auth-workspace" aria-labelledby="auth-title">
      <div className="auth-panel">
        <header className="auth-head">
          <span className="auth-mark" aria-hidden="true">M</span>
          <span>
            <p className="eyebrow">{t("Protected access")}</p>
            <h1 id="auth-title">{bootstrap ? t("Create the first administrator") : t("Sign in to MagicHandy")}</h1>
          </span>
        </header>
        <p className="auth-intro">
          {bootstrap
            ? t("Create the local administrator that will manage access to this installation.")
            : t("Use an account from this MagicHandy installation. Your session is kept in a protected browser cookie.")}
        </p>
        {bootstrapUnavailable ? (
          <div className="auth-notice" role="alert">
            <strong>{t("Local setup required")}</strong>
            <span>{t("Create the first administrator from the computer running MagicHandy, then sign in remotely.")}</span>
          </div>
        ) : (
          <form className="auth-form" onSubmit={(event) => void submit(event)}>
            <label className="field">
              <span className="label">{t("Username")}</span>
              <input
                ref={usernameInput}
                type="text"
                name="username"
                autoComplete="username"
                spellCheck={false}
                value={username}
                disabled={busy}
                onChange={(event) => setUsername(event.target.value)}
              />
            </label>
            <label className="field">
              <span className="label">{t("Password")}</span>
              <input
                type="password"
                name="password"
                autoComplete={bootstrap ? "new-password" : "current-password"}
                value={password}
                disabled={busy}
                onChange={(event) => setPassword(event.target.value)}
              />
              {bootstrap && <span className="hint">{t("At least 12 bytes. A long, unique passphrase is recommended.")}</span>}
            </label>
            {bootstrap && (
              <label className="field">
                <span className="label">{t("Confirm password")}</span>
                <input
                  type="password"
                  name="password-confirmation"
                  autoComplete="new-password"
                  value={confirmation}
                  disabled={busy}
                  onChange={(event) => setConfirmation(event.target.value)}
                />
              </label>
            )}
            {error && <p className="form-status auth-error" role="alert">{error}</p>}
            <button className="btn btn-primary auth-submit" type="submit" disabled={busy || !username.trim() || !password}>
              {busy ? t("Checking…") : bootstrap ? t("Create administrator") : t("Sign in")}
            </button>
          </form>
        )}
        <div className="auth-safety-note">
          <strong>{t("Emergency Stop remains available.")}</strong>
          <span>{t("Signing in does not transfer device control; the existing controller lease still decides which browser may command motion.")}</span>
        </div>
      </div>
    </section>
  );
}
