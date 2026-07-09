import { render, screen, within } from "@testing-library/react";
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
    diagnostics: { verbosity: "normal" },
    options: {
      hsp_dispatch_owners: ["cloud_rest", "browser_bluetooth"],
      api_application_id_sources: ["bundled", "developer_override"],
      diagnostics_verbosities: ["normal", "debug", "trace"],
      llm_providers: ["llama_cpp", "ollama"],
      llama_cpp_modes: ["managed", "external"],
      prompt_sets: ["default"],
    },
  },
  controller: { active: true, read_only: false },
  motion: { available: true },
  modes: {},
  memory: { enabled: true, memories: [] },
};

function jsonRes(data: unknown) {
  return { ok: true, status: 200, text: async () => JSON.stringify(data) } as Response;
}

function installFetch(opts: { state?: typeof baseState & { bluetooth_bridge?: unknown }; memory?: unknown; fail?: boolean; chatLog?: unknown[] } = {}) {
  const state = (opts.state ?? baseState) as typeof baseState & { bluetooth_bridge?: unknown };
  const chatLog = opts.chatLog ?? [];
  const fn = vi.fn(async (input: RequestInfo | URL) => {
    if (opts.fail) throw new Error("offline");
    const u = String(input);
    if (u.includes("/api/transport/bluetooth/status")) return jsonRes({ status: "success", dispatch_owner: state.settings.device.hsp_dispatch_owner, bluetooth: state.bluetooth_bridge ?? {} });
    if (u.includes("/api/chat/messages")) return jsonRes({ messages: chatLog, latest_seq: chatLog.length, cursor: 0 });
    if (u.includes("/api/chat/cursor")) return jsonRes({ cursor: chatLog.length });
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

  it("shows voice worker status as readouts and keeps controls inert while disabled", async () => {
    installFetch();
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    go("#/settings/voice");
    expect(await screen.findByRole("heading", { name: /voice workers/i })).toBeInTheDocument();
    // Both roles are visible with a dot+text state, even with voice off.
    expect(screen.getByText(/speech output \(tts\)/i)).toBeInTheDocument();
    expect(screen.getByText(/speech input \(asr\)/i)).toBeInTheDocument();
    expect(screen.getAllByText(/^disabled$/i).length).toBeGreaterThanOrEqual(2);
    // A missing/disabled worker never blocks the app; its controls are inert.
    for (const button of screen.getAllByRole("button", { name: /^start$/i })) {
      expect(button).toBeDisabled();
    }
    expect(screen.getByRole("button", { name: /emergency stop/i })).toBeEnabled();
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
