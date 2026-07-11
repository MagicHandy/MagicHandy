import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { act } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";
import { streamChat } from "./api/client";
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
    device: { hsp_dispatch_owner: "cloud_rest", firmware_api_requirement: "v4/v3", api_application_id_source: "bundled", connection_key_set: false },
    motion: { speed_min_percent: 20, speed_max_percent: 80, stroke_min_percent: 0, stroke_max_percent: 100, reverse_direction: false, style: "balanced" },
    llm: { provider: "llama_cpp", llama_cpp_mode: "managed", llama_cpp_base_url: "", ollama_base_url: "", model: "", prompt_set: "default", request_timeout_ms: 120000 },
    voice: { enabled: false, tts_provider: "none", asr_provider: "none", tts_worker_path: "", tts_worker_args: [], asr_worker_path: "", asr_worker_args: [], speak_replies: false, elevenlabs_key_set: false },
    diagnostics: { verbosity: "normal" },
    options: {
      hsp_dispatch_owners: ["cloud_rest", "browser_bluetooth"],
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

function jsonRes(data: unknown) {
  return { ok: true, status: 200, text: async () => JSON.stringify(data) } as Response;
}

function installFetch(opts: { state?: typeof baseState & { bluetooth_bridge?: unknown }; memory?: unknown; fail?: boolean; chatLog?: unknown[]; voiceStatus?: unknown; library?: typeof libraryFixture } = {}) {
  const state = (opts.state ?? baseState) as typeof baseState & { bluetooth_bridge?: unknown };
  const chatLog = opts.chatLog ?? [];
  const fn = vi.fn(async (input: RequestInfo | URL, _init?: RequestInit) => {
    if (opts.fail) throw new Error("offline");
    const u = String(input);
    if (u.includes("/api/transport/bluetooth/status")) return jsonRes({ status: "success", dispatch_owner: state.settings.device.hsp_dispatch_owner, bluetooth: state.bluetooth_bridge ?? {} });
    if (u.includes("/api/chat/messages")) return jsonRes({ messages: chatLog, latest_seq: chatLog.length, cursor: 0 });
    if (u.includes("/api/chat/cursor")) return jsonRes({ cursor: chatLog.length });
    if (u.includes("/api/voice/status")) return jsonRes(opts.voiceStatus ?? {});
    if (u.includes("/api/library")) return jsonRes({ library: opts.library ?? libraryFixture });
    if (u.includes("/api/settings")) return jsonRes({ settings: state.settings });
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

  it("keeps the status bar status-only (no sliders or buttons)", async () => {
    installFetch();
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    const status = screen.getByRole("region", { name: /status/i });
    expect(within(status).queryByRole("slider")).toBeNull();
    expect(within(status).queryByRole("button")).toBeNull();
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
    // The readout stays status-only: no controls join the bar with it.
    const status = screen.getByRole("region", { name: /status/i });
    expect(within(status).queryByRole("button")).toBeNull();
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
    installFetch({ state: { ...baseState, controller: { active: false, read_only: true } } });
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    go("#/settings/device");
    expect(await screen.findByRole("button", { name: /save settings/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: /check connection/i })).toBeDisabled();
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
    go("#/settings/device");
    expect(await screen.findByText(/bluetooth disconnected/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /connect bluetooth/i })).toBeInTheDocument();
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
