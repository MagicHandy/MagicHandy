import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { MotionSettings } from "../api/types";
import { QuickSettings } from "./QuickSettings";

const app = vi.hoisted(() => ({
  motion: null as MotionSettings | null,
  refresh: vi.fn(),
  show: vi.fn(),
}));

vi.mock("../api/client", () => ({
  api: { applyQuick: vi.fn() },
}));

vi.mock("../state/app-state", () => ({
  useAppState: () => ({
    state: { settings: { motion: app.motion } },
    backendOnline: true,
    readOnly: false,
    refresh: app.refresh,
  }),
  useToast: () => ({ show: app.show }),
}));

const applyQuick = vi.mocked(api.applyQuick);
const initialMotion: MotionSettings = {
  speed_min_percent: 10,
  speed_max_percent: 40,
  stroke_min_percent: 20,
  stroke_max_percent: 80,
  reverse_direction: false,
  style: "balanced",
};

describe("QuickSettings", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    app.motion = { ...initialMotion };
    app.refresh.mockReset();
    app.show.mockReset();
    applyQuick.mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("does not let stale polls overwrite an unconfirmed local edit", async () => {
    let resolveFirst!: () => void;
    applyQuick.mockImplementationOnce(() => new Promise<void>((resolve) => { resolveFirst = resolve; }));
    applyQuick.mockResolvedValueOnce(undefined);
    const result = render(<QuickSettings section="limits" />);
    const minimum = screen.getByRole("slider", { name: "Speed minimum" });

    fireEvent.change(minimum, { target: { value: "20" } });
    app.motion = { ...initialMotion };
    result.rerender(<QuickSettings section="limits" />);
    expect(screen.getByRole("slider", { name: "Speed minimum" })).toHaveValue("20");

    await act(async () => vi.advanceTimersByTimeAsync(180));
    expect(applyQuick).toHaveBeenCalledWith({ speed_min_percent: 20 });

    fireEvent.change(screen.getByRole("slider", { name: "Speed minimum" }), { target: { value: "22" } });
    app.motion = { ...initialMotion };
    result.rerender(<QuickSettings section="limits" />);
    expect(screen.getByRole("slider", { name: "Speed minimum" })).toHaveValue("22");

    await act(async () => {
      resolveFirst();
      await Promise.resolve();
      await vi.runOnlyPendingTimersAsync();
    });
    expect(applyQuick).toHaveBeenLastCalledWith({ speed_min_percent: 22 });

    app.motion = { ...initialMotion, speed_min_percent: 22 };
    result.rerender(<QuickSettings section="limits" />);
    expect(screen.getByRole("slider", { name: "Speed minimum" })).toHaveValue("22");

    app.motion = { ...initialMotion, speed_min_percent: 25 };
    result.rerender(<QuickSettings section="limits" />);
    expect(screen.getByRole("slider", { name: "Speed minimum" })).toHaveValue("25");
  });

  it("reverts the latest optimistic value when the patch fails", async () => {
    applyQuick.mockRejectedValue(new Error("backend rejected the range"));
    render(<QuickSettings section="limits" />);
    fireEvent.change(screen.getByRole("slider", { name: "Speed minimum" }), { target: { value: "20" } });

    await act(async () => vi.advanceTimersByTimeAsync(180));

    expect(screen.getByRole("slider", { name: "Speed minimum" })).toHaveValue("10");
    expect(app.show).toHaveBeenCalledWith("backend rejected the range", "error");
  });

  it("clears its debounce timer on unmount", async () => {
    const result = render(<QuickSettings section="limits" />);
    fireEvent.change(screen.getByRole("slider", { name: "Speed minimum" }), { target: { value: "20" } });

    result.unmount();
    await act(async () => vi.advanceTimersByTimeAsync(500));

    expect(applyQuick).not.toHaveBeenCalled();
  });
});
