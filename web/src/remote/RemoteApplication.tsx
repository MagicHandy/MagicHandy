import { useEffect, useState } from "react";
import { ConfirmHost } from "../components/ConfirmHost";
import { t, translateKnown } from "../i18n";
import { useAuth } from "../state/auth";
import { LoginRoute } from "../routes/LoginRoute";
import { RemoteConnection, RemoteRoute } from "../routes/RemoteRoute";
import { StopButton } from "../shell/StopButton";
import { PersonaIcon, RefreshIcon } from "../shell/icons";
import { SessionSettingsPanel } from "../components/SessionSettingsPanel";
import { RecoveryCodesPanel } from "../components/RecoveryCodesPanel";
import { NoticePreferencesPanel } from "../components/NoticePreferencesPanel";
import { AccountPasswordPanel } from "../components/AccountPasswordPanel";
import { useRemoteState } from "./useRemoteState";
import "../styles/remote.css";

// One canonical frontend, with a different, backend-selected application shell.
// It never mounts the controller connection, device UI or full-app data poll.
export function RemoteApplication() {
  const auth = useAuth();
  const [accountOpen, setAccountOpen] = useState(false);
  const [logoutError, setLogoutError] = useState("");
  const [signingOut, setSigningOut] = useState(false);
  const signedIn = Boolean(auth.status && (!auth.status.authentication_required || auth.status.authenticated));
  const canControl = signedIn && auth.status?.capabilities?.remote_control !== false;
  const remote = useRemoteState(canControl);
  useEffect(() => { document.title = "MagicHandy Remote"; }, []);

  async function logout() {
    setSigningOut(true);
    setLogoutError("");
    try {
      await auth.logout();
      setAccountOpen(false);
    } catch (reason) {
      setLogoutError(reason instanceof Error ? translateKnown(reason.message) : t("Request failed"));
    } finally {
      setSigningOut(false);
    }
  }

  return <div className="remote-application">
    <header className="remote-header">
      <div className="nav-brand remote-brand">
        <span className="nav-brand-mark" aria-hidden="true">M</span>
        <h1 className="nav-brand-copy" aria-label={t("MagicHandy Remote")}>
          <span className="nav-brand-name">{t("MagicHandy")}</span>
          <span className="nav-brand-context">{t("Remote")}</span>
        </h1>
      </div>
      {signedIn && <div className="remote-header-status">
        <RemoteConnection remote={remote} />
        <button className="icon-button btn-quiet" type="button" aria-label={t("Refresh")} title={t("Refresh")} onClick={() => { void auth.refresh(); remote.refresh(); }}><RefreshIcon /></button>
      </div>}
      {auth.status?.authenticated && <div className="remote-account-actions">
        <button className="btn btn-secondary btn-sm remote-account-button" type="button" aria-label={t("Your account")} title={auth.status.account?.username} aria-expanded={accountOpen} onClick={() => setAccountOpen(value => !value)}><PersonaIcon /><span>{auth.status.account?.username}</span></button>
        <button className="btn btn-secondary btn-sm" type="button" disabled={signingOut} onClick={() => void logout()}>{t("Sign out")}</button>
      </div>}
    </header>
    <main className="remote-main">
      {logoutError && <p className="form-status" role="alert">{logoutError}</p>}
      {!auth.status ? <section className="remote-empty" role="status"><p>{auth.error || t("Checking access…")}</p><button className="btn btn-secondary" type="button" onClick={() => void auth.refresh()}>{t("Retry core connection")}</button></section>
        : !signedIn ? <LoginRoute />
        : accountOpen ? <section className="remote-account settings-panel" aria-label={t("Your account")}>
          <div className="remote-account-heading"><h2>{t("Your account")}</h2><button className="btn btn-secondary btn-sm" type="button" onClick={() => setAccountOpen(false)}>{t("Back to remote")}</button></div>
          <AccountPasswordPanel disabled={false} onChanged={auth.refresh} />
          <RecoveryCodesPanel backendOnline />
          <SessionSettingsPanel backendOnline onSignedOut={auth.refresh} />
          <NoticePreferencesPanel />
        </section>
        : <RemoteRoute remote={remote} canControl={canControl} onRefresh={() => { void auth.refresh(); remote.refresh(); }} />}
    </main>
    <ConfirmHost />
    <footer className="remote-safety">
      <span>{remote.state?.connected && !remote.stale ? t("Desktop connected") : t("No desktop connected")}</span>
      <StopButton className="remote-stop" onStopped={remote.refresh} />
    </footer>
  </div>;
}
