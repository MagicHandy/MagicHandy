import { useTranslation } from "react-i18next";
import { LOCALE_LABELS, type AppLocale } from "../i18n";
import { useLocale } from "../contexts/LocaleContext";

export function LanguageSelector() {
  const { t } = useTranslation();
  const { locale, supportedLocales, changeLanguage } = useLocale();

  return (
    <label className="field">
      <span>{t("config.language.label")}</span>
      <select
        value={locale}
        aria-label={t("config.language.aria")}
        onChange={(e) => void changeLanguage(e.target.value as AppLocale)}
      >
        {supportedLocales.map((code) => (
          <option key={code} value={code}>
            {LOCALE_LABELS[code]}
          </option>
        ))}
      </select>
      <p className="hint">{t("config.language.hint")}</p>
    </label>
  );
}
