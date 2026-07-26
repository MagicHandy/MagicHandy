import { t } from "../i18n";
import { useEffect, useRef } from "react";
import type { ChatSession } from "../api/types";
import { trapModalTab } from "../util/modal";

interface Props {
  action: "new" | "switch";
  active: ChatSession;
  targetTitle?: string;
  autopilotActive: boolean;
  pending: boolean;
  onCancel: () => void;
  onContinue: (saveCurrent: boolean) => void;
}

export function ChatSessionDialog({ action, active, targetTitle, autopilotActive, pending, onCancel, onContinue }: Props) {
  const dialogRef = useRef<HTMLElement>(null);
  const cancelRef = useRef(onCancel);
  const pendingRef = useRef(pending);
  cancelRef.current = onCancel;
  pendingRef.current = pending;
  useEffect(() => {
    const previousOverflow = document.body.style.overflow;
    const returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    document.body.style.overflow = "hidden";
    dialogRef.current?.focus();
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !pendingRef.current) cancelRef.current();
      else if (dialogRef.current) trapModalTab(event, dialogRef.current);
    };
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", onKeyDown);
      returnFocus?.focus();
    };
  }, []);

  const startingNew = action === "new";
  const canSave = !active.saved && active.message_count > 0;
  const continueLabel = startingNew && canSave
    ? t("Discard and start")
    : startingNew
      ? t("Start new chat")
      : t("Switch without saving");

  return (
    <div className="modal-scrim" onMouseDown={(event) => { if (!pending && event.target === event.currentTarget) onCancel(); }}>
      <section
        ref={dialogRef}
        className="chat-session-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="chat-session-dialog-title"
        tabIndex={-1}
      >
        <header>
          <h2 id="chat-session-dialog-title">{startingNew ? t("Start a new chat?") : t("Switch to {title}?", { title: targetTitle ?? "" })}</h2>
        </header>
        <div className="chat-session-dialog-body">
          <p>
            {active.saved
              ? t("{title} is saved and will remain available in the tab bar.", { title: active.title })
              : canSave
                ? t("{title} has not been saved. Use Save and continue to keep it after MagicHandy closes.", { title: active.title })
                : t("The current chat is empty and will be replaced.")}
          </p>
          {autopilotActive && <p className="chat-session-dialog-note">{t("Autopilot will stop before the active chat changes.")}</p>}
        </div>
        <footer>
          <button type="button" className="btn btn-secondary" disabled={pending} onClick={onCancel}>{t("Cancel")}</button>
          {canSave && (
            <button type="button" className="btn btn-secondary" disabled={pending} onClick={() => onContinue(true)}>{startingNew ? t("Save and start") : t("Save and switch")}
            </button>
          )}
          <button type="button" className="btn btn-primary" disabled={pending} onClick={() => onContinue(false)}>
            {pending ? t("Updating chat…") : continueLabel}
          </button>
        </footer>
      </section>
    </div>
  );
}
