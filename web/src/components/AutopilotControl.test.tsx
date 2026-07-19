import { act, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { AutopilotControl } from "./AutopilotControl";

const app = vi.hoisted(() => ({
  backendOnline: true,
  readOnly: false,
  state: { modes: {}, settings: {} } as {
    modes: { mode?: string; segment_index?: number; decision_source?: string };
    settings: Record<string, unknown>;
  },
  motion: { engine: { running: true, paused: false } } as { engine?: { running?: boolean; paused?: boolean } },
  refresh: vi.fn(),
  show: vi.fn(),
}));

vi.mock("../api/client", () => ({
  api: {
    startMode: vi.fn(),
    stopMode: vi.fn(),
    pauseMotion: vi.fn(),
    resumeMotion: vi.fn(),
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

describe("AutopilotControl", () => {
  beforeEach(() => {
    app.backendOnline = true;
    app.readOnly = false;
    app.state = { modes: {}, settings: {} };
    app.motion = { engine: { running: true, paused: false } };
    app.refresh.mockReset();
    app.show.mockReset();
    startMode.mockReset();
    stopMode.mockReset();
    pauseMotion.mockReset();
    resumeMotion.mockReset();
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

    app.motion = { engine: { running: false, paused: true } };
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
});
