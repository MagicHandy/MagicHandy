import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { PublicSettings } from "../api/types";
import { setLocaleForTest } from "../i18n";
import english from "../i18n/locales/en.json";
import japanese from "../i18n/locales/ja.json";
import { SettingsRoute } from "./SettingsRoute";

const app = vi.hoisted(() => ({
  hash: "#/settings/diagnostics",
  refresh: vi.fn(),
  show: vi.fn(),
}));

vi.mock("../api/client", () => ({
  api: {
    getSettings: vi.fn(),
    saveSettings: vi.fn(),
    resetSettings: vi.fn(),
    exportTrace: vi.fn(),
    startManualTest: vi.fn(),
    stopMotion: vi.fn(),
    mediaVideos: vi.fn(() => Promise.resolve({ videos: [] })),
    mediaScan: vi.fn(() => Promise.resolve({ scan: { running: false, cancellable: false, cancelled: false, files_visited: 0, videos_found: 0 } })),
    getPromptSets: vi.fn(() => Promise.resolve({ prompt_sets: [], selected: "default" })),
    getMemory: vi.fn(() => Promise.resolve({ enabled: false, items: [] })),
    llmModels: vi.fn(() => Promise.reject(new Error("model store unavailable"))),
    llmStatus: vi.fn(() => Promise.resolve({ provider: "llama_cpp", base_url: "", model: "", available: false, message: "No model loaded" })),
  },
}));

vi.mock("../state/app-state", () => ({
  useAppState: () => ({
    backendOnline: true,
    readOnly: false,
    refresh: app.refresh,
    motion: { engine: { running: true, target: { source: "autopilot" } } },
    state: { version: "test", commit: "abc", uptime_seconds: 10, motion: { available: true } },
  }),
  useHashRoute: () => app.hash,
  useToast: () => ({ show: app.show }),
}));

const getSettings = vi.mocked(api.getSettings);
const saveSettings = vi.mocked(api.saveSettings);
const resetSettings = vi.mocked(api.resetSettings);

function settings(verbosity: string): PublicSettings {
  return {
    version: 1,
    server: { port: 49717 },
    ui: { locale: "en", theme: "steel-azure" },
    media: {
      library_paths: ["C:\\Media"],
      auto_scan_on_startup: false,
      remove_missing_on_scan: true,
      script_offset_ms: 125,
      script_smoothing_percent: 3,
      peak_rounding_ms: 84,
    },
    device: {
      hsp_dispatch_owner: "cloud_rest",
      intiface_server_address: "ws://127.0.0.1:12345",
      firmware_api_requirement: "firmware_v4_api_v3_required",
      api_application_id_source: "bundled",
      api_application_id_override: "",
      connection_key_set: true,
    },
    motion: {
      speed_min_percent: 10,
      speed_max_percent: 40,
      stroke_min_percent: 20,
      stroke_max_percent: 80,
      reverse_direction: false,
      apply_video_speed_limit: false,
      style: "balanced",
    },
    llm: {
      provider: "llama_cpp",
      llama_cpp_mode: "managed",
      llama_cpp_base_url: "",
      ollama_base_url: "",
      model: "",
      prompt_set: "default",
      request_timeout_ms: 120000,
      max_output_tokens: 256,
      reasoning_mode: "off",
      chat_voice: "utility",
      user_anatomy: "penis",
      custom_anatomy: "",
      persona_description: "",
    },
    voice: {
      enabled: false,
      tts_provider: "none",
      asr_provider: "none",
      tts_worker_path: "",
      tts_worker_args: [],
      asr_worker_path: "",
      asr_worker_args: [],
      speak_replies: false,
      parakeet_source: "app_managed",
      input_mode: "hands_free",
      input_sensitivity: 55,
      input_silence_ms: 900,
      input_noise_suppression: true,
      neutts_sampling_mode: "fixed",
      neutts_sampler_seed: 3,
    },
    chat: { startup_behavior: "previous", keep_unsaved_on_exit: false },
    diagnostics: { verbosity },
    options: {
      hsp_dispatch_owners: ["cloud_rest", "browser_bluetooth", "intiface"],
      api_application_id_sources: ["bundled", "developer_override"],
      diagnostics_verbosities: ["normal", "debug", "trace"],
      motion_styles: ["gentle", "balanced", "intense"],
      llm_providers: ["llama_cpp", "ollama"],
      llama_cpp_modes: ["managed", "external"],
      llm_reasoning_modes: ["off", "auto"],
      llm_max_output_tokens: [128, 256, 512],
      llm_chat_voices: ["utility", "warm", "intimate", "explicit"],
      llm_user_anatomies: ["penis", "vagina", "custom"],
      prompt_sets: ["default"],
      tts_providers: ["none"],
      asr_providers: ["none"],
      parakeet_sources: ["app_managed"],
      neutts_sampling_modes: ["fixed", "random"],
      chat_startup_behaviors: ["previous", "new"],
      locales: ["en", "es", "pt-BR", "zh-Hans", "ja"],
      themes: ["steel-azure", "deep-violet", "paperwhite-crt"],
    },
  } as unknown as PublicSettings;
}

describe("SettingsRoute", () => {
  beforeEach(() => {
    setLocaleForTest("en", english);
    app.hash = "#/settings/diagnostics";
    app.refresh.mockReset();
    app.show.mockReset();
    getSettings.mockReset();
    saveSettings.mockReset();
    resetSettings.mockReset();
    saveSettings.mockResolvedValue({ settings: settings("normal") });
    resetSettings.mockResolvedValue({ settings: settings("normal") });
  });

  it("persists the selected interface language", async () => {
    app.hash = "#/settings/general";
    getSettings.mockResolvedValue({ settings: settings("normal") });
    render(<SettingsRoute />);

    const language = await screen.findByRole("combobox", { name: "Language" });
    expect(language).toHaveValue("en");
    expect(screen.getByRole("option", { name: "Português (Brasil)" })).toHaveValue("pt-BR");
    fireEvent.change(language, { target: { value: "ja" } });
    fireEvent.click(screen.getByRole("button", { name: "Save settings" }));

    await waitFor(() => expect(saveSettings).toHaveBeenCalledOnce());
    expect(saveSettings.mock.calls[0][0].ui).toEqual({ locale: "ja", theme: "steel-azure" });
  });


  it("lists backend-supported themes and persists the selected palette", async () => {
    app.hash = "#/settings/general";
    getSettings.mockResolvedValue({ settings: settings("normal") });
    render(<SettingsRoute />);

    const steel = await screen.findByRole("radio", { name: /Steel Azure/ });
    expect(steel).toBeChecked();
    expect(screen.getAllByRole("radio")).toHaveLength(3);
    expect(screen.queryByRole("radio", { name: "Carbon Stealth" })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("radio", { name: "Deep Violet" }));
    fireEvent.click(screen.getByRole("button", { name: "Save settings" }));

    await waitFor(() => expect(saveSettings).toHaveBeenCalledOnce());
    expect(saveSettings.mock.calls[0][0].ui).toEqual({ locale: "en", theme: "deep-violet" });
  });
  it("does not overwrite immediate playback filters from a stale settings draft", async () => {
    app.hash = "#/settings/general";
    getSettings.mockResolvedValue({ settings: settings("normal") });
    render(<SettingsRoute />);

    fireEvent.click(await screen.findByRole("button", { name: "Save settings" }));

    await waitFor(() => expect(saveSettings).toHaveBeenCalledOnce());
    const media = saveSettings.mock.calls[0][0].media;
    // The guard is the omission: these two save through their own immediate
    // endpoint, so a stale Settings draft must never carry them.
    expect(media).not.toHaveProperty("script_smoothing_percent");
    expect(media).not.toHaveProperty("peak_rounding_ms");
    expect(media.library_paths).toEqual(["C:\\Media"]);
    expect(media.script_offset_ms).toBe(125);
  });

  it("localizes settings navigation, firmware guidance, and chat option labels", async () => {
    setLocaleForTest("ja", japanese);
    getSettings.mockResolvedValue({ settings: settings("normal") });

    app.hash = "#/settings/device";
    const deviceView = render(<SettingsRoute />);
    expect(await screen.findByRole("link", { name: "一般" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "デバイス" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "メディアライブラリ" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "プロンプトとメモリ" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "診断" })).toBeInTheDocument();
    expect(screen.getByText("Cloud REST には、API v3 アクセスが有効な Handy ファームウェア v4 が必要です。")).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "\u540c\u68b1\u30a2\u30d7\u30ea\u30b1\u30fc\u30b7\u30e7\u30f3 ID" })).toBeInTheDocument();
    deviceView.unmount();

    app.hash = "#/settings/prompts";
    render(<SettingsRoute />);
    expect(await screen.findByRole("option", { name: "実用（中立的なアシスタント）" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "ペニス" })).toBeInTheDocument();
  });

  // Driving the device by hand is a device tool, not a diagnostic, so it sits
  // with the connection that carries it.
  it("offers the manual motion test from the device section", async () => {
    app.hash = "#/settings/device";
    getSettings.mockResolvedValue({ settings: settings("normal") });
    render(<SettingsRoute />);

    expect(await screen.findByRole("heading", { name: /Manual motion/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Start test" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Stop test" })).toBeDisabled();
  });

  it("keeps device settings to one card layer", async () => {
    app.hash = "#/settings/device";
    getSettings.mockResolvedValue({ settings: settings("normal") });
    const view = render(<SettingsRoute />);

    await screen.findByRole("heading", { level: 2, name: "Device" });
    const panel = view.container.querySelector(".panel");
    expect(panel).not.toBeNull();
    expect(panel?.querySelector(":scope > h2.section-title")).toHaveTextContent("Device");
    expect(panel?.querySelectorAll(".group .group")).toHaveLength(0);
    expect(panel?.querySelectorAll(":scope > label.field, :scope > label.toggle-line")).toHaveLength(0);
  });

  it("uses one top-level card layer for model settings", async () => {
    app.hash = "#/settings/model";
    getSettings.mockResolvedValue({ settings: settings("normal") });
    const view = render(<SettingsRoute />);

    await screen.findByRole("heading", { level: 2, name: "Model" });
    await screen.findByRole("alert");
    const panel = view.container.querySelector(".panel");
    expect(panel).not.toBeNull();
    expect(panel?.querySelector(":scope > h2.section-title")).toHaveTextContent("Model");
    expect(panel?.querySelectorAll(":scope > .group")).toHaveLength(3);
    expect(panel?.querySelectorAll(".group .group")).toHaveLength(0);
    expect(screen.getByRole("group", { name: "Model permissions" })).toBeInTheDocument();
  });

  it("reloads the routed form after factory reset before it can be saved again", async () => {
    let current = settings("trace");
    getSettings.mockImplementation(async () => ({ settings: current }));
    resetSettings.mockImplementation(async () => {
      current = settings("normal");
      return { settings: current };
    });
    render(<SettingsRoute />);
    expect(await screen.findByRole("combobox", { name: "Diagnostics verbosity" })).toHaveValue("trace");

    fireEvent.click(screen.getByRole("button", { name: "Reset all settings" }));
    fireEvent.click(screen.getByRole("button", { name: "Confirm reset all settings" }));

    await waitFor(() => expect(screen.getByRole("combobox", { name: "Diagnostics verbosity" })).toHaveValue("normal"));
    fireEvent.click(screen.getByRole("button", { name: "Save settings" }));
    await waitFor(() => expect(saveSettings).toHaveBeenCalledOnce());
    expect(saveSettings.mock.calls[0][0].diagnostics).toEqual({ verbosity: "normal" });
  });

  it("renders a persistent first-load error and recovers through Retry", async () => {
    getSettings
      .mockRejectedValueOnce(new Error("settings database unavailable"))
      .mockResolvedValueOnce({ settings: settings("normal") });

    render(<SettingsRoute />);

    expect(await screen.findByRole("alert")).toHaveTextContent("settings database unavailable");
    expect(screen.queryByText("Loading settings…")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));

    await waitFor(() => expect(screen.getByRole("combobox", { name: "Diagnostics verbosity" })).toHaveValue("normal"));
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("deduplicates Save while the first settings request is pending", async () => {
    getSettings.mockResolvedValue({ settings: settings("normal") });
    let release!: (value: { settings: PublicSettings }) => void;
    saveSettings.mockImplementation(() => new Promise((resolve) => { release = resolve; }));
    render(<SettingsRoute />);
    const button = await screen.findByRole("button", { name: "Save settings" });

    fireEvent.click(button);
    fireEvent.click(button);

    expect(saveSettings).toHaveBeenCalledOnce();
    release({ settings: settings("normal") });
    await waitFor(() => expect(screen.getByRole("button", { name: "Save settings" })).toBeEnabled());
  });

  it("applies reset defaults when runtime teardown reports a partial failure", async () => {
    getSettings.mockResolvedValue({ settings: settings("trace") });
    resetSettings.mockRejectedValue(Object.assign(
      new Error("Settings were reset, but the active runtime could not be stopped."),
      { body: { settings: settings("normal") } },
    ));
    render(<SettingsRoute />);
    expect(await screen.findByRole("combobox", { name: "Diagnostics verbosity" })).toHaveValue("trace");

    fireEvent.click(screen.getByRole("button", { name: "Reset all settings" }));
    fireEvent.click(screen.getByRole("button", { name: "Confirm reset all settings" }));

    await waitFor(() => expect(screen.getByRole("combobox", { name: "Diagnostics verbosity" })).toHaveValue("normal"));
    expect(app.show).toHaveBeenCalledWith(
      "Settings were reset, but the active runtime could not be stopped.",
      "error",
    );
  });

  it("makes clean-start behavior incompatible with retaining an unsaved draft", async () => {
    app.hash = "#/settings/chat";
    getSettings.mockResolvedValue({ settings: settings("normal") });
    render(<SettingsRoute />);

    const startup = await screen.findByRole("combobox", { name: /When MagicHandy starts/ });
    const retain = screen.getByRole("checkbox", { name: /Keep an unsaved current chat/ });
    expect(retain).toBeEnabled();

    fireEvent.change(startup, { target: { value: "new" } });
    expect(retain).not.toBeChecked();
    expect(retain).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Save settings" }));

    await waitFor(() => expect(saveSettings).toHaveBeenCalledOnce());
    expect(saveSettings.mock.calls[0][0].chat).toEqual({
      startup_behavior: "new",
      keep_unsaved_on_exit: false,
    });
  });

  it("persists the selected chat voice from the prompts section", async () => {
    app.hash = "#/settings/prompts";
    getSettings.mockResolvedValue({ settings: settings("normal") });
    render(<SettingsRoute />);

    const voice = await screen.findByRole("combobox", { name: /Chat voice/ });
    expect(voice).toHaveValue("utility");
    fireEvent.change(voice, { target: { value: "explicit" } });
    fireEvent.click(screen.getByRole("button", { name: "Save settings" }));

    await waitFor(() => expect(saveSettings).toHaveBeenCalledOnce());
    expect(saveSettings.mock.calls[0][0].llm.chat_voice).toBe("explicit");
  });

  it("persists anatomy and persona prompt context", async () => {
    app.hash = "#/settings/prompts";
    getSettings.mockResolvedValue({ settings: settings("normal") });
    render(<SettingsRoute />);

    const anatomy = await screen.findByRole("combobox", { name: /User anatomy/ });
    expect(screen.queryByRole("textbox", { name: /Custom anatomy wording/ })).not.toBeInTheDocument();
    fireEvent.change(anatomy, { target: { value: "custom" } });
    const custom = screen.getByRole("textbox", { name: /Custom anatomy wording/ });
    const persona = screen.getByRole("textbox", { name: /Persona description/ });
    fireEvent.change(custom, { target: { value: "😀".repeat(121) } });
    fireEvent.change(persona, { target: { value: "😀".repeat(501) } });
    expect(custom).toHaveValue("😀".repeat(120));
    expect(persona).toHaveValue("😀".repeat(500));
    fireEvent.change(custom, { target: { value: "chosen wording" } });
    fireEvent.change(persona, { target: { value: "An energetic partner" } });
    fireEvent.click(screen.getByRole("button", { name: "Save settings" }));

    await waitFor(() => expect(saveSettings).toHaveBeenCalledOnce());
    expect(saveSettings.mock.calls[0][0].llm).toMatchObject({
      user_anatomy: "custom",
      custom_anatomy: "chosen wording",
      persona_description: "An energetic partner",
    });
  });

  it("persists the opt-in video script speed limit", async () => {
    app.hash = "#/settings/media";
    getSettings.mockResolvedValue({ settings: settings("normal") });
    render(<SettingsRoute />);

    const toggle = await screen.findByRole("checkbox", { name: /Apply motion speed limit to video scripts/ });
    fireEvent.click(toggle);
    fireEvent.click(screen.getByRole("button", { name: "Save settings" }));

    await waitFor(() => expect(saveSettings).toHaveBeenCalledOnce());
    expect(saveSettings.mock.calls[0][0].motion.apply_video_speed_limit).toBe(true);
  });

  it("persists startup and missing-file scan policy", async () => {
    app.hash = "#/settings/media";
    getSettings.mockResolvedValue({ settings: settings("normal") });
    render(<SettingsRoute />);

    const autoScan = await screen.findByRole("checkbox", { name: /Scan library when MagicHandy starts/ });
    const removeMissing = screen.getByRole("checkbox", { name: /Remove missing catalog entries/ });
    expect(autoScan).not.toBeChecked();
    expect(removeMissing).toBeChecked();
    fireEvent.click(autoScan);
    fireEvent.click(removeMissing);
    fireEvent.click(screen.getByRole("button", { name: "Save settings" }));

    await waitFor(() => expect(saveSettings).toHaveBeenCalledOnce());
    expect(saveSettings.mock.calls[0][0].media).toMatchObject({
      auto_scan_on_startup: true,
      remove_missing_on_scan: false,
    });
  });
});
