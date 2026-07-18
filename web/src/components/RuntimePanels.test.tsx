import { act, render, screen } from "@testing-library/react";
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
  ollama_base_url: "",
  ollama_models_path: "",
  model: "",
  prompt_set: "default",
  request_timeout_ms: 120000,
  max_output_tokens: 256,
  reasoning_mode: "off",
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
});

function renderModelPanel(settings: PublicSettings["llm"] = llmSettings) {
  return render(
    <ModelSettingsPanel
      settings={settings}
      saved={settings}
      providers={["llama_cpp", "ollama"]}
      llamaModes={["managed", "external"]}
      reasoningModes={["off", "auto"]}
      maxOutputOptions={[128, 256, 512]}
      locked={false}
      patch={vi.fn()}
    />,
  );
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
    },
    options: {
      asr_providers: ["none", "parakeet_managed"],
      tts_providers: ["none", "neutts_air"],
      parakeet_sources: ["app_managed"],
      neutts_sampling_modes: ["fixed", "random"],
    },
  } as unknown as PublicSettings;
}
