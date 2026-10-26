import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { VoiceState, VoiceWorkerStatus } from "../api/types";
import { useVoiceRuntimeStatus, VOICE_STATUS_POLL_MS } from "./useVoiceRuntimeStatus";

vi.mock("../api/client", () => ({ api: { voiceStatus: vi.fn() } }));

const voiceStatus = vi.mocked(api.voiceStatus);

function snapshot(state: string) {
  const asr: VoiceWorkerStatus = { role: "asr", state, configured: true, worker_queue_depth: 0, queue_depth: 0 };
  return { voice: { enabled: true, protocol_version: 1, workers: { asr }, modules: {} } as VoiceState, requests: [] };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail; });
  return { promise, resolve, reject };
}

// Let a settled request's promise chain reach its state updates inside act().
const settle = () => act(async () => {
  for (let step = 0; step < 6; step += 1) await Promise.resolve();
});

function setHidden(hidden: boolean, announce = true) {
  Object.defineProperty(document, "hidden", { configurable: true, get: () => hidden });
  if (announce) document.dispatchEvent(new Event("visibilitychange"));
}

describe("useVoiceRuntimeStatus", () => {
  beforeEach(() => {
    voiceStatus.mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
    // Rendered hooks are unmounted after this hook runs; do not wake them.
    setHidden(false, false);
  });

  it("separates the first load, a later poll failure and recovery", async () => {
    vi.useFakeTimers();
    voiceStatus
      .mockResolvedValueOnce(snapshot("running"))
      .mockRejectedValueOnce(new TypeError("Failed to fetch"))
      .mockResolvedValueOnce(snapshot("stopped"));
    const { result, unmount } = renderHook(() => useVoiceRuntimeStatus());

    expect(result.current.loaded).toBe(false);
    expect(result.current.error).toBe("");
    await act(async () => { await Promise.resolve(); });
    expect(result.current.loaded).toBe(true);
    expect(result.current.workers.asr?.state).toBe("running");
    const firstRequestedAt = result.current.requestedAt;
    expect(firstRequestedAt).toEqual(expect.any(Number));

    await act(async () => { await vi.advanceTimersByTimeAsync(VOICE_STATUS_POLL_MS); });
    expect(result.current.error).toBe("MagicHandy could not be reached.");
    expect(result.current.workers.asr?.state).toBe("running");
    expect(result.current.receivedAt).toEqual(expect.any(Number));

    // A failed poll keeps the shown snapshot's request time.
    expect(result.current.requestedAt).toBe(firstRequestedAt);

    await act(async () => { await result.current.retry(); });
    expect(result.current.error).toBe("");
    expect(result.current.workers.asr?.state).toBe("stopped");
    expect(result.current.requestedAt).toBeGreaterThan(firstRequestedAt ?? 0);
    unmount();
  });

  it("reports a failure before any snapshot without inventing worker state", async () => {
    voiceStatus.mockRejectedValue(new Error("voice endpoint unavailable"));
    const { result } = renderHook(() => useVoiceRuntimeStatus());
    await settle();
    expect(result.current.error).toBe("voice endpoint unavailable");
    expect(result.current.loaded).toBe(false);
    expect(result.current.workers).toEqual({});
  });

  it("follows an action with a fresh read instead of reusing an older in-flight one", async () => {
    const first = deferred<ReturnType<typeof snapshot>>();
    const second = deferred<ReturnType<typeof snapshot>>();
    voiceStatus.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const { result } = renderHook(() => useVoiceRuntimeStatus());
    expect(voiceStatus).toHaveBeenCalledTimes(1);

    let refreshed = false;
    let duplicate: Promise<void> | undefined;
    act(() => {
      void result.current.refresh().then(() => { refreshed = true; });
      duplicate = result.current.refresh();
    });
    // Both callers share one follow-up request queued behind the first.
    expect(voiceStatus).toHaveBeenCalledTimes(1);

    first.resolve(snapshot("stopped"));
    await settle();
    expect(result.current.workers.asr?.state).toBe("stopped");
    expect(refreshed).toBe(false);
    expect(voiceStatus).toHaveBeenCalledTimes(2);

    second.resolve(snapshot("running"));
    await settle();
    await act(async () => { await duplicate; });
    expect(refreshed).toBe(true);
    expect(result.current.workers.asr?.state).toBe("running");
    expect(voiceStatus).toHaveBeenCalledTimes(2);
  });

  it("pauses polling while the document is hidden and refreshes on return", async () => {
    vi.useFakeTimers();
    voiceStatus.mockResolvedValue(snapshot("running"));
    const { unmount } = renderHook(() => useVoiceRuntimeStatus());
    await act(async () => { await Promise.resolve(); });
    expect(voiceStatus).toHaveBeenCalledTimes(1);

    act(() => setHidden(true));
    await act(async () => { await vi.advanceTimersByTimeAsync(VOICE_STATUS_POLL_MS * 3); });
    expect(voiceStatus).toHaveBeenCalledTimes(1);

    await act(async () => {
      setHidden(false);
      await Promise.resolve();
    });
    expect(voiceStatus).toHaveBeenCalledTimes(2);
    unmount();
  });

  it("ignores a response that arrives after unmount", async () => {
    const pending = deferred<ReturnType<typeof snapshot>>();
    voiceStatus.mockReturnValue(pending.promise);
    const { result, unmount } = renderHook(() => useVoiceRuntimeStatus());
    unmount();
    await act(async () => { pending.resolve(snapshot("running")); });
    expect(result.current.loaded).toBe(false);
  });
});
