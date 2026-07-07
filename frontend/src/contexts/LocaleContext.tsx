import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { useTranslation } from "react-i18next";
import { api } from "../api/client";
import type { UIPreferences } from "../api/types";
import i18n, { normalizeLocale, type AppLocale } from "../i18n";
import { LanguageGate } from "../components/LanguageGate";

type LocaleContextValue = {
  locale: AppLocale;
  ready: boolean;
  localePromptDismissed: boolean;
  supportedLocales: AppLocale[];
  changeLanguage: (locale: AppLocale, opts?: { dismissPrompt?: boolean }) => Promise<void>;
};

const LocaleContext = createContext<LocaleContextValue | null>(null);

function applyDocumentLocale(locale: AppLocale) {
  document.documentElement.lang = locale;
}

export function LocaleProvider({ children }: { children: ReactNode }) {
  const { i18n: i18nInstance } = useTranslation();
  const [ready, setReady] = useState(false);
  const [prefs, setPrefs] = useState<UIPreferences | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const data = await api.getUIPreferences();
        if (cancelled) return;
        const locale = normalizeLocale(data.locale);
        await i18n.changeLanguage(locale);
        applyDocumentLocale(locale);
        setPrefs(data);
      } catch {
        if (cancelled) return;
        await i18n.changeLanguage("en");
        applyDocumentLocale("en");
        setPrefs({
          locale: "en",
          locale_prompt_dismissed: false,
          supported_locales: ["en", "fr", "pt", "ru"],
        });
      } finally {
        if (!cancelled) setReady(true);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    const onChange = (lng: string) => applyDocumentLocale(normalizeLocale(lng));
    i18nInstance.on("languageChanged", onChange);
    return () => {
      i18nInstance.off("languageChanged", onChange);
    };
  }, [i18nInstance]);

  const changeLanguage = useCallback(
    async (locale: AppLocale, opts?: { dismissPrompt?: boolean }) => {
      const next = normalizeLocale(locale);
      await i18n.changeLanguage(next);
      applyDocumentLocale(next);
      const body: { locale?: string; locale_prompt_dismissed?: boolean } = {
        locale: next,
      };
      if (opts?.dismissPrompt) body.locale_prompt_dismissed = true;
      try {
        const saved = await api.saveUIPreferences(body);
        setPrefs(saved);
      } catch {
        setPrefs((prev) =>
          prev
            ? {
                ...prev,
                locale: next,
                locale_prompt_dismissed:
                  opts?.dismissPrompt ?? prev.locale_prompt_dismissed,
              }
            : {
                locale: next,
                locale_prompt_dismissed: Boolean(opts?.dismissPrompt),
                supported_locales: ["en", "fr", "pt", "ru"],
              },
        );
      }
    },
    [],
  );

  const value = useMemo<LocaleContextValue>(
    () => ({
      locale: normalizeLocale(prefs?.locale ?? i18n.language),
      ready,
      localePromptDismissed: prefs?.locale_prompt_dismissed ?? false,
      supportedLocales: (prefs?.supported_locales ?? ["en", "fr", "pt", "ru"]).map(
        (l) => normalizeLocale(l),
      ),
      changeLanguage,
    }),
    [prefs, ready, changeLanguage],
  );

  if (!ready) {
    return (
      <div className="locale-loading" aria-busy="true">
        <span className="hint">…</span>
      </div>
    );
  }

  return (
    <LocaleContext.Provider value={value}>
      {!value.localePromptDismissed && <LanguageGate />}
      {children}
    </LocaleContext.Provider>
  );
}

export function useLocale() {
  const ctx = useContext(LocaleContext);
  if (!ctx) throw new Error("useLocale must be used within LocaleProvider");
  return ctx;
}
