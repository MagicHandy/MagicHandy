import { act, fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { ManualMotionTest } from "./ManualMotionTest";

const app = vi.hoisted(() => ({
  backendOnline: true,
  readOnly: false,
  state: { settings: { motion: { speed_min_percent: 1, speed_max_percent: 100 } } },
  motion: {
    engine: {
      running: true,
      paused: false,
      target: { source: "autopilot" },
    },
  },
  refresh: vi.fn(),
  show: vi.fn(),
}));

vi.mock("../api/client", () => ({
  api: {
    startManualTest: vi.fn(),
    stopMotion: vi.fn(),
  },
}));

vi.mock("../state/app-state", () => ({
  useMotionState: () => app.motion,
  useAppState: () => app,
  useToast: () => ({ show: app.show }),
}));

const startManualTest = vi.mocked(api.startManualTest);
const stopMotion = vi.mocked(api.stopMotion);

describe("ManualMotionTest", () => {
  beforeEach(() => {
    app.backendOnline = true;
    app.readOnly = false;
    app.state.settings.motion = { speed_min_percent: 1, speed_max_percent: 100 };
    app.motion = {
      engine: {
        running: true,
        paused: false,
        target: { source: "autopilot" },
      },
    };
    app.refresh.mockReset();
    app.show.mockReset();
    startManualTest.mockReset();
    stopMotion.mockReset();
  });

  it("does not present Autopilot motion as an active manual test", () => {
    render(<ManualMotionTest />);

    expect(screen.getByRole("button", { name: "Start test" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Stop test" })).toBeDisabled();
  });

  it("uses backend target provenance for active manual-test controls", async () => {
    app.motion.engine.target.source = "manual_ui";
    stopMotion.mockResolvedValue({});
    render(<ManualMotionTest />);

    expect(screen.getByRole("button", { name: "Restart test" })).toBeEnabled();
    const stop = screen.getByRole("button", { name: "Stop test" });
    expect(stop).toBeEnabled();

    await act(async () => stop.click());
    expect(stopMotion).toHaveBeenCalledOnce();
    expect(app.refresh).toHaveBeenCalledOnce();
  });

  it.each([
    ["Full sweeps", "flow-full-sweeps"],
    ["Base-anchored variety", "flow-base-anchored"],
    ["Pace wave", "flow-pace-wave"],
  ])("starts the supported %s pattern", async (name, pattern) => {
    startManualTest.mockResolvedValue({});
    render(<ManualMotionTest />);

    expect(screen.getByRole("option", { name })).toHaveValue(pattern);
    fireEvent.change(screen.getByRole("combobox", { name: "Pattern" }), { target: { value: pattern } });
    await act(async () => screen.getByRole("button", { name: "Start test" }).click());

    expect(startManualTest).toHaveBeenCalledWith({ pattern, speed_percent: 25 });
  });

  it("keeps the displayed and submitted speed within current backend limits", async () => {
    startManualTest.mockResolvedValue({});
    app.state.settings.motion = { speed_min_percent: 5, speed_max_percent: 20 };
    const view = render(<ManualMotionTest />);
    const speed = screen.getByRole("slider", { name: "Speed" });
    expect(speed).toHaveValue("20");
    expect(speed).toHaveAttribute("min", "5");
    expect(speed).toHaveAttribute("max", "20");

    fireEvent.change(speed, { target: { value: "15" } });
    app.state.settings.motion = { speed_min_percent: 5, speed_max_percent: 10 };
    view.rerender(<ManualMotionTest />);
    expect(speed).toHaveValue("10");
    await act(async () => screen.getByRole("button", { name: "Start test" }).click());
    expect(startManualTest).toHaveBeenLastCalledWith({ pattern: "flow-full-sweeps", speed_percent: 10 });

    app.state.settings.motion = { speed_min_percent: 30, speed_max_percent: 40 };
    view.rerender(<ManualMotionTest />);
    expect(speed).toHaveValue("30");
    await act(async () => screen.getByRole("button", { name: "Start test" }).click());
    expect(startManualTest).toHaveBeenLastCalledWith({ pattern: "flow-full-sweeps", speed_percent: 30 });
  });
});
