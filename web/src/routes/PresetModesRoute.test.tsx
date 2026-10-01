import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { FreestyleSettings, FreestyleStatus } from "../api/types";
import { PresetModesRoute } from "./PresetModesRoute";

const balanced: FreestyleSettings = {
  feel: "balanced",
  pace_percent: 50,
  length_percent: 75,
  focus_percent: 50,
  roaming_percent: 50,
  variety_percent: 50,
  accent: "even",
  shape: "steady",
  shape_minutes: 15,
};

const app = vi.hoisted(() => ({
  readOnly: false,
  state: {
    modes: {} as { running?: boolean; mode?: string; freestyle?: FreestyleStatus },
    settings: {} as { motion?: { style: string }; freestyle?: FreestyleSettings },
  },
  refresh: vi.fn(),
  show: vi.fn(),
}));

vi.mock("../api/client", () => ({
  api: {
    startMode: vi.fn(),
    stopMode: vi.fn(),
    saveFreestylePreferences: vi.fn(),
  },
}));

vi.mock("../state/app-state", () => ({
  useMotionState: () => ({ engine: { paused: false } }),
  useAppState: () => ({
    state: app.state,
    backendOnline: true,
    readOnly: app.readOnly,
    motion: { engine: { paused: false } },
    refresh: app.refresh,
  }),
  useToast: () => ({ show: app.show }),
}));

const startMode = vi.mocked(api.startMode);
const savePreferences = vi.mocked(api.saveFreestylePreferences);

describe("PresetModesRoute", () => {
  beforeEach(() => {
    app.readOnly = false;
    app.state = { modes: {}, settings: { motion: { style: "balanced" }, freestyle: balanced } };
    app.refresh.mockReset();
    app.show.mockReset();
    startMode.mockReset();
    vi.mocked(api.stopMode).mockReset();
    savePreferences.mockReset();
  });

  it("deduplicates rapid mode starts before React can disable the control", async () => {
    let release!: (value: unknown) => void;
    startMode.mockImplementation(() => new Promise((resolve) => { release = resolve; }));
    render(<PresetModesRoute />);
    const start = screen.getByRole("button", { name: "Start Freestyle" });

    act(() => {
      start.click();
      start.click();
    });

    expect(startMode).toHaveBeenCalledOnce();
    await act(async () => release({}));
    await waitFor(() => expect(start).toBeEnabled());
  });

  it("serializes preference writes and sends the newest choice last", async () => {
    const releases: Array<() => void> = [];
    savePreferences.mockImplementation((freestyle) => new Promise((resolve) => {
      releases.push(() => resolve({ freestyle }));
    }));
    render(<PresetModesRoute />);

    act(() => {
      screen.getByRole("radio", { name: "Intense" }).click();
      screen.getByRole("radio", { name: "Gentle" }).click();
    });

    expect(savePreferences).toHaveBeenCalledOnce();
    expect(savePreferences.mock.calls[0][0].feel).toBe("intense");
    await act(async () => releases[0]());
    await waitFor(() => expect(savePreferences).toHaveBeenCalledTimes(2));
    expect(savePreferences.mock.calls[1][0].feel).toBe("gentle");
    await act(async () => releases[1]());
    expect(screen.queryByText(/Phase 11|Phase 14|arrangement/i)).not.toBeInTheDocument();
  });

  it("makes a moved control a custom feel", async () => {
    savePreferences.mockImplementation((freestyle) => Promise.resolve({ freestyle }));
    render(<PresetModesRoute />);

    fireEvent.change(screen.getByRole("slider", { name: "Pace" }), { target: { value: "4" } });

    await waitFor(() => expect(savePreferences).toHaveBeenCalledOnce());
    expect(savePreferences.mock.calls[0][0]).toMatchObject({ feel: "custom", pace_percent: 100, focus_percent: 50 });
  });

  it("applies a queued slider edit to the acknowledged feel preset", async () => {
    let release!: () => void;
    const intense = { ...balanced, feel: "intense", pace_percent: 75, length_percent: 50, variety_percent: 75 };
    savePreferences.mockImplementationOnce(() => new Promise((resolve) => {
      release = () => resolve({ freestyle: intense });
    })).mockImplementation((freestyle) => Promise.resolve({ freestyle }));
    render(<PresetModesRoute />);

    act(() => screen.getByRole("radio", { name: "Intense" }).click());
    fireEvent.change(screen.getByRole("slider", { name: "Focus" }), { target: { value: "4" } });
    await act(async () => release());

    await waitFor(() => expect(savePreferences).toHaveBeenCalledTimes(2));
    expect(savePreferences.mock.calls[1][0]).toEqual({ ...intense, feel: "custom", focus_percent: 100 });
  });

  it("shows the running session shape", () => {
    app.state = {
      modes: { mode: "freestyle", freestyle: { feel: "balanced", shape: "build", shape_phase: "building", shape_progress_percent: 42, energy_percent: 64 } },
      settings: { freestyle: { ...balanced, shape: "build" } },
    };
    render(<PresetModesRoute />);

    expect(screen.getByText("Slow build · Building · 42%")).toHaveAttribute("role", "status");
    expect(screen.getByRole("spinbutton", { name: "Shape minutes" })).toHaveValue(15);
  });

  it("keeps mode-specific Stop unavailable to read-only clients", () => {
    app.readOnly = true;
    app.state = { modes: { mode: "freestyle" }, settings: { freestyle: balanced } };
    render(<PresetModesRoute />);

    expect(screen.getByRole("button", { name: "Stop Freestyle" })).toBeDisabled();
    expect(screen.getByRole("radio", { name: "Gentle" })).toBeDisabled();
  });

  it("does not duplicate Chat Autopilot in Preset Modes", () => {
    render(<PresetModesRoute />);

    expect(screen.queryByText("Autopilot")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Start Freestyle" })).toBeEnabled();
  });
});
