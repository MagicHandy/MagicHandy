import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";
import { api } from "../api/client";
import type { AccountRole, UserAccount } from "../api/types";
import { t, translateKnown } from "../i18n";
import { useAuth } from "../state/auth";
import { useHashRoute, useToast } from "../state/app-state";
import { PencilIcon, TrashIcon } from "../shell/icons";
import { PROFILE_IMAGE_MAX_EDGE, resizeImageToJPEG } from "../util/profile-image";
import { passwordMeetsMinimum } from "../util/password";
import { AccountAvatar } from "./AccountAvatar";
import { PasswordConfirmationField } from "./PasswordConfirmationField";
import { NetworkSettingsPanel } from "./NetworkSettingsPanel";
import { ControlGrantPanel } from "./ControlGrantPanel";
import { SessionSettingsPanel } from "./SessionSettingsPanel";
import { AuditSettingsPanel } from "./AuditSettingsPanel";
import { RecoveryCodesPanel } from "./RecoveryCodesPanel";
import { AccessSettingsNavigation, resolveAccessSection } from "./AccessSettingsNavigation";

const errorMessage = (reason: unknown) => reason instanceof Error ? translateKnown(reason.message) : t("Request failed");

export function AccountSettingsPanel({ backendOnline }: { backendOnline: boolean }) {
  const auth = useAuth();
  const account = auth.status?.account ?? null;
  const route = useHashRoute();
  const administrator = account?.role === "admin";
  const section = resolveAccessSection(route.split("/")[3] || "profile", administrator);

  if (!auth.status?.initialized) {
    return (
      <>
        <h2 className="section-title">{t("Access")}</h2>
        <section className="group">
          <h3 className="group-title">{t("Password protection")}</h3>
          <p className="hint-block">{t("This installation currently opens without an account. Creating the first administrator turns on sign-in immediately and on future launches.")}</p>
          <BootstrapAccountForm disabled={!backendOnline} onCreated={auth.bootstrap} />
        </section>
        <NetworkSettingsPanel backendOnline={backendOnline} administrator />
      </>
    );
  }

  if (!account) return null;
  return (
    <>
      <h2 className="section-title">{t("Access")}</h2>
      <div className="access-layout">
        <AccessSettingsNavigation section={section} administrator={administrator} />
        <div className="access-content" key={`${auth.status.session_id || account.id}:${account.role}:${section}`}>
          {section === "profile" && <><ProfileGroup account={account} disabled={!backendOnline} onChanged={auth.refresh} /><LinkedProfilesGroup /></>}
          {section === "security" && <><PasswordGroup disabled={!backendOnline} onChanged={auth.refresh} /><RecoveryCodesPanel backendOnline={backendOnline} /></>}
          {section === "sessions" && <SessionSettingsPanel backendOnline={backendOnline} onSignedOut={auth.refresh} />}
          {section === "network" && administrator && <NetworkSettingsPanel backendOnline={backendOnline} administrator />}
          {(section === "accounts" || section === "history") && administrator && <AccountDirectory current={account} backendOnline={backendOnline} history={section === "history"} />}
        </div>
      </div>
    </>
  );
}

// Directory reads exist only on their two administrator pages. Leaving the
// page or switching login cancels them and discards that account's view.
function AccountDirectory({ current, backendOnline, history }: { current: UserAccount; backendOnline: boolean; history: boolean }) {
  const [accounts, setAccounts] = useState<UserAccount[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [creating, setCreating] = useState(false);
  const mounted = useRef(false);
  const active = useRef<{ controller: AbortController; timer: number } | null>(null);
  const cancel = useCallback(() => {
    const request = active.current; active.current = null;
    if (request) { window.clearTimeout(request.timer); request.controller.abort(); }
  }, []);
  const loadAccounts = useCallback(async () => {
    if (!mounted.current || !backendOnline) return;
    cancel(); setLoading(true); setError("");
    const request = { controller: new AbortController(), timer: 0 };
    active.current = request;
    request.timer = window.setTimeout(() => {
      if (active.current !== request) return;
      cancel(); setLoading(false); setError(t("Accounts are unavailable. Refresh and try again."));
    }, 10_000);
    try {
      const response = await api.accounts(request.controller.signal);
      if (active.current === request) setAccounts(response.accounts);
    } catch (reason) {
      if (active.current === request) setError(errorMessage(reason));
    } finally {
      window.clearTimeout(request.timer);
      if (active.current === request) { active.current = null; setLoading(false); }
    }
  }, [backendOnline, cancel]);
  useEffect(() => {
    mounted.current = true;
    if (backendOnline) void loadAccounts();
    else { setAccounts([]); setLoading(false); }
    return () => { mounted.current = false; cancel(); };
  }, [backendOnline, loadAccounts, cancel]);
  if (history) return <AuditSettingsPanel backendOnline={backendOnline} accounts={accounts} initiallyExpanded />;
  return <section className="group account-management">
    <div className="access-section-header">
      <h3 className="group-title">{t("Installation accounts")}</h3>
      <button className="btn btn-secondary" aria-label={t("Refresh accounts")} disabled={!backendOnline || loading} onClick={() => void loadAccounts()}>{t("Refresh")}</button>
    </div>
    <p className="hint-block">{t("Operators begin as observers. An administrator grants time-limited permission to control motion, chat and synchronized playback. Accounts share this installation's content; host configuration remains administrator-only.")}</p>
    {!backendOnline && <p role="status">{t("Core offline")}</p>}
    {error && <p className="form-status auth-error" role="alert">{error}</p>}
    {loading ? <p className="form-status" role="status">{t("Loading accounts…")}</p> : <AccountList current={current} accounts={accounts} disabled={!backendOnline} onChanged={loadAccounts} />}
    <button className="btn btn-secondary" type="button" aria-expanded={creating} disabled={!backendOnline || loading} onClick={() => setCreating(value => !value)}>{creating ? t("Cancel") : t("Add an account")}</button>
    {creating && <CreateAccountForm disabled={!backendOnline || loading} onCreated={async () => { setCreating(false); await loadAccounts(); }} />}
  </section>;
}

function BootstrapAccountForm({ disabled, onCreated }: {
  disabled: boolean;
  onCreated: (username: string, password: string) => Promise<void>;
}) {
  const { show } = useToast();
  const [username, setUsername] = useState("");
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
      await onCreated(username.trim(), password);
      setPassword("");
      setConfirmation("");
      show(t("Password protection enabled."), "success");
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  };
  return (
    <form className="account-form" onSubmit={(event) => void submit(event)}>
      <div className="settings-grid two">
        <label className="field"><span className="label">{t("Administrator username")}</span><input type="text" autoComplete="username" spellCheck={false} value={username} disabled={disabled || busy} onChange={(event) => setUsername(event.target.value)} /></label>
        <label className="field"><span className="label">{t("New password")}</span><input type="password" autoComplete="new-password" value={password} disabled={disabled || busy} onChange={(event) => setPassword(event.target.value)} /></label>
        <PasswordConfirmationField password={password} confirmation={confirmation} disabled={disabled || busy} onChange={setConfirmation} />
      </div>
      <p className="hint-block">{t("Use at least 8 characters and a unique passphrase. MagicHandy never stores or returns the plaintext password.")}</p>
      {error && <p className="form-status auth-error" role="alert">{error}</p>}
      <button className="btn btn-primary" type="submit" disabled={disabled || busy || !username.trim() || !password}>{busy ? t("Creating…") : t("Enable password protection")}</button>
    </form>
  );
}

function ProfileGroup({ account, disabled, onChanged }: {
  account: UserAccount;
  disabled: boolean;
  onChanged: () => Promise<unknown>;
}) {
  const { show } = useToast();
  const fileInput = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const choose = async (file?: File) => {
    if (!file) return;
    setBusy(true);
    setError("");
    try {
      await api.saveAccountProfileImage(await resizeImageToJPEG(file));
      await onChanged();
      show(t("Profile image saved."), "success");
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
      if (fileInput.current) fileInput.current.value = "";
    }
  };
  const remove = async () => {
    setBusy(true);
    setError("");
    try {
      await api.deleteAccountProfileImage();
      await onChanged();
      show(t("Profile image removed."));
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  };
  return (
    <section className="group account-profile-group">
      <h3 className="group-title">{t("Your profile")}</h3>
      <div className="account-profile-editor">
        <span className="account-profile-preview">
          <AccountAvatar account={account} />
          <span className="account-profile-actions">
            <input ref={fileInput} className="visually-hidden" type="file" accept="image/*" onChange={(event) => void choose(event.target.files?.[0])} />
            <button type="button" className="icon-button" disabled={disabled || busy} onClick={() => fileInput.current?.click()} aria-label={account.has_profile_image ? t("Replace profile image") : t("Choose profile image")} title={account.has_profile_image ? t("Replace profile image") : t("Choose profile image")}><PencilIcon size={15} /></button>
            {account.has_profile_image && <button type="button" className="icon-button" disabled={disabled || busy} onClick={() => void remove()} aria-label={t("Remove profile image")} title={t("Remove profile image")}><TrashIcon size={15} /></button>}
          </span>
        </span>
        <span className="account-profile-copy">
          <strong>{account.username}</strong>
          <span>{account.role === "admin" ? t("Administrator") : t("Operator")}</span>
          <small>{t("Used in the account selector and future linked control sessions.")}</small>
        </span>
      </div>
      <p className="hint-block">{t("Images are scaled to {edge}px, validated as JPEG, and stored only in MagicHandy app data.", { edge: PROFILE_IMAGE_MAX_EDGE })}</p>
      {error && <p className="form-status auth-error" role="alert">{error}</p>}
    </section>
  );
}

function PasswordGroup({ disabled, onChanged }: { disabled: boolean; onChanged: () => Promise<unknown> }) {
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

function LinkedProfilesGroup() {
  const identities = useAuth().status?.control_identities ?? [];
  const linked = identities.filter((identity) => identity.relationship === "linked");
  if (!linked.length) return null;
  return (
    <section className="group">
      <h3 className="group-title">{t("Linked control profiles")}</h3>
      <div className="linked-profile-list">{linked.map((identity) => (
        <div key={identity.account.id}><AccountAvatar account={identity.account} /><span><strong>{identity.label}</strong><small>{identity.account.username}</small></span></div>
      ))}</div>
    </section>
  );
}

function AccountList({ current, accounts, disabled, onChanged }: {
  current: UserAccount;
  accounts: UserAccount[];
  disabled: boolean;
  onChanged: () => Promise<void>;
}) {
  const { show } = useToast();
  const [busy, setBusy] = useState("");
  const [resetID, setResetID] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [error, setError] = useState("");
  const [permissionID, setPermissionID] = useState("");
  const toggle = async (account: UserAccount) => {
    if (!account.disabled && !window.confirm(t("Disable {username}? Every active session for this account will be signed out.", { username: account.username }))) return;
    setBusy(account.id);
    setError("");
    try {
      await api.setAccountDisabled(account.id, !account.disabled);
      await onChanged();
      show(account.disabled ? t("Account enabled.") : t("Account disabled."));
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  };
  const reset = async (event: FormEvent, account: UserAccount) => {
    event.preventDefault();
    const validation = newPasswordError(password, confirmation);
    if (validation) {
      setError(validation);
      return;
    }
    setBusy(account.id);
    setError("");
    try {
      await api.resetAccountPassword(account.id, password);
      setResetID("");
      setPassword("");
      setConfirmation("");
      show(t("Password reset. Existing sessions were signed out."), "success");
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  };
  return (
    <ul className="account-list">
      {accounts.map((account) => (
        <li className="account-row" key={account.id}>
          <div className="account-row-summary">
            <AccountAvatar account={account} />
            <span className="account-row-name"><strong>{account.username}</strong><span className="account-row-meta"><small>{account.role === "admin" ? t("Administrator") : t("Operator")}</small><small>{account.disabled ? t("Disabled") : t("Enabled")}</small></span>
              <small>{account.last_login_at ? t("Last sign-in {time}", { time: new Date(account.last_login_at).toLocaleString() }) : t("Never signed in")}</small>
            </span>
            <span className="row-actions">
              {account.id !== current.id && <button className="btn btn-secondary small" type="button" disabled={disabled || Boolean(busy)} onClick={() => void toggle(account)}>{account.disabled ? t("Enable") : t("Disable")}</button>}
              {account.id !== current.id && <button className="btn btn-secondary small" type="button" disabled={disabled || Boolean(busy)} onClick={() => { setResetID(resetID === account.id ? "" : account.id); setPassword(""); setConfirmation(""); setError(""); }}>{t("Reset password")}</button>}
              {account.role === "operator" && <button className="btn btn-secondary small" type="button" disabled={disabled} aria-expanded={permissionID === account.id} onClick={() => setPermissionID(permissionID === account.id ? "" : account.id)}>{t("Control permission")}</button>}
            </span>
          </div>
          {resetID === account.id && <form className="account-reset-form" onSubmit={(event) => void reset(event, account)}>
            <label className="field"><span className="label">{t("New password for {username}", { username: account.username })}</span><input type="password" autoComplete="new-password" value={password} disabled={disabled || Boolean(busy)} onChange={(event) => setPassword(event.target.value)} /></label>
            <PasswordConfirmationField password={password} confirmation={confirmation} disabled={disabled || Boolean(busy)} onChange={setConfirmation} />
            <button className="btn btn-primary" type="submit" disabled={disabled || Boolean(busy) || !password}>{busy ? t("Saving…") : t("Save new password")}</button>
          </form>}
          {account.role === "operator" && permissionID === account.id && <ControlGrantPanel accountID={account.id} disabled={disabled || account.disabled || Boolean(busy)} />}
        </li>
      ))}
      {error && <li className="form-status auth-error" role="alert">{error}</li>}
    </ul>
  );
}

function CreateAccountForm({ disabled, onCreated }: { disabled: boolean; onCreated: () => Promise<void> }) {
  const { show } = useToast();
  const [username, setUsername] = useState("");
  const [role, setRole] = useState<AccountRole>("operator");
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
      await api.createAccount(username.trim(), password, role);
      setUsername("");
      setPassword("");
      setConfirmation("");
      setRole("operator");
      await onCreated();
      show(t("Account created."), "success");
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  };
  return (
    <form className="account-form account-create-form" onSubmit={(event) => void submit(event)}>
      <h4>{t("Add an account")}</h4>
      <div className="settings-grid two">
        <label className="field"><span className="label">{t("Username")}</span><input type="text" autoComplete="off" spellCheck={false} value={username} disabled={disabled || busy} onChange={(event) => setUsername(event.target.value)} /></label>
        <label className="field"><span className="label">{t("Role")}</span><select value={role} disabled={disabled || busy} onChange={(event) => setRole(event.target.value as AccountRole)}><option value="operator">{t("Operator")}</option><option value="admin">{t("Administrator")}</option></select></label>
        <label className="field"><span className="label">{t("Password")}</span><input type="password" autoComplete="new-password" value={password} disabled={disabled || busy} onChange={(event) => setPassword(event.target.value)} /></label>
        <PasswordConfirmationField password={password} confirmation={confirmation} disabled={disabled || busy} onChange={setConfirmation} />
      </div>
      {error && <p className="form-status auth-error" role="alert">{error}</p>}
      <button className="btn btn-primary" type="submit" disabled={disabled || busy || !username.trim() || !password}>{busy ? t("Creating…") : t("Create account")}</button>
    </form>
  );
}

function newPasswordError(password: string, confirmation: string): string {
  if (!passwordMeetsMinimum(password)) return t("Use a password or passphrase of at least 8 characters.");
  if (password !== confirmation) return t("The passwords do not match.");
  return "";
}
