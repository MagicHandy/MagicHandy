import { useState } from "react";
import { useTranslation } from "react-i18next";
import { LOCALE_LABELS, type AppLocale } from "../i18n";
import { useLocale } from "../contexts/LocaleContext";

export function LanguageGate() {
  const { t } = useTranslation();
  const { supportedLocales, changeLanguage } = useLocale();
  const [selected, setSelected] = useState<AppLocale>("en");
  const [busy, setBusy] = useState(false);

  const confirm = async () => {
    setBusy(true);
    try {
      await changeLanguage(selected, { dismissPrompt: true });
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="language-gate" role="dialog" aria-modal="true" aria-labelledby="language-gate-title">
      <div className="language-gate-backdrop" />
      <div className="language-gate-card glass">
        <img
          className="language-gate-logo"
          src="/logo.jpeg"
          alt="MagicHandy"
          width={72}
          height={72}
          decoding="async"
        />
        <h1 id="language-gate-title" className="language-gate-title">
          {t("languageGate.title")}
        </h1>
        <p className="hint language-gate-hint">{t("languageGate.hint")}</p>
        <div className="language-gate-options" role="radiogroup" aria-label={t("languageGate.choose")}>
          {supportedLocales.map((code) => (
            <label key={code} className={`language-gate-option${selected === code ? " active" : ""}`}>
              <input
                type="radio"
                name="locale"
                value={code}
                checked={selected === code}
                onChange={() => setSelected(code)}
              />
              <span className="language-gate-option-label">{LOCALE_LABELS[code]}</span>
            </label>
          ))}
        </div>
        <button
          type="button"
          className="btn btn-primary language-gate-confirm"
          disabled={busy}
          onClick={confirm}
        >
          {busy ? t("common.loading") : t("languageGate.confirm")}
        </button>
      </div>
    </div>
  );
}
