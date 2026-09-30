import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { LLMCatalog } from "../api/catalog-types";
import type { LLMModelManagerSnapshot, LLMProviderStatus, PublicSettings } from "../api/types";
import { ModelSettingsPanel } from "./ModelSettingsPanel";

const app = vi.hoisted(() => ({ show: vi.fn() }));

vi.mock("../api/client", () => ({
  api: {
    llmModels: vi.fn(),
    llmStatus: vi.fn(),
    llmCatalog: vi.fn(),
    downloadCatalogModel: vi.fn(),
    ollamaModels: vi.fn(),
  },
}));

vi.mock("../state/app-state", () => ({
  useToast: () => ({ show: app.show }),
}));

const settings = {
  provider: "llama_cpp",
  llama_cpp_mode: "managed",
  llama_cpp_base_url: "",
  llama_cpp_context_size: 16384,
  ollama_base_url: "",
  ollama_models_path: "",
  model: "",
  prompt_set: "default",
  request_timeout_ms: 120000,
  max_output_tokens: 256,
  reasoning_mode: "off",
  motion_generation_mode: "creative_v2",
} as PublicSettings["llm"];

const manager = {
  models: [],
  imports: [],
  store_path: "C:\\MagicHandy\\models",
  suggested_ollama_path: "",
  runtime: {
    state: "ready",
    installed: true,
    current: true,
    build_supported: true,
    supported_backends: ["auto", "cpu", "cuda"],
    expected_version: "test",
    message: "Managed runtime is ready.",
  },
} as LLMModelManagerSnapshot;

const baseModel = {
  summary: "The tested default: reliable motion commands and replies in about two seconds.",
  family: "gemma4",
  parameter_size: "11.9B",
  quantization: "Q4_K_M",
  size_bytes: 7381381696,
  license: "Apache-2.0 (Gemma 4)",
  license_url: "https://ai.google.dev/gemma/docs/gemma_4_license",
  source_name: "fixture/gemma:latest",
  source_url: "https://ollama.com/fixture/gemma",
  download_url: "https://registry.ollama.ai/v2/fixture/gemma/blobs/sha256:" + "c".repeat(64),
  sha256: "c".repeat(64),
  vram_mib: 8144,
  min_vram_mib: 10240,
};

function catalog(overrides: Partial<LLMCatalog["models"][number]> = {}): LLMCatalog {
  return {
    models: [{ ...baseModel, id: "gemma-fixture", display_name: "Gemma 4 12B fixture", default: true, fit: "recommended", ...overrides }],
    hardware: { nvidia: true, gpu_name: "Test GPU", vram_mib: 16303 },
  };
}

function renderPanel() {
  return render(<ModelSettingsPanel
    settings={settings}
    saved={settings}
    providers={["llama_cpp", "ollama"]}
    llamaModes={["managed", "external"]}
    managedLoadPolicies={["startup", "on_demand"]}
    llamaContextSizes={[16384, 32768]}
    reasoningModes={["off", "auto"]}
    maxOutputOptions={[128, 256, 512]}
    locked={false}
    patch={vi.fn()}
  />);
}

describe("model catalog downloads", () => {
  beforeEach(() => {
    app.show.mockReset();
    vi.mocked(api.llmModels).mockResolvedValue(manager);
    vi.mocked(api.llmStatus).mockResolvedValue({ provider: "llama_cpp", base_url: "", model: "", available: false, loaded: false, message: "Runtime is not loaded." } as LLMProviderStatus);
    vi.mocked(api.llmCatalog).mockResolvedValue(catalog());
    vi.mocked(api.downloadCatalogModel).mockReset();
  });

  it("offers the tested model with its fit, license and source before downloading", async () => {
    vi.mocked(api.downloadCatalogModel).mockResolvedValue({ import: {
      id: "download-1", source: "ollama", display_name: "Gemma 4 12B fixture", status: "queued",
      bytes_copied: 0, total_bytes: 7381381696, started_at: "", updated_at: "",
    } });
    renderPanel();

    const section = await screen.findByRole("region", { name: "Download a tested model" });
    expect(section).toHaveTextContent("Recommended");
    expect(section).toHaveTextContent("6.87 GiB download.");
    expect(section).toHaveTextContent("Uses about 7.95 GiB of the 15.9 GiB on your Test GPU.");
    expect(screen.getByRole("link", { name: "License terms" })).toHaveAttribute("href", baseModel.license_url);
    expect(screen.getByRole("link", { name: "Model page" })).toHaveAttribute("href", baseModel.source_url);

    fireEvent.click(screen.getByRole("button", { name: "Download" }));
    await waitFor(() => expect(api.downloadCatalogModel).toHaveBeenCalledWith("gemma-fixture"));
    expect(app.show).toHaveBeenCalledWith("Download started. The model is verified before it enters the store.");
  });

  it("labels installed and partially downloaded models instead of offering a fresh download", async () => {
    vi.mocked(api.llmCatalog).mockResolvedValue({
      ...catalog(),
      models: [
        { ...catalog().models[0], installed_model_id: "gemma-installed" },
        { ...baseModel, id: "alternative", display_name: "Alternative fixture", default: false, fit: "supported", partial_bytes: 1 << 30 },
      ],
    });
    renderPanel();

    expect(await screen.findByText("In your store")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Resume download" })).toBeEnabled();
    expect(screen.getByText(/1.00 GiB of 6.87 GiB already downloaded; the rest resumes./)).toBeInTheDocument();
  });

  it("folds generation tuning away and explains the selected motion mode", async () => {
    renderPanel();

    const advanced = (await screen.findByText("Advanced generation settings")).closest("details")!;
    expect(advanced).not.toHaveAttribute("open");
    expect(advanced).toContainElement(screen.getByRole("combobox", { name: "Context size" }));
    expect(advanced).toContainElement(screen.getByRole("combobox", { name: "Maximum output" }));
    expect(advanced).not.toContainElement(screen.getByRole("combobox", { name: "Model loading" }));
    expect(screen.getByText("A flowing phrase that keeps developing over many strokes. The default.")).toBeInTheDocument();
  });
});
