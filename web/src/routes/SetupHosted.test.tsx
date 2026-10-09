import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { CloudPlanningStatus } from "../api/cloud-types";
import type { LLMModelManagerSnapshot, PublicSettings, SetupStatus, SetupJob } from "../api/types";
import { useAppState, useToast } from "../state/app-state";
import { useAuth } from "../state/auth";
import { setupFixture } from "../test/setup-fixture";
import { SetupRoute } from "./SetupRoute";
import { newModelConnection } from "../util/model-connections";

vi.mock("../state/app-state", () => ({ useAppState: vi.fn(), useToast: vi.fn() }));
vi.mock("../state/auth", () => ({ useAuth: vi.fn() }));

const assessment = {
  free_disk_bytes: 100 * 2 ** 30, chat: { status: "met", reason: "gpu_fits", bytes: 5 * 2 ** 30, vram_mib: 4096 },
  voice_output: { status: "unmet", reason: "unavailable", bytes: 0 }, voice_input: { status: "unmet", reason: "unavailable", bytes: 0 },
  runtime_backend: "cuda", voice_module: "chatterbox", voice_device: "cuda",
} as NonNullable<SetupStatus["assessment"]>;
const emptyModels = { models: [], imports: [], runtime: { installed: false, current: false, state: "missing" } } as unknown as LLMModelManagerSnapshot;
const cloudStatus: CloudPlanningStatus = {
  connection: { active: "profile", profiles: [{ id: "profile", label: "Test account", connected: true, plan_authorized: true, welcome_pending: false }], pending: false, state: "connected", decisions_key_set: false, generation: 1 },
  models: [], readiness: { provider: "", model: "", ready: false, state: "untested" }, motion_planner: { provider: "conversation", model: "" },
};

describe("Easy hosted setup", () => {
  let settings: PublicSettings;
  beforeEach(() => {
    settings = {
      version: 2, server: { port: 49717 }, ui: { locale: "en", setup_completed: false }, device: { hsp_dispatch_owner: "cloud_rest", connection_key_set: false },
      motion: { handy_model: "handy_original" }, llm: { provider: "llama_cpp", llama_cpp_mode: "managed", model: "local-model", prompt_set: "magichandy_motion_v1", llama_cpp_base_url: "http://127.0.0.1:8080", ollama_base_url: "http://127.0.0.1:11434" },
    } as PublicSettings;
    vi.mocked(useAppState).mockReturnValue({ state: { settings }, backendOnline: true, readOnly: false, refresh: vi.fn(async () => undefined) } as unknown as ReturnType<typeof useAppState>);
    vi.mocked(useToast).mockReturnValue({ show: vi.fn() } as unknown as ReturnType<typeof useToast>);
    vi.mocked(useAuth).mockReturnValue({ status: { initialized: false }, refresh: vi.fn() } as unknown as ReturnType<typeof useAuth>);
    vi.spyOn(api, "networkStatus").mockResolvedValue({ active: { mode: "local", listen_address: "127.0.0.1:49717" }, saved: null } as never);
    vi.spyOn(api, "setupStatus").mockResolvedValue({ ...setupFixture, assessment });
    vi.spyOn(api, "llmModels").mockResolvedValue(emptyModels);
    vi.spyOn(api, "llmCatalog").mockResolvedValue({ models: [], hardware: { nvidia: true } });
    vi.spyOn(api, "cloudPlanningStatus").mockResolvedValue(cloudStatus);
    vi.spyOn(api, "modelConnectionKey").mockImplementation(async (_connection, key) => {
      const key_set = Boolean(key);
      vi.mocked(api.modelConnectionKeyStatus).mockResolvedValue({ key_set });
      return { key_set };
    });
    vi.spyOn(api, "modelConnectionKeyStatus").mockResolvedValue({ key_set: false });
    vi.spyOn(api, "modelConnectionModels").mockResolvedValue({ models: [{ id: "hosted-model", name: "Hosted model" }] });
    vi.spyOn(api, "modelConnectionTest").mockResolvedValue({ ready: true, message: "", provider: "compatible", model: "hosted-model" });
    vi.spyOn(api, "installSetupPlan").mockResolvedValue({ installation: { id: "install", status: "queued", kind: "install_plan" } as never });
    vi.spyOn(api, "checkSetupLocalModel").mockResolvedValue({ ready: true, model: "local-model", message: "" });
    vi.spyOn(api, "ollamaModels").mockResolvedValue({ available: true, models: [{ name: "installed-gemma", size_bytes: 1000 }] as never });
    vi.spyOn(api, "completeSetup").mockResolvedValue({ settings, signed_out: false });
    vi.spyOn(api, "saveSetupPreferences").mockImplementation(async update => {
      if (update.llm) settings = { ...settings, llm: { ...settings.llm, ...update.llm } };
      return { settings };
    });
  });
  afterEach(() => vi.restoreAllMocks());
  async function openEasy() {
    render(<SetupRoute />);
    await screen.findByRole("heading", { name: "Set up MagicHandy" });
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    await screen.findByRole("heading", { name: "Easy setup" });
  }
  async function chooseAPIModel() {
    fireEvent.click(screen.getByRole("radio", { name: /^API \/ Cloud/ }));
    fireEvent.change(screen.getByLabelText("API key"), { target: { value: "test-key" } });
    fireEvent.click(screen.getByRole("button", { name: "Connect provider" }));
    const select = await screen.findByRole("combobox", { name: "Available models" });
    fireEvent.change(select, { target: { value: "hosted-model" } });
    await waitFor(() => expect(screen.getByRole("button", { name: "Check model" })).toBeEnabled());
  }
  const continueButton = () => screen.getByRole("button", { name: /^(Continue|Install and continue)$/ });
  it("shows three choices and no OpenRouter or expert controls until API/Cloud is chosen", async () => {
    await openEasy();
    expect(screen.getByRole("radio", { name: /^Local-only AI/ })).toBeChecked();
    expect(screen.getByRole("radio", { name: /^ChatGPT/ })).toBeVisible();
    expect(screen.queryByText("OpenRouter")).not.toBeInTheDocument();
    expect(screen.queryByText("Role assignments")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("radio", { name: /^API \/ Cloud/ }));
    expect(screen.getByRole("combobox", { name: "Cloud provider" })).toBeVisible();
    expect(screen.getByRole("heading", { name: "Combine local and cloud models?" })).toBeVisible();
    expect(screen.queryByText("Structured output")).not.toBeInTheDocument();
  });
  it("requires a real test and skips local runtime and model installation for cloud only", async () => {
    await openEasy(); await chooseAPIModel();
    expect(continueButton()).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Check model" }));
    await screen.findByText("Ready");
    fireEvent.click(continueButton());
    await screen.findByRole("heading", { name: "Local backup (optional)" });
    expect(continueButton()).toBeDisabled();
    fireEvent.click(screen.getByRole("radio", { name: /^Continue without a backup/ }));
    fireEvent.click(continueButton());
    await screen.findByRole("heading", { name: "Setup is ready" });
    expect(settings.llm.retry_refusal_locally).toBe(false);
    expect(api.installSetupPlan).not.toHaveBeenCalled();
    expect(settings.llm.conversation_connection_id).toBe("openrouter-1");
    expect(settings.llm.motion_planner?.provider).toBe("conversation");
  });
  it("offers an existing local backup after ChatGPT and checks it without changing the primary", async () => {
    settings.llm = { ...settings.llm, provider: "ollama", model: "installed-gemma" };
    await openEasy();
    fireEvent.click(screen.getByRole("radio", { name: /^ChatGPT/ }));
    await screen.findByRole("combobox", { name: "Available models" });
    fireEvent.click(screen.getByRole("button", { name: "Check model" }));
    await screen.findByText("Ready");
    fireEvent.click(continueButton());
    await screen.findByRole("heading", { name: "Local backup (optional)" });
    fireEvent.click(screen.getByRole("radio", { name: /^Set up a local backup/ }));
    expect(await screen.findByRole("combobox", { name: "Ollama model" })).toHaveValue("installed-gemma");
    fireEvent.click(continueButton());
    await screen.findByRole("heading", { name: "Setup is ready" });
    expect(settings.llm).toMatchObject({ conversation_connection_id: "chatgpt-1", provider: "ollama", model: "installed-gemma", retry_refusal_locally: true });
    expect(api.installSetupPlan).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Open MagicHandy" })).toBeDisabled();
    vi.mocked(api.checkSetupLocalModel).mockResolvedValueOnce({ ready: false, message: "Local endpoint unavailable", model: "installed-gemma" });
    fireEvent.click(screen.getByRole("button", { name: "Check local backup" }));
    await screen.findByRole("alert");
    expect(screen.getByRole("button", { name: "Open MagicHandy" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Check local backup" }));
    await screen.findByRole("button", { name: "Local backup checked" });
    expect(screen.getByRole("button", { name: "Open MagicHandy" })).toBeEnabled();
  });

  it("can leave Finish after a failed local check without restarting installation", async () => {
    settings.llm = { ...settings.llm, provider: "ollama", model: "installed-gemma" };
    await openEasy(); await chooseAPIModel();
    fireEvent.click(screen.getByRole("button", { name: "Check model" }));
    await screen.findByText("Ready"); fireEvent.click(continueButton());
    await screen.findByRole("heading", { name: "Local backup (optional)" });
    fireEvent.click(screen.getByRole("radio", { name: /^Set up a local backup/ }));
    await waitFor(() => expect(continueButton()).toBeEnabled());
    fireEvent.click(continueButton()); await screen.findByRole("heading", { name: "Setup is ready" });
    vi.mocked(api.checkSetupLocalModel).mockResolvedValueOnce({ ready: false, message: "Unavailable", model: "installed-gemma" });
    fireEvent.click(screen.getByRole("button", { name: "Check local backup" }));
    await screen.findByRole("alert");
    fireEvent.click(screen.getByRole("button", { name: "Continue without a backup" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Open MagicHandy" })).toBeEnabled());
    expect(settings.llm.retry_refusal_locally).toBe(false);
    expect(settings.llm.conversation_connection_id).toBe("openrouter-1");
    expect(api.installSetupPlan).not.toHaveBeenCalled();
  });

  it("clears a stale local model when changing server types and explains the disabled Continue", async () => {
    await openEasy(); await chooseAPIModel();
    fireEvent.click(screen.getByRole("button", { name: "Check model" }));
    await screen.findByText("Ready"); fireEvent.click(continueButton());
    await screen.findByRole("heading", { name: "Local backup (optional)" });
    expect(screen.getByText("Choose whether to set up a local backup.")).toBeVisible();
    fireEvent.click(screen.getByRole("radio", { name: /^Set up a local backup/ }));
    fireEvent.click(screen.getByRole("radio", { name: /^Use my Ollama/ }));
    expect(await screen.findByRole("combobox", { name: "Ollama model" })).toHaveValue("");
    expect(continueButton()).toBeDisabled();
    expect(screen.getByText("Choose a local model to continue.")).toBeVisible();
    fireEvent.change(screen.getByRole("combobox", { name: "Ollama model" }), { target: { value: "installed-gemma" } });
    fireEvent.click(screen.getByRole("button", { name: "Check local backup" }));
    await screen.findByRole("button", { name: "Local backup checked" });
    fireEvent.click(continueButton()); await screen.findByRole("heading", { name: "Setup is ready" });
    expect(screen.getByRole("button", { name: "Open MagicHandy" })).toBeEnabled();
    expect(api.checkSetupLocalModel).toHaveBeenCalledOnce();
  });

  it("includes the selected managed backup download in the install plan while preserving API chat", async () => {
    vi.mocked(api.llmCatalog).mockResolvedValue({ hardware: { nvidia: true }, models: [{ id: "backup-catalog", display_name: "Backup Gemma", fit: "recommended", size_bytes: 1000 }] as never });
    await openEasy(); await chooseAPIModel();
    fireEvent.click(screen.getByRole("button", { name: "Check model" }));
    await screen.findByText("Ready");
    fireEvent.click(continueButton());
    await screen.findByRole("heading", { name: "Local backup (optional)" });
    fireEvent.click(screen.getByRole("radio", { name: /^Set up a local backup/ }));
    expect(screen.queryByRole("radio", { name: /^Add a model later/ })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Install and continue" }));
    await screen.findByRole("heading", { name: "Installing selected features" });
    expect(api.installSetupPlan).toHaveBeenCalledWith({ llama: { backend: "auto" }, model: { catalog_id: "backup-catalog" }, parakeet: false });
    expect(settings.llm.conversation_connection_id).toBe("openrouter-1");
    expect(settings.llm.retry_refusal_locally).toBe(true);
  });

  it("offers backup configuration in Custom setup and can turn off a previously enabled backup", async () => {
    const connection = { ...newModelConnection("chatgpt", []), model: "hosted-model" };
    settings.llm = { ...settings.llm, connections: [connection], conversation_connection_id: connection.id, retry_refusal_locally: true };
    await openEasy();
    // Saved custom settings are also supported by the Custom Chat AI page.
    fireEvent.click(screen.getByRole("button", { name: "Back" }));
    fireEvent.click(screen.getByRole("radio", { name: /^Custom setup/ }));
    fireEvent.click(continueButton());
    await screen.findByRole("heading", { name: "Choose who can open MagicHandy" });
    await waitFor(() => expect(continueButton()).toBeEnabled());
    fireEvent.click(continueButton());
    await screen.findByRole("heading", { name: "Choose how MagicHandy reaches your device" });
    fireEvent.click(screen.getByRole("button", { name: "Skip for now" }));
    await screen.findByRole("heading", { name: "Set up the chat AI" });
    expect(screen.queryByRole("checkbox", { name: "Retry declined chat requests with the local model" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Discard model changes" })).not.toBeInTheDocument();
    fireEvent.click(continueButton());
    await screen.findByRole("heading", { name: "Local backup (optional)" });
    expect(screen.getByRole("radio", { name: /^Set up a local backup/ })).toBeChecked();
    fireEvent.click(screen.getByRole("radio", { name: /^Continue without a backup/ }));
    fireEvent.click(continueButton());
    await screen.findByRole("heading", { name: "Add voice features" });
    expect(settings.llm.conversation_connection_id).toBe(connection.id);
    expect(settings.llm.retry_refusal_locally).toBe(false);
  });

  it("keeps OpenRouter out of the ChatGPT path and saves the explicit local/cloud split", async () => {
    await openEasy();
    fireEvent.click(screen.getByRole("radio", { name: /^ChatGPT/ }));
    expect(screen.queryByText("OpenRouter")).not.toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: /^Keep chat local/ })).not.toBeChecked();
    fireEvent.click(screen.getByRole("checkbox", { name: /^Keep chat local/ }));
    await screen.findByRole("combobox", { name: "Available models" });
    fireEvent.click(screen.getByRole("button", { name: "Check model" }));
    await screen.findByText("Ready");
    fireEvent.click(screen.getByRole("button", { name: "Install and continue" }));
    await screen.findByRole("heading", { name: "Installing selected features" });
    expect(settings.llm.conversation_connection_id).toBe("local");
    expect(settings.llm.motion_planner).toMatchObject({ provider: "connection", connection_id: "chatgpt-1", context_policy: "technical" });
    expect(api.installSetupPlan).toHaveBeenCalledWith(expect.objectContaining({ llama: { backend: "auto" } }));
  });
  it("allows cloud only when local hardware is unavailable and holds on a failed test", async () => {
    vi.mocked(api.setupStatus).mockResolvedValue({ ...setupFixture, hardware: { platform: "windows/amd64", nvidia: false, cuda: false }, assessment: { ...assessment, chat: { status: "unmet", reason: "no_nvidia", bytes: 0 } } });
    vi.mocked(api.modelConnectionTest).mockResolvedValue({ ready: false, message: "Cloud usage limit reached", provider: "openrouter", model: "hosted-model" });
    await openEasy(); await chooseAPIModel();
    expect(screen.getByRole("checkbox", { name: /^Keep chat local/ })).not.toBeChecked();
    expect(screen.getByRole("checkbox", { name: /^Keep chat local/ })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Check model" }));
    await screen.findByText("Cloud usage limit reached");
    expect(continueButton()).toBeDisabled();
    expect(api.modelConnectionTest).toHaveBeenCalledTimes(1);
    expect(api.installSetupPlan).not.toHaveBeenCalled();
  });

  it("blocks insufficient disk space for a local install while cloud only remains available", async () => {
    vi.mocked(api.setupStatus).mockResolvedValue({ ...setupFixture, assessment: { ...assessment, free_disk_bytes: 2 ** 30 } });
    await openEasy(); await chooseAPIModel();
    fireEvent.click(screen.getByRole("checkbox", { name: /^Keep chat local/ }));
    fireEvent.click(screen.getByRole("button", { name: "Check model" }));
    await screen.findByText("Ready");
    expect(continueButton()).toBeDisabled();
    expect(screen.getByRole("alert")).toHaveTextContent("Free up space before continuing");
    fireEvent.click(screen.getByRole("checkbox", { name: /^Keep chat local/ }));
    expect(continueButton()).toBeEnabled();
    expect(screen.getByText("Nothing needs to be downloaded.")).toBeVisible();
  });

  it("keeps an external local model when combining with ChatGPT on otherwise unsupported hardware", async () => {
    settings.llm = { ...settings.llm, llama_cpp_mode: "external", model: "saved-external-model" };
    vi.mocked(api.setupStatus).mockResolvedValue({ ...setupFixture, assessment: { ...assessment, chat: { status: "unmet", reason: "no_nvidia", bytes: 0 } } });
    await openEasy();
    fireEvent.click(screen.getByRole("radio", { name: /^ChatGPT/ }));
    expect(screen.getByRole("checkbox", { name: /^Keep chat local/ })).not.toBeChecked();
    fireEvent.click(screen.getByRole("checkbox", { name: /^Keep chat local/ }));
    await screen.findByRole("combobox", { name: "Available models" });
    fireEvent.click(screen.getByRole("button", { name: "Check model" }));
    await screen.findByText("Ready");
    fireEvent.click(continueButton());
    await screen.findByRole("heading", { name: "Setup is ready" });
    expect(settings.llm.model).toBe("saved-external-model");
    expect(settings.llm.llama_cpp_mode).toBe("external");
    expect(api.installSetupPlan).not.toHaveBeenCalled();
  });

  it("preserves custom saved roles and sharing without claiming the simple split", async () => {
    const connection = { ...newModelConnection("openrouter", []), model: "hosted-model" };
    const planner = { provider: "connection", connection_id: connection.id, model: "", context_policy: "conversation" } as const;
    settings.llm = { ...settings.llm, llama_cpp_mode: "external", connections: [connection], conversation_connection_id: "local", motion_planner: planner };
    await openEasy();
    expect(screen.getByText("Keep your current AI setup")).toBeVisible();
    expect(screen.queryByRole("heading", { name: "Combine local and cloud models?" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Test saved AI setup" }));
    await screen.findByText("Ready");
    fireEvent.click(continueButton());
    await screen.findByRole("heading", { name: "Setup is ready" });
    expect(settings.llm.motion_planner).toEqual(planner);
    expect(settings.llm.conversation_connection_id).toBe("local");
    expect(api.installSetupPlan).not.toHaveBeenCalled();
  });

  it("does not restore readiness for a changed endpoint with the same model ID", async () => {
    const connection = { ...newModelConnection("compatible", []), model: "hosted-model", no_authentication: true };
    settings.llm = { ...settings.llm, connections: [connection], conversation_connection_id: connection.id };
    vi.mocked(api.cloudPlanningStatus).mockResolvedValue({ ...cloudStatus, connection_readiness: { [connection.id]: { provider: connection.provider, model: connection.model, state: "ready", ready: true, connection: { ...connection, base_url: "http://127.0.0.1:9999/v1" } } } });
    await openEasy();
    expect(continueButton()).toBeDisabled();
    expect(screen.queryByText("Ready")).not.toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole("button", { name: "Check model" })).toBeEnabled());
    fireEvent.click(screen.getByRole("button", { name: "Check model" }));
    await screen.findByText("Ready");
    expect(continueButton()).toBeEnabled();
  });

  it("discovers connected ChatGPT models automatically without running a generation", async () => {
    await openEasy();
    fireEvent.click(screen.getByRole("radio", { name: /^ChatGPT/ }));
    const select = await screen.findByRole("combobox", { name: "Available models" });
    await waitFor(() => expect(select).toHaveValue("hosted-model"));
    expect(api.modelConnectionModels).toHaveBeenCalledOnce();
    expect(api.modelConnectionTest).not.toHaveBeenCalled();
    expect(screen.getByRole("checkbox", { name: /^Keep chat local/ })).not.toBeChecked();
    expect(screen.getByText("Enter a model ID").parentElement).not.toHaveAttribute("open");
    expect(continueButton()).toBeDisabled();
    expect(screen.getByText("Check a model to continue.")).toBeVisible();
  });

  it("restores exact readiness after reload and invalidates a changed model", async () => {
    const connection = { ...newModelConnection("chatgpt", []), model: "hosted-model" };
    settings.llm = { ...settings.llm, connections: [connection], conversation_connection_id: connection.id };
    vi.mocked(api.cloudPlanningStatus).mockResolvedValue({ ...cloudStatus, connection_readiness: { [connection.id]: { provider: "chatgpt", model: connection.model, ready: true, state: "ready", connection } } });
    await openEasy();
    const select = await screen.findByRole("combobox", { name: "Available models" });
    await waitFor(() => expect(continueButton()).toBeEnabled());
    expect(screen.getByText("Ready")).toBeVisible();
    expect(api.modelConnectionTest).not.toHaveBeenCalled();
    fireEvent.change(select, { target: { value: "" } });
    expect(continueButton()).toBeDisabled();
    expect(screen.queryByText("Ready")).not.toBeInTheDocument();
  });

  it("explains read-only setup and disables language and provider changes", async () => {
    vi.mocked(useAppState).mockReturnValue({ state: { settings }, backendOnline: true, readOnly: true, refresh: vi.fn() } as unknown as ReturnType<typeof useAppState>);
    render(<SetupRoute />);
    await screen.findByRole("heading", { name: "Set up MagicHandy" });
    for (const select of screen.getAllByRole("combobox")) expect(select).toBeDisabled();
    expect(screen.getByRole("button", { name: "Continue" })).toBeDisabled();
    expect(screen.getByText("Take control to change setup.")).toBeVisible();
    expect(api.modelConnectionTest).not.toHaveBeenCalled();
    expect(api.saveSetupPreferences).not.toHaveBeenCalled();
  });

  it("keeps a saved draft API key available after changing choices and can remove it", async () => {
    vi.mocked(api.modelConnectionKeyStatus).mockResolvedValue({ key_set: true });
    await openEasy();
    fireEvent.click(screen.getByRole("radio", { name: /^API \/ Cloud/ }));
    await screen.findByRole("combobox", { name: "Available models" });
    expect(screen.getByLabelText("API key")).toHaveAttribute("placeholder", "Saved key will be kept");
    fireEvent.click(screen.getByRole("radio", { name: /^Local-only AI/ }));
    fireEvent.click(screen.getByRole("radio", { name: /^API \/ Cloud/ }));
    await screen.findByRole("combobox", { name: "Available models" });
    expect(screen.getByLabelText("API key")).toHaveValue("");
    expect(screen.getByLabelText("API key")).toHaveAttribute("placeholder", "Saved key will be kept");
    expect(api.modelConnectionTest).not.toHaveBeenCalled();
    fireEvent.click(screen.getByText("Manage key"));
    fireEvent.click(screen.getByRole("button", { name: "Remove API key" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Connect provider" })).toBeVisible());
    expect(api.modelConnectionKey).toHaveBeenCalledWith(expect.objectContaining({ provider: "openrouter" }), "");
    expect(continueButton()).toBeDisabled();
  });

  it("keeps a verified manual model ready when its endpoint has no catalog", async () => {
    const connection = { ...newModelConnection("compatible", []), model: "manual-model", no_authentication: true };
    settings.llm = { ...settings.llm, connections: [connection], conversation_connection_id: connection.id };
    vi.mocked(api.modelConnectionModels).mockRejectedValue(new Error("Catalog unavailable"));
    vi.mocked(api.cloudPlanningStatus).mockResolvedValue({ ...cloudStatus, connection_readiness: { [connection.id]: { provider: "compatible", model: connection.model, ready: true, state: "ready", connection } } });
    await openEasy();
    await screen.findByText("Catalog unavailable");
    expect(continueButton()).toBeEnabled();
    expect(screen.getByText("Ready")).toBeVisible();
    expect(api.modelConnectionTest).not.toHaveBeenCalled();
  });

  it("retries the recorded backend plan after reload", async () => {
    const failed = { id: "failed", kind: "install_plan", status: "failed", message: "Voice download interrupted.", retry_available: true } as SetupJob;
    vi.mocked(api.setupStatus).mockResolvedValue({ ...setupFixture, required: true, assessment, installation: failed });
    vi.spyOn(api, "retrySetupPlan").mockResolvedValue({ installation: { ...failed, id: "retry", status: "queued" } });
    render(<SetupRoute />);
    await screen.findByRole("button", { name: "Retry installation" });
    fireEvent.click(screen.getByRole("button", { name: "Retry installation" }));
    await waitFor(() => expect(api.retrySetupPlan).toHaveBeenCalledOnce());
    expect(api.installSetupPlan).not.toHaveBeenCalled();
  });
});
