import { act, cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { AppState, MotionInfo } from "../api/types";
import { AppStateProvider, useAppState } from "./app-state";

vi.mock("../api/client", () => ({ api: { getState: vi.fn() }, clientId: "test-tab" }));

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
let current: ReturnType<typeof useAppState>;
function Harness() { current = useAppState(); return null; }
const view = (enabled = true) => <AppStateProvider enabled={enabled}><Harness /></AppStateProvider>;

beforeEach(() => {
  vi.useFakeTimers();
  vi.mocked(api.getState).mockReset().mockResolvedValue(snapshot());
  FakeEventSource.instances = [];
  vi.stubGlobal("EventSource", FakeEventSource);
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("backend snapshot lifecycle", () => {
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
