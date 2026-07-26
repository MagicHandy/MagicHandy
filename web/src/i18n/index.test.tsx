import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import english from "./locales/en.json";
import spanish from "./locales/es.json";
import portuguese from "./locales/pt-BR.json";
import chinese from "./locales/zh-Hans.json";
import japanese from "./locales/ja.json";
import {
  getActiveLocale,
  I18nProvider,
  normalizeLocale,
  setLocaleForTest,
  t,
  translateKnown,
} from ".";

const app = vi.hoisted(() => ({ locale: "en" }));

vi.mock("../state/app-state", () => ({
  useAppState: () => ({ state: { settings: { ui: { locale: app.locale } } } }),
}));

afterEach(() => {
  app.locale = "en";
  setLocaleForTest("en", english);
  document.documentElement.lang = "en";
});

describe("localization", () => {
  it("normalizes unsupported and missing locale values to English", () => {
    expect(normalizeLocale("pt-BR")).toBe("pt-BR");
    expect(normalizeLocale("pt-br")).toBe("en");
    expect(normalizeLocale("fr")).toBe("en");
    expect(normalizeLocale(undefined)).toBe("en");
  });

  it.each([
    ["en", english, "Save settings", "3 videos"],
    ["es", spanish, "Guardar ajustes", "3 videos"],
    ["pt-BR", portuguese, "Salvar configurações", "3 vídeos"],
    ["zh-Hans", chinese, "保存设置", "3 个视频"],
    ["ja", japanese, "設定を保存", "3 個ビデオ"],
  ] as const)("renders and interpolates the %s catalog", (locale, catalog, save, videos) => {
    setLocaleForTest(locale, catalog);
    expect(t("Save settings")).toBe(save);
    expect(t("{count} videos", { count: 3 })).toBe(videos);
    expect(t("{count} videos")).toContain("{count}");
  });

  it("translates known runtime status and preserves unknown diagnostics", () => {
    setLocaleForTest("es", spanish);
    expect(translateKnown("Ready")).toBe("Listo");
    expect(translateKnown("llama.cpp exited with code 17")).toBe("llama.cpp exited with code 17");
  });

  it("loads the saved catalog, updates the document language, and reacts to changes", async () => {
    function Probe() {
      return <span>{t("Save settings")}</span>;
    }

    app.locale = "es";
    const view = render(<I18nProvider><Probe /></I18nProvider>);
    expect(await screen.findByText("Guardar ajustes")).toBeInTheDocument();
    expect(getActiveLocale()).toBe("es");
    expect(document.documentElement.lang).toBe("es");

    app.locale = "ja";
    view.rerender(<I18nProvider><Probe /></I18nProvider>);
    expect(await screen.findByText("設定を保存")).toBeInTheDocument();
    await waitFor(() => expect(document.documentElement.lang).toBe("ja"));
  });
});