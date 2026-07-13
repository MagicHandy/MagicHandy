import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { act } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";
import { streamChat } from "./api/client";
import type { LLMModelManagerSnapshot } from "./api/types";
import { AppStateProvider, ToastProvider } from "./state/app-state";

// These tests guard the safety-critical UI invariants from
// docs/decisions/0009-react-frontend.md and docs/ui-design.md.

const baseState = {
  version: "test",
  commit: "abc",
  uptime_seconds: 1,
  settings: {
    version: 1,
    server: { port: 49717 },
    device: { hsp_dispatch_owner: "cloud_rest", intiface_server_address: "ws://127.0.0.1:12345", firmware_api_requirement: "v4/v3", api_application_id_source: "bundled_app_id", connection_key_set: false },
    motion: { speed_min_percent: 20, speed_max_percent: 80, stroke_min_percent: 0, stroke_max_percent: 100, reverse_direction: false, style: "balanced" },
    llm: { provider: "llama_cpp", llama_cpp_mode: "managed", llama_cpp_base_url: "", ollama_base_url: "", model: "", prompt_set: "default", request_timeout_ms: 120000 },
    voice: { enabled: false, tts_provider: "none", asr_provider: "none", tts_worker_path: "", tts_worker_args: [], asr_worker_path: "", asr_worker_args: [], speak_replies: false, elevenlabs_key_set: false },
    diagnostics: { verbosity: "normal" },
    options: {
      hsp_dispatch_owners: ["cloud_rest", "browser_bluetooth", "intiface"],
      api_application_id_sources: ["bundled", "developer_override"],
      diagnostics_verbosities: ["normal", "debug", "trace"],
      llm_providers: ["llama_cpp", "ollama"],
      llama_cpp_modes: ["managed", "external"],
      prompt_sets: ["default"],
      tts_providers: ["none", "elevenlabs", "neutts_air", "custom"],
      asr_providers: ["none", "parakeet_managed", "openai_compatible", "custom"],
    },
  },
  controller: { active: true, read_only: false },
  motion: { available: true },
  modes: {},
  memory: { enabled: true, memories: [] },
};

const libraryFixture = {
  patterns: [
    { id: "stroke", name: "Stroke", description: "Even full-span reversals.", origin: "builtin", kind: "routine", enabled: true, weight: 1, cycle_ms: 6600, points: [{ time_ms: 0, position_percent: 0 }, { time_ms: 3300, position_percent: 100 }, { time_ms: 6600, position_percent: 0 }], preview_samples: [{ time_ms: 0, position_percent: 0 }, { time_ms: 1650, position_percent: 50 }, { time_ms: 3300, position_percent: 100 }, { time_ms: 6600, position_percent: 0 }], tags: ["steady"], created_at: "now", updated_at: "now" },
    { id: "pulse", name: "Pulse", description: "Alternating peaks.", origin: "builtin", kind: "routine", enabled: false, weight: 0.8, cycle_ms: 6600, points: [{ time_ms: 0, position_percent: 15 }, { time_ms: 3300, position_percent: 100 }, { time_ms: 6600, position_percent: 15 }], preview_samples: [{ time_ms: 0, position_percent: 15 }, { time_ms: 3300, position_percent: 100 }, { time_ms: 6600, position_percent: 15 }], tags: ["rhythmic"], created_at: "now", updated_at: "now" },
  ],
  programs: [
    { id: "program-demo", name: "Demo program", origin: "imported", duration_ms: 10000, points: [{ time_ms: 0, position_percent: 0 }, { time_ms: 10000, position_percent: 100 }], preview_samples: [{ time_ms: 0, position_percent: 0 }, { time_ms: 5000, position_percent: 50 }, { time_ms: 10000, position_percent: 100 }], created_at: "now", updated_at: "now" },
  ],
  feedback: [],
  auto_disable: false,
};

const modelManagerFixture: LLMModelManagerSnapshot = {
  models: [
    {
      id: "managed-model-a1b2c3",
      display_name: "Managed model",
      provider: "llama_cpp",
      source: "ollama",
      source_name: "fixture:latest",
      format: "gguf",
      family: "llama",
      parameter_size: "3B",
      quantization: "Q4_K_M",
      size_bytes: 2_147_483_648,
      sha256: "a".repeat(64),
      model_path: "C:\\MagicHandy\\models\\managed-model-a1b2c3\\model.gguf",
      imported_at: "now",
      updated_at: "now",
      state: "ready",
    },
  ],
  imports: [],
  store_path: "C:\\MagicHandy\\models",
  suggested_ollama_path: "C:\\Users\\Test User\\.ollama\\models",
  runtime: {
    state: "ready",
    installed: true,
    current: true,
    build_supported: true,
    supported_backends: ["auto", "cpu", "cuda"],
    expected_version: "b9966",
    version: "b9966",
    commit: "c749cb041706647f460bb918cccc9d91995205ab",
    backend: "cpu",
    source: "built_from_source",
    built_at: "2026-07-11T00:00:00Z",
    message: "Managed llama.cpp b9966 (cpu) is installed.",
  },
};

const ollamaScanFixture = {
  path: "D:\\Ollama\\models",
  candidates: [
    {
      id: "ollama-candidate-1",
      name: "qwen-test:latest",
      format: "gguf",
      family: "qwen",
      parameter_size: "7B",
      quantization: "Q4_K_M",
      size_bytes: 4_294_967_296,
      digest: `sha256:${"b".repeat(64)}`,
      license: "Apache 2.0",
      importable: true,
    },
  ],
};

function jsonRes(data: unknown) {
  return { ok: true, status: 200, text: async () => JSON.stringify(data) } as Response;
}

function installFetch(opts: { state?: typeof baseState & { bluetooth_bridge?: unknown }; memory?: unknown; fail?: boolean; stopError?: string; stopStatus?: number; connectionCheckGate?: Promise<void>; chatLog?: unknown[]; voiceStatus?: unknown; library?: typeof libraryFixture; modelManager?: LLMModelManagerSnapshot } = {}) {
  const state = JSON.parse(JSON.stringify(opts.state ?? baseState)) as typeof baseState & { bluetooth_bridge?: unknown };
  const chatLog = opts.chatLog ?? [];
  let intiface = (state as typeof state & { intiface_transport?: Record<string, any> }).intiface_transport ?? {
    dispatch_owner: "intiface",
    address: state.settings.device.intiface_server_address,
    status: { connected: false, scanning: false, playback_state: "idle", max_ping_time_ms: 0, queue_depth: 0, devices: [] },
    diagnostics: {},
  };
  const fn = vi.fn(async (input: RequestInfo | URL, _init?: RequestInit) => {
    if (opts.fail) throw new Error("offline");
    const u = String(input);
    if (u.includes("/api/motion/stop")) {
      const status = opts.stopStatus ?? 200;
      return {
        ok: status >= 200 && status < 300,
        status,
        text: async () => JSON.stringify(opts.stopError ? { error: opts.stopError } : {}),
      } as Response;
    }
    if (u.includes("/api/transport/cloud/check")) {
      await opts.connectionCheckGate;
      (state as typeof state & { cloud_transport?: { connected: boolean } }).cloud_transport = { connected: true };
      return jsonRes({ ok: true, status: "http_200", hsp_available: true, playback_state: "idle", latency_ms: 42 });
    }
    if (u.includes("/api/transport/intiface")) {
      if (u.endsWith("/connect")) intiface = { ...intiface, status: { ...intiface.status, connected: true, max_ping_time_ms: 500, devices: [{ device_index: 7, device_name: "Test Linear", linear_actuators: [{ index: 0, feature_descriptor: "Position" }] }] } };
      if (u.endsWith("/disconnect")) intiface = { ...intiface, status: { ...intiface.status, connected: false, devices: [] } };
      if (u.endsWith("/scan")) intiface = { ...intiface, status: { ...intiface.status, scanning: _init?.method !== "DELETE" } };
      if (u.endsWith("/select")) intiface = { ...intiface, status: { ...intiface.status, selected_device_index: 7, selected_actuator_index: 0 } };
      return jsonRes(intiface);
    }
    if (u.includes("/api/transport/bluetooth/status")) return jsonRes({ status: "success", dispatch_owner: state.settings.device.hsp_dispatch_owner, bluetooth: state.bluetooth_bridge ?? {} });
    if (u.includes("/api/chat/messages")) return jsonRes({ messages: chatLog, latest_seq: chatLog.length, cursor: 0 });
    if (u.includes("/api/chat/cursor")) return jsonRes({ cursor: chatLog.length });
    if (u.includes("/api/voice/status")) return jsonRes(opts.voiceStatus ?? {});
    if (u.includes("/api/library")) return jsonRes({ library: opts.library ?? libraryFixture });
    if (u.includes("/api/llm/ollama/scan")) return jsonRes(ollamaScanFixture);
    if (u.includes("/api/llm/ollama/models")) return jsonRes({ available: true, models: [{ name: "qwen-test:latest", size_bytes: 4_294_967_296, format: "gguf", family: "qwen", parameter_size: "7B", quantization: "Q4_K_M" }] });
    if (u.includes("/api/llm/imports/ollama")) return jsonRes({ import: { id: "import-1", source: "ollama", display_name: "qwen-test:latest", status: "copying", bytes_copied: 1024, total_bytes: 4_294_967_296, started_at: "now", updated_at: "now" } });
    if (u.includes("/api/llm/runtime/build")) return jsonRes({ build: { id: "runtime-build-1", backend: "auto", status: "queued", message: "Queued managed llama.cpp source build.", started_at: "now", updated_at: "now" } });
    if (u.includes("/api/llm/models")) return jsonRes(opts.modelManager ?? modelManagerFixture);
    if (u.includes("/api/llm/status")) return jsonRes({ provider: state.settings.llm.provider, base_url: "http://127.0.0.1:8080", model: state.settings.llm.model, available: false, managed: state.settings.llm.llama_cpp_mode === "managed", loaded: false, models: state.settings.llm.llama_cpp_mode === "external" ? ["server-model-a", "server-model-b"] : undefined, message: `llama.cpp runner is not loaded${state.settings.llm.model ? ` (saved model: ${state.settings.llm.model})` : ""}` });
    if (u.includes("/api/settings")) {
      if (_init?.method === "PUT" && _init.body) {
        const update = JSON.parse(String(_init.body)) as { connection_key?: string; llm?: typeof state.settings.llm };
        if (u.endsWith("/device/connection-key") && update.connection_key) state.settings.device.connection_key_set = true;
        if (update.llm) state.settings.llm = { ...state.settings.llm, ...update.llm };
      }
      return jsonRes({ settings: state.settings });
    }
    if (u.includes("/api/memory")) return jsonRes(opts.memory ?? baseState.memory);
    if (u.includes("/api/prompt-sets")) return jsonRes({ sets: [] });
    if (u.includes("/api/state")) return jsonRes(state);
    return jsonRes({});
  });
  vi.stubGlobal("fetch", fn);
  return fn;
}

class FakeEventSource {
  onerror: ((e: unknown) => void) | null = null;
  addEventListener() {}
  close() {}
}

function go(hash: string) {
  act(() => {
    window.location.hash = hash;
    window.dispatchEvent(new Event("hashchange"));
  });
}

function renderApp() {
  return render(
    <AppStateProvider>
      <ToastProvider>
        <App />
      </ToastProvider>
    </AppStateProvider>,
  );
}

beforeEach(() => {
  vi.stubGlobal("EventSource", FakeEventSource);
  window.location.hash = "#/chat";
});

describe("app shell safety invariants", () => {
  it("keeps Stop mounted outside the status bar on every route", async () => {
    installFetch();
    renderApp();
    const stop = await screen.findByRole("button", { name: /emergency stop/i });
    const status = screen.getByRole("region", { name: /status/i });
    expect(status).not.toContainElement(stop);
    for (const hash of ["#/modes", "#/library", "#/settings", "#/chat"]) {
      go(hash);
      expect(screen.getByRole("button", { name: /emergency stop/i })).toBeInTheDocument();
    }
  });

  it("keeps only the connection disclosure in the compact top bar", async () => {
    installFetch();
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    const status = screen.getByRole("region", { name: /status/i });
    expect(within(status).queryByRole("slider")).toBeNull();
    expect(within(status).getAllByRole("button")).toHaveLength(1);
    expect(within(status).getByRole("button", { name: /open connection manager/i })).toBeInTheDocument();
  });

  it("keeps connection and live limits in one floating manager on every route", async () => {
    installFetch();
    renderApp();
    const trigger = await screen.findByRole("button", { name: /the handy connection key required/i });
    for (const hash of ["#/modes", "#/library", "#/settings", "#/chat"]) {
      go(hash);
      expect(screen.getByRole("button", { name: /the handy connection key required/i })).toBeInTheDocument();
    }
    fireEvent.click(trigger);
    const close = screen.getByRole("button", { name: /^close connection manager$/i });
    await waitFor(() => expect(close).toHaveFocus());
    fireEvent.click(close);
    await waitFor(() => expect(trigger).toHaveFocus());
    fireEvent.click(trigger);
    const manager = screen.getByRole("region", { name: /connection manager/i });
    for (const name of [/speed min/i, /speed max/i, /stroke min/i, /stroke max/i]) {
      expect(within(manager).getByRole("slider", { name })).toBeInTheDocument();
    }
    const motionControls = screen.getByRole("complementary", { name: /motion controls/i });
    expect(within(motionControls).queryByRole("slider", { name: /speed min/i })).toBeNull();
  });

  it("composes the hand, three signals, and Handy target without a runtime mask", async () => {
    installFetch();
    renderApp();
    fireEvent.click(await screen.findByRole("button", { name: /the handy connection key required/i }));
    const artwork = screen.getByRole("img", { name: /the handy wireless connection/i });
    expect(artwork.querySelector("image")?.getAttribute("href")).toMatch(/conductor-hand-v2\.png/);
    expect(artwork.querySelector("mask")).toBeNull();
    expect(artwork.querySelector("clipPath")).toBeNull();
    expect(artwork.querySelectorAll(".connection-signal path")).toHaveLength(3);
    expect(artwork.querySelectorAll(".connection-handy-body")).toHaveLength(2);
    expect(artwork.querySelector(".connection-handy-led")).toBeInTheDocument();
    expect(artwork.querySelector(".connection-handy-marker")).toBeInTheDocument();
    expect(artwork.querySelectorAll(".connection-disconnected path")).toHaveLength(2);
    expect(artwork).toHaveAttribute("viewBox", "0 0 360 260");
    expect(artwork).toHaveAttribute("data-phase", "disconnected");
  });

  it("saves the Cloud connection key through the scoped redacted endpoint", async () => {
    const fetchMock = installFetch();
    renderApp();
    fireEvent.click(await screen.findByRole("button", { name: /the handy connection key required/i }));
    const manager = screen.getByRole("region", { name: /connection manager/i });
    expect(within(manager).getByText(/built-in handy api v3 id/i)).toBeInTheDocument();

    fireEvent.change(within(manager).getByLabelText(/handy connection key/i), { target: { value: "test-connection-key" } });
    fireEvent.click(within(manager).getByRole("button", { name: /save key/i }));

    await waitFor(() => {
      const call = fetchMock.mock.calls.find(([input]) => String(input).endsWith("/api/settings/device/connection-key"));
      expect(call).toBeDefined();
      expect(JSON.parse(String(call?.[1]?.body))).toEqual({ connection_key: "test-connection-key" });
    });
    await waitFor(() => expect(within(manager).getByLabelText(/handy connection key/i)).toHaveValue(""));
  });

  it("applies a floating limit change through the semantic quick API", async () => {
    const fetchMock = installFetch();
    renderApp();
    fireEvent.click(await screen.findByRole("button", { name: /the handy connection key required/i }));
    fireEvent.change(screen.getByRole("slider", { name: /speed max/i }), { target: { value: "40" } });
    await waitFor(() => {
      const call = fetchMock.mock.calls.find(([input]) => String(input).includes("/api/motion/quick"));
      expect(call).toBeDefined();
      expect(JSON.parse(String(call?.[1]?.body))).toEqual({ speed_max_percent: 40 });
    });
  });

  it("shows the wireless wave state while checking The Handy", async () => {
    let release!: () => void;
    const gate = new Promise<void>((resolve) => { release = resolve; });
    installFetch({
      connectionCheckGate: gate,
      state: {
        ...baseState,
        settings: { ...baseState.settings, device: { ...baseState.settings.device, connection_key_set: true } },
      },
    });
    renderApp();
    fireEvent.click(await screen.findByRole("button", { name: /the handy not checked/i }));
    fireEvent.click(screen.getByRole("button", { name: /check connection/i }));
    expect(screen.getByRole("img", { name: /in progress/i })).toHaveAttribute("data-phase", "connecting");
    await act(async () => release());
    await waitFor(() => expect(screen.getByRole("img", { name: /the handy wireless connection$/i })).toHaveAttribute("data-phase", "connected"));
  });

  it("shows the backend-loss banner when the core is unreachable", async () => {
    installFetch({ fail: true });
    renderApp();
    expect(await screen.findByText(/core connection lost/i)).toBeInTheDocument();
    // Stop is still present so an offline client can still attempt it.
    expect(screen.getByRole("button", { name: /emergency stop/i })).toBeInTheDocument();
  });

  it("locks command controls for a read-only client but keeps Stop", async () => {
    installFetch({ state: { ...baseState, controller: { active: false, read_only: true } } });
    renderApp();
    const box = await screen.findByPlaceholderText(/read-only client/i);
    expect(box).toBeDisabled();
    expect(screen.getByRole("button", { name: /emergency stop/i })).toBeEnabled();
  });

  it("renders Settings as a route, not a stacked overlay", async () => {
    installFetch();
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    go("#/settings");
    expect(await screen.findByRole("navigation", { name: /settings sections/i })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /^settings$/i })).toBeInTheDocument();
  });

  it("renders the Phase 14 library as a backend-backed catalog", async () => {
    installFetch();
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    go("#/library");
    expect(await screen.findByText("Stroke")).toBeInTheDocument();
    expect(screen.getByText(/1 pattern available to chat/i)).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: /enable stroke/i })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: /enable pulse/i })).not.toBeChecked();
    expect(screen.getByRole("button", { name: /audition stroke/i })).toBeEnabled();
    expect(screen.getByRole("button", { name: /audition pulse/i })).toBeDisabled();
  });

  it("keeps library exports available read-only while locking mutations", async () => {
    installFetch({ state: { ...baseState, controller: { active: false, read_only: true } } });
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    go("#/library");
    expect(await screen.findByRole("button", { name: /export stroke/i })).toBeEnabled();
    expect(screen.getByRole("checkbox", { name: /enable stroke/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: /rate stroke up/i })).toBeDisabled();
  });

  it("keeps programs, authoring, and training in one library workspace", async () => {
    installFetch();
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    go("#/library");
    fireEvent.click(await screen.findByRole("tab", { name: /^programs$/i }));
    expect(screen.getByText("Demo program")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^play$/i })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("tab", { name: /^author$/i }));
    expect(screen.getByLabelText(/pattern drawing canvas/i)).toBeInTheDocument();
    expect(screen.getByText(/saved knots/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("tab", { name: /^training$/i }));
    expect(screen.getByRole("button", { name: /audition/i })).toBeInTheDocument();
    expect(screen.getByText(/auto-disable at low weight/i)).toBeInTheDocument();
  });

  it("keeps program intensity inside the backend speed envelope", async () => {
    const state = {
      ...baseState,
      settings: {
        ...baseState.settings,
        motion: { ...baseState.settings.motion, speed_max_percent: 25 },
      },
    };
    installFetch({ state });
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    go("#/library");
    fireEvent.click(await screen.findByRole("tab", { name: /^programs$/i }));
    const intensity = screen.getByRole("slider", { name: /intensity/i });
    expect(intensity).toHaveAttribute("max", "25");
    await waitFor(() => expect(intensity).toHaveValue("25"));
  });

  it("shows the deterministic curation fallback when every pattern is disabled", async () => {
    const disabled = { ...libraryFixture, patterns: libraryFixture.patterns.map((pattern) => ({ ...pattern, enabled: false })) };
    installFetch({ library: disabled });
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    go("#/library");
    expect(await screen.findByText(/deterministic fallback active/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("tab", { name: /^training$/i }));
    expect(screen.getByText(/no enabled patterns/i)).toBeInTheDocument();
  });

  it("renders every settings section without blanking the app", async () => {
    installFetch({ memory: { enabled: true } });
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    for (const hash of ["#/settings/device", "#/settings/model", "#/settings/voice", "#/settings/prompts", "#/settings/diagnostics"]) {
      go(hash);
      expect(await screen.findByRole("navigation", { name: /settings sections/i })).toBeInTheDocument();
      expect(screen.queryByText(/this view could not render/i)).toBeNull();
    }
  });

  it("renders backend-managed models and selects one through the settings form", async () => {
    const fetch = installFetch();
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    go("#/settings/model");

    expect(await screen.findByText("Managed model")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /import from ollama/i })).toBeEnabled();
    expect(screen.getByText(/built from pinned source/i)).toBeInTheDocument();
    expect(screen.getByText(/runner is not loaded/i)).toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: /llama-server path/i })).toBeNull();
    expect(screen.queryByRole("textbox", { name: /gguf model path/i })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /^use$/i }));

    expect(screen.getByRole("button", { name: /^selected$/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: /^load$/i })).toBeDisabled();
    expect(screen.getByText(/save settings to check this configuration/i)).toBeInTheDocument();
    expect(screen.getByText(/save settings before runtime actions/i)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /save settings/i }));
    await waitFor(() => expect(fetch.mock.calls.some(([, init]) => (init as RequestInit | undefined)?.method === "PUT")).toBe(true));
    const [, request] = fetch.mock.calls.find(([, init]) => (init as RequestInit | undefined)?.method === "PUT") ?? [];
    const payload = JSON.parse(String((request as RequestInit).body));
    expect(payload.llm.model).toBe("managed-model-a1b2c3");
    expect(payload.llm).not.toHaveProperty("llama_cpp_runner_path");
    expect(payload.llm).not.toHaveProperty("llama_cpp_model_path");
    expect(await screen.findByText(/saved model: managed-model-a1b2c3/i)).toBeInTheDocument();
  });

  it("scans a user-selected Ollama path and starts an explicit copy", async () => {
    const fetch = installFetch();
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    go("#/settings/model");
    fireEvent.click(await screen.findByRole("button", { name: /import from ollama/i }));

    const path = screen.getByRole("textbox", { name: /ollama models path/i });
    fireEvent.change(path, { target: { value: "D:\\Ollama" } });
    fireEvent.click(screen.getByRole("button", { name: /scan library/i }));
    expect(await screen.findByText("qwen-test:latest")).toBeInTheDocument();
    expect(screen.getByText(/4.00 GiB/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /import copy/i }));

    await waitFor(() => expect(fetch.mock.calls.some(([url]) => String(url).includes("/api/llm/imports/ollama"))).toBe(true));
    const [, scanRequest] = fetch.mock.calls.find(([url]) => String(url).includes("/api/llm/ollama/scan")) ?? [];
    expect(JSON.parse(String((scanRequest as RequestInit).body))).toEqual({ path: "D:\\Ollama" });
    const [, importRequest] = fetch.mock.calls.find(([url]) => String(url).includes("/api/llm/imports/ollama")) ?? [];
    expect(JSON.parse(String((importRequest as RequestInit).body))).toEqual({ path: "D:\\Ollama\\models", candidate_id: "ollama-candidate-1" });
  });

  it("lists and selects models reported by an external llama.cpp server", async () => {
    const state = {
      ...baseState,
      settings: {
        ...baseState.settings,
        llm: { ...baseState.settings.llm, llama_cpp_mode: "external", model: "server-model-a" },
      },
    };
    installFetch({ state });
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    go("#/settings/model");

    const list = await screen.findByLabelText(/models reported by llama\.cpp/i);
    expect(within(list).getByText("server-model-a")).toBeInTheDocument();
    fireEvent.click(within(list).getByRole("button", { name: /^use$/i }));
    await waitFor(() => expect(screen.getByRole("combobox", { name: /^model$/i })).toHaveValue("server-model-b"));
  });

  it("keeps model inventory visible but locks model mutations for read-only clients", async () => {
    installFetch({ state: { ...baseState, controller: { active: false, read_only: true } } });
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    go("#/settings/model");
    expect(await screen.findByText("Managed model")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /import from ollama/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: /^use$/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: /remove managed model/i })).toBeDisabled();
  });

  it("keeps the saved managed model protected while provider edits are unsaved", async () => {
    const state = {
      ...baseState,
      settings: {
        ...baseState.settings,
        llm: { ...baseState.settings.llm, model: "managed-model-a1b2c3" },
      },
    };
    installFetch({ state });
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    go("#/settings/model");

    const remove = await screen.findByRole("button", { name: /remove managed model/i });
    expect(remove).toBeDisabled();
    fireEvent.change(screen.getByRole("combobox", { name: /^provider$/i }), { target: { value: "ollama" } });
    await screen.findByLabelText(/models reported by ollama/i);
    expect(remove).toBeDisabled();
  });

  it("refreshes provider health after a managed runtime build finishes", async () => {
    const fetch = installFetch({
      modelManager: {
        ...modelManagerFixture,
        runtime_build: {
          id: "runtime-build-complete",
          backend: "cpu",
          status: "complete",
          message: "Managed llama.cpp b9966 (cpu) is installed.",
          started_at: "now",
          updated_at: "now",
        },
      },
    });
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    go("#/settings/model");
    expect((await screen.findAllByText(/managed llama\.cpp b9966 \(cpu\) is installed/i)).length).toBeGreaterThanOrEqual(1);

    await waitFor(() => {
      const statusCalls = fetch.mock.calls.filter(([url]) => String(url).includes("/api/llm/status"));
      expect(statusCalls.length).toBeGreaterThanOrEqual(2);
    });
  });

  it("shows voice worker status as readouts and hides unavailable controls", async () => {
    installFetch();
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    go("#/settings/voice");
    expect(await screen.findByRole("heading", { name: /^voice$/i })).toBeInTheDocument();
    // Both roles are visible with a dot+text state, even with voice off. The
    // worker rows sit inside the role sections, so they are labeled "Worker".
    expect(screen.getAllByText(/speech output \(tts\)/i).length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText(/speech input \(asr\)/i).length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText(/^worker$/i).length).toBeGreaterThanOrEqual(2);
    expect(screen.getAllByText(/^disabled$/i).length).toBeGreaterThanOrEqual(2);
    // A missing/disabled worker never blocks the app or adds a row of unusable controls.
    expect(screen.queryByRole("button", { name: /^(start|stop|restart|load model|unload model|send test)$/i })).toBeNull();
    expect(screen.getByRole("button", { name: /emergency stop/i })).toBeEnabled();
  });

  it("preserves hidden custom worker arguments when another provider is selected", async () => {
    const fetch = installFetch();
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    go("#/settings/voice");

    const providers = await screen.findAllByRole("combobox", { name: /provider/i });
    fireEvent.change(providers[0], { target: { value: "custom" } });
    fireEvent.change(await screen.findByRole("textbox", { name: /worker arguments/i }), {
      target: {
        value: "-server-path\nC:\\Program Files\\MagicHandy\\parakeet-server.exe\n-server-model\nC:\\Users\\Test User\\AppData\\Roaming\\MagicHandy\\voice\\parakeet\\model.gguf",
      },
    });
    fireEvent.change(providers[0], { target: { value: "none" } });
    fireEvent.click(screen.getByRole("button", { name: /save settings/i }));

    await waitFor(() => {
      expect(fetch.mock.calls.some(([, init]) => (init as RequestInit | undefined)?.method === "PUT")).toBe(true);
    });
    const [, request] = fetch.mock.calls.find(([, init]) => (init as RequestInit | undefined)?.method === "PUT") ?? [];
    const payload = JSON.parse(String((request as RequestInit).body));
    expect(payload.voice.asr_worker_args).toEqual([
      "-server-path",
      "C:\\Program Files\\MagicHandy\\parakeet-server.exe",
      "-server-model",
      "C:\\Users\\Test User\\AppData\\Roaming\\MagicHandy\\voice\\parakeet\\model.gguf",
    ]);
  });

  it("discloses only fields for the selected voice provider and keeps status visible", async () => {
    installFetch();
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    go("#/settings/voice");
    const providers = await screen.findAllByRole("combobox", { name: /provider/i });

    expect(screen.queryByLabelText(/^api key/i)).toBeNull();
    fireEvent.change(providers[1], { target: { value: "elevenlabs" } });
    expect(screen.getByLabelText(/^api key/i)).toHaveAttribute("type", "password");
    expect(screen.getByLabelText(/voice id/i)).toBeInTheDocument();
    expect(screen.queryByLabelText(/reference transcript/i)).toBeNull();

    fireEvent.change(providers[1], { target: { value: "neutts_air" } });
    expect(screen.getByLabelText(/reference transcript/i)).toBeInTheDocument();
    expect(screen.queryByLabelText(/^api key/i)).toBeNull();
    expect(screen.getAllByText(/^disabled$/i).length).toBeGreaterThanOrEqual(2);
  });

  it("hides the chat voice controls when voice is not configured", async () => {
    installFetch();
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    expect(screen.queryByRole("button", { name: /hold to talk/i })).toBeNull();
    expect(screen.queryByText(/speak replies/i)).toBeNull();
  });

  it("disables the mic button when the ASR worker is not running", async () => {
    const state = {
      ...baseState,
      settings: { ...baseState.settings, voice: { ...baseState.settings.voice, enabled: true, asr_provider: "parakeet_managed" } },
      voice: { enabled: true, protocol_version: 1, workers: { asr: { role: "asr", state: "stopped", configured: true, worker_queue_depth: 0, queue_depth: 0 } } },
    };
    installFetch({ state });
    renderApp();
    const mic = await screen.findByRole("button", { name: /hold to talk/i });
    expect(mic).toBeDisabled();
    expect(mic.getAttribute("title")).toMatch(/settings/i);
  });

  it("keeps the speak-replies toggle hidden while voice workers are globally disabled", async () => {
    const state = {
      ...baseState,
      settings: { ...baseState.settings, voice: { ...baseState.settings.voice, enabled: false, tts_provider: "elevenlabs" } },
    };
    installFetch({ state });
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    expect(screen.queryByText(/speak replies/i)).toBeNull();
  });

  it("shows a status-bar voice readout only when speak-replies cannot deliver", async () => {
    const state = {
      ...baseState,
      settings: { ...baseState.settings, voice: { ...baseState.settings.voice, enabled: true, tts_provider: "elevenlabs", speak_replies: true } },
      voice: { enabled: true, protocol_version: 1, workers: { tts: { role: "tts", state: "stopped", configured: true, worker_queue_depth: 0, queue_depth: 0 } } },
    };
    installFetch({ state });
    renderApp();
    expect(await screen.findByText(/voice not ready/i)).toBeInTheDocument();
    // Voice remains a readout; the connection disclosure is the only top-bar control.
    const status = screen.getByRole("region", { name: /status/i });
    expect(within(status).getAllByRole("button")).toHaveLength(1);
    expect(within(status).getByRole("button", { name: /open connection manager/i })).toBeInTheDocument();
  });

  it("locks worker controls behind unsaved voice edits", async () => {
    const state = {
      ...baseState,
      settings: { ...baseState.settings, voice: { ...baseState.settings.voice, enabled: true, tts_provider: "custom", tts_worker_path: "C:/workers/tts.exe" } },
    };
    installFetch({
      state,
      voiceStatus: {
        voice: { enabled: true, protocol_version: 1, workers: { tts: { role: "tts", state: "stopped", configured: true, worker_queue_depth: 0, queue_depth: 0 }, asr: { role: "asr", state: "disabled", configured: false, worker_queue_depth: 0, queue_depth: 0 } } },
        requests: [],
      },
    });
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    go("#/settings/voice");
    const start = await screen.findByRole("button", { name: /^start$/i });
    expect(start).toBeEnabled();
    const providers = screen.getAllByRole("combobox", { name: /provider/i });
    fireEvent.change(providers[1], { target: { value: "elevenlabs" } });
    expect(await screen.findByText(/save settings to apply/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^start$/i })).toBeDisabled();
  });

  it("locks settings, prompt, and memory mutations for read-only clients", async () => {
    installFetch({ state: {
      ...baseState,
      controller: { active: false, read_only: true },
      settings: { ...baseState.settings, device: { ...baseState.settings.device, connection_key_set: true } },
    } });
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    go("#/settings/device");
    expect(await screen.findByRole("button", { name: /save settings/i })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: /the handy not checked/i }));
    const manager = screen.getByRole("region", { name: /connection manager/i });
    expect(within(manager).getByLabelText(/handy connection key/i)).toBeDisabled();
    expect(within(manager).getByRole("button", { name: /save key/i })).toBeDisabled();
    expect(within(manager).getByRole("button", { name: /check connection/i })).toBeDisabled();
    go("#/settings/prompts");
    expect(await screen.findByRole("button", { name: /duplicate as new/i })).toBeDisabled();
    expect(await screen.findByRole("button", { name: /add memory/i })).toBeDisabled();
  });

  it("shows browser Bluetooth bridge controls when that dispatch owner is selected", async () => {
    installFetch({
      state: {
        ...baseState,
        settings: { ...baseState.settings, device: { ...baseState.settings.device, hsp_dispatch_owner: "browser_bluetooth" } },
        bluetooth_bridge: { connected: false, ready: false, status: "disconnected", message: "Bluetooth disconnected" },
      },
    });
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    fireEvent.click(screen.getByRole("button", { name: /the handy bluetooth disconnected/i }));
    const manager = screen.getByRole("region", { name: /connection manager/i });
    expect(within(manager).getAllByText(/bluetooth disconnected/i).length).toBeGreaterThan(0);
    expect(within(manager).getByRole("button", { name: /connect bluetooth/i })).toBeInTheDocument();
    expect(within(manager).queryByLabelText(/handy connection key/i)).toBeNull();
  });

  it("reports an unreachable transport after preserving local Stop state", async () => {
    installFetch({ stopError: "stop could not reach the configured transport" });
    renderApp();
    const stop = await screen.findByRole("button", { name: /emergency stop/i });
    fireEvent.click(stop);
    expect(await screen.findByText(/stop could not reach the configured transport/i)).toBeInTheDocument();
  });

  it("shows the backend delivery error when transport Stop fails", async () => {
    installFetch({ stopError: "Intiface Stop was rejected", stopStatus: 502 });
    renderApp();
    const stop = await screen.findByRole("button", { name: /emergency stop/i });
    fireEvent.click(stop);
    expect(await screen.findByText(/Intiface Stop was rejected/i)).toBeInTheDocument();
  });

  it("connects and selects a discovered Intiface linear actuator", async () => {
    installFetch({
      state: {
        ...baseState,
        settings: { ...baseState.settings, device: { ...baseState.settings.device, hsp_dispatch_owner: "intiface" } },
        intiface_transport: {
          dispatch_owner: "intiface",
          address: "ws://127.0.0.1:12345",
          status: { connected: false, scanning: false, playback_state: "idle", max_ping_time_ms: 0, queue_depth: 0, devices: [] },
          diagnostics: {},
        },
      } as typeof baseState,
    });
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    fireEvent.click(screen.getByRole("button", { name: /the handy intiface disconnected/i }));
    fireEvent.click(await screen.findByRole("button", { name: /^connect$/i }));
    expect(await screen.findByText(/connected - select a linear actuator/i)).toBeInTheDocument();
    fireEvent.change(screen.getByRole("combobox", { name: /linear actuator/i }), { target: { value: "7:0" } });
    fireEvent.click(screen.getByRole("button", { name: /use actuator/i }));
    expect(await screen.findByText(/connected - ready/i)).toBeInTheDocument();
  });

  it("seeds chat history from the shared server log", async () => {
    installFetch({
      chatLog: [
        { seq: 1, role: "user", content: "hello from another tab", created_at: "2026-07-09T00:00:00Z" },
        { seq: 2, role: "assistant", content: "reply preserved across reloads", created_at: "2026-07-09T00:00:01Z" },
      ],
    });
    renderApp();
    expect(await screen.findByText(/hello from another tab/i)).toBeInTheDocument();
    expect(screen.getByText(/reply preserved across reloads/i)).toBeInTheDocument();
  });

  it("labels the visualizer as an engine-state image (commanded estimate)", async () => {
    installFetch();
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    expect(screen.getAllByRole("img", { name: /motion/i }).length).toBeGreaterThan(0);
    expect(screen.getByText(/commanded position estimate/i)).toBeInTheDocument();
  });
});

describe("chat stream API", () => {
  it("throws JSON error responses before trying to read an SSE body", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => ({ ok: false, status: 409, json: async () => ({ error: "read-only client" }) } as Response)));
    await expect(streamChat("hello", [], () => undefined)).rejects.toThrow("read-only client");
  });

  it("parses final message events", async () => {
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(new TextEncoder().encode('event: message\ndata: {"reply":"Hi","motion":{"action":"none"}}\n\n'));
        controller.close();
      },
    });
    vi.stubGlobal("fetch", vi.fn(async () => ({ ok: true, status: 200, body } as Response)));
    const events: Array<{ event: string }> = [];
    await streamChat("hello", [], (event) => events.push(event));
    expect(events).toEqual([expect.objectContaining({ event: "message" })]);
  });
});
