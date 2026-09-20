import { act, cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api, COMMAND_RECOVERED_EVENT } from "../api/client";
import type { AppState, MotionInfo } from "../api/types";
import { AppStateProvider, useAppState, useMotionState } from "./app-state";

vi.mock("../api/client", () => ({ api: { getState: vi.fn(), controllerState: vi.fn(), controllerHeartbeat: vi.fn() }, clientId: "test-tab", COMMAND_RECOVERED_EVENT: "magichandy:command-recovered" }));

class FakeEventSource {
  static instances: FakeEventSource[] = [];
  listener?: EventListener;
  onerror?: () => void;
  close = vi.fn();
  constructor() { FakeEventSource.instances.push(this); }
  addEventListener(_name: string, listener: EventListener) { this.listener = listener; }
  emit(motion: MotionInfo) { this.listener?.(new MessageEvent("motion", { data: JSON.stringify(motion) })); }
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
}

const idle = { available: true } as MotionInfo;
const running = { available: true, engine: { running: true } } as MotionInfo;
const snapshot = (version = "current") => ({ version, controller: { read_only: false }, motion: idle }) as AppState;
let current: ReturnType<typeof useAppState> & { motion: MotionInfo | null };
function Harness() { current = { ...useAppState(), motion: useMotionState() }; return null; }
const view = (enabled = true) => <AppStateProvider enabled={enabled}><Harness /></AppStateProvider>;

beforeEach(() => {
  vi.useFakeTimers();
  vi.mocked(api.getState).mockReset().mockResolvedValue(snapshot());
  vi.mocked(api.controllerHeartbeat).mockReset();
  vi.mocked(api.controllerState).mockReset().mockResolvedValue({ active: true, read_only: false });
  FakeEventSource.instances = [];
  vi.stubGlobal("EventSource", FakeEventSource);
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("backend snapshot lifecycle", () => {
  it("refreshes canonical state after receipt recovery and removes that listener when disabled", async () => {
    const rendered = render(view());
    await act(async () => {});
    vi.mocked(api.getState).mockResolvedValue(snapshot("reconciled"));
    await act(async () => { window.dispatchEvent(new Event(COMMAND_RECOVERED_EVENT)); });
    expect(current.state?.version).toBe("reconciled");
    const calls = vi.mocked(api.getState).mock.calls.length;
    rendered.rerender(view(false));
    await act(async () => { window.dispatchEvent(new Event(COMMAND_RECOVERED_EVENT)); });
    expect(api.getState).toHaveBeenCalledTimes(calls);
  });
  it("establishes a protected lease with a client heartbeat before enabling control", async () => {
    const state = snapshot();
    state.controller = { active: false, read_only: true, heartbeat_required: true, generation: 0, epoch: "server", revision: 1 };
    state.observation = { epoch: "server", revision: 1, observed_at: "2026-09-13T00:00:00Z" };
    vi.mocked(api.getState).mockResolvedValue(state);
    vi.mocked(api.controllerState).mockResolvedValue(state.controller);
    const heartbeat = deferred<NonNullable<AppState["controller"]>>();
    vi.mocked(api.controllerHeartbeat).mockReturnValue(heartbeat.promise);
    vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
    render(view());
    await act(async () => {});
    expect(current.state?.version).toBe("current");
    expect(current.readOnly).toBe(true);
    expect(api.controllerHeartbeat).toHaveBeenCalledOnce();
    await act(async () => heartbeat.resolve({ active: true, read_only: false, generation: 1, heartbeat_required: true, epoch: "server", revision: 2 }));
    expect(current.state?.controller?.generation).toBe(1);
    expect(current.readOnly).toBe(false);
  });

  it("does not renew protected control from a hidden document or telemetry", async () => {
    const state = snapshot();
    state.controller = { active: true, read_only: false, heartbeat_required: true, generation: 2, epoch: "server", revision: 3 };
    state.observation = { epoch: "server", revision: 1, observed_at: "2026-09-13T00:00:00Z" };
    vi.mocked(api.getState).mockResolvedValue(state);
    vi.mocked(api.controllerState).mockResolvedValue(state.controller);
    vi.mocked(api.controllerHeartbeat).mockResolvedValue(state.controller);
    const visibility = vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
    render(view());
    await act(async () => {});
    const source = FakeEventSource.instances[FakeEventSource.instances.length - 1];
    const heartbeats = vi.mocked(api.controllerHeartbeat).mock.calls.length;
    const polls = vi.mocked(api.getState).mock.calls.length;
    visibility.mockReturnValue("hidden");
    act(() => document.dispatchEvent(new Event("visibilitychange")));
    act(() => source.emit(running));
    await act(async () => vi.advanceTimersByTimeAsync(20000));
    expect(source.close).toHaveBeenCalled();
    expect(api.controllerHeartbeat).toHaveBeenCalledTimes(heartbeats);
    expect(api.getState).toHaveBeenCalledTimes(polls);
    expect(current.readOnly).toBe(true);
  });

  it("does not restart a waiting explicit refresh after access is disabled", async () => {
    const pending = deferred<AppState>();
    vi.mocked(api.getState).mockReturnValueOnce(pending.promise);
    const rendered = render(view());
    let refresh!: Promise<void>;
    act(() => { refresh = current.refresh(); });
    rendered.rerender(view(false));
    await act(async () => {
      pending.reject(new DOMException("Aborted", "AbortError"));
      await refresh;
    });
    expect(api.getState).toHaveBeenCalledTimes(1);
    expect(current.state).toBeNull();
    expect(current.stale).toBe(false);
  });

  it("starts a fresh poll immediately after re-enabling and ignores the old request", async () => {
    const old = deferred<AppState>();
    vi.mocked(api.getState).mockReturnValueOnce(old.promise);
    const rendered = render(view());
    let refresh!: Promise<void>;
    act(() => { refresh = current.refresh(); });
    rendered.rerender(view(false));
    await act(async () => { rendered.rerender(view()); });
    expect(api.getState).toHaveBeenCalledTimes(2);
    expect(current.state?.version).toBe("current");
    await act(async () => { old.resolve(snapshot("old")); await refresh; });
    expect(api.getState).toHaveBeenCalledTimes(2);
    expect(current.state?.version).toBe("current");
  });

  it("ignores queued callbacks from a closed motion event stream", async () => {
    const rendered = render(view());
    await act(async () => {});
    const oldSource = FakeEventSource.instances[0];
    rendered.rerender(view(false));
    act(() => { oldSource.emit(running); });
    expect(oldSource.close).toHaveBeenCalledOnce();
    expect(current.motion).toBeNull();
    await act(async () => { rendered.rerender(view()); });
    act(() => { FakeEventSource.instances[1].emit(running); oldSource.onerror?.(); });
    expect(current.motion).toEqual(running);
  });

  it("reconciles an older live event with a newer completed state poll", async () => {
    render(view());
    await act(async () => {});
    act(() => { FakeEventSource.instances[0].emit(running); });
    expect(current.motion).toEqual(running);
    await act(async () => { await current.refresh(); });
    expect(current.motion).toEqual(idle);
  });

  it("preserves live events received while a state poll is in flight", async () => {
    render(view());
    await act(async () => {});
    const pending = deferred<AppState>();
    vi.mocked(api.getState).mockReturnValueOnce(pending.promise);
    let refresh!: Promise<void>;
    act(() => { refresh = current.refresh(); });
    act(() => { FakeEventSource.instances[0].emit(running); });
    await act(async () => { pending.resolve(snapshot()); await refresh; });
    expect(current.motion).toEqual(running);
  });
});
