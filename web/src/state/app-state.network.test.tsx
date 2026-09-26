import { act, cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { AppState, ControllerSnapshot, MotionInfo } from "../api/types";
import { AppStateProvider, useAppState, useMotionState } from "./app-state";

vi.mock("../api/client", () => ({
  api: { getState: vi.fn(), controllerState: vi.fn(), controllerHeartbeat: vi.fn(), takeControl: vi.fn(), resumeMotion: vi.fn() },
  clientId: "network-tab", COMMAND_RECOVERED_EVENT: "magichandy:command-recovered",
}));

class NetworkEventSource {
  static instances: NetworkEventSource[] = [];
  listener?: EventListener;
  onerror?: () => void;
  close = vi.fn();
  constructor() { NetworkEventSource.instances.push(this); }
  addEventListener(_name: string, listener: EventListener) { this.listener = listener; }
  emit(value: MotionInfo) { this.listener?.(new MessageEvent("motion", { data: JSON.stringify(value) })); }
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}

const stamp = (epoch: string, revision: number) => ({ epoch, revision, observed_at: "2026-09-13T00:00:00Z" });
const control = (epoch = "boot-a", revision = 1): ControllerSnapshot => ({
  epoch, revision, generation: 1, active: true, read_only: false, heartbeat_required: true, lease_expires_in_ms: 15000,
});
const motion = (epoch: string, revision: number, running: boolean): MotionInfo => ({
  available: true, observation: stamp(epoch, revision), engine: { running },
}) as MotionInfo;
const full = (epoch = "boot-a", revision = 1, motionRevision = 2, running = false) => ({
  version: epoch, observation: stamp(epoch, revision), controller: control(epoch), motion: motion(epoch, motionRevision, running),
}) as AppState;
let current: ReturnType<typeof useAppState> & { motion: MotionInfo | null };
function Harness() { current = { ...useAppState(), motion: useMotionState() }; return null; }

beforeEach(() => {
  vi.useFakeTimers();
  vi.mocked(api.getState).mockReset().mockResolvedValue(full());
  vi.mocked(api.controllerState).mockReset().mockResolvedValue(control());
  vi.mocked(api.controllerHeartbeat).mockReset().mockResolvedValue(control("boot-a", 2));
  vi.mocked(api.takeControl).mockReset();
  vi.mocked(api.resumeMotion).mockReset();
  vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
  vi.spyOn(Math, "random").mockReturnValue(0.5);
  NetworkEventSource.instances = [];
  vi.stubGlobal("EventSource", NetworkEventSource);
});
afterEach(() => { cleanup(); vi.useRealTimers(); vi.unstubAllGlobals(); vi.restoreAllMocks(); });

describe("remote observation ordering", () => {
  it("revalidates after phone-style backgrounding without reclaiming an expired controller or resuming motion", async () => {
    vi.mocked(api.getState).mockResolvedValue(full("boot-a", 1, 2, true));
    render(<AppStateProvider><Harness /></AppStateProvider>);
    await act(async () => {});
    const previous = NetworkEventSource.instances[NetworkEventSource.instances.length - 1];
    const visibility = vi.spyOn(document, "visibilityState", "get");
    visibility.mockReturnValue("hidden");
    act(() => document.dispatchEvent(new Event("visibilitychange")));
    await act(async () => vi.advanceTimersByTimeAsync(20000));
    const resumed = deferred<AppState>();
    const expired = { ...control("boot-a", 60), active: false, read_only: true, generation: 2 };
    vi.mocked(api.controllerState).mockResolvedValue(expired);
    vi.mocked(api.controllerHeartbeat).mockResolvedValue(expired);
    vi.mocked(api.getState).mockReturnValueOnce(resumed.promise);
    const sources = NetworkEventSource.instances.length;
    visibility.mockReturnValue("visible");
    act(() => document.dispatchEvent(new Event("visibilitychange")));
    // The last view stays mounted but cannot be acted on until rediscovered.
    expect(current.state?.observation?.revision).toBe(1);
    expect(current.stale).toBe(true);
    expect(current.readOnly).toBe(true);
    expect(NetworkEventSource.instances).toHaveLength(sources + 1);
    await act(async () => resumed.resolve({ ...full("boot-a", 61, 62), controller: expired }));
    act(() => previous.emit(motion("boot-a", 999, true)));
    expect(current.motion?.engine?.running).toBe(false);
    expect(current.readOnly).toBe(true);
    expect(api.takeControl).not.toHaveBeenCalled();
    expect(api.resumeMotion).not.toHaveBeenCalled();
  });
  it("keeps one mounted view and settles after the page turns visible repeatedly", async () => {
    const states: Array<AppState | null> = [];
    function Recorder() { states.push(useAppState().state); return null; }
    // Like the backend, every state read carries a new observation revision.
    let revision = 0;
    vi.mocked(api.getState).mockImplementation(async () => full("boot-a", ++revision));
    render(<AppStateProvider><Recorder /><Harness /></AppStateProvider>);
    await act(async () => {});
    expect(current.readOnly).toBe(false);
    const loaded = states.length;
    const reads = vi.mocked(api.getState).mock.calls.length;
    const sources = NetworkEventSource.instances.length;
    const visibility = vi.spyOn(document, "visibilityState", "get");
    // An embedded or occluded browser can report a brief return every few seconds.
    for (let flicker = 0; flicker < 5; flicker++) {
      visibility.mockReturnValue("hidden");
      act(() => document.dispatchEvent(new Event("visibilitychange")));
      await act(async () => vi.advanceTimersByTimeAsync(2000));
      visibility.mockReturnValue("visible");
      act(() => document.dispatchEvent(new Event("visibilitychange")));
      await act(async () => vi.advanceTimersByTimeAsync(5));
    }
    expect(states.slice(loaded).every((state) => state !== null)).toBe(true);
    expect(vi.mocked(api.getState).mock.calls.length - reads).toBe(5);
    expect(NetworkEventSource.instances.length - sources).toBe(5);
    await act(async () => vi.advanceTimersByTimeAsync(2000));
    expect(current.stale).toBe(false);
    expect(current.readOnly).toBe(false);
  });
  it("cannot enable protected controls from a malformed controller response", async () => {
    vi.mocked(api.controllerState).mockResolvedValue({} as ControllerSnapshot);
    render(<AppStateProvider><Harness /></AppStateProvider>);
    await act(async () => {});
    expect(current.state).not.toBeNull();
    expect(current.readOnly).toBe(true);
    expect(api.controllerHeartbeat).not.toHaveBeenCalled();
  });
  it("bounds heartbeat retries to one in-flight request and cancels them when access ends", async () => {
    let active = 0;
    let peak = 0;
    vi.mocked(api.controllerHeartbeat).mockImplementation((signal) => new Promise((_resolve, reject) => {
      active++;
      peak = Math.max(peak, active);
      signal!.addEventListener("abort", () => { active--; reject(new DOMException("Aborted", "AbortError")); }, { once: true });
    }));
    const rendered = render(<AppStateProvider><Harness /></AppStateProvider>);
    await act(async () => vi.advanceTimersByTimeAsync(9000));
    expect(peak).toBe(1);
    expect(api.controllerHeartbeat).toHaveBeenCalledTimes(2);
    expect(current.readOnly).toBe(true);
    rendered.rerender(<AppStateProvider enabled={false}><Harness /></AppStateProvider>);
    await act(async () => vi.advanceTimersByTimeAsync(20000));
    expect(active).toBe(0);
    expect(api.controllerHeartbeat).toHaveBeenCalledTimes(2);
  });

  it("owns one reconnect timer and rejects callbacks from a failed source", async () => {
    render(<AppStateProvider><Harness /></AppStateProvider>);
    await act(async () => {});
    const previous = NetworkEventSource.instances[NetworkEventSource.instances.length - 1];
    const count = NetworkEventSource.instances.length;
    act(() => { previous.onerror?.(); previous.onerror?.(); });
    await act(async () => vi.advanceTimersByTimeAsync(1000));
    expect(NetworkEventSource.instances).toHaveLength(count + 1);
    const next = NetworkEventSource.instances[NetworkEventSource.instances.length - 1];
    act(() => next.emit(motion("boot-a", 20, false)));
    act(() => previous.emit(motion("boot-a", 99, true)));
    expect(current.motion?.observation?.revision).toBe(20);
    expect(current.motion?.engine?.running).toBe(false);
  });
  it("keeps foreground heartbeats independent of a state read blocked beyond the lease period", async () => {
    vi.mocked(api.getState).mockReturnValue(new Promise(() => {}));
    render(<AppStateProvider><Harness /></AppStateProvider>);
    await act(async () => vi.advanceTimersByTimeAsync(20000));
    expect(api.controllerState).toHaveBeenCalledOnce();
    expect(vi.mocked(api.controllerHeartbeat).mock.calls.length).toBeGreaterThanOrEqual(8);
    expect(api.getState).toHaveBeenCalledOnce();
    expect(current.readOnly).toBe(true); // No usable full snapshot has arrived.
  });

  it("rejects an older event even when that packet arrives last", async () => {
    render(<AppStateProvider><Harness /></AppStateProvider>);
    await act(async () => {});
    const source = NetworkEventSource.instances[NetworkEventSource.instances.length - 1];
    act(() => source.emit(motion("boot-a", 10, true)));
    act(() => source.emit(motion("boot-a", 9, false)));
    expect(current.motion?.observation?.revision).toBe(10);
    expect(current.motion?.engine?.running).toBe(true);
  });

  it("does not restore an older snapshot or clear freshness loss using a pre-failure request", async () => {
    render(<AppStateProvider><Harness /></AppStateProvider>);
    await act(async () => {});
    const source = NetworkEventSource.instances[NetworkEventSource.instances.length - 1];
    const earlier = deferred<AppState>();
    const afterFailure = deferred<AppState>();
    vi.mocked(api.getState).mockReturnValueOnce(earlier.promise).mockReturnValueOnce(afterFailure.promise);
    let pending!: Promise<void>;
    act(() => { pending = current.refresh(); });
    act(() => source.emit(motion("boot-a", 10, true)));
    act(() => source.onerror?.());
    expect(current.motion?.observation?.revision).toBe(10);
    expect(current.readOnly).toBe(true);
    await act(async () => { earlier.resolve(full("boot-a", 8, 9)); await pending; });
    expect(current.motion?.observation?.revision).toBe(10);
    expect(current.stale).toBe(true);
    await act(async () => afterFailure.resolve(full("boot-a", 11, 12)));
    expect(current.motion?.observation?.revision).toBe(12);
    expect(current.motion?.engine?.running).toBe(false);
    expect(current.stale).toBe(false);
  });

  it("resyncs a new boot and ignores queued motion from the retired event stream", async () => {
    render(<AppStateProvider><Harness /></AppStateProvider>);
    await act(async () => {});
    const previous = NetworkEventSource.instances[NetworkEventSource.instances.length - 1];
    act(() => previous.emit(motion("boot-a", 50, true)));
    const restarted = deferred<AppState>();
    vi.mocked(api.getState).mockReturnValueOnce(restarted.promise);
    act(() => previous.emit(motion("boot-b", 1, false)));
    expect(previous.close).toHaveBeenCalled();
    expect(current.state).toBeNull();
    expect(current.readOnly).toBe(true);
    await act(async () => restarted.resolve(full("boot-b", 2, 3)));
    act(() => previous.emit(motion("boot-a", 900, true)));
    expect(current.state?.observation?.epoch).toBe("boot-b");
    expect(current.motion?.observation?.epoch).toBe("boot-b");
    expect(current.motion?.engine?.running).toBe(false);
  });
});
