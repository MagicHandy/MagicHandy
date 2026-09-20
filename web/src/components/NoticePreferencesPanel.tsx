import { t, translateKnown } from "../i18n";
import { noticeDefinitions } from "../notice-catalog";
import { useNoticePreferences } from "../state/notice-preferences";

export function NoticePreferencesPanel() {
  const preferences = useNoticePreferences();
  if (!preferences) return null;
  const hidden = noticeDefinitions.filter(entry => preferences.hidden.includes(entry.id));
  return <section className="group notice-preferences" aria-label={t("Informational notices")}>
    <h3 className="group-title">{t("Informational notices")}</h3>
    <p className="hint-block">{preferences.scope === "account" ? t("Saved choices follow your account. Sign-in notices are saved for this browser.") : t("Saved choices apply to this browser. They are stored in MagicHandy's database.")}</p>
    {preferences.loading && <p role="status">{t("Loading notices…")}</p>}
    {preferences.error && <p className="form-status auth-error" role="alert">{preferences.error}</p>}
    {!preferences.loading && !preferences.error && hidden.length === 0 && <p className="hint-block">{t("All informational notices are shown.")}</p>}
    <ul className="notice-preferences-list">{hidden.map(entry => <li key={entry.id}>
      <span>{translateKnown(entry.label)}<small>{preferences.temporary.includes(entry.id) ? t("Just this time") : entry.browser_only || preferences.scope === "browser" ? t("This browser") : t("Your account")}</small></span>
      <button type="button" className="btn btn-secondary" disabled={preferences.loading || preferences.busy} onClick={() => void preferences.save(entry.id, false).catch(() => undefined)}>{t("Show again")}</button>
    </li>)}</ul>
    <div className="row-actions">
      {hidden.length > 0 && <button className="btn btn-secondary" type="button" disabled={preferences.loading || preferences.busy} onClick={() => void preferences.reset().catch(() => undefined)}>{t("Show all notices again")}</button>}
      <button className="btn btn-secondary" type="button" disabled={preferences.loading || preferences.busy} onClick={() => void preferences.refresh().catch(() => undefined)}>{t("Refresh")}</button>
    </div>
  </section>;
}
