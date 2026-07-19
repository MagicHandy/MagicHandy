import { act, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { ManualMotionTest } from "./ManualMotionTest";

const app = vi.hoisted(() => ({
  backendOnline: true,
  readOnly: false,
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
  useAppState: () => app,
  useToast: () => ({ show: app.show }),
}));

const startManualTest = vi.mocked(api.startManualTest);
const stopMotion = vi.mocked(api.stopMotion);

describe("ManualMotionTest", () => {
  beforeEach(() => {
    app.backendOnline = true;
    app.readOnly = false;
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
});
