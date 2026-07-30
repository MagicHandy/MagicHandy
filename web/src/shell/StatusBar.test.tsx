import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StatusBar } from "./StatusBar";

const mocks = vi.hoisted(() => ({
  app: {
    backendOnline: true,
    motion: {},
    readOnly: true,
    refresh: vi.fn(),
    state: {
      controller: {
        active: false,
        read_only: true,
        takeover_in_progress: false,
      },
    },
  },
  show: vi.fn(),
  takeControl: vi.fn(),
  stopAllAudioPlayback: vi.fn(),
}));

vi.mock("../api/client", () => {
  class MockApiError extends Error {}
  return {
    api: { takeControl: mocks.takeControl },
    ApiError: MockApiError,
  };
});
vi.mock("../components/MotionVisualizer", () => ({
  MotionVisualizer: () => <div>Motion visualizer</div>,
}));
vi.mock("../state/app-state", () => ({
  useAppState: () => mocks.app,
  useToast: () => ({ show: mocks.show }),
}));
vi.mock("../util/audio", () => ({
  stopAllAudioPlayback: mocks.stopAllAudioPlayback,
}));
vi.mock("./ConnectionManager", () => ({
  ConnectionManager: () => <div>Connection manager</div>,
}));
vi.mock("./NotificationCenter", () => ({
  NotificationCenter: () => <div>Notification center</div>,
}));

describe("StatusBar controller handoff", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.app.backendOnline = true;
    mocks.app.readOnly = true;
    mocks.app.state = {
      controller: {
        active: false,
        read_only: true,
        takeover_in_progress: false,
      },
    };
    mocks.takeControl.mockResolvedValue({
      controller: { active: true, read_only: false },
      changed: true,
      stop_confirmed: true,
      stop_sequence: 2,
    });
    vi.spyOn(window, "confirm").mockReturnValue(true);
  });

  it("requires confirmation, stops browser-owned work, and takes control", async () => {
    const emergencyStop = vi.fn();
    window.addEventListener("magichandy:emergency-stop", emergencyStop);
    render(<StatusBar />);

    fireEvent.click(screen.getByRole("button", { name: "Take control" }));

    await waitFor(() => {
      expect(mocks.takeControl).toHaveBeenCalledOnce();
      expect(mocks.app.refresh).toHaveBeenCalledOnce();
    });
    expect(window.confirm).toHaveBeenCalledWith(expect.stringContaining("Active motion"));
    expect(mocks.stopAllAudioPlayback).toHaveBeenCalledOnce();
    expect(emergencyStop).toHaveBeenCalledOnce();
    expect(mocks.show).toHaveBeenCalledWith("This tab now controls MagicHandy.", "success");
    window.removeEventListener("magichandy:emergency-stop", emergencyStop);
  });

  it("does not request control when confirmation is declined", () => {
    vi.mocked(window.confirm).mockReturnValue(false);
    render(<StatusBar />);

    fireEvent.click(screen.getByRole("button", { name: "Take control" }));

    expect(mocks.takeControl).not.toHaveBeenCalled();
    expect(mocks.stopAllAudioPlayback).not.toHaveBeenCalled();
  });

  it("locks the action while another handoff is in progress", () => {
    mocks.app.state.controller.takeover_in_progress = true;
    render(<StatusBar />);

    expect(screen.getByRole("button", { name: "Controller handoff in progress" })).toBeDisabled();
  });

  it("warns when ownership transfers without a confirmed physical Stop", async () => {
    mocks.takeControl.mockResolvedValue({
      controller: { active: true, read_only: false },
      changed: true,
      stop_confirmed: false,
      stop_sequence: 3,
      warning: "physical Stop timed out",
    });
    render(<StatusBar />);

    fireEvent.click(screen.getByRole("button", { name: "Take control" }));

    await waitFor(() => expect(mocks.show).toHaveBeenCalledWith(
      "Control transferred, but physical Stop could not be confirmed.",
      "warning",
    ));
  });

  it("keeps the active controller as a status readout instead of an action", () => {
    mocks.app.readOnly = false;
    mocks.app.state.controller = {
      active: true,
      read_only: false,
      takeover_in_progress: false,
    };
    render(<StatusBar />);

    expect(screen.getByLabelText("This tab is the controller")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Take control" })).not.toBeInTheDocument();
  });
});
