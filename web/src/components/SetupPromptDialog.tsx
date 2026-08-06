import { useEffect, useRef } from "react";
import { t } from "../i18n";
import { trapModalTab } from "../util/modal";

interface Props {
  pending: boolean;
  readOnly: boolean;
  error: string;
  onRunSetup: () => void;
  onDismiss: () => void;
}

/**
 * Asks whether to run setup, instead of redirecting into it.
 *
 * A brand-new store still goes straight to the wizard: that is onboarding and
 * it is what a first run should do. This dialog is for the other case, a store
 * that already holds saved settings but is not marked as configured, which is
 * what happens after an update or when a previous run left setup part-way. That
 * case used to hijack the route on every launch with no way to decline.
 */
export function SetupPromptDialog({ pending, readOnly, error, onRunSetup, onDismiss }: Props) {
  const dialogRef = useRef<HTMLElement>(null);
  const dismissRef = useRef(onDismiss);
  const pendingRef = useRef(pending);
  dismissRef.current = onDismiss;
  pendingRef.current = pending;

  useEffect(() => {
    const previousOverflow = document.body.style.overflow;
    const returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    document.body.style.overflow = "hidden";
    dialogRef.current?.focus();
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !pendingRef.current) dismissRef.current();
      else if (dialogRef.current) trapModalTab(event, dialogRef.current);
    };
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", onKeyDown);
      returnFocus?.focus();
    };
  }, []);

  return (
    <div
      className="modal-scrim"
      onMouseDown={(event) => {
        if (!pending && event.target === event.currentTarget) onDismiss();
      }}
    >
      <section
        ref={dialogRef}
        className="setup-prompt-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="setup-prompt-title"
        aria-describedby="setup-prompt-description"
        tabIndex={-1}
      >
        <header>
          <p className="eyebrow">{t("Setup")}</p>
          <h2 id="setup-prompt-title">{t("Run setup again?")}</h2>
        </header>
        <div className="setup-prompt-body" id="setup-prompt-description">
          <p>{t("Your saved settings are still here. Setup is only needed if you want to change your device connection, model runtime, or voice modules.")}</p>
          <p>{t("You can run it at any time from Settings > General.")}</p>
        </div>
        {error && <p className="form-status" role="alert">{error}</p>}
        <footer className="setup-prompt-actions">
          <button type="button" className="btn btn-secondary" disabled={pending} onClick={onDismiss}>
            {pending ? t("Saving...") : t("Not now")}
          </button>
          <button type="button" className="btn btn-primary" disabled={pending || readOnly} onClick={onRunSetup}>
            {t("Run setup")}
          </button>
        </footer>
      </section>
    </div>
  );
}
