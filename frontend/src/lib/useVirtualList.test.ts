import { describe, expect, it } from "vitest";
import { renderHook } from "@testing-library/react";
import { useVirtualList } from "./useVirtualList";

describe("useVirtualList", () => {
  it("returns full range when disabled", () => {
    const containerRef = { current: null };
    const { result } = renderHook(() =>
      useVirtualList({ count: 50, itemHeight: 80, containerRef, enabled: false }),
    );
    expect(result.current.start).toBe(0);
    expect(result.current.end).toBe(50);
    expect(result.current.totalHeight).toBe(0);
  });

  it("computes total height when enabled", () => {
    const el = document.createElement("div");
    Object.defineProperty(el, "clientHeight", { value: 400, configurable: true });
    const containerRef = { current: el };
    const { result } = renderHook(() =>
      useVirtualList({ count: 10, itemHeight: 80, containerRef, enabled: true }),
    );
    expect(result.current.totalHeight).toBe(800);
    expect(result.current.end).toBeLessThanOrEqual(10);
  });
});
