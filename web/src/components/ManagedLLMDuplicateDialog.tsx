import { useEffect, useRef } from "react";
import type { ManagedLLMDuplicateSnapshot } from "../api/types";
import { t } from "../i18n";
import { trapModalTab } from "../util/modal";

interface Props {
  snapshot: ManagedLLMDuplicateSnapshot;
  pending: boolean;
  readOnly: boolean;
  error: string;
  onCancel: () => void;
  onTerminate: () => void;
}

export function ManagedLLMDuplicateDialog({ snapshot, pending, readOnly, error, onCancel, onTerminate }: Props) {
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

  const count = snapshot.processes.length;
  return (
    <div className="modal-scrim" onMouseDown={(event) => { if (!pending && event.target === event.currentTarget) onCancel(); }}>
      <section
        ref={dialogRef}
        className="managed-duplicate-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="managed-duplicate-title"
        aria-describedby="managed-duplicate-description"
        tabIndex={-1}
      >
        <header>
          <p className="eyebrow">{t("Managed model")}</p>
          <h2 id="managed-duplicate-title">{t("Another model process is running")}</h2>
        </header>
        <div className="managed-duplicate-body" id="managed-duplicate-description">
          <p>{t("MagicHandy found another process using the exact executable configured for its managed {runner} runtime. A second copy can consume several gigabytes of memory and make reply speed inconsistent.", {
            runner: snapshot.runner_name || t("managed runner"),
          })}</p>
          <p>{t("MagicHandy will not launch another copy. If another MagicHandy window owns this process, close that window first. Otherwise, terminate the leftover process here.")}</p>
          <div className="managed-duplicate-processes" aria-label={t("Detected processes")}>
            {snapshot.processes.map((process) => (
              <span key={process.pid}>{t("{executable} | PID {pid}", {
                executable: process.executable || snapshot.runner_name || t("managed runner"),
                pid: process.pid,
              })}</span>
            ))}
          </div>
          {readOnly && <p className="managed-duplicate-note">{t("Take control from the top bar before terminating a process.")}</p>}
          {error && <p className="field-error" role="alert">{error}</p>}
        </div>
        <footer>
          <button type="button" className="btn btn-secondary" disabled={pending} onClick={onCancel}>{t("Leave running")}</button>
          <button type="button" className="btn btn-primary" disabled={pending || readOnly} onClick={onTerminate}>
            {pending
              ? t("Terminating process...")
              : count === 1
                ? t("Terminate duplicate")
                : t("Terminate duplicates")}
          </button>
        </footer>
      </section>
    </div>
  );
}
