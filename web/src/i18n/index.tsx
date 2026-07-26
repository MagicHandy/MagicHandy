import {
  createContext,
  Fragment,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { useAppState } from "../state/app-state";
import english from "./locales/en.json";

export const SUPPORTED_LOCALES = ["en", "es", "pt-BR", "zh-Hans", "ja"] as const;
export type Locale = (typeof SUPPORTED_LOCALES)[number];
export type MessageKey = keyof typeof english;
export type MessageValues = Record<string, string | number>;
type Catalog = Record<MessageKey, string>;

export const LOCALE_OPTIONS: ReadonlyArray<{ value: Locale; label: string }> = [
  { value: "en", label: "English" },
  { value: "es", label: "Español" },
  { value: "pt-BR", label: "Português (Brasil)" },
  { value: "zh-Hans", label: "简体中文" },
  { value: "ja", label: "日本語" },
];

const loaders: Record<Exclude<Locale, "en">, () => Promise<{ default: Catalog }>> = {
  es: () => import("./locales/es.json"),
  "pt-BR": () => import("./locales/pt-BR.json"),
  "zh-Hans": () => import("./locales/zh-Hans.json"),
  ja: () => import("./locales/ja.json"),
};

let activeLocale: Locale = "en";
let activeCatalog: Catalog = english;

export function normalizeLocale(value: string | null | undefined): Locale {
  return SUPPORTED_LOCALES.includes(value as Locale) ? value as Locale : "en";
}

function interpolate(message: string, values?: MessageValues): string {
  if (!values) return message;
  return message.replace(/\{([A-Za-z0-9_]+)\}/g, (placeholder, name: string) => (
    Object.prototype.hasOwnProperty.call(values, name) ? String(values[name]) : placeholder
  ));
}

export function t(message: MessageKey, values?: MessageValues): string {
  return interpolate(activeCatalog[message] ?? english[message], values);
}

// Server and model status text is not always a compile-time literal. Translate
// it when it matches a catalog key and preserve the diagnostic otherwise.
export function translateKnown(message: string, values?: MessageValues): string {
  const translated = (activeCatalog as Record<string, string>)[message]
    ?? (english as Record<string, string>)[message]
    ?? message;
  return interpolate(translated, values);
}

async function loadCatalog(locale: Locale): Promise<Catalog> {
  if (locale === "en") return english;
  return (await loaders[locale]()).default;
}

interface I18nContextValue {
  locale: Locale;
}

const I18nContext = createContext<I18nContextValue>({ locale: "en" });

export function I18nProvider({ children }: { children: ReactNode }) {
  const { state } = useAppState();
  const requested = normalizeLocale(state?.settings?.ui?.locale);
  const [loaded, setLoaded] = useState<{ locale: Locale; catalog: Catalog }>({
    locale: "en",
    catalog: english,
  });

  useEffect(() => {
    let cancelled = false;
    void loadCatalog(requested).then((catalog) => {
      if (!cancelled) setLoaded({ locale: requested, catalog });
    });
    return () => { cancelled = true; };
  }, [requested]);

  activeLocale = loaded.locale;
  activeCatalog = loaded.catalog;

  useEffect(() => {
    document.documentElement.lang = loaded.locale;
  }, [loaded.locale]);

  const value = useMemo(() => ({ locale: loaded.locale }), [loaded.locale]);
  return (
    <I18nContext.Provider value={value}>
      <Fragment key={loaded.locale}>{children}</Fragment>
    </I18nContext.Provider>
  );
}

export function useLocale(): Locale {
  return useContext(I18nContext).locale;
}

export function setLocaleForTest(locale: Locale, catalog: Catalog = english): void {
  activeLocale = locale;
  activeCatalog = catalog;
}

export function getActiveLocale(): Locale {
  return activeLocale;
}