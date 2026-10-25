import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { AutopilotControl } from "./AutopilotControl";

const app = vi.hoisted(() => ({
  backendOnline: true,
  readOnly: false,
  state: { modes: {}, settings: {} } as {
    modes: {
      mode?: string;
      edge_hold?: { active: boolean };
      status_at?: string;
      segment_index?: number;
      decision_source?: string;
      motion_change_in_ms?: number;
      motion_change_due_at?: string;
      speech_in_ms?: number;
      speech_due_at?: string;
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
  useMotionState: () => app.motion,
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
  motion_cadence: "scaled",
  motion_change_level: 4,
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
    vi.useRealTimers();
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

  afterEach(() => {
    vi.useRealTimers();
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

  it("says when Autopilot is holding because the user asked", () => {
    app.state = { modes: { mode: "autopilot", segment_index: 3, decision_source: "requested_hold" }, settings: {} };
    render(<AutopilotControl />);

    expect(screen.getByRole("status")).toHaveTextContent("Segment 3 · Keeping the motion as you asked");
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
      fireEvent.change(screen.getByLabelText("Motion change rate"), { target: { value: "5" } });
    });

    expect(saveAutopilotPreferences).toHaveBeenCalledWith({
      ...autopilotPreferences,
      motion_cadence: "scaled",
      motion_change_level: 6,
    });
  });

  it("renders every countdown second between backend snapshots", () => {
    vi.useFakeTimers();
    app.state = {
      modes: {
        mode: "autopilot",
        segment_index: 2,
        status_at: "2026-08-22T12:00:00Z",
        motion_change_due_at: "2026-08-22T12:00:14.5Z",
        motion_change_in_ms: 14_500,
      },
      settings: { autopilot: { ...autopilotPreferences, speech_cadence: "off" } },
    };
    render(<AutopilotControl />);

    expect(screen.getByText("Motion 15 s · Speech Off")).toBeInTheDocument();
    act(() => vi.advanceTimersByTime(1_000));
    expect(screen.getByText("Motion 14 s · Speech Off")).toBeInTheDocument();
    act(() => vi.advanceTimersByTime(1_000));
    expect(screen.getByText("Motion 13 s · Speech Off")).toBeInTheDocument();
  });

  it("renders every speech countdown second across slower backend polls", () => {
    vi.useFakeTimers();
    app.state = {
      modes: {
        mode: "autopilot",
        segment_index: 2,
        status_at: "2026-08-22T12:00:00Z",
        motion_planned: true,
        speech_due_at: "2026-08-22T12:01:34.5Z",
        speech_in_ms: 94_500,
      },
      settings: { autopilot: autopilotPreferences },
    };
    const result = render(<AutopilotControl />);

    expect(screen.getByText("Motion planned · Speech 1m 35s")).toBeInTheDocument();
    act(() => vi.advanceTimersByTime(1_000));
    expect(screen.getByText("Motion planned · Speech 1m 34s")).toBeInTheDocument();
    act(() => vi.advanceTimersByTime(1_000));
    expect(screen.getByText("Motion planned · Speech 1m 33s")).toBeInTheDocument();

    // A two-second state poll reconciles to the same absolute deadline without
    // falling back to a low-refresh countdown or double-consuming elapsed time.
    app.state = {
      ...app.state,
      modes: {
        ...app.state.modes,
        status_at: "2026-08-22T12:00:02Z",
        speech_in_ms: 92_500,
      },
    };
    result.rerender(<AutopilotControl />);
    expect(screen.getByText("Motion planned · Speech 1m 33s")).toBeInTheDocument();
    act(() => vi.advanceTimersByTime(1_000));
    expect(screen.getByText("Motion planned · Speech 1m 32s")).toBeInTheDocument();
  });

  it("freezes the interpolated clock without jumping when Autopilot is paused", () => {
    vi.useFakeTimers();
    app.state = {
      modes: {
        mode: "autopilot",
        segment_index: 2,
        status_at: "2026-08-22T12:00:00Z",
        motion_change_due_at: "2026-08-22T12:00:14.5Z",
        motion_change_in_ms: 14_500,
      },
      settings: { autopilot: { ...autopilotPreferences, speech_cadence: "off" } },
    };
    const result = render(<AutopilotControl />);

    expect(screen.getByText("Motion 15 s · Speech Off")).toBeInTheDocument();
    act(() => vi.advanceTimersByTime(2_000));
    expect(screen.getByText("Motion 13 s · Speech Off")).toBeInTheDocument();

    app.motion = { engine: { running: false, paused: true, target: { source: "autopilot" } } };
    result.rerender(<AutopilotControl />);
    act(() => vi.advanceTimersByTime(3_000));
    expect(screen.getByText("Motion 13 s · Speech Off")).toBeInTheDocument();
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

    const slider = screen.getByRole("slider", { name: "Motion change rate" });
    fireEvent.change(slider, { target: { value: "4" } });
    fireEvent.change(slider, { target: { value: "7" } });
    expect(saveAutopilotPreferences).toHaveBeenCalledTimes(1);

    await act(async () => {
      resolveFirst({ autopilot: { ...autopilotPreferences, motion_change_level: 5 } });
      await Promise.resolve();
    });
    await waitFor(() => expect(saveAutopilotPreferences).toHaveBeenCalledTimes(2));
    expect(saveAutopilotPreferences).toHaveBeenLastCalledWith({
      ...autopilotPreferences,
      motion_cadence: "scaled",
      motion_change_level: 8,
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

  it("shows custom speech timing beside its selector without opening Advanced", () => {
    app.state = {
      modes: { mode: "autopilot" },
      settings: {
        autopilot: {
          ...autopilotPreferences,
          speech_cadence: "custom",
        },
      },
    };
    render(<AutopilotControl />);

    const advanced = screen.getByText("Advanced").closest("details");
    const speechSelector = screen.getByRole("combobox", { name: "Spoken check-ins" });
    const speechMinimum = screen.getByRole("spinbutton", { name: "Speech minimum seconds" });
    expect(advanced).not.toContainElement(speechMinimum);
    expect(speechSelector.parentElement?.nextElementSibling).toContainElement(speechMinimum);
    expect(screen.getByText("Speech range")).toBeVisible();
  });

  it("renders Motion change rate as an explicit numbered 1-8 scale", () => {
    app.state = {
      modes: { mode: "autopilot" },
      settings: { autopilot: autopilotPreferences },
    };
    render(<AutopilotControl />);

    const slider = screen.getByRole("slider", { name: "Motion change rate" });
    expect(slider).toHaveAttribute("aria-valuetext", "4");
    expect(screen.queryByText("1 = fewer · 7 = more")).not.toBeInTheDocument();
    expect(Array.from(
      slider.closest(".autopilot-setpoints")?.querySelectorAll(".setpoint-stop") ?? [],
      (stop) => stop.textContent,
    )).toEqual(["1", "2", "3", "4", "5", "6", "7", "8"]);
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

  it("offers Auto timing for hosted routes in older snapshots", async () => {
    const route = (kind: string) => ({
      chat: { kind, provider: "compatible", model: "m", context_policy: "conversation", state: "configured" },
      autopilot: { kind, provider: "compatible", model: "m", context_policy: "conversation", state: "configured" },
    });
    app.state = {
      modes: { mode: "autopilot" },
      settings: { autopilot: autopilotPreferences, model_routing: route("local") },
    };
    const result = render(<AutopilotControl />);
    expect(screen.queryByRole("checkbox", { name: "Auto timing" })).not.toBeInTheDocument();

    saveAutopilotPreferences.mockImplementation(async (next) => ({ autopilot: next }));
    app.state = {
      modes: { mode: "autopilot" },
      settings: { autopilot: autopilotPreferences, model_routing: route("hosted") },
    };
    result.rerender(<AutopilotControl />);
    const toggle = screen.getByRole("checkbox", { name: "Auto timing" });
    expect(toggle).not.toBeChecked();
    expect(screen.getByText("The model decides when motion changes, from 8 seconds to 5 minutes. This can use more requests.")).toHaveClass("autopilot-timing-hint");
    expect(screen.getByText("Advanced").closest("details")).not.toContainElement(toggle);

    await act(async () => {
      fireEvent.click(toggle);
      await Promise.resolve();
    });
    expect(saveAutopilotPreferences).toHaveBeenCalledWith(expect.objectContaining({
      auto_motion_timing: true,
      motion_change_level: 4,
    }));
  });

  it("follows saved planner features for Advanced local models and hides them after downgrade", () => {
    const update = (advanced: boolean) => {
      app.state = { modes: { mode: "autopilot" }, settings: {
        autopilot: { ...autopilotPreferences, auto_motion_timing: true, prepare_next_decision: true },
        model_routing: {
          chat: { kind: "hosted", features: { auto_timing: true, prepare_decisions: true } },
          autopilot: { kind: "local", provider: "ollama", features: { auto_timing: advanced, prepare_decisions: advanced } },
        },
      } };
    };
    update(true);
    const view = render(<AutopilotControl />);
    expect(screen.getByRole("checkbox", { name: "Auto timing" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Prepare decisions early" })).toBeChecked();
    update(false);
    view.rerender(<AutopilotControl />);
    expect(screen.queryByRole("checkbox", { name: "Auto timing" })).not.toBeInTheDocument();
    expect(screen.queryByRole("checkbox", { name: "Prepare decisions early" })).not.toBeInTheDocument();
  });

  it("offers prepared decisions for any external planner and saves the choice", async () => {
    const route = (kind: string) => ({
      chat: { kind: "local", provider: "llama_cpp", model: "m", context_policy: "conversation", state: "configured" },
      autopilot: { kind, provider: "openai", model: "m", context_policy: "technical", state: "configured" },
    });
    app.state = {
      modes: { mode: "autopilot" },
      settings: { autopilot: autopilotPreferences, model_routing: route("local") },
    };
    const result = render(<AutopilotControl />);
    expect(screen.queryByRole("checkbox", { name: "Prepare decisions early" })).not.toBeInTheDocument();

    app.state = {
      modes: { mode: "autopilot" },
      settings: { autopilot: autopilotPreferences, model_routing: route("decisions") },
    };
    result.rerender(<AutopilotControl />);
    expect(screen.queryByRole("checkbox", { name: "Auto timing" })).not.toBeInTheDocument();
    const toggle = screen.getByRole("checkbox", { name: "Prepare decisions early" });
    expect(toggle).toBeChecked();

    saveAutopilotPreferences.mockImplementation(async (next) => ({ autopilot: next }));
    await act(async () => {
      fireEvent.click(toggle);
      await Promise.resolve();
    });
    expect(saveAutopilotPreferences).toHaveBeenCalledWith(expect.objectContaining({ prepare_next_decision: false }));
  });

  it("keeps counting down while a prepared decision waits for its boundary", () => {
    vi.useFakeTimers();
    app.state = {
      modes: {
        mode: "autopilot",
        segment_index: 3,
        status_at: "2026-08-22T12:00:00Z",
        motion_planned: true,
        motion_change_due_at: "2026-08-22T12:00:32Z",
        motion_change_in_ms: 32_000,
      },
      settings: { autopilot: { ...autopilotPreferences, speech_cadence: "off" } },
    };
    render(<AutopilotControl />);

    expect(screen.getByText("Motion 32 s, ready · Speech Off")).toBeInTheDocument();
    act(() => vi.advanceTimersByTime(1_000));
    expect(screen.getByText("Motion 31 s, ready · Speech Off")).toBeInTheDocument();
  });

  it("offers extra planning only when the saved planner supports it, and saves both choices", async () => {
    saveAutopilotPreferences.mockImplementation(async (next) => ({ autopilot: next }));
    const settings = {
      autopilot: { ...autopilotPreferences, intermediate_planning: true },
      model_routing: {
        chat: { kind: "hosted", features: { intermediate_planning: false } },
        autopilot: { kind: "local", features: { intermediate_planning: true } },
      },
    };
    app.state = { modes: {}, settings };
    const view = render(<AutopilotControl />);
    const toggle = screen.getByRole("checkbox", { name: "Extra planning" });
    expect(toggle).toBeChecked();
    expect(toggle).toHaveAccessibleDescription(/0.4–0.7 s.*Gemma 12B/);
    await act(async () => fireEvent.click(toggle));
    expect(saveAutopilotPreferences).toHaveBeenLastCalledWith(expect.objectContaining({ intermediate_planning: false, motion_change_level: 4 }));
    expect(toggle).not.toBeChecked();
    await act(async () => fireEvent.click(toggle));
    expect(saveAutopilotPreferences).toHaveBeenLastCalledWith(expect.objectContaining({ intermediate_planning: true }));
    app.state = { modes: {}, settings: { ...settings, model_routing: {
      chat: { features: { intermediate_planning: true } },
      autopilot: { kind: "hosted", features: { intermediate_planning: false } },
    } } };
    view.rerender(<AutopilotControl />);
    expect(screen.queryByRole("checkbox", { name: "Extra planning" })).not.toBeInTheDocument();
  });

  it("restores the saved extra-planning choice when saving fails", async () => {
    saveAutopilotPreferences.mockRejectedValue(new Error("Save failed"));
    app.state = { modes: {}, settings: {
      autopilot: { ...autopilotPreferences, intermediate_planning: true },
      model_routing: { autopilot: { features: { intermediate_planning: true } } },
    } };
    render(<AutopilotControl />);
    const toggle = screen.getByRole("checkbox", { name: "Extra planning" });
    await act(async () => fireEvent.click(toggle));
    expect(toggle).toBeChecked();
    expect(app.show).toHaveBeenCalledWith("Save failed", "error");
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


it("keeps pause available and hides due clocks during an Edge hold", async () => {
  app.state.modes = { mode: "autopilot", segment_index: 2, decision_source: "edge_hold", edge_hold: { active: true } };
  app.state.settings = { autopilot: autopilotPreferences };
  app.motion = { engine: { running: true, paused: false, target: { source: "edge" } } };
  app.backendOnline = true; app.readOnly = false;
  render(<AutopilotControl />);
  expect(screen.getByText("Gentle holding pattern")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Pause Autopilot" })).toBeEnabled();
  expect(screen.queryByText(/Motion Due/)).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Pause Autopilot" }));
  await waitFor(() => expect(pauseMotion).toHaveBeenCalled());
});
