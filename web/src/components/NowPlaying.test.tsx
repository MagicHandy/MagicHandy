import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { NowPlayingCard, NowPlayingPill } from "./NowPlaying";

const app = vi.hoisted(() => ({
  state: {} as Record<string, unknown>,
  motion: null as unknown,
  ctx: {} as Record<string, unknown>,
}));

vi.mock("../state/app-state", () => ({
  useAppState: () => ({ state: app.state, backendOnline: true, readOnly: false, refresh: vi.fn() }),
  useMotionState: () => app.motion,
}));
vi.mock("../devices/runtime", () => ({ useDeviceWatch: () => undefined, useDeviceContext: () => app.ctx }));
vi.mock("./StartupRecoveryNotice", () => ({ StartupRecoveryNotice: () => null }));
vi.mock("./MotionVisualizer", () => ({ MotionVisualizer: () => <div role="img" aria-label="Device drawing" /> }));

function tempoRun() {
  app.state = { settings: { device: { configured_devices: ["vacuglide"], playback_devices: ["vacuglide"] } }, modes: { mode: "autopilot" } };
  app.motion = { available: true, engine: { running: true, paused: false, running_ms: 252_000,
    target: { tempo_only: true, tempo: { mode: "routine", routine: "swells" } },
    tempo: { mode: "routine", routine: "swells", level_percent: 47.6, ahead: [50, 55, 60, 64, 68, 70, 69, 66] } } };
  app.ctx = { autoblow: { vacuglide: { connected: true, commanded_percent: 49.2, stroke_steps: 1, round_trip_ms: 84 } } };
}

describe("Now playing", () => {
  beforeEach(() => tempoRun());

  it("shows a VacuGlide routine as tempo, who chose it, and what was sent", () => {
    render(<NowPlayingCard />);
    expect(screen.getByText("Playing")).toBeInTheDocument();
    expect(screen.getByText("Autopilot is choosing")).toBeInTheDocument();
    expect(screen.getByText("Swells")).toBeInTheDocument();
    expect(screen.getByText("VacuGlide routine")).toBeInTheDocument();
    expect(screen.getByRole("meter", { name: "Tempo: 48%" })).toHaveAttribute("value", "48");
    expect(screen.getByRole("img", { name: "Tempo over the next 8 seconds" })).toBeInTheDocument();
    expect(screen.getByText("Motor 49% sent · stroke +1 · cloud 84 ms")).toBeInTheDocument();
    expect(screen.queryByRole("img", { name: "Device drawing" })).not.toBeInTheDocument();
    expect(screen.getByText("04:12")).toBeInTheDocument();
  });

  it("lists every device in a mixed run, with the VacuGlide following the pace", () => {
    app.state = { settings: { device: { configured_devices: ["handy", "vacuglide", "vibrator"], playback_devices: ["handy", "vacuglide", "vibrator"] } } };
    app.motion = { available: true, engine: { running: true, paused: false, current_sample: { position_percent: 62 },
      target: { source: "chat", pattern_name: "Long strokes", speed_percent: 70, tempo: { mode: "follow" } }, pace: { effective_percent: 62, requested_percent: 70, limited: false },
      settings: { stroke_min_percent: 20, stroke_max_percent: 85 } } };
    app.ctx = { autoblow: { vacuglide: { connected: true, commanded_percent: 52 } }, intiface: { vibrator: { motors: [{ index: 0, level_percent: 40 }] } } };
    render(<NowPlayingCard />);
    expect(screen.getByText("From chat")).toBeInTheDocument();
    expect(screen.getByRole("meter", { name: "Pace: 62%" })).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "Device drawing" })).toBeInTheDocument();
    const devices = screen.getByRole("list", { name: "In this run" });
    expect(within(devices).getByText("follows the pace")).toBeInTheDocument();
    expect(within(devices).getByText("motor at 52%")).toBeInTheDocument();
    expect(within(devices).getByText("40%")).toBeInTheDocument();
  });

  it("says nothing plays when stopped, and the pill names only the state", () => {
    app.motion = { available: true, engine: { running: false, paused: false } };
    render(<><NowPlayingCard /><NowPlayingPill /></>);
    expect(screen.getByText("Nothing playing")).toBeInTheDocument();
    expect(screen.queryByRole("meter")).not.toBeInTheDocument();
    expect(screen.getByRole("status", { name: "Stopped" })).toBeInTheDocument();
    expect(screen.queryByText("00:00")).not.toBeInTheDocument();
    expect(screen.queryByText("Commanded values, not measurements.")).not.toBeInTheDocument();
  });

  it("shows a vibrator without a Handy as vibration, with no drawing or pace", () => {
    app.state = { settings: { device: { configured_devices: ["vibrator"], playback_devices: ["vibrator"] } } };
    app.motion = { available: true, engine: { running: true, paused: false, running_ms: 30_000,
      target: { vibration_only: true, source: "chat", label: "Waves" } } };
    app.ctx = { intiface: { vibrator: { motors: [{ index: 0, level_percent: 35 }, { index: 1, level_percent: 60 }] } } };
    render(<NowPlayingCard />);
    expect(screen.getByText("Waves")).toBeInTheDocument();
    expect(screen.getByRole("meter", { name: "Vibration: 60%" })).toHaveAttribute("value", "60");
    expect(screen.queryByRole("img", { name: "Device drawing" })).not.toBeInTheDocument();
    expect(screen.getByText("Last commands sent to Intiface, not measured vibration.")).toBeInTheDocument();
  });

  it("gives the status bar one readout with the title and level", () => {
    render(<NowPlayingPill />);
    expect(screen.getByRole("status", { name: "Now playing: Swells, Tempo 48%" })).toBeInTheDocument();
  });

  it("keeps a paused tempo's name without claiming that it plays", () => {
    app.motion = { available: true, engine: { paused: true, running: false,
      target: { tempo_only: true }, tempo: { mode: "wave", level_percent: 40 } } };
    render(<NowPlayingCard />);
    expect(screen.getByText("Paused")).toBeInTheDocument();
    expect(screen.getByText("Waves")).toBeInTheDocument();
    expect(screen.queryByText("Stopped")).not.toBeInTheDocument();
    expect(screen.getByRole("meter", { name: "Tempo: 0%" })).toBeInTheDocument();
  });

  it("reports idle and retired participants from the backend instead of saying they follow", () => {
    app.state = { settings: { device: { playback_devices: ["vacuglide", "coyote", "vibrator"] } } };
    app.motion = { available: true, engine: { running: true, target: { tempo_only: true },
      linked_session: { status: "degraded", idle_devices: ["coyote"], retired_devices: ["vibrator"] },
      tempo: { mode: "steady", level_percent: 40 } } };
    render(<NowPlayingCard />);
    const list = screen.getByRole("list", { name: "In this run" });
    expect(within(list).getByText("Idle")).toBeInTheDocument();
    expect(within(list).getByText("Unavailable")).toBeInTheDocument();
    expect(within(list).queryByText("follows")).not.toBeInTheDocument();
  });

  it("names an Edge hold even while Autopilot owns the session", () => {
    app.motion = { available: true, engine: { running: true, target: { source: "edge", tempo_only: true }, tempo: { mode: "steady", level_percent: 10 } } };
    render(<NowPlayingCard />);
    expect(screen.getByText("Edge hold")).toBeInTheDocument();
    expect(screen.queryByText("Autopilot is choosing")).not.toBeInTheDocument();
  });

  it("keeps position status for an Ultra with a vibrator", () => {
    app.state = { settings: { device: { playback_devices: ["autoblow", "vibrator"] } } };
    app.motion = { available: true, engine: { running: true, target: { speed_percent: 60 } } };
    render(<NowPlayingCard />);
    expect(screen.getByRole("meter", { name: "Pace: 60%" })).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "Device drawing" })).toBeInTheDocument();
  });
});
