import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { EdgeButton } from "./EdgeButton";
import { StopButton } from "./StopButton";
import { api } from "../api/client";
import { setLocaleForTest } from "../i18n";
import english from "../i18n/locales/en.json";
import type { ModesStatus } from "../api/types";

const app = vi.hoisted(() => ({ refresh: vi.fn(), show: vi.fn(), readOnly: false, backendOnline: true, stale: false,
  state: { modes: {} as ModesStatus, stop_sequence: 0, motion: { available: true, engine: { running: true, paused: false, edge_allowed: true, edge_phase: "", still_hold: false, target: {} } } },
}));
vi.mock("../state/app-state", () => ({ useAppState: () => app, useMotionState: () => null, useToast: () => ({ show: app.show }) }));
vi.mock("../api/client", async (original) => ({ ...await original<typeof import("../api/client")>(), api: {
  activateEdge: vi.fn(async () => ({})), resumeEdge: vi.fn(async () => ({})), stopMotion: vi.fn(async () => ({})),
  holdStill: vi.fn(async () => ({})), resumeStillHold: vi.fn(async () => ({})),
} }));

describe("EdgeButton", () => {
  it("renders the engine's queued and slowing phases and offers an explicit still hold", async () => {
    app.state.modes.edge_hold = { active: true, phase: "planning", automatic: true };
    app.state.motion.engine.edge_phase = "queued";
    const view = render(<EdgeButton />);
    expect(screen.getByRole("status")).toHaveTextContent("Edge requested");
    app.state.motion.engine.edge_phase = "slowing";
    view.rerender(<EdgeButton />);
    expect(screen.getByRole("status")).toHaveTextContent("Slowing");
    fireEvent.click(screen.getByRole("button", { name: "Hold still" }));
    await waitFor(() => expect(api.holdStill).toHaveBeenCalledTimes(1));
    app.state.motion.engine.edge_allowed = false;
    app.state.motion.engine.still_hold = true;
    view.rerender(<EdgeButton />);
    expect(screen.getByRole("status")).toHaveTextContent("Holding still");
    fireEvent.click(screen.getByRole("button", { name: "Return" }));
    await waitFor(() => expect(api.resumeStillHold).toHaveBeenCalledTimes(1));
  });
  beforeEach(() => {
    vi.clearAllMocks(); setLocaleForTest("en", english);
    app.readOnly = false; app.backendOnline = true; app.stale = false; app.state.modes = {};
    app.state.motion.engine = { running: true, paused: false, edge_allowed: true, edge_phase: "", still_hold: false, target: {} };
  });
  it("submits once and waits for backend holding state", async () => {
    const view = render(<EdgeButton />);
    fireEvent.click(screen.getByRole("button", { name: "Edge — gentle hold" }));
    await waitFor(() => expect(app.refresh).toHaveBeenCalled());
    expect(api.activateEdge).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("button", { name: "Return" })).toBeNull();
    app.state.modes.edge_hold = { active: true, phase: "holding", automatic: true };
    view.rerender(<EdgeButton />);
    expect(screen.getByRole("status")).toHaveTextContent("Autopilot will resume");
    fireEvent.click(screen.getByRole("button", { name: "Return" }));
    await waitFor(() => expect(api.resumeEdge).toHaveBeenCalledTimes(1));
  });
  it("uses the manager's ownership for Return and mutes only countdown ticks", () => {
    app.state.motion.engine.edge_phase = "holding";
    const view = render(<EdgeButton />);
    expect(screen.queryByRole("button", { name: "Return" })).toBeNull();
    expect(screen.queryByText("Holding until you resume")).toBeNull();
    app.state.modes.edge_hold = { active: true, phase: "holding", automatic: true, resume_in_seconds: 15 };
    view.rerender(<EdgeButton />);
    expect(screen.getByRole("status")).toHaveAttribute("aria-live", "off");
    app.state.motion.engine.edge_phase = "slowing";
    view.rerender(<EdgeButton />);
    expect(screen.getByRole("status")).toHaveAttribute("aria-live", "polite");
    expect(screen.getByRole("button", { name: "Return" })).toBeEnabled();
  });
  it("names the still request while waiting for transport confirmation", async () => {
    let finish!: (value: object) => void;
    vi.mocked(api.holdStill).mockImplementationOnce(() => new Promise(resolve => { finish = resolve; }));
    render(<EdgeButton />);
    fireEvent.click(screen.getByRole("button", { name: "Hold still" }));
    expect(screen.getByRole("status")).toHaveTextContent("Pausing");
    expect(screen.getByRole("button", { name: "Edge — gentle hold" })).toHaveTextContent("Edge");
    finish({});
    await waitFor(() => expect(app.refresh).toHaveBeenCalled());
  });
  it.each(["offline", "readOnly", "stale", "paused", "stopped", "media", "program"])("disables Edge for %s while Stop stays reachable", (kind) => {
    if (kind === "offline") app.backendOnline = false;
    if (kind === "readOnly") app.readOnly = true;
    if (kind === "stale") app.stale = true;
    if (kind === "paused") app.state.motion.engine.paused = true;
    if (kind === "stopped") app.state.motion.engine.running = false;
    if (kind === "media") app.state.motion.engine.target = { media_id: "movie" };
    if (kind === "program") app.state.motion.engine.target = { program_id: "finite" };
    if (["paused", "stopped", "media", "program"].includes(kind)) app.state.motion.engine.edge_allowed = false;
    render(<><EdgeButton /><StopButton /></>);
    expect(screen.getByRole("button", { name: "Edge — gentle hold" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Emergency stop all motion" })).toBeEnabled();
  });
  it("keeps Stop usable while an Edge request is pending", async () => {
    let release!: (value: object) => void;
    vi.mocked(api.activateEdge).mockImplementationOnce(() => new Promise(resolve => { release = resolve; }));
    render(<><EdgeButton /><StopButton /></>);
    fireEvent.click(screen.getByRole("button", { name: "Edge — gentle hold" }));
    expect(screen.getByRole("button", { name: "Edge — gentle hold" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Emergency stop all motion" }));
    await waitFor(() => expect(api.stopMotion).toHaveBeenCalledTimes(1));
    release({}); await waitFor(() => expect(screen.getByRole("button", { name: "Edge — gentle hold" })).toBeEnabled());
  });
});
