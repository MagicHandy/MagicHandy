import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import en from "./locales/en.json";
import fr from "./locales/fr.json";
import pt from "./locales/pt.json";
import ru from "./locales/ru.json";

export const SUPPORTED_LOCALES = ["en", "fr", "pt", "ru"] as const;
export type AppLocale = (typeof SUPPORTED_LOCALES)[number];

export const LOCALE_LABELS: Record<AppLocale, string> = {
  en: "English",
  fr: "Français",
  pt: "Português",
  ru: "Русский",
};

export function normalizeLocale(locale: string | undefined | null): AppLocale {
  const raw = (locale ?? "en").toLowerCase().trim();
  if (raw.startsWith("en")) return "en";
  if (raw.startsWith("fr")) return "fr";
  if (raw.startsWith("pt")) return "pt";
  if (raw.startsWith("ru")) return "ru";
  if ((SUPPORTED_LOCALES as readonly string[]).includes(raw)) return raw as AppLocale;
  return "en";
}

void i18n.use(initReactI18next).init({
  resources: {
    en: { translation: en },
    fr: { translation: fr },
    pt: { translation: pt },
    ru: { translation: ru },
  },
  lng: "en",
  fallbackLng: "en",
  interpolation: { escapeValue: false },
  returnEmptyString: false,
});

export default i18n;
