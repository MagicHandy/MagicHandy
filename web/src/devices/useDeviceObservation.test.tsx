import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useDeviceObservation, useDevicePoll } from "./useDeviceObservation";

const seen = (revision: number, connected: boolean, epoch = "process") => ({
  observation: { epoch, revision, observed_at: "" }, connected,
});

describe("device observations", () => {
  it("keeps a disconnect newer than both a panel read and a late full-state poll", () => {
    const hook = renderHook(({ full }) => useDeviceObservation(full), { initialProps: { full: seen(1, true) } });
    act(() => hook.result.current[1](seen(4, false)));
    act(() => hook.result.current[1](seen(3, true)));
    hook.rerender({ full: seen(2, true) });
    expect(hook.result.current[0]).toEqual(seen(4, false));
    hook.rerender({ full: seen(1, false, "replacement") });
    act(() => hook.result.current[1](seen(5, true)));
    expect(hook.result.current[0]).toEqual(seen(1, false, "replacement"));
  });

  it("has one read in flight and discards an aborted panel's late result", async () => {
    vi.useFakeTimers();
    try {
      let release!: (value: number) => void;
      const read = vi.fn((_signal: AbortSignal) => new Promise<number>(resolve => { release = resolve; }));
      const accept = vi.fn();
      const hook = renderHook(({ session }) => useDevicePoll(true, true, read, accept, session), { initialProps: { session: "one" } });
      await act(async () => { await vi.advanceTimersByTimeAsync(2000); });
      expect(read).toHaveBeenCalledTimes(1);
      const old = release;
      hook.rerender({ session: "two" });
      expect(read.mock.calls[0][0].aborted).toBe(true);
      await act(async () => { release(2); old(1); });
      expect(accept.mock.calls).toEqual([[2]]);
      await act(async () => { await vi.advanceTimersByTimeAsync(500); });
      hook.unmount();
      await act(async () => { release(3); });
      expect(accept.mock.calls).toEqual([[2]]);
    } finally { vi.useRealTimers(); }
  });
});
