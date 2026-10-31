import { act, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { MotionInfo } from "../api/types";
import { api } from "../api/client";
import { StartupRecoveryNotice } from "./StartupRecoveryNotice";

const app = vi.hoisted(() => ({
  backendOnline: true, readOnly: false,
  motion: null as MotionInfo | null,
  refresh: vi.fn(), show: vi.fn(),
}));
vi.mock("../api/client", () => ({ api: { resetMotionPosition: vi.fn() } }));
vi.mock("../state/app-state", () => ({
  useMotionState: () => app.motion, useAppState: () => app, useToast: () => ({ show: app.show }),
}));

beforeEach(() => {
  vi.resetAllMocks();
  app.backendOnline = true;
  app.readOnly = false;
  app.motion = { available: true, engine: { running: false, paused: false, startup_recovery: { id: "failure-one", kind: "arrival_not_reached", resetting: false } } };
});

describe("StartupRecoveryNotice", () => {
  it("offers a stopped recheck only and lets backend state clear the notice", async () => {
    vi.mocked(api.resetMotionPosition).mockResolvedValue({ available: true });
    const view = render(<StartupRecoveryNotice />);
    expect(screen.getByText("The device did not settle at the requested starting position.")).toBeVisible();
    expect(screen.queryByRole("button", { name: /override|resume|play/i })).not.toBeInTheDocument();
    await act(async () => screen.getByRole("button", { name: "Reset and recheck" }).click());
    expect(api.resetMotionPosition).toHaveBeenCalledOnce();
    expect(api.resetMotionPosition).toHaveBeenCalledWith("failure-one");
    expect(app.refresh).toHaveBeenCalledOnce();
    expect(screen.getByText("Position needs a recheck")).toBeVisible();
    app.motion = { available: true };
    view.rerender(<StartupRecoveryNotice />);
    expect(screen.queryByText("Position needs a recheck")).not.toBeInTheDocument();
  });

  it.each(["readOnly", "offline", "backendBusy"])("disables reset when %s", (mode) => {
    app.readOnly = mode === "readOnly";
    app.backendOnline = mode !== "offline";
    if (mode === "backendBusy") app.motion!.engine!.startup_recovery!.resetting = true;
    render(<StartupRecoveryNotice />);
    expect(screen.getByRole("button")).toBeDisabled();
    expect(api.resetMotionPosition).not.toHaveBeenCalled();
  });

  it("does not double-send and leaves a failed reset recoverable", async () => {
    let reject!: (reason: Error) => void;
    vi.mocked(api.resetMotionPosition).mockImplementation(() => new Promise((_resolve, failed) => { reject = failed; }));
    render(<StartupRecoveryNotice />);
    const button = screen.getByRole("button");
    act(() => { button.click(); button.click(); });
    expect(api.resetMotionPosition).toHaveBeenCalledOnce();
    expect(button).toHaveTextContent("Rechecking…");
    await act(async () => reject(new Error("Device remains in motion")));
    expect(button).toBeEnabled();
    expect(app.show).toHaveBeenCalledWith("Device remains in motion", "error");
    expect(screen.getByText("Position needs a recheck")).toBeVisible();
  });

  it("explains invalid calibration without displaying diagnostic payloads", () => {
    app.motion!.engine!.startup_recovery!.kind = "unsafe_geometry";
    render(<StartupRecoveryNotice />);
    expect(screen.getByText("The device reported a position outside its calibrated travel.")).toBeVisible();
  });
});
