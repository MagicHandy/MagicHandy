import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { LLMCatalog } from "../api/catalog-types";
import type { LLMModelManagerSnapshot, PublicSettings, SetupStatus } from "../api/types";
import { useAppState, useToast } from "../state/app-state";
import { useAuth } from "../state/auth";
import { setupFixture } from "../test/setup-fixture";
import { SetupRoute } from "./SetupRoute";

const bootstrapAccount = vi.fn(async () => undefined);
const refreshAuthentication = vi.fn(async () => null);

vi.mock("../state/app-state", () => ({
  useAppState: vi.fn(),
  useToast: vi.fn(),
}));

vi.mock("../state/auth", () => ({
  useAuth: vi.fn(),
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

const catalogModel = {
  id: "gemma-fixture",
  display_name: "Gemma 4 12B fixture",
  summary: "The tested default: reliable motion commands and replies in about two seconds.",
  family: "gemma4",
  parameter_size: "11.9B",
  quantization: "Q4_K_M",
  size_bytes: 7381381696,
  sha256: "b".repeat(64),
  license: "Apache-2.0 (Gemma 4)",
  license_url: "https://ai.google.dev/gemma/docs/gemma_4_license",
  source_name: "fixture/gemma:latest",
  source_url: "https://ollama.com/fixture/gemma",
  download_url: "https://registry.ollama.ai/v2/fixture/gemma/blobs/sha256:" + "b".repeat(64),
  vram_mib: 8144,
  min_vram_mib: 10240,
  default: true,
} as const;

function catalogFixture(fit: LLMCatalog["models"][number]["fit"], nvidia = true): LLMCatalog {
  return {
    models: [{ ...catalogModel, fit }],
    hardware: nvidia ? { nvidia: true, gpu_name: "Test NVIDIA GPU", vram_mib: 16303 } : { nvidia: false },
  };
}

function freshSettings(): PublicSettings {
  return {
    version: 2,
    server: { port: 49717 },
    ui: { locale: "en", theme: "steel-azure", setup_completed: false },
    device: { hsp_dispatch_owner: "cloud_rest", connection_key_set: false },
    motion: { handy_model: "handy_original" },
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

async function continueTo(heading: string) {
  fireEvent.click(screen.getByRole("button", { name: "Continue" }));
  await screen.findByRole("heading", { name: heading });
}

// Easy Setup is the default path; these tests walk the Custom steps.
async function startCustomSetup() {
  await screen.findByRole("heading", { name: "Set up MagicHandy" });
  fireEvent.click(screen.getByRole("radio", { name: /Custom setup/ }));
  await continueTo("Choose who can open MagicHandy");
}

async function skipTo(heading: string) {
  fireEvent.click(screen.getByRole("button", { name: "Skip for now" }));
  await screen.findByRole("heading", { name: heading });
}

async function openChatStep() {
  render(<SetupRoute />);
  await startCustomSetup();
  await continueTo("Choose how MagicHandy reaches your device");
  await skipTo("Set up the chat AI");
}

describe("SetupRoute", () => {
  let settings: PublicSettings;

  it("offers the last failure report when returning to setup after onboarding", async () => {
    vi.mocked(api.setupStatus).mockResolvedValue({ ...setupFixture, required: false, installation: {
      id: "last-failure", kind: "voice_module", module: "chatterbox", device: "cpu", status: "failed",
      message: "Chatterbox dependency installation failed.", started_at: "", updated_at: "",
    } });
    render(<SetupRoute />);
    expect(await screen.findByRole("button", { name: "Download failure report" })).toBeVisible();
    expect(screen.getByText("Chatterbox dependency installation failed.")).toBeVisible();
  });

  beforeEach(() => {
    bootstrapAccount.mockClear();
    refreshAuthentication.mockClear();
    settings = freshSettings();
    vi.mocked(useAppState).mockReturnValue({
      state: { settings },
      backendOnline: true,
      readOnly: false,
      refresh: vi.fn(async () => undefined),
    } as unknown as ReturnType<typeof useAppState>);
    vi.mocked(useToast).mockReturnValue({ show: vi.fn() } as unknown as ReturnType<typeof useToast>);
    vi.mocked(useAuth).mockReturnValue({
      status: { initialized: false, authentication_required: false, authenticated: false, bootstrap_available: true, ui_locale: "en", account: null, control_identities: null },
      bootstrap: bootstrapAccount,
      refresh: refreshAuthentication,
    } as unknown as ReturnType<typeof useAuth>);
    vi.spyOn(api, "setupStatus").mockResolvedValue(setupFixture);
    vi.spyOn(api, "networkStatus").mockResolvedValue({ active: { mode: "local", listen_address: "127.0.0.1:49717", public_url: "", trusted_proxies: [], tls_certificate: "", tls_private_key: "" }, saved: null, restart_required: false, interfaces: [{ name: "Ethernet", address: "192.168.1.8", loopback: false, private: true }], forwarded: false, authentication_required: false, secure_cookie: false });
    vi.spyOn(api, "llmModels").mockResolvedValue(modelFixture);
    vi.spyOn(api, "llmCatalog").mockResolvedValue(catalogFixture("recommended"));
    vi.spyOn(api, "getSettings").mockImplementation(async () => ({ settings }));
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
    vi.spyOn(api, "completeSetup").mockResolvedValue({ settings, signed_out: true });
    vi.spyOn(api, "saveSetupPreferences").mockImplementation(async (update) => {
      if (update.ui_locale) settings = { ...settings, ui: { ...settings.ui, locale: update.ui_locale } };
      if (update.device_owner) settings = { ...settings, device: { ...settings.device, hsp_dispatch_owner: update.device_owner } };
      if (update.handy_model) settings = { ...settings, motion: { ...settings.motion, handy_model: update.handy_model } };
      if (update.llm) settings = { ...settings, llm: { ...settings.llm, ...update.llm } };
      return { settings };
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("creates the first administrator inside the Access step before continuing", async () => {
    render(<SetupRoute />);

    await startCustomSetup();
    fireEvent.click(screen.getByRole("checkbox", { name: /require an account and password/i }));
    fireEvent.change(screen.getByRole("textbox", { name: "Administrator username" }), { target: { value: "owner" } });
    const password = screen.getByText("Password", { selector: ".label" }).closest("label")!.querySelector("input")!;
    const confirmation = screen.getByText("Confirm password", { selector: ".label" }).closest("label")!.querySelector("input")!;
    fireEvent.change(password, { target: { value: "fifteen-char8888" } });
    fireEvent.change(confirmation, { target: { value: "eight889" } });
    expect(confirmation).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByText("The passwords do not match.")).toHaveAttribute("data-state", "mismatch");
    expect(screen.getByRole("button", { name: "Continue" })).toBeDisabled();

    fireEvent.change(confirmation, { target: { value: "fifteen-char8888" } });
    expect(confirmation).toHaveAttribute("aria-invalid", "false");
    expect(screen.getByText("Passwords match.")).toHaveAttribute("data-state", "match");
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));

    await waitFor(() => expect(bootstrapAccount).toHaveBeenCalledWith("owner", "fifteen-char8888"));
    expect(await screen.findByRole("heading", { name: "Choose how MagicHandy reaches your device" })).toBeInTheDocument();
  });

  it("keeps remote access folded away until the user asks for it", async () => {
    render(<SetupRoute />);
    await startCustomSetup();

    expect(screen.getByText(/opens only on this computer/i)).toBeVisible();
    expect(screen.queryByRole("radio", { name: /^Public/ })).not.toBeInTheDocument();
    const reveal = screen.getByRole("button", { name: "Use MagicHandy from a phone or another computer" });
    await waitFor(() => expect(reveal).toBeEnabled());
    fireEvent.click(reveal);
    expect(screen.getByRole("radio", { name: /^Local only/ })).toBeChecked();
    expect(screen.getByRole("radio", { name: /^LAN \+ local/ })).toBeInTheDocument();
  });

  it("keeps Public setup in the Access step until HTTPS is saved", async () => {
    vi.spyOn(api, "discoverInternet").mockResolvedValue({ public_ip: "8.8.8.8", terms_url: "https://letsencrypt.org/documents/test.pdf", ip_error: false, ca_error: false, external_port: 443 });
    render(<SetupRoute />);
    await startCustomSetup();
    const reveal = screen.getByRole("button", { name: "Use MagicHandy from a phone or another computer" });
    await waitFor(() => expect(reveal).toBeEnabled());
    fireEvent.click(reveal);
    fireEvent.click(screen.getByRole("radio", { name: /^Public/ }));
    fireEvent.change(screen.getByLabelText("Administrator username"), { target: { value: "owner" } });
    fireEvent.change(screen.getByText("Password", { selector: ".label" }).closest("label")!.querySelector("input")!, { target: { value: "fifteen-char8888" } });
    fireEvent.change(screen.getByText("Confirm password", { selector: ".label" }).closest("label")!.querySelector("input")!, { target: { value: "fifteen-char8888" } });
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    expect(await screen.findByRole("button", { name: "Set up HTTPS and save" })).toBeVisible();
    expect(screen.getByRole("heading", { name: "Choose who can open MagicHandy" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Continue" })).toBeDisabled();
    expect(bootstrapAccount).toHaveBeenCalledOnce();
    expect(await screen.findByText(/Open incoming TCP 443/)).toBeVisible();
  });

  it("ends the temporary bootstrap session when protected setup finishes", async () => {
    render(<SetupRoute />);

    await startCustomSetup();
    fireEvent.click(screen.getByRole("checkbox", { name: /require an account and password/i }));
    fireEvent.change(screen.getByRole("textbox", { name: "Administrator username" }), { target: { value: "owner" } });
    const password = screen.getByText("Password", { selector: ".label" }).closest("label")!.querySelector("input")!;
    fireEvent.change(password, { target: { value: "fifteen-char8888" } });
    fireEvent.change(screen.getByLabelText("Confirm password"), { target: { value: "fifteen-char8888" } });
    await continueTo("Choose how MagicHandy reaches your device");
    await skipTo("Set up the chat AI");
    await skipTo("Add voice features");
    await skipTo("Installing selected features");
    await continueTo("Setup is ready");

    expect(screen.getByText("Sign-in required after setup")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Finish and sign in" }));

    await waitFor(() => expect(api.completeSetup).toHaveBeenCalledWith(true));
    expect(refreshAuthentication).toHaveBeenCalledOnce();
    expect(window.location.hash).toBe("#/chat");
  });

  it("saves the Handy model and connection owner from the Device step", async () => {
    render(<SetupRoute />);
    await startCustomSetup();
    await continueTo("Choose how MagicHandy reaches your device");

    expect(screen.getByText(/Handy Onboarding app/)).toBeVisible();
    expect(screen.getByRole("radio", { name: /^Original/ })).toBeChecked();
    fireEvent.click(screen.getByRole("radio", { name: /^2 Pro/ }));
    await continueTo("Set up the chat AI");

    expect(api.saveSetupPreferences).toHaveBeenCalledWith(expect.objectContaining({
      device_owner: "cloud_rest",
      handy_model: "handy_2_pro",
    }));
  });

  it("saves the Ollama engine choice when leaving the chat step", async () => {
    vi.mocked(api.ollamaModels).mockResolvedValue({ available: true, models: [{ name: "gemma3:4b", size_bytes: 4096 }] as never });
    await openChatStep();
    fireEvent.click(screen.getByRole("radio", { name: /use my existing ollama/i }));
    fireEvent.change(await screen.findByRole("combobox", { name: "Ollama model" }), { target: { value: "gemma3:4b" } });
    await continueTo("Add voice features");

    await waitFor(() => expect(api.saveSetupPreferences).toHaveBeenCalledWith(expect.objectContaining({
      llm: expect.objectContaining({ provider: "ollama", model: "gemma3:4b" }),
    })));
  });

  it("keeps managed llama.cpp and the tested download as the defaults", async () => {
    await openChatStep();

    expect(screen.getByRole("radio", { name: /Managed llama\.cpp.*Recommended/i })).toBeChecked();
    expect(screen.getByRole("radio", { name: /Gemma 4 12B fixture/ })).toBeChecked();
    expect(screen.getByText(/Detected Test NVIDIA GPU with 15\.9 GiB of graphics memory/)).toBeVisible();
    expect(screen.getByRole("link", { name: "License terms" })).toHaveAttribute("href", catalogModel.license_url);
    expect(screen.queryByRole("button", { name: /build managed|install managed/i })).not.toBeInTheDocument();
    expect(screen.getByRole("radio", { name: /Use my existing Ollama/i })).not.toBeChecked();
  });

  it("downloads the recommended model as part of the installation plan", async () => {
    await openChatStep();
    await continueTo("Add voice features");
    await continueTo("Installing selected features");

    expect(api.installSetupPlan).toHaveBeenCalledWith({
      llama: { backend: "auto" },
      model: { catalog_id: "gemma-fixture" },
      parakeet: false,
    });
  });

  it("keeps the engine install when the model is added later", async () => {
    await openChatStep();
    fireEvent.click(screen.getByRole("radio", { name: /Add a model later/ }));
    await continueTo("Add voice features");
    await continueTo("Installing selected features");

    expect(api.installSetupPlan).toHaveBeenCalledWith({ llama: { backend: "auto" }, parakeet: false });
  });

  it("explains why Continue is unavailable until a model is chosen", async () => {
    vi.mocked(api.llmModels).mockResolvedValue({ ...modelFixture, models: [managedModel] });
    await openChatStep();
    fireEvent.click(screen.getByRole("radio", { name: /Use a model already in MagicHandy/ }));

    expect(screen.getByRole("button", { name: "Continue" })).toBeDisabled();
    expect(screen.getByText("Choose a model to download or import, or pick Add a model later.")).toBeVisible();
    fireEvent.change(screen.getByRole("combobox", { name: "Managed model" }), { target: { value: managedModel.id } });
    expect(screen.getByRole("button", { name: "Continue" })).toBeEnabled();
  });

  it("keeps a saved external model name and uses an installed catalog model when switching to managed", async () => {
    settings = { ...settings, llm: { ...settings.llm, llama_cpp_mode: "external", model: "my-server-model" } };
    vi.mocked(useAppState).mockReturnValue({
      state: { settings },
      backendOnline: true,
      readOnly: false,
      refresh: vi.fn(async () => undefined),
    } as unknown as ReturnType<typeof useAppState>);
    vi.mocked(api.llmCatalog).mockResolvedValue({ ...catalogFixture("recommended"), models: [{ ...catalogModel, fit: "recommended", installed_model_id: "gemma-installed" }] });
    await openChatStep();

    expect(screen.getByRole("radio", { name: /External llama\.cpp server/ })).toBeChecked();
    await waitFor(() => expect(screen.getByRole("textbox", { name: "Model" })).toHaveValue("my-server-model"));

    fireEvent.click(screen.getByRole("radio", { name: /Managed llama\.cpp/ }));
    expect(screen.getByRole("radio", { name: /Gemma 4 12B fixture/ })).toBeChecked();
    await continueTo("Add voice features");
    expect(api.saveSetupPreferences).toHaveBeenLastCalledWith({
      llm: expect.objectContaining({ provider: "llama_cpp", llama_cpp_mode: "managed", model: "gemma-installed" }),
    });
    await continueTo("Installing selected features");
    expect(api.installSetupPlan).toHaveBeenCalledWith({ llama: { backend: "auto" }, parakeet: false });
  });

  it("starts with chat skipped when no NVIDIA card is available", async () => {
    vi.mocked(api.setupStatus).mockResolvedValue({ ...setupFixture, hardware: { platform: "windows/amd64", nvidia: false, cuda: false } });
    vi.mocked(api.llmCatalog).mockResolvedValue(catalogFixture("no_gpu", false));
    await openChatStep();

    await waitFor(() => expect(screen.getByRole("radio", { name: /Skip chat model setup/ })).toBeChecked());
    expect(screen.getByText(/No NVIDIA graphics card was found/)).toBeVisible();
    expect(screen.getByRole("radio", { name: /Managed llama\.cpp/ })).not.toHaveTextContent("Recommended");
  });

  it("imports a selected model from an existing Ollama library during managed setup", async () => {
    await openChatStep();
    fireEvent.click(screen.getByText("Import a model file instead"));

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

    await openChatStep();
    await waitFor(() => expect(screen.getByRole("radio", { name: /Use a model already in MagicHandy/ })).toBeChecked());
    await continueTo("Add voice features");
    await continueTo("Installing selected features");

    expect(api.installSetupPlan).toHaveBeenCalledWith({ llama: { backend: "auto" }, parakeet: false });
    expect(screen.getByRole("progressbar", { name: "Installation progress" })).toHaveAttribute("value", "0");
    expect(screen.getByRole("log", { name: "Installation terminal output" })).toBeInTheDocument();
    expect(screen.getByRole("log", { name: "Installation terminal output" })).toHaveTextContent("Waiting for installer output...");
  });

  it("asks the backend to turn chosen voice features on after installing them", async () => {
    await openChatStep();
    await skipTo("Add voice features");
    fireEvent.click(screen.getByRole("radio", { name: /Parakeet/i }));
    expect(screen.getByRole("checkbox", { name: /Turn voice on when installation finishes/ })).toBeChecked();
    await continueTo("Installing selected features");

    expect(api.installSetupPlan).toHaveBeenCalledWith({ parakeet: true, enable_voice: true });
  });

  it("preselects Parakeet when the backend finds a saved runtime or resumable partial", async () => {
    vi.mocked(api.setupStatus).mockResolvedValue({
      ...setupFixture,
      parakeet: { ...setupFixture.parakeet, preselected: true },
    });
    await openChatStep();
    await skipTo("Add voice features");

    expect(screen.getByRole("radio", { name: /Parakeet/i })).toBeChecked();
  });

  const chatterboxModule = {
    id: "chatterbox", name: "Chatterbox Turbo", provider: "chatterbox",
    summary: "Local voice cloning with CPU fallback.", license: "MIT", model: "ResembleAI/chatterbox-turbo",
    model_license: "MIT", python_version: "3.10", disk_estimate: "Several GiB",
    supported_devices: ["cpu", "cuda"], recommended_for_nvidia: false, ready_after_install: true,
    reference_requirement: "The included voice works immediately.", source_url: "https://example.invalid",
    source_revision: "fixture", port: 8992,
  };

  function easyVoiceSetup(insufficient = false): SetupStatus {
    const memory = { status: insufficient ? "insufficient" as const : "fits" as const, llm_known: true, llm_vram_mib: 8400, voice_vram_mib: 4096, reserve_mib: 1024, required_mib: 13520, available_mib: insufficient ? 12288 : 16384 };
    return {
      ...setupFixture,
      hardware: { ...setupFixture.hardware, vram_mib: String(memory.available_mib) },
      voice_modules: [...setupFixture.voice_modules, chatterboxModule],
      assessment: {
        free_disk_bytes: 200 * 2 ** 30,
        chat: { status: "met", reason: "gpu_fits", bytes: 8 * 2 ** 30, vram_mib: 8400, min_vram_mib: 10240 },
        voice_output: { status: "met", reason: insufficient ? "gpu" : "qwen_gpu", bytes: (insufficient ? 6 : 8) * 2 ** 30 },
        voice_input: { status: "met", reason: "cpu", bytes: 800 * 2 ** 20 },
        model_id: catalogModel.id, runtime_backend: "cuda", voice_module: insufficient ? "chatterbox" : "faster-qwen3-tts", voice_device: "cuda",
        voice_options: [
          { module: "faster-qwen3-tts", device: "cuda", requirement: { status: insufficient ? "partial" : "met", reason: insufficient ? "qwen_insufficient" : "qwen_gpu", bytes: 8 * 2 ** 30 }, memory },
          { module: "chatterbox", device: "cuda", requirement: { status: "met", reason: "gpu", bytes: 6 * 2 ** 30 }, memory: { ...memory, status: "fits", voice_vram_mib: 2048, required_mib: 11472 } },
        ],
      },
    };
  }

  it("Easy setup prefers Qwen when the backend says the combination fits, and explains reference setup", async () => {
    vi.mocked(api.setupStatus).mockResolvedValue(easyVoiceSetup());
    render(<SetupRoute />);
    await screen.findByRole("heading", { name: "Set up MagicHandy" });
    await continueTo("Easy setup");
    fireEvent.click(screen.getByRole("checkbox", { name: /Speak replies aloud/ }));
    expect(screen.getByRole("radio", { name: /Faster Qwen3-TTS/ })).toBeChecked();
    expect(screen.getByText(/Qwen3-TTS requires an audio sample and its exact transcript/)).toBeVisible();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Install and continue" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Set up voice later" }));
    fireEvent.click(screen.getByRole("button", { name: "Install and continue" }));
    await screen.findByRole("heading", { name: "Installing selected features" });
    expect(api.installSetupPlan).toHaveBeenCalledWith({ llama: { backend: "auto" }, model: { catalog_id: catalogModel.id }, voice: { module: "faster-qwen3-tts", device: "cuda", auto_launch: true }, parakeet: false });
  });

  it("Easy setup warns about the combined budget but allows forcing Qwen installation", async () => {
    vi.mocked(api.setupStatus).mockResolvedValue(easyVoiceSetup(true));
    render(<SetupRoute />);
    await screen.findByRole("heading", { name: "Set up MagicHandy" });
    await continueTo("Easy setup");
    fireEvent.click(screen.getByRole("checkbox", { name: /Speak replies aloud/ }));
    expect(screen.getByRole("radio", { name: /Chatterbox Turbo/ })).toBeChecked();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("radio", { name: /Faster Qwen3-TTS/ }));
    expect(screen.getByRole("alert")).toHaveTextContent(/You can still install this voice/);
    fireEvent.click(screen.getByRole("button", { name: "Set up voice later" }));
    expect(screen.getByRole("button", { name: "Install and continue" })).toBeEnabled();
    fireEvent.click(screen.getByRole("button", { name: "Install and continue" }));
    await screen.findByRole("heading", { name: "Installing selected features" });
    expect(api.installSetupPlan).toHaveBeenCalledWith(expect.objectContaining({ voice: { module: "faster-qwen3-tts", device: "cuda", auto_launch: true } }));
  });

  it("Easy setup submits the backend's CPU fallback and clears the GPU warning when voice is off", async () => {
    const setup = easyVoiceSetup(true);
    const option = setup.assessment!.voice_options![1];
    option.device = "cpu";
    option.requirement = { ...option.requirement, status: "partial", reason: "cpu" };
    option.memory = { ...option.memory, status: "fits", voice_vram_mib: 0, required_mib: 10240 };
    setup.assessment!.voice_device = "cpu";
    vi.mocked(api.setupStatus).mockResolvedValue(setup);
    render(<SetupRoute />);
    await screen.findByRole("heading", { name: "Set up MagicHandy" });
    await continueTo("Easy setup");
    fireEvent.click(screen.getByRole("checkbox", { name: /Speak replies aloud/ }));
    expect(screen.getByText(/Voice will run on the CPU/)).toBeVisible();
    fireEvent.click(screen.getByRole("radio", { name: /Faster Qwen3-TTS/ }));
    expect(screen.getByRole("alert")).toBeVisible();
    fireEvent.click(screen.getByRole("checkbox", { name: /Speak replies aloud/ }));
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("checkbox", { name: /Speak replies aloud/ }));
    fireEvent.click(screen.getByRole("button", { name: "Install and continue" }));
    await screen.findByRole("heading", { name: "Installing selected features" });
    expect(api.installSetupPlan).toHaveBeenCalledWith(expect.objectContaining({ voice: { module: "chatterbox", device: "cpu", auto_launch: true }, enable_voice: true }));
  });

  it("Easy setup labels unknown memory and still permits Qwen, but enforces its NVIDIA requirement", async () => {
    const setup = easyVoiceSetup();
    const qwen = setup.assessment!.voice_options![0];
    qwen.memory = { ...qwen.memory, status: "unknown", llm_known: false };
    qwen.requirement = { ...qwen.requirement, status: "partial", reason: "qwen_unknown" };
    vi.mocked(api.setupStatus).mockResolvedValue(setup);
    const view = render(<SetupRoute />);
    await screen.findByRole("heading", { name: "Set up MagicHandy" });
    await continueTo("Easy setup");
    fireEvent.click(screen.getByRole("checkbox", { name: /Speak replies aloud/ }));
    expect(screen.getByRole("alert")).toHaveTextContent(/memory use is unknown/);
    fireEvent.click(screen.getByRole("button", { name: "Set up voice later" }));
    expect(screen.getByRole("button", { name: "Install and continue" })).toBeEnabled();
    view.unmount();

    setup.hardware.nvidia = false;
    setup.assessment!.voice_module = "chatterbox";
    qwen.requirement = { status: "unmet", reason: "no_nvidia", bytes: 0 };
    render(<SetupRoute />);
    await screen.findByRole("heading", { name: "Set up MagicHandy" });
    await continueTo("Easy setup");
    fireEvent.click(screen.getByRole("checkbox", { name: /Speak replies aloud/ }));
    expect(screen.getByRole("radio", { name: /Faster Qwen3-TTS/ })).toBeDisabled();
    expect(screen.getByRole("radio", { name: /Chatterbox Turbo/ })).toBeChecked();
  });

  it("guides Qwen sample and exact transcript into the install plan without changing the current voice", async () => {
    vi.mocked(api.setupStatus).mockResolvedValue(easyVoiceSetup());
    vi.spyOn(api, "checkSetupQwenReference").mockResolvedValue({ duration_ms: 3000 });
    render(<SetupRoute />);
    await screen.findByRole("heading", { name: "Set up MagicHandy" });
    await continueTo("Easy setup");
    fireEvent.click(screen.getByRole("checkbox", { name: /Speak replies aloud/ }));
    expect(screen.getByRole("button", { name: "Install and continue" })).toBeDisabled();
    fireEvent.change(screen.getByRole("textbox", { name: "Voice sample (WAV)" }), { target: { value: "C:\\voice\\sample.wav" } });
    await screen.findByText(/Sample checked: 3.0 seconds/);
    expect(screen.getByRole("button", { name: "Install and continue" })).toBeDisabled();
    fireEvent.change(screen.getByRole("textbox", { name: "Exact words spoken in the sample" }), { target: { value: "  Hello, this is my voice.  " } });
    expect(screen.getByRole("button", { name: "Install and continue" })).toBeEnabled();
    fireEvent.click(screen.getByRole("button", { name: "Install and continue" }));
    await screen.findByRole("heading", { name: "Installing selected features" });
    expect(api.installSetupPlan).toHaveBeenCalledWith(expect.objectContaining({
      voice: { module: "faster-qwen3-tts", device: "cuda", auto_launch: true, reference: { wav: "C:\\voice\\sample.wav", transcript: "Hello, this is my voice." } }, enable_voice: true,
    }));
    expect(api.saveSetupPreferences).not.toHaveBeenCalledWith(expect.objectContaining({ voice: expect.anything() }));
  });

  it("keeps a saved Qwen sample and transcript when setup is run again", async () => {
    settings.voice = { ...settings.voice, tts_reference_wav: "C:\\voice\\saved.wav", tts_reference_text: "My saved words." };
    vi.mocked(api.setupStatus).mockResolvedValue(easyVoiceSetup());
    vi.spyOn(api, "checkSetupQwenReference").mockResolvedValue({ duration_ms: 4000 });
    render(<SetupRoute />);
    await screen.findByRole("heading", { name: "Set up MagicHandy" });
    await continueTo("Easy setup");
    fireEvent.click(screen.getByRole("checkbox", { name: /Speak replies aloud/ }));
    expect(screen.getByRole("textbox", { name: "Voice sample (WAV)" })).toHaveValue("C:\\voice\\saved.wav");
    expect(screen.getByRole("textbox", { name: "Exact words spoken in the sample" })).toHaveValue("My saved words.");
    fireEvent.click(screen.getByRole("button", { name: "Set up voice later" }));
    fireEvent.click(screen.getByRole("button", { name: "Configure voice now" }));
    expect(screen.getByRole("textbox", { name: "Exact words spoken in the sample" })).toHaveValue("My saved words.");
  });

  it("Easy setup installs the model it picked and the voice features chosen", async () => {
    vi.mocked(api.setupStatus).mockResolvedValue({
      ...setupFixture,
      voice_modules: [...setupFixture.voice_modules, chatterboxModule],
      assessment: {
        free_disk_bytes: 200 * 2 ** 30,
        chat: { status: "met", reason: "gpu_fits", bytes: 8 * 2 ** 30, vram_mib: 8144, min_vram_mib: 10240 },
        voice_output: { status: "met", reason: "gpu", bytes: 6 * 2 ** 30 },
        voice_input: { status: "met", reason: "cpu", bytes: 800 * 2 ** 20 },
        model_id: catalogModel.id, runtime_backend: "cuda", voice_module: "chatterbox", voice_device: "cuda",
      },
    });
    render(<SetupRoute />);
    await screen.findByRole("heading", { name: "Set up MagicHandy" });
    expect(screen.getByRole("radio", { name: /Easy setup/ })).toBeChecked();
    await continueTo("Easy setup");

    expect(screen.getByText("Chat: Gemma 4 12B fixture")).toBeVisible();
    expect(screen.getAllByText("Requirements met")).toHaveLength(1);
    fireEvent.click(screen.getByRole("radio", { name: /Explicit/ }));
    fireEvent.click(screen.getByRole("checkbox", { name: /Speak replies aloud/ }));
    fireEvent.click(screen.getByRole("checkbox", { name: /Talk instead of typing/ }));
    expect(screen.getAllByText("Requirements met")).toHaveLength(3);
    fireEvent.click(screen.getByRole("button", { name: "Install and continue" }));
    await screen.findByRole("heading", { name: "Installing selected features" });

    expect(api.saveSetupPreferences).toHaveBeenCalledWith({
      llm: expect.objectContaining({ chat_voice: "explicit", provider: "llama_cpp", llama_cpp_mode: "managed" }),
    });
    expect(api.installSetupPlan).toHaveBeenCalledWith({
      llama: { backend: "auto" },
      model: { catalog_id: catalogModel.id },
      voice: { module: "chatterbox", device: "cuda", auto_launch: true },
      parakeet: true,
      enable_voice: true,
    });
  });

  it("Easy setup says which requirements a computer without an NVIDIA card misses", async () => {
    vi.mocked(api.setupStatus).mockResolvedValue({
      ...setupFixture,
      hardware: { platform: "windows/amd64", nvidia: false, cuda: false },
      assessment: {
        free_disk_bytes: 200 * 2 ** 30,
        chat: { status: "unmet", reason: "no_nvidia", bytes: 0 },
        voice_output: { status: "partial", reason: "cpu", bytes: 6 * 2 ** 30 },
        voice_input: { status: "met", reason: "cpu", bytes: 800 * 2 ** 20 },
        runtime_backend: "cpu", voice_module: "chatterbox", voice_device: "cpu",
      },
    });
    vi.mocked(api.llmCatalog).mockResolvedValue(catalogFixture("no_gpu", false));
    render(<SetupRoute />);
    await screen.findByRole("heading", { name: "Set up MagicHandy" });
    await continueTo("Easy setup");

    expect(screen.getByText("Local chat needs an NVIDIA graphics card")).toBeVisible();
    expect(screen.getByText("Requirements not met")).toBeVisible();
    expect(screen.getByText("Requirements partly met")).toBeVisible();
    expect(screen.queryByRole("heading", { name: "How explicit should chat be?" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    await screen.findByRole("heading", { name: "Setup is ready" });
    expect(api.installSetupPlan).not.toHaveBeenCalled();
  });
});
