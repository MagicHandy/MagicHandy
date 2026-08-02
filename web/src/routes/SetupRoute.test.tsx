import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { LLMModelManagerSnapshot, PublicSettings } from "../api/types";
import { useAppState, useToast } from "../state/app-state";
import { setupFixture } from "../test/setup-fixture";
import { SetupRoute } from "./SetupRoute";

vi.mock("../state/app-state", () => ({
  useAppState: vi.fn(),
  useToast: vi.fn(),
}));

const modelFixture = {
  models: [],
  imports: [],
  store_path: "C:\\MagicHandy\\models",
  suggested_ollama_path: "",
  runtime: {
    state: "missing",
    installed: false,
    current: false,
    build_supported: true,
    supported_backends: ["auto", "cpu", "cuda"],
    expected_version: "fixture",
    message: "Managed runtime is not installed.",
  },
} as LLMModelManagerSnapshot;

function freshSettings(): PublicSettings {
  return {
    version: 2,
    server: { port: 49717 },
    ui: { locale: "en", theme: "steel-azure", setup_completed: false },
    device: { hsp_dispatch_owner: "cloud_rest", connection_key_set: false },
    llm: {
      provider: "llama_cpp",
      llama_cpp_mode: "managed",
      llama_cpp_base_url: "http://127.0.0.1:8080",
      ollama_base_url: "http://127.0.0.1:11434",
      model: "",
      prompt_set: "magichandy_motion_v1",
    },
  } as PublicSettings;
}

describe("SetupRoute", () => {
  let settings: PublicSettings;

  beforeEach(() => {
    settings = freshSettings();
    vi.mocked(useAppState).mockReturnValue({
      state: { settings },
      backendOnline: true,
      readOnly: false,
      refresh: vi.fn(async () => undefined),
    } as unknown as ReturnType<typeof useAppState>);
    vi.mocked(useToast).mockReturnValue({ show: vi.fn() } as unknown as ReturnType<typeof useToast>);
    vi.spyOn(api, "setupStatus").mockResolvedValue(setupFixture);
    vi.spyOn(api, "llmModels").mockResolvedValue(modelFixture);
    vi.spyOn(api, "ollamaModels").mockResolvedValue({ available: true, models: [] });
    vi.spyOn(api, "saveSetupPreferences").mockImplementation(async (update) => {
      if (update.ui_locale) settings = { ...settings, ui: { ...settings.ui, locale: update.ui_locale } };
      if (update.device_owner) settings = { ...settings, device: { ...settings.device, hsp_dispatch_owner: update.device_owner } };
      if (update.llm) settings = { ...settings, llm: { ...settings.llm, ...update.llm } };
      return { settings };
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("persists a runtime choice before model selection can be skipped", async () => {
    render(<SetupRoute />);

    await screen.findByRole("heading", { name: "Set up MagicHandy" });
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    await screen.findByRole("heading", { name: "Choose how MagicHandy reaches your device" });
    fireEvent.click(screen.getByRole("button", { name: "Skip for now" }));
    await screen.findByRole("heading", { name: "Choose your model runtime" });
    fireEvent.click(screen.getByRole("radio", { name: /use my existing ollama/i }));
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));

    await screen.findByRole("heading", { name: "Choose a chat model" });
    await waitFor(() => expect(api.saveSetupPreferences).toHaveBeenCalledWith(expect.objectContaining({
      llm: expect.objectContaining({ provider: "ollama" }),
    })));
  });
});
