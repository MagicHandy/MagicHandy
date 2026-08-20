import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { LLMModelManagerSnapshot, LLMProviderStatus, PublicSettings } from "../api/types";
import { ModelSettingsPanel } from "./ModelSettingsPanel";
import { VoiceSettingsPanel } from "./VoiceSettingsPanel";

const app = vi.hoisted(() => ({ show: vi.fn() }));

vi.mock("../api/client", () => ({
  api: {
    llmModels: vi.fn(),
    llmStatus: vi.fn(),
    ollamaModels: vi.fn(),
    voiceStatus: vi.fn(),
  },
}));

vi.mock("../state/app-state", () => ({
  useToast: () => ({ show: app.show }),
}));

vi.mock("./VoiceWorkers", () => ({
  VoiceWorkers: ({ role }: { role: string }) => <div>{role} worker readout</div>,
}));

vi.mock("./VoiceRequestQueue", () => ({
  VoiceRequestQueue: () => <div>Voice request queue</div>,
}));

const llmModels = vi.mocked(api.llmModels);
const llmStatus = vi.mocked(api.llmStatus);
const ollamaModels = vi.mocked(api.ollamaModels);
const voiceStatus = vi.mocked(api.voiceStatus);

const llmSettings = {
  provider: "llama_cpp",
  llama_cpp_mode: "managed",
  llama_cpp_base_url: "",
  llama_cpp_context_size: 32768,
  ollama_base_url: "",
  ollama_models_path: "",
  model: "",
  prompt_set: "default",
  request_timeout_ms: 120000,
  max_output_tokens: 256,
  reasoning_mode: "off",
  motion_generation_mode: "pattern",
} as PublicSettings["llm"];

const emptyManager = {
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
    expected_version: "test",
    message: "Managed runtime is not installed.",
  },
} as LLMModelManagerSnapshot;

const providerStatus = {
  provider: "llama_cpp",
  base_url: "",
  model: "",
  available: false,
  loaded: false,
  message: "Runtime is not loaded.",
} as LLMProviderStatus;

describe("runtime panels", () => {
  beforeEach(() => {
    app.show.mockReset();
    llmModels.mockReset();
    llmStatus.mockReset();
    ollamaModels.mockReset();
    voiceStatus.mockReset();
    llmStatus.mockResolvedValue(providerStatus);
  });

  it("distinguishes a pending model list from a valid empty model store", async () => {
    let release!: (value: LLMModelManagerSnapshot) => void;
    llmModels.mockImplementation(() => new Promise((resolve) => { release = resolve; }));
    renderModelPanel();

    expect(await screen.findByText("Loading model list...")).toBeInTheDocument();
    expect(screen.queryByText("No managed models.")).not.toBeInTheDocument();

    await act(async () => release(emptyManager));
    expect(await screen.findByText("No managed models.")).toBeInTheDocument();
  });

  it("does not misreport a failed model-list request as an empty store", async () => {
    llmModels.mockRejectedValue(new Error("model catalog unavailable"));
    renderModelPanel();

    expect(await screen.findByRole("alert")).toHaveTextContent("model catalog unavailable");
    expect(screen.queryByText("No managed models.")).not.toBeInTheDocument();
  });

  it("does not render an empty Ollama list when the daemon request failed", async () => {
    llmModels.mockResolvedValue(emptyManager);
    ollamaModels.mockRejectedValue(new Error("Ollama daemon unavailable"));
    renderModelPanel({ ...llmSettings, provider: "ollama" });

    expect(await screen.findByRole("alert")).toHaveTextContent("Ollama daemon unavailable");
    expect(screen.queryByText("No models reported by Ollama.")).not.toBeInTheDocument();
  });

  it("persists model motion capability choices as one complete gate set", async () => {
    llmModels.mockResolvedValue(emptyManager);
    const patch = vi.fn();
    const user = userEvent.setup();
    renderModelPanel({
      ...llmSettings,
      motion_capabilities: { motion: true, patterns: true, area_focus: true, experimental_patterns: false },
    }, patch);

    await user.click(screen.getByRole("checkbox", { name: "Experimental patterns" }));

    expect(patch).toHaveBeenCalledWith({
      motion_capabilities: { motion: true, patterns: true, area_focus: true, experimental_patterns: true },
    });
  });

  it("uses the mode list for chat-only motion and hides pattern-only gates", async () => {
    llmModels.mockResolvedValue(emptyManager);
    renderModelPanel({
      ...llmSettings,
      motion_generation_mode: "off",
      motion_capabilities: { motion: false, patterns: true, area_focus: true, experimental_patterns: true },
    });

    expect(await screen.findByText("No managed models.")).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "LLM motion" })).toHaveValue("off");
    expect(screen.queryByRole("checkbox", { name: "Area focus" })).not.toBeInTheDocument();
    expect(screen.queryByRole("checkbox", { name: "Experimental patterns" })).not.toBeInTheDocument();
  });

  it("keeps the selected mode and legacy motion master gate consistent", async () => {
    llmModels.mockResolvedValue(emptyManager);
    const patch = vi.fn();
    renderModelPanel({
      ...llmSettings,
      motion_capabilities: { motion: true, patterns: true, area_focus: true, experimental_patterns: false },
    }, patch);

    await screen.findByText("No managed models.");
    fireEvent.change(screen.getByRole("combobox", { name: "LLM motion" }), { target: { value: "off" } });

    expect(patch).toHaveBeenCalledWith({
      motion_generation_mode: "off",
      motion_capabilities: { motion: false, patterns: true, area_focus: true, experimental_patterns: false },
    });
  });

  it("keeps a large Ollama inventory collapsed until requested", async () => {
    llmModels.mockResolvedValue(emptyManager);
    ollamaModels.mockResolvedValue({
      available: true,
      models: [
        { name: "small:latest", size_bytes: 1024, parameter_size: "3B", quantization: "Q4_K_M" },
        { name: "large:latest", size_bytes: 2048, parameter_size: "8B", quantization: "Q4_K_M" },
      ],
    });
    const user = userEvent.setup();
    const { container } = renderModelPanel({ ...llmSettings, provider: "ollama", model: "small:latest" });

    await screen.findByText("Installed Ollama models");
    const disclosure = container.querySelector<HTMLDetailsElement>(".ollama-daemon-disclosure");
    expect(disclosure).not.toBeNull();
    expect(disclosure).not.toHaveAttribute("open");
    await user.click(screen.getByText("Installed Ollama models"));
    expect(disclosure).toHaveAttribute("open");
  });

  it("shows only managed context sizes and preserves the current saved option", async () => {
    llmModels.mockResolvedValue(emptyManager);
    const patch = vi.fn();
    const user = userEvent.setup();
    const managed = { ...llmSettings, llama_cpp_context_size: 65536 };
    const view = renderModelPanel(managed, patch, [16384, 32768, 131072]);

    const select = await screen.findByRole("combobox", { name: "Context size" });
    expect(select).toHaveValue("65536");
    expect(screen.getByRole("option", { name: "65536 tokens" })).toBeInTheDocument();
    expect(screen.getByText(/A context smaller than the prompt cannot fit the request/)).toBeInTheDocument();
    await user.selectOptions(select, "131072");
    expect(patch).toHaveBeenCalledWith({ llama_cpp_context_size: 131072 });

    view.rerender(modelPanel({ ...managed, llama_cpp_mode: "external" }, patch));
    expect(screen.queryByRole("combobox", { name: "Context size" })).not.toBeInTheDocument();
  });

  it("makes managed startup loading an explicit latency versus memory choice", async () => {
    llmModels.mockResolvedValue(emptyManager);
    const patch = vi.fn();
    const user = userEvent.setup();
    renderModelPanel({ ...llmSettings, managed_load_policy: "on_demand" }, patch);

    const policy = await screen.findByRole("combobox", { name: "Model loading" });
    expect(policy).toHaveValue("on_demand");
    expect(screen.getByText(/first request must wait for the model to load/i)).toBeInTheDocument();
    await user.selectOptions(policy, "startup");
    expect(patch).toHaveBeenCalledWith({ managed_load_policy: "startup" });
  });

  it("refreshes provider status after the saved managed context changes", async () => {
    llmModels.mockResolvedValue(emptyManager);
    const initial = { ...llmSettings, llama_cpp_context_size: 32768 };
    const view = render(modelPanel(initial));
    await waitFor(() => expect(llmStatus).toHaveBeenCalled());
    llmStatus.mockClear();

    view.rerender(modelPanel({ ...initial, llama_cpp_context_size: 65536 }));

    await waitFor(() => expect(llmStatus).toHaveBeenCalledOnce());
  });

  it("keeps polling while a managed llama.cpp model is loading", async () => {
    llmModels.mockResolvedValue(emptyManager);
    llmStatus
      .mockResolvedValueOnce({
        provider: "llama_cpp",
        base_url: "http://127.0.0.1:8080",
        model: "local-model",
        available: false,
        managed: true,
        loaded: true,
        loading: true,
        message: "llama.cpp is loading the model",
      })
      .mockResolvedValue({
        provider: "llama_cpp",
        base_url: "http://127.0.0.1:8080",
        model: "local-model",
        available: true,
        model_available: true,
        managed: true,
        loaded: true,
        message: "ready",
      });

    renderModelPanel();

    const loadingMessage = await screen.findByText("llama.cpp is loading the model");
    expect(loadingMessage.closest('[role="status"]')).toHaveAttribute("aria-busy", "true");
    await waitFor(() => expect(screen.getByText("ready")).toBeInTheDocument(), { timeout: 2500 });
    expect(llmStatus).toHaveBeenCalledTimes(2);
  });

  it("names speech providers distinctly and surfaces voice-status failures", async () => {
    voiceStatus.mockRejectedValue(new Error("voice endpoint unavailable"));
    render(
      <VoiceSettingsPanel
        settings={voiceSettings()}
        locked={false}
        dirty={false}
        patch={vi.fn()}
        newKey=""
        setNewKey={vi.fn()}
        clearKey={false}
        setClearKey={vi.fn()}
      />,
    );

    expect(screen.getByRole("combobox", { name: "Speech input provider" })).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Speech output provider" })).toBeInTheDocument();
    expect(await screen.findByRole("alert")).toHaveTextContent("voice endpoint unavailable");
  });

  it("does not present unsupported CPU execution for Faster Qwen", async () => {
    voiceStatus.mockImplementation(() => new Promise(() => undefined));
    const settings = voiceSettings();
    settings.voice = {
      ...settings.voice,
      tts_provider: "faster_qwen3_tts",
      tts_device: "cuda",
      tts_model: "Qwen/Qwen3-TTS-12Hz-0.6B-Base",
    };
    render(
      <VoiceSettingsPanel
        settings={settings}
        locked={false}
        dirty={false}
        patch={vi.fn()}
        newKey=""
        setNewKey={vi.fn()}
        clearKey={false}
        setClearKey={vi.fn()}
      />,
    );

    expect(screen.getByText("Faster Qwen3-TTS requires an NVIDIA GPU with CUDA.")).toBeInTheDocument();
    const device = screen.getByRole("combobox", { name: "Device" });
    expect(device).toHaveTextContent("NVIDIA CUDA");
    expect(device).not.toHaveTextContent("CPU");
    await userEvent.click(screen.getByText("Advanced"));
    const responseFormat = screen.getByRole("combobox", { name: "Response format" });
    expect(responseFormat).toHaveTextContent("WAV");
    expect(responseFormat).not.toHaveTextContent("MP3");
  });

  it("offers fixed and varied Faster Qwen seed controls", async () => {
    voiceStatus.mockImplementation(() => new Promise(() => undefined));
    const settings = voiceSettings();
    settings.voice = {
      ...settings.voice,
      tts_provider: "faster_qwen3_tts",
      tts_device: "cuda",
      tts_seed: 1337,
      tts_seed_mode: "fixed",
    };
    const patch = vi.fn();
    const user = userEvent.setup();
    render(
      <VoiceSettingsPanel
        settings={settings}
        locked={false}
        dirty={false}
        patch={patch}
        newKey=""
        setNewKey={vi.fn()}
        clearKey={false}
        setClearKey={vi.fn()}
      />,
    );

    await user.click(screen.getByText("Advanced"));
    const repeatable = screen.getByRole("checkbox", { name: "Repeatable voice generation" });
    expect(repeatable).toBeChecked();
    expect(screen.getByRole("spinbutton", { name: "Generation seed" })).toHaveValue(1337);
    await user.click(repeatable);
    expect(patch).toHaveBeenCalledWith({ tts_seed_mode: "varied" });
    await user.click(screen.getByRole("button", { name: "New seed" }));
    const randomSeedPatch = patch.mock.calls[patch.mock.calls.length - 1]?.[0];
    expect(randomSeedPatch.tts_seed_mode).toBe("fixed");
    expect(randomSeedPatch.tts_seed).toEqual(expect.any(Number));
  });

  it("offers an explicit chat speech interruption policy", async () => {
    voiceStatus.mockImplementation(() => new Promise(() => undefined));
    const settings = voiceSettings();
    settings.voice = {
      ...settings.voice,
      tts_provider: "faster_qwen3_tts",
      speak_replies: true,
      chat_speech_policy: "interrupt",
    };
    settings.options.chat_speech_policies = ["interrupt", "finish_current"];
    const patch = vi.fn();
    const user = userEvent.setup();
    render(
      <VoiceSettingsPanel
        settings={settings}
        locked={false}
        dirty={false}
        patch={patch}
        newKey=""
        setNewKey={vi.fn()}
        clearKey={false}
        setClearKey={vi.fn()}
      />,
    );

    const policy = screen.getByRole("combobox", { name: "When a new message is sent" });
    expect(policy).toHaveValue("interrupt");
    expect(screen.getByText(/frees a shared local GPU sooner/i)).toBeInTheDocument();
    await user.selectOptions(policy, "finish_current");
    expect(patch).toHaveBeenCalledWith({ chat_speech_policy: "finish_current" });
  });

  it("offers reviewed Faster Qwen tone presets and reveals the custom prompt", async () => {
    voiceStatus.mockImplementation(() => new Promise(() => undefined));
    const settings = voiceSettings();
    settings.voice = {
      ...settings.voice,
      tts_provider: "faster_qwen3_tts",
      tts_device: "cuda",
      tts_tone_preset: "natural",
      tts_tone_prompt: "",
    };
    settings.options.tts_tone_presets = [
      "natural",
      "warm",
      "playful",
      "tender",
      "commanding",
      "excited",
      "custom",
    ];
    const patch = vi.fn();
    const user = userEvent.setup();
    const view = render(
      <VoiceSettingsPanel
        settings={settings}
        locked={false}
        dirty={false}
        patch={patch}
        newKey=""
        setNewKey={vi.fn()}
        clearKey={false}
        setClearKey={vi.fn()}
      />,
    );

    const tone = screen.getByRole("combobox", { name: "Voice tone" });
    expect(tone).toHaveValue("natural");
    expect(tone).toHaveTextContent("Warm and intimate");
    expect(tone).toHaveTextContent("Playful and teasing");
    expect(screen.queryByRole("textbox", { name: "Custom tone prompt" })).not.toBeInTheDocument();

    await user.selectOptions(tone, "custom");
    expect(patch).toHaveBeenCalledWith({ tts_tone_preset: "custom" });

    settings.voice = { ...settings.voice, tts_tone_preset: "custom" };
    view.rerender(
      <VoiceSettingsPanel
        settings={settings}
        locked={false}
        dirty={true}
        patch={patch}
        newKey=""
        setNewKey={vi.fn()}
        clearKey={false}
        setClearKey={vi.fn()}
      />,
    );
    const customPrompt = screen.getByRole("textbox", { name: "Custom tone prompt" });
    fireEvent.change(customPrompt, { target: { value: "Speak with quiet anticipation." } });
    expect(patch).toHaveBeenLastCalledWith({ tts_tone_prompt: "Speak with quiet anticipation." });
  });
});

function modelPanel(settings: PublicSettings["llm"] = llmSettings, patch = vi.fn(), contextSizes = [16384, 32768, 65536, 131072]) {
  return (
    <ModelSettingsPanel
      settings={settings}
      saved={settings}
      providers={["llama_cpp", "ollama"]}
      llamaModes={["managed", "external"]}
      managedLoadPolicies={["startup", "on_demand"]}
      llamaContextSizes={contextSizes}
      reasoningModes={["off", "auto"]}
      maxOutputOptions={[128, 256, 512]}
      locked={false}
      patch={patch}
    />
  );
}

function renderModelPanel(settings: PublicSettings["llm"] = llmSettings, patch = vi.fn(), contextSizes = [16384, 32768, 65536, 131072]) {
  return render(modelPanel(settings, patch, contextSizes));
}

function voiceSettings(): PublicSettings {
  return {
    voice: {
      enabled: false,
      asr_provider: "none",
      tts_provider: "none",
      asr_worker_path: "",
      asr_worker_args: [],
      tts_worker_path: "",
      tts_worker_args: [],
      speak_replies: false,
      tts_tone_preset: "natural",
      tts_tone_prompt: "",
    },
    options: {
      asr_providers: ["none", "parakeet_managed"],
      tts_providers: ["none", "faster_qwen3_tts", "chatterbox_tts", "openai_compatible"],
      parakeet_sources: ["app_managed"],
      tts_devices: ["auto", "cuda", "cpu"],
      tts_tone_presets: ["natural", "warm", "playful", "tender", "commanding", "excited", "custom"],
    },
  } as unknown as PublicSettings;
}
