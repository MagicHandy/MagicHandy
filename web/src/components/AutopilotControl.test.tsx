import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { AutopilotControl } from "./AutopilotControl";

const app = vi.hoisted(() => ({
  backendOnline: true,
  readOnly: false,
  state: { modes: {}, settings: {} } as {
    modes: {
      mode?: string;
      segment_index?: number;
      decision_source?: string;
      motion_change_in_ms?: number;
      speech_in_ms?: number;
      motion_planned?: boolean;
      speech_waiting_playback?: boolean;
      session_arc?: { enabled: boolean; percent: number; minutes: number };
    };
    settings: Record<string, unknown>;
  },
  motion: { engine: { running: true, paused: false, target: { source: "autopilot" } } } as {
    engine?: { running?: boolean; starting?: boolean; completing?: boolean; paused?: boolean; target?: { source?: string } };
  },
  refresh: vi.fn(),
  show: vi.fn(),
}));

vi.mock("../api/client", () => ({
  api: {
    startMode: vi.fn(),
    stopMode: vi.fn(),
    pauseMotion: vi.fn(),
    resumeMotion: vi.fn(),
    saveAutopilotPreferences: vi.fn(),
    resetAutopilotArc: vi.fn(),
  },
}));

vi.mock("../state/app-state", () => ({
  useAppState: () => app,
  useToast: () => ({ show: app.show }),
}));

const startMode = vi.mocked(api.startMode);
const stopMode = vi.mocked(api.stopMode);
const pauseMotion = vi.mocked(api.pauseMotion);
const resumeMotion = vi.mocked(api.resumeMotion);
const saveAutopilotPreferences = vi.mocked(api.saveAutopilotPreferences);
const resetAutopilotArc = vi.mocked(api.resetAutopilotArc);

const autopilotPreferences = {
  speech_cadence: "natural",
  speech_min_seconds: 35,
  speech_max_seconds: 120,
  motion_cadence: "natural",
  motion_min_seconds: 20,
  motion_max_seconds: 60,
  adaptive_speech_timing: true,
  adaptive_motion_timing: true,
  speech_motion_authority: "chat_only",
  session_tracking: true,
  session_arc: false,
  session_arc_minutes: 30,
};

describe("AutopilotControl", () => {
  beforeEach(() => {
    app.backendOnline = true;
    app.readOnly = false;
    app.state = { modes: {}, settings: {} };
    app.motion = { engine: { running: true, paused: false, target: { source: "autopilot" } } };
    app.refresh.mockReset();
    app.show.mockReset();
    startMode.mockReset();
    stopMode.mockReset();
    pauseMotion.mockReset();
    resumeMotion.mockReset();
    saveAutopilotPreferences.mockReset();
  });

  it("starts Autopilot from the Chat surface", async () => {
    startMode.mockResolvedValue({});
    render(<AutopilotControl />);

    await act(async () => screen.getByRole("button", { name: "Start Autopilot" }).click());

    expect(startMode).toHaveBeenCalledWith("autopilot");
    expect(app.show).toHaveBeenCalledWith("Autopilot started.");
    expect(app.refresh).toHaveBeenCalledOnce();
  });

  it("shows concise decision provenance and stops the active session", async () => {
    app.state = {
      modes: { mode: "autopilot", segment_index: 4, decision_source: "fallback" },
      settings: {},
    };
    stopMode.mockResolvedValue({});
    render(<AutopilotControl />);

    expect(screen.getByRole("status")).toHaveTextContent("Segment 4 · Planner fallback");
    await act(async () => screen.getByRole("button", { name: "Stop Autopilot" }).click());

    expect(stopMode).toHaveBeenCalledOnce();
    expect(app.show).toHaveBeenCalledWith("Autopilot stopped.");
  });

  it("keeps the control visible but disabled for read-only clients", () => {
    app.readOnly = true;
    render(<AutopilotControl />);

    expect(screen.getByRole("button", { name: "Start Autopilot" })).toBeDisabled();
  });

  it("pauses and resumes an active Autopilot session", async () => {
    app.state = { modes: { mode: "autopilot", segment_index: 2, decision_source: "model" }, settings: {} };
    pauseMotion.mockResolvedValue({});
    resumeMotion.mockResolvedValue({});
    const result = render(<AutopilotControl />);

    await act(async () => screen.getByRole("button", { name: "Pause Autopilot" }).click());
    expect(pauseMotion).toHaveBeenCalledOnce();

    app.motion = { engine: { running: false, paused: true, target: { source: "autopilot" } } };
    result.rerender(<AutopilotControl />);
    await act(async () => screen.getByRole("button", { name: "Resume Autopilot" }).click());
    expect(resumeMotion).toHaveBeenCalledOnce();
  });

  it("keeps Pause unavailable until motion has actually started", () => {
    app.state = { modes: { mode: "autopilot" }, settings: {} };
    app.motion = {};

    render(<AutopilotControl />);

    expect(screen.getByRole("button", { name: "Pause Autopilot" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Stop Autopilot" })).toBeEnabled();
  });

  it("does not expose Autopilot pause controls for motion owned by manual testing", () => {
    app.state = { modes: { mode: "autopilot", segment_index: 2 }, settings: {} };
    app.motion = { engine: { running: false, paused: true, target: { source: "manual_ui" } } };

    render(<AutopilotControl />);

    expect(screen.getByRole("button", { name: "Pause Autopilot" })).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Resume Autopilot" })).not.toBeInTheDocument();
  });

  it("renders independent clocks and persists cadence choices", async () => {
    app.state = {
      modes: {
        mode: "autopilot",
        segment_index: 2,
        decision_source: "model",
        motion_change_in_ms: 31_000,
        speech_in_ms: 91_000,
      },
      settings: { autopilot: autopilotPreferences },
    };
    saveAutopilotPreferences.mockImplementation(async (next) => ({ autopilot: next }));
    render(<AutopilotControl />);

    expect(screen.getByText("Motion 31 s · Speech 1m 31s")).toBeInTheDocument();
    await act(async () => {
      fireEvent.change(screen.getByLabelText("Motion changes"), { target: { value: "2" } });
    });

    expect(saveAutopilotPreferences).toHaveBeenCalledWith({
      ...autopilotPreferences,
      motion_cadence: "dynamic",
    });
  });

  it("serializes rapid set-point edits and persists the final choice", async () => {
    app.state = {
      modes: { mode: "autopilot" },
      settings: { autopilot: autopilotPreferences },
    };
    let resolveFirst!: (value: { autopilot: typeof autopilotPreferences }) => void;
    saveAutopilotPreferences
      .mockImplementationOnce(() => new Promise((resolve) => { resolveFirst = resolve; }))
      .mockImplementation(async (next) => ({ autopilot: next }));
    render(<AutopilotControl />);

    const slider = screen.getByRole("slider", { name: "Motion changes" });
    fireEvent.change(slider, { target: { value: "2" } });
    fireEvent.change(slider, { target: { value: "3" } });
    expect(saveAutopilotPreferences).toHaveBeenCalledTimes(1);

    await act(async () => {
      resolveFirst({ autopilot: { ...autopilotPreferences, motion_cadence: "dynamic" } });
      await Promise.resolve();
    });
    await waitFor(() => expect(saveAutopilotPreferences).toHaveBeenCalledTimes(2));
    expect(saveAutopilotPreferences).toHaveBeenLastCalledWith({
      ...autopilotPreferences,
      motion_cadence: "custom",
    });
  });

  // Session buildup is only defensible because it is visible. If it can be
  // enabled without appearing, the safety argument for the feature is gone.
  it("renders session buildup with its value when the backend reports one", () => {
    app.state = {
      modes: {
        mode: "autopilot",
        segment_index: 3,
        session_arc: { enabled: true, percent: 42, minutes: 30 },
      },
      settings: { autopilot: { ...autopilotPreferences, session_arc: true } },
    };
    render(<AutopilotControl />);

    const meter = screen.getByRole("meter", { name: "Session buildup" });
    expect(meter).toHaveAttribute("aria-valuenow", "42");
    expect(meter).toHaveAttribute("aria-valuemin", "0");
    expect(meter).toHaveAttribute("aria-valuemax", "100");
    expect(screen.getByText("42% of 30 min")).toBeInTheDocument();
    // The copy has to say the limits do not move, because that is the guarantee.
    expect(screen.getByText(/limits never move/i)).toBeInTheDocument();
  });

  it("shows no buildup while the switch is off", () => {
    app.state = {
      modes: { mode: "autopilot", segment_index: 3 },
      settings: { autopilot: autopilotPreferences },
    };
    render(<AutopilotControl />);
    expect(screen.queryByRole("meter", { name: "Session buildup" })).not.toBeInTheDocument();
  });

  // A bar the user cannot pull back would be a readout, not an override.
  it("lets the user reset the arc", async () => {
    resetAutopilotArc.mockResolvedValue({ session_arc: { enabled: true, percent: 0, minutes: 30 } });
    app.state = {
      modes: {
        mode: "autopilot",
        segment_index: 3,
        session_arc: { enabled: true, percent: 80, minutes: 30 },
      },
      settings: { autopilot: { ...autopilotPreferences, session_arc: true } },
    };
    render(<AutopilotControl />);

    fireEvent.click(screen.getByRole("button", { name: "Reset buildup" }));
    await act(async () => {
      await Promise.resolve();
    });
    expect(resetAutopilotArc).toHaveBeenCalledOnce();
  });

  it("enables required session tracking with session buildup", async () => {
    saveAutopilotPreferences.mockResolvedValue({
      autopilot: { ...autopilotPreferences, session_tracking: true, session_arc: true },
    });
    app.state = {
      modes: { mode: "autopilot" },
      settings: { autopilot: { ...autopilotPreferences, session_tracking: false } },
    };
    render(<AutopilotControl />);
    fireEvent.click(screen.getByRole("checkbox", { name: "Session buildup" }));

    await act(async () => {
      await Promise.resolve();
    });
    expect(saveAutopilotPreferences).toHaveBeenCalledWith(expect.objectContaining({
      session_tracking: true,
      session_arc: true,
    }));
  });

  it("shows custom cadence timing beside its selector without opening Advanced", () => {
    app.state = {
      modes: { mode: "autopilot" },
      settings: {
        autopilot: {
          ...autopilotPreferences,
          motion_cadence: "custom",
          speech_cadence: "custom",
        },
      },
    };
    render(<AutopilotControl />);

    const advanced = screen.getByText("Advanced").closest("details");
    const motionSelector = screen.getByRole("slider", { name: "Motion changes" });
    const speechSelector = screen.getByRole("slider", { name: "Spoken check-ins" });
    const motionMinimum = screen.getByRole("spinbutton", { name: "Motion minimum seconds" });
    const speechMinimum = screen.getByRole("spinbutton", { name: "Speech minimum seconds" });
    expect(advanced).not.toContainElement(motionMinimum);
    expect(advanced).not.toContainElement(speechMinimum);
    expect(motionSelector.parentElement?.nextElementSibling).toContainElement(motionMinimum);
    expect(speechSelector.parentElement?.nextElementSibling).toContainElement(speechMinimum);
    expect(screen.getByText("Motion range")).toBeVisible();
    expect(screen.getByText("Speech range")).toBeVisible();
  });

  it.each([1, 24 * 60])("saves a %d minute custom buildup", async (minutes) => {
    saveAutopilotPreferences.mockImplementation(async (next) => ({ autopilot: next }));
    app.state = {
      modes: { mode: "autopilot" },
      settings: { autopilot: { ...autopilotPreferences, session_arc: true } },
    };
    render(<AutopilotControl />);
    const duration = screen.getByRole("spinbutton", { name: "Session buildup minutes" });

    await act(async () => {
      fireEvent.change(duration, { target: { value: String(minutes) } });
      fireEvent.blur(duration);
      await Promise.resolve();
    });

    expect(duration).toHaveAttribute("min", "1");
    expect(duration).not.toHaveAttribute("max");
    expect(saveAutopilotPreferences).toHaveBeenCalledWith(expect.objectContaining({
      session_arc_minutes: minutes,
    }));
  });

  it("preserves an edited buildup duration across background snapshots", async () => {
    saveAutopilotPreferences.mockImplementation(async (next) => ({ autopilot: next }));
    app.state = {
      modes: { mode: "autopilot" },
      settings: { autopilot: { ...autopilotPreferences, session_arc: true } },
    };
    const result = render(<AutopilotControl />);
    const duration = screen.getByRole("spinbutton", { name: "Session buildup minutes" });

    fireEvent.change(duration, { target: { value: "5" } });
    app.state = {
      ...app.state,
      settings: { autopilot: { ...autopilotPreferences, session_arc: true } },
    };
    result.rerender(<AutopilotControl />);

    expect(duration).toHaveValue(5);
    await act(async () => {
      fireEvent.blur(duration);
      await Promise.resolve();
    });
    expect(saveAutopilotPreferences).toHaveBeenCalledWith(expect.objectContaining({
      session_arc_minutes: 5,
    }));
  });

  it("clears the arc when session tracking is switched off", async () => {
    saveAutopilotPreferences.mockResolvedValue({ autopilot: autopilotPreferences });
    app.state = {
      modes: { mode: "autopilot" },
      settings: { autopilot: { ...autopilotPreferences, session_arc: true } },
    };
    render(<AutopilotControl />);
    fireEvent.click(screen.getByText("Advanced"));
    expect(screen.getByText("Shares timing and pace; limits stay unchanged.")).toHaveClass("autopilot-session-hint");
    fireEvent.click(screen.getByRole("checkbox", { name: /Session tracking/ }));

    await act(async () => {
      await Promise.resolve();
    });
    expect(saveAutopilotPreferences).toHaveBeenCalledWith(
      expect.objectContaining({ session_tracking: false, session_arc: false }),
    );
  });
});
