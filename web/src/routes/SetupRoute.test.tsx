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

const managedModel = {
  id: "managed-fixture",
  display_name: "Gemma fixture",
  provider: "llama_cpp",
  source: "gguf",
  format: "gguf",
  size_bytes: 1024,
  sha256: "a".repeat(64),
  model_path: "C:\\MagicHandy\\models\\gemma.gguf",
  imported_at: "2026-08-02T12:00:00Z",
  updated_at: "2026-08-02T12:00:00Z",
  state: "ready",
} as const;

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
    vi.spyOn(api, "scanOllamaModels").mockResolvedValue({
      path: "C:\\Users\\Test\\.ollama\\models",
      candidates: [{
        id: "library-gemma",
        name: "gemma3:4b",
        format: "gguf",
        size_bytes: 4096,
        digest: `sha256:${"b".repeat(64)}`,
        importable: true,
      }],
    });
    vi.spyOn(api, "importOllamaModel").mockResolvedValue({
      import: {
        id: "import-fixture",
        source: "ollama",
        display_name: "gemma3:4b",
        status: "queued",
        bytes_copied: 0,
        total_bytes: 4096,
        started_at: "2026-08-02T12:00:00Z",
        updated_at: "2026-08-02T12:00:00Z",
      },
    });
    vi.spyOn(api, "installSetupPlan").mockResolvedValue({
      installation: {
        id: "setup-plan-fixture",
        kind: "install_plan",
        module: "selected_components",
        device: "",
        status: "queued",
        message: "Selected component installation queued.",
        steps: [{ id: "llama_runtime", label: "Managed llama.cpp", status: "queued" }],
        completed_steps: 0,
        total_steps: 1,
        started_at: "2026-08-02T12:00:00Z",
        updated_at: "2026-08-02T12:00:00Z",
      },
    });
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

  it("keeps managed llama.cpp as the recommended default without a build action", async () => {
    render(<SetupRoute />);

    await screen.findByRole("heading", { name: "Set up MagicHandy" });
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    await screen.findByRole("heading", { name: "Choose how MagicHandy reaches your device" });
    fireEvent.click(screen.getByRole("button", { name: "Skip for now" }));

    await screen.findByRole("heading", { name: "Choose your model runtime" });
    expect(screen.getByRole("radio", { name: /Managed llama\.cpp.*Recommended/i })).toBeChecked();
    expect(screen.queryByRole("button", { name: /build managed|install managed/i })).not.toBeInTheDocument();
    expect(screen.getByRole("radio", { name: /Use my existing Ollama/i })).not.toBeChecked();
  });

  it("imports a selected model from an existing Ollama library during managed setup", async () => {
    render(<SetupRoute />);

    await screen.findByRole("heading", { name: "Set up MagicHandy" });
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    await screen.findByRole("heading", { name: "Choose how MagicHandy reaches your device" });
    fireEvent.click(screen.getByRole("button", { name: "Skip for now" }));
    await screen.findByRole("heading", { name: "Choose your model runtime" });
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));

    await screen.findByRole("heading", { name: "Choose a chat model" });
    expect(screen.getByRole("region", { name: "Managed model" })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Import a GGUF file" })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Import from an existing Ollama library" })).toBeInTheDocument();
    fireEvent.change(screen.getByRole("textbox", { name: "Ollama models path" }), {
      target: { value: "C:\\Users\\Test\\.ollama\\models" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Scan library" }));
    await screen.findByText("gemma3:4b");
    fireEvent.click(screen.getByRole("button", { name: "Import copy" }));

    await waitFor(() => expect(api.importOllamaModel).toHaveBeenCalledWith(
      "C:\\Users\\Test\\.ollama\\models",
      "library-gemma",
    ));
  });

  it("submits one backend-owned installation plan after all choices are made", async () => {
    settings = { ...settings, llm: { ...settings.llm, model: managedModel.id } };
    vi.mocked(useAppState).mockReturnValue({
      state: { settings },
      backendOnline: true,
      readOnly: false,
      refresh: vi.fn(async () => undefined),
    } as unknown as ReturnType<typeof useAppState>);
    vi.mocked(api.llmModels).mockResolvedValue({ ...modelFixture, models: [managedModel] });

    render(<SetupRoute />);

    await screen.findByRole("heading", { name: "Set up MagicHandy" });
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    await screen.findByRole("heading", { name: "Choose how MagicHandy reaches your device" });
    fireEvent.click(screen.getByRole("button", { name: "Skip for now" }));
    await screen.findByRole("heading", { name: "Choose your model runtime" });
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    await screen.findByRole("heading", { name: "Choose a chat model" });
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    await screen.findByRole("heading", { name: "Add voice features" });
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));

    await screen.findByRole("heading", { name: "Installing selected features" });
    expect(api.installSetupPlan).toHaveBeenCalledWith({ llama: { backend: "auto" }, parakeet: false });
    expect(screen.getByRole("progressbar", { name: "Installation progress" })).toHaveAttribute("value", "0");
    expect(screen.getByRole("log", { name: "Installation terminal output" })).toBeInTheDocument();
    expect(screen.getByRole("log", { name: "Installation terminal output" })).toHaveTextContent("Waiting for installer output...");
  });
});
