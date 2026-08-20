import { act, fireEvent, render, screen, within } from "@testing-library/react";
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
    state: { settings: { motion: app.motion, options: { handy_models: ["handy_original", "handy_2_standard", "handy_2_pro"] } } },
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
  apply_video_speed_limit: false,
  style: "balanced",
  handy_model: "handy_original",
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

  it("keeps pattern area focus automatic rather than exposing a manual limit", () => {
    render(<QuickSettings section="limits" />);

    expect(screen.queryByRole("slider", { name: "Focus minimum" })).not.toBeInTheDocument();
    expect(screen.queryByRole("slider", { name: "Focus maximum" })).not.toBeInTheDocument();
    expect(screen.getByRole("slider", { name: "Speed minimum" })).toBeInTheDocument();
    expect(screen.getByRole("slider", { name: "Stroke minimum" })).toBeInTheDocument();
    expect(screen.queryByRole("radiogroup", { name: "Handy model" })).not.toBeInTheDocument();
  });

  it("persists the documented Handy model calibration from the connection menu", async () => {
    applyQuick.mockResolvedValue(undefined);
    render(<QuickSettings section="connection" />);

    const model = screen.getByRole("radiogroup", { name: "Handy model" });
    expect(within(model).getByRole("radio", { name: "Original" })).toBeChecked();
    expect(within(model).getByRole("radio", { name: "2 Pro" })).not.toBeChecked();
    expect(screen.getByText("110 mm · 32–400 mm/s")).toBeInTheDocument();
    fireEvent.click(within(model).getByRole("radio", { name: "2 Pro" }));
    await act(async () => vi.advanceTimersByTimeAsync(180));

    expect(applyQuick).toHaveBeenCalledWith({ handy_model: "handy_2_pro" });
    expect(within(model).getByRole("radio", { name: "2 Pro" })).toBeChecked();
    expect(screen.getByText("125 mm · 32–450 mm/s")).toBeInTheDocument();

    fireEvent.click(within(model).getByRole("radio", { name: "Original" }));
    await act(async () => vi.advanceTimersByTimeAsync(180));

    expect(applyQuick).toHaveBeenLastCalledWith({ handy_model: "handy_original" });
    expect(within(model).getByRole("radio", { name: "Original" })).toBeChecked();
    expect(screen.getByText("110 mm · 32–400 mm/s")).toBeInTheDocument();
  });

  it("places direction with connection limits and keeps style separate", async () => {
    applyQuick.mockResolvedValue(undefined);
    const result = render(<QuickSettings section="connection" />);

    expect(screen.getByRole("slider", { name: "Speed minimum" })).toBeInTheDocument();
    expect(screen.getByRole("slider", { name: "Stroke minimum" })).toBeInTheDocument();
    expect(within(screen.getByRole("radiogroup", { name: "Handy model" })).getByRole("radio", { name: "Original" })).toBeChecked();
    const reverse = screen.getByRole("switch", { name: "Reverse direction" });
    expect(screen.queryByRole("combobox", { name: /style/i })).not.toBeInTheDocument();

    fireEvent.click(reverse);
    await act(async () => vi.advanceTimersByTimeAsync(180));
    expect(applyQuick).toHaveBeenCalledWith({ reverse_direction: true });

    result.rerender(<QuickSettings section="style" />);
    expect(screen.queryByRole("switch", { name: "Reverse direction" })).not.toBeInTheDocument();
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
    expect(screen.getByRole("slider", { name: "Style" })).toHaveAttribute("aria-valuetext", "Balanced");
  });

  it("reverts the latest optimistic value when the patch fails", async () => {
    applyQuick.mockRejectedValue(new Error("backend rejected the range"));
    render(<QuickSettings section="limits" />);
    fireEvent.change(screen.getByRole("slider", { name: "Speed minimum" }), { target: { value: "20" } });

    await act(async () => vi.advanceTimersByTimeAsync(180));

    expect(screen.getByRole("slider", { name: "Speed minimum" })).toHaveValue("10");
    expect(app.show).toHaveBeenCalledWith("backend rejected the range", "error");
  });

  it("flushes a pending edit on unmount", async () => {
    applyQuick.mockResolvedValue(undefined);
    const result = render(<QuickSettings section="limits" />);
    fireEvent.change(screen.getByRole("slider", { name: "Speed minimum" }), { target: { value: "20" } });

    result.unmount();
    await act(async () => Promise.resolve());

    expect(applyQuick).toHaveBeenCalledWith({ speed_min_percent: 20 });
  });

  it("flushes an edit queued behind an in-flight request after unmount", async () => {
    let resolveFirst!: () => void;
    applyQuick
      .mockImplementationOnce(() => new Promise<void>((resolve) => { resolveFirst = resolve; }))
      .mockResolvedValueOnce(undefined);
    const result = render(<QuickSettings section="limits" />);

    fireEvent.change(screen.getByRole("slider", { name: "Speed minimum" }), { target: { value: "20" } });
    await act(async () => vi.advanceTimersByTimeAsync(180));
    fireEvent.change(screen.getByRole("slider", { name: "Speed maximum" }), { target: { value: "35" } });
    result.unmount();

    await act(async () => {
      resolveFirst();
      await Promise.resolve();
    });

    expect(applyQuick).toHaveBeenCalledTimes(2);
    expect(applyQuick).toHaveBeenLastCalledWith({ speed_max_percent: 35 });
  });
});
