import { useEffect, useId, useRef, useState } from "react";
import { t } from "../i18n";
import { registerConfirmHost, type ConfirmRequest } from "../util/confirm";
import { trapModalTab } from "../util/modal";

// The one place confirmations render. It sits below the shell's Emergency
// Stop layer, so Stop stays clickable (and Escape still stops motion) while a
// question is open; Escape also answers the question with Cancel.
export function ConfirmHost() {
  const [request, setRequest] = useState<ConfirmRequest | null>(null);
  useEffect(() => registerConfirmHost((next) => setRequest((current) => {
    current?.resolve(false);
    return next;
  })), []);
  if (!request) return null;
  return <ConfirmDialog key={request.id} request={request} onDone={(confirmed) => {
    setRequest(null);
    request.resolve(confirmed);
  }} />;
}

function ConfirmDialog({ request, onDone }: { request: ConfirmRequest; onDone: (confirmed: boolean) => void }) {
  const dialogRef = useRef<HTMLElement>(null);
  const cancelRef = useRef<HTMLButtonElement>(null);
  const doneRef = useRef(onDone);
  doneRef.current = onDone;
  const messageID = useId();
  useEffect(() => {
    const returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    cancelRef.current?.focus();
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") doneRef.current(false);
      else if (dialogRef.current) trapModalTab(event, dialogRef.current);
    };
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("keydown", onKeyDown);
      returnFocus?.focus();
    };
  }, []);
  return (
    <div className="modal-scrim" onMouseDown={(event) => { if (event.target === event.currentTarget) onDone(false); }}>
      <section ref={dialogRef} className="chat-session-dialog confirm-dialog" role="alertdialog" aria-modal="true" aria-labelledby={messageID} tabIndex={-1}>
        <div className="chat-session-dialog-body">
          <p id={messageID}>{request.message}</p>
        </div>
        <footer>
          <button ref={cancelRef} type="button" className="btn btn-secondary" onClick={() => onDone(false)}>{t("Cancel")}</button>
          <button type="button" className={request.destructive ? "btn btn-danger-outline" : "btn btn-primary"} onClick={() => onDone(true)}>{request.confirmLabel}</button>
        </footer>
      </section>
    </div>
  );
}
