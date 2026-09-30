import { useEffect, useRef, type ReactNode } from "react";
import { trapModalTab } from "../util/modal";

interface Props {
  titleID: string;
  title: string;
  /** While a write is pending, Escape and the scrim do not close the dialog. */
  busy?: boolean;
  className?: string;
  onClose: () => void;
  children: ReactNode;
  footer: ReactNode;
}

// A modal for the video workspace. Focus starts on the first element marked
// data-autofocus, stays inside the dialog (plus the global Stop), and returns
// to the control that opened it.
export function VideoDialog({ titleID, title, busy = false, className, onClose, children, footer }: Props) {
  const dialogRef = useRef<HTMLElement>(null);
  const closeRef = useRef(onClose);
  const busyRef = useRef(busy);
  closeRef.current = onClose;
  busyRef.current = busy;
  useEffect(() => {
    const previousOverflow = document.body.style.overflow;
    const returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    document.body.style.overflow = "hidden";
    const initial = dialogRef.current?.querySelector<HTMLElement>("[data-autofocus]");
    (initial ?? dialogRef.current)?.focus();
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !busyRef.current) closeRef.current();
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
    <div className="modal-scrim" onMouseDown={(event) => { if (!busy && event.target === event.currentTarget) onClose(); }}>
      <section
        ref={dialogRef}
        className={className ? `video-dialog ${className}` : "video-dialog"}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleID}
        tabIndex={-1}
      >
        <header><h2 id={titleID}>{title}</h2></header>
        <div className="video-dialog-body">{children}</div>
        <footer>{footer}</footer>
      </section>
    </div>
  );
}
