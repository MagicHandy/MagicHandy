import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useEffect, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import english from "./locales/en.json";
import spanish from "./locales/es.json";
import portuguese from "./locales/pt-BR.json";
import chinese from "./locales/zh-Hans.json";
import japanese from "./locales/ja.json";
import {
  getActiveLocale,
  formatNumber,
  I18nProvider,
  normalizeLocale,
  setLocaleForTest,
  t,
  translateKnown,
  useI18n,
  useLocale,
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

  it("formats numbers with the selected UI locale", () => {
    setLocaleForTest("es", spanish);
    expect(formatNumber(12345.5)).toBe(new Intl.NumberFormat("es").format(12345.5));
    expect(formatNumber(12345.5)).not.toBe(new Intl.NumberFormat("en").format(12345.5));
  });

  it("loads the saved catalog, updates the document language, and reacts to changes", async () => {
    let mounts = 0;
    function Probe() {
      useLocale();
      const [draft, setDraft] = useState("");
      useEffect(() => {
        mounts += 1;
      }, []);
      return (
        <>
          <span>{t("Save settings")}</span>
          <input aria-label="draft" value={draft} onChange={(event) => setDraft(event.target.value)} />
        </>
      );
    }

    app.locale = "es";
    const view = render(<I18nProvider><Probe /></I18nProvider>);
    expect(await screen.findByText("Guardar ajustes")).toBeInTheDocument();
    // getActiveLocale reads a module global that I18nProvider assigns during
    // render. React may discard or reorder a render, so the global can trail
    // the committed DOM by a tick — asserting it synchronously right after the
    // text appears is a race, and it fails on slower CI runners. See the note
    // on render-time assignment in the provider.
    await waitFor(() => expect(getActiveLocale()).toBe("es"));
    await waitFor(() => expect(document.documentElement.lang).toBe("es"));
    fireEvent.change(screen.getByLabelText("draft"), { target: { value: "keep me" } });

    app.locale = "ja";
    view.rerender(<I18nProvider><Probe /></I18nProvider>);
    expect(await screen.findByText("設定を保存")).toBeInTheDocument();
    await waitFor(() => expect(document.documentElement.lang).toBe("ja"));
    expect(screen.getByLabelText("draft")).toHaveValue("keep me");
    expect(mounts).toBe(1);
  });

  it("keeps the active catalog after a load failure and retries on request", async () => {
    let attempts = 0;
    const loader = vi.fn(async () => {
      attempts += 1;
      if (attempts === 1) throw new Error("chunk unavailable");
      return spanish;
    });
    function Probe() {
      const i18n = useI18n();
      return (
        <>
          <span>{t("Save settings")}</span>
          <span>{i18n.loadError ? "failed" : "ready"}</span>
          <button onClick={i18n.retry}>retry</button>
        </>
      );
    }

    app.locale = "es";
    render(<I18nProvider catalogLoader={loader}><Probe /></I18nProvider>);
    expect(await screen.findByText("failed")).toBeInTheDocument();
    expect(screen.getByText("Save settings")).toBeInTheDocument();
    expect(document.documentElement.lang).toBe("en");

    fireEvent.click(screen.getByText("retry"));
    expect(await screen.findByText("Guardar ajustes")).toBeInTheDocument();
    expect(screen.getByText("ready")).toBeInTheDocument();
    expect(document.documentElement.lang).toBe("es");
    expect(loader).toHaveBeenCalledTimes(2);
  });
});