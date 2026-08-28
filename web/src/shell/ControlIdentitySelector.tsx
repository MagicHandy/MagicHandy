import { useEffect, useRef, useState } from "react";
import type { ControlIdentity } from "../api/types";
import { t } from "../i18n";
import { AccountAvatar } from "../components/AccountAvatar";
import { ChevronUpIcon, CloseIcon } from "./icons";

interface ControlIdentitySelectorProps {
  identities: ControlIdentity[];
  open: boolean;
  restoreFocusOnClose: boolean;
  onOpenChange: (open: boolean) => void;
  onSelect: (accountID: string) => Promise<void>;
  onLogout: () => Promise<void>;
}

export function ControlIdentitySelector({
  identities,
  open,
  restoreFocusOnClose,
  onOpenChange,
  onSelect,
  onLogout,
}: ControlIdentitySelectorProps) {
  const trigger = useRef<HTMLButtonElement>(null);
  const close = useRef<HTMLButtonElement>(null);
  const wasOpen = useRef(false);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const selected = identities.find((identity) => identity.selected) ?? identities[0];

  useEffect(() => {
    if (open) {
      close.current?.focus();
    } else if (wasOpen.current && restoreFocusOnClose) {
      trigger.current?.focus();
    }
    wasOpen.current = open;
  }, [open, restoreFocusOnClose]);

  if (!selected) return null;

  const select = async (identity: ControlIdentity) => {
    if (busy || identity.selected) return;
    setBusy(identity.account.id);
    setError("");
    try {
      await onSelect(identity.account.id);
      onOpenChange(false);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("Control profile could not be changed."));
    } finally {
      setBusy("");
    }
  };

  const logout = async () => {
    if (busy) return;
    setBusy("logout");
    setError("");
    try {
      await onLogout();
      onOpenChange(false);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("Sign out failed."));
    } finally {
      setBusy("");
    }
  };

  return (
    <div className="control-identity">
      <button
        ref={trigger}
        type="button"
        className="control-identity-trigger"
        aria-label={t("Control profile: {name}; {action} selector", {
          name: selected.relationship === "self" ? t("Self") : selected.label,
          action: open ? t("close") : t("open"),
        })}
        aria-expanded={open}
        onClick={() => onOpenChange(!open)}
      >
        <AccountAvatar account={selected.account} className="account-avatar-small" />
        <span>
          <small>{t("Control profile")}</small>
          <strong>{selected.relationship === "self" ? t("Self") : selected.label}</strong>
        </span>
        <ChevronUpIcon className={open ? "chevron-open" : ""} />
      </button>
      {open && (
        <section className="control-identity-panel" aria-label={t("Control profile selector")}>
          <header>
            <span>
              <strong>{t("Control profile")}</strong>
              <small>{t("Signed in as {username}", { username: identities[0].account.username })}</small>
            </span>
            <button ref={close} type="button" className="icon-button" onClick={() => onOpenChange(false)} aria-label={t("Close selector")}>
              <CloseIcon />
            </button>
          </header>
          <div className="control-identity-options" role="radiogroup" aria-label={t("Available control profiles")}>
            {identities.map((identity) => (
              <button
                key={`${identity.relationship}:${identity.account.id}`}
                type="button"
                role="radio"
                aria-checked={identity.selected}
                disabled={Boolean(busy)}
                onClick={() => void select(identity)}
              >
                <AccountAvatar account={identity.account} />
                <span>
                  <strong>{identity.relationship === "self" ? t("Self") : identity.label}</strong>
                  <small>{identity.relationship === "linked"
                    ? t("{username} · Linked account", { username: identity.account.username })
                    : identity.account.username}</small>
                </span>
                <span className="status-dot" data-state={identity.selected ? "active" : "idle"} aria-hidden="true" />
              </button>
            ))}
          </div>
          <p className="control-identity-note">
            {t("This labels this browser session. It does not sign in as another account or transfer device control.")}
          </p>
          {error && <p className="form-status auth-error" role="alert">{error}</p>}
          <footer>
            <a href="#/settings/access" onClick={() => onOpenChange(false)}>{t("Manage account")}</a>
            <button type="button" className="btn btn-secondary small" disabled={Boolean(busy)} onClick={() => void logout()}>
              {busy === "logout" ? t("Signing out…") : t("Sign out")}
            </button>
          </footer>
        </section>
      )}
    </div>
  );
}
