import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { PublicSettings } from "../api/types";
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
      prompt_sets: ["default"],
      tts_providers: ["none"],
      asr_providers: ["none"],
      parakeet_sources: ["app_managed"],
      neutts_sampling_modes: ["fixed", "random"],
      chat_startup_behaviors: ["previous", "new"],
    },
  } as unknown as PublicSettings;
}

describe("SettingsRoute", () => {
  beforeEach(() => {
    app.hash = "#/settings/diagnostics";
    app.refresh.mockReset();
    app.show.mockReset();
    getSettings.mockReset();
    saveSettings.mockReset();
    resetSettings.mockReset();
    saveSettings.mockResolvedValue({ settings: settings("normal") });
    resetSettings.mockResolvedValue({ settings: settings("normal") });
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
    expect(screen.getByRole("heading", { name: /Manual motion/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Start test" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Stop test" })).toBeDisabled();

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
});
