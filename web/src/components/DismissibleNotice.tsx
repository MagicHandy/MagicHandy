import { useEffect, useRef, useState, type ReactNode } from "react";
import { t, translateKnown } from "../i18n";
import { noticeDefinition } from "../notice-catalog";
import { useNoticePreferences } from "../state/notice-preferences";

export function DismissibleNotice({ id, className = "", children }: { id: string; className?: string; children: ReactNode }) {
  const preferences = useNoticePreferences();
  const [asking, setAsking] = useState(false);
  const [error, setError] = useState("");
  const close = useRef<HTMLButtonElement>(null);
  const once = useRef<HTMLButtonElement>(null);
  const wasAsking = useRef(false);
  const entry = noticeDefinition(id);
  const title = translateKnown(entry?.label || id);
  useEffect(() => {
    if (asking) once.current?.focus();
    else if (wasAsking.current) close.current?.focus();
    wasAsking.current = asking;
  }, [asking]);
  if (preferences?.loading || preferences?.hidden.includes(id)) return null;
  const cancel = () => { setAsking(false); setError(""); close.current?.focus(); };
  const save = async () => {
    try { await preferences?.save(id, true); setAsking(false); }
    catch (reason) { setError(reason instanceof Error ? reason.message : t("Request failed")); }
  };
  return <div className={`dismissible-notice ${className}`} role="note" aria-label={title}>
    <div className="notice-content">
      {asking ? <div className="notice-dismiss-choice" role="group" aria-label={t("Hide this notice?")} onKeyDown={event => { if (event.key === "Escape" && !preferences?.busy) cancel(); }}>
        <strong>{t("Hide this notice?")}</strong>
        <span>{entry?.browser_only || preferences?.scope === "browser" ? t("Don't show again will be saved for this browser.") : t("Don't show again will follow your account across browsers.")}</span>
        {(error || preferences?.error) && <p className="form-status auth-error" role="alert">{error || preferences?.error}</p>}
        <div className="row-actions">
          <button ref={once} type="button" className="btn btn-secondary" disabled={preferences?.busy} onClick={() => { setAsking(false); preferences?.dismissOnce(id); }}>{t("Just this time")}</button>
          <button type="button" className="btn btn-secondary" disabled={preferences?.busy} onClick={() => void save()}>{t("Don't show again")}</button>
          <button type="button" className="btn btn-secondary" disabled={preferences?.busy} onClick={cancel}>{t("Cancel")}</button>
        </div>
      </div> : children}
    </div>
    {preferences && !asking && <button ref={close} className="notice-dismiss" type="button" aria-label={t("Dismiss {notice}", { notice: title })} title={t("Dismiss {notice}", { notice: title })} onClick={() => setAsking(true)}>×</button>}
  </div>;
}
