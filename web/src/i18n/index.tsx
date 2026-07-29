import {
  createContext,
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
type CatalogLoader = (locale: Locale) => Promise<Catalog>;

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

export function formatNumber(value: number): string {
  return new Intl.NumberFormat(activeLocale).format(value);
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
  requestedLocale: Locale;
  loadError: boolean;
  retry: () => void;
}

const I18nContext = createContext<I18nContextValue>({
  locale: "en",
  requestedLocale: "en",
  loadError: false,
  retry: () => {},
});

export function I18nProvider({
  children,
  catalogLoader = loadCatalog,
}: {
  children: ReactNode;
  catalogLoader?: CatalogLoader;
}) {
  const { state } = useAppState();
  const requested = normalizeLocale(state?.settings?.ui?.locale);
  const [loaded, setLoaded] = useState<{ locale: Locale; catalog: Catalog }>({
    locale: "en",
    catalog: english,
  });
  const [loadError, setLoadError] = useState(false);
  const [retrySequence, setRetrySequence] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setLoadError(false);
    void catalogLoader(requested).then(
      (catalog) => {
        if (!cancelled) setLoaded({ locale: requested, catalog });
      },
      () => {
        if (!cancelled) setLoadError(true);
      },
    );
    return () => { cancelled = true; };
  }, [catalogLoader, requested, retrySequence]);

  // Assigned during render so t() and formatNumber, which are plain functions
  // called from render, see the active catalog in the same pass that renders
  // with it. This is a side effect in render, which React may discard or
  // reorder, so the globals can trail the committed tree by a tick. Readers
  // that need certainty should observe the context value instead; callers that
  // only format text are unaffected because a discarded render is not shown.
  activeLocale = loaded.locale;
  activeCatalog = loaded.catalog;

  useEffect(() => {
    document.documentElement.lang = loaded.locale;
  }, [loaded.locale]);

  const value = useMemo(() => ({
    locale: loaded.locale,
    requestedLocale: requested,
    loadError,
    retry: () => setRetrySequence((value) => value + 1),
  }), [loadError, loaded.locale, requested]);
  return (
    <I18nContext.Provider value={value}>
      {children}
    </I18nContext.Provider>
  );
}

export function useI18n(): I18nContextValue {
  return useContext(I18nContext);
}

export function useLocale(): Locale {
  return useI18n().locale;
}

export function setLocaleForTest(locale: Locale, catalog: Catalog = english): void {
  activeLocale = locale;
  activeCatalog = catalog;
}

export function getActiveLocale(): Locale {
  return activeLocale;
}