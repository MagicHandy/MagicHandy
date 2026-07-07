import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";
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
    options: {},
  },
  controller: { active: true, read_only: false },
  motion: { available: true },
  modes: {},
  memory: { enabled: true, memories: [] },
};

function jsonRes(data: unknown) {
  return { ok: true, status: 200, text: async () => JSON.stringify(data) } as Response;
}

function installFetch(opts: { state?: unknown; fail?: boolean } = {}) {
  const state = opts.state ?? baseState;
  const fn = vi.fn(async (input: RequestInfo | URL) => {
    if (opts.fail) throw new Error("offline");
    const u = String(input);
    if (u.includes("/api/settings")) return jsonRes({ settings: baseState.settings });
    if (u.includes("/api/memory")) return jsonRes(baseState.memory);
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
  window.location.hash = hash;
  window.dispatchEvent(new Event("hashchange"));
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

  it("labels the visualizer as an engine-state image (commanded estimate)", async () => {
    installFetch();
    renderApp();
    await screen.findByRole("button", { name: /emergency stop/i });
    expect(screen.getAllByRole("img", { name: /motion/i }).length).toBeGreaterThan(0);
    expect(screen.getByText(/commanded position estimate/i)).toBeInTheDocument();
  });
});
