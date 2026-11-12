import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TempoPlayCard } from "./TempoReadout";

const mock = vi.hoisted(() => ({
  app: { backendOnline: true, readOnly: false, refresh: vi.fn(), state: { stop_sequence: 7 } },
  motion: { engine: { running: true, paused: false, target: { tempo: { mode: "steady" } }, tempo: { mode: "steady", level_percent: 40 } } },
  ctx: { autoblow: { vacuglide: { connected: true, commanded_percent: 40 } } } as Record<string, unknown>,
  setTempo: vi.fn().mockResolvedValue({}),
  routines: vi.fn().mockResolvedValue({ routines: [] }),
  show: vi.fn(),
}));

vi.mock("../api/client", () => ({ api: { tempoRoutines: mock.routines, setTempo: mock.setTempo } }));
vi.mock("../state/app-state", () => ({ useAppState: () => mock.app, useMotionState: () => mock.motion, useToast: () => ({ show: mock.show }) }));
vi.mock("../devices/runtime", () => ({ useDeviceWatch: vi.fn(), useDeviceContext: () => mock.ctx }));

describe("tempo control admission", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    mock.setTempo.mockClear();
    mock.app.state.stop_sequence = 7;
    mock.app.readOnly = false;
    mock.motion.engine.running = true;
    mock.motion.engine.tempo.level_percent = 40;
    mock.ctx = { autoblow: { vacuglide: { connected: true, commanded_percent: 40 } } };
  });
  afterEach(() => vi.useRealTimers());

  it("cancels a queued slider change when Stop arrives", async () => {
    const view = render(<TempoPlayCard />);
    await act(async () => {});
    fireEvent.change(screen.getByRole("slider", { name: "Tempo" }), { target: { value: "80" } });
    mock.app.state.stop_sequence++;
    mock.motion.engine.running = false;
    view.rerender(<TempoPlayCard />);
    await act(async () => { vi.advanceTimersByTime(400); });
    expect(mock.setTempo).not.toHaveBeenCalled();
  });

  it("does not replay a queued slider change after Pause the motor", async () => {
    render(<TempoPlayCard />);
    await act(async () => {});
    fireEvent.change(screen.getByRole("slider", { name: "Tempo" }), { target: { value: "80" } });
    fireEvent.click(screen.getByRole("button", { name: "Pause the motor" }));
    await act(async () => { vi.advanceTimersByTime(400); });
    expect(mock.setTempo).toHaveBeenCalledTimes(1);
    expect(mock.setTempo.mock.calls[0][0]).toEqual({ mode: "pause" });
  });

  it("renders the backend tempo after a model changes it", async () => {
    const view = render(<TempoPlayCard />);
    await act(async () => {});
    mock.motion.engine.tempo.level_percent = 72;
    view.rerender(<TempoPlayCard />);
    expect(screen.getByRole("slider", { name: "Tempo" })).toHaveValue("72");
  });

  it("asks for the VacuGlide before offering anything to play", async () => {
    mock.ctx = { autoblow: { vacuglide: { connected: false } } };
    render(<TempoPlayCard />);
    await act(async () => {});
    expect(screen.getByText("Connect the VacuGlide to play its routines.")).toBeInTheDocument();
    expect(screen.queryByRole("slider")).not.toBeInTheDocument();
  });
});
