import { act, fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { IntifaceTransportSnapshot, IntifaceVibratorStatus } from "../api/types";
import { VibratorPanel } from "./VibratorPanel";

const api = vi.hoisted(() => ({
  intifaceStatus: vi.fn(),
  intifaceConnect: vi.fn(),
  intifaceDisconnect: vi.fn(),
  intifaceStartScan: vi.fn(),
  intifaceStopScan: vi.fn(),
  vibratorSelect: vi.fn(),
  vibratorIntensity: vi.fn(),
  vibratorTest: vi.fn(),
}));
const toast = vi.hoisted(() => vi.fn());
vi.mock("../api/client", () => ({ api }));
vi.mock("../state/app-state", () => ({ useToast: () => ({ show: toast }) }));

const devices = [
  { device_index: 0, device_name: "Simulated Stroker", linear_actuators: [{ index: 0, step_count: 1000 }], scalar_actuators: [] },
  { device_index: 1, device_name: "Simulated Vibrator", linear_actuators: [], scalar_actuators: [
    { index: 0, actuator_type: "Vibrate", step_count: 20 },
    { index: 1, actuator_type: "Vibrate", step_count: 20 },
  ] },
];

function vibrator(overrides: Partial<IntifaceVibratorStatus> = {}): IntifaceVibratorStatus {
  return {
    selected: true, device_index: 1, device_name: "Simulated Vibrator",
    motors: [{ index: 0, step_count: 20 }, { index: 1, step_count: 20 }],
    intensity_percent: 0, ceiling_percent: 40, effective_ceiling_percent: 40, level_percent: 0,
    playback_state: "idle", testing: false, commanded_count: 0, acknowledged_count: 0, rejected_count: 0, timeout_count: 0,
    ...overrides,
  };
}

function snapshot(options: { leases?: string[]; vibrator?: IntifaceVibratorStatus; ping?: boolean; simulated?: boolean } = {}): IntifaceTransportSnapshot {
  return {
    dispatch_owner: "cloud_rest",
    address: "ws://127.0.0.1:12345",
    status: { connected: (options.leases ?? []).length > 0, scanning: false, playback_state: "idle", max_ping_time_ms: 1000, queue_depth: 0, devices },
    diagnostics: {},
    leases: options.leases ?? [],
    ping_watchdog: options.ping ?? true,
    simulated: options.simulated ?? false,
    vibrator: options.vibrator ?? vibrator({ selected: false, device_index: undefined, device_name: undefined, motors: [] }),
  } as IntifaceTransportSnapshot;
}

function renderPanel(initial: IntifaceTransportSnapshot, props: Partial<Parameters<typeof VibratorPanel>[0]> = {}) {
  const onChanged = vi.fn();
  render(<VibratorPanel visible locked={false} canConfigureHost initial={initial} stopSequence={7} onChanged={onChanged} {...props} />);
  return onChanged;
}

beforeEach(() => {
  for (const fn of Object.values(api)) fn.mockReset();
  api.intifaceStatus.mockReturnValue(new Promise(() => undefined));
  toast.mockReset();
});

describe("VibratorPanel", () => {
  it("connects its own lease on the shared Intiface session", async () => {
    api.intifaceConnect.mockResolvedValue(snapshot({ leases: ["vibrator"] }));
    const onChanged = renderPanel(snapshot());
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Connect vibrator" })));
    expect(api.intifaceConnect).toHaveBeenCalledWith("vibrator");
    expect(onChanged).toHaveBeenCalled();
  });

  it("offers only devices with vibration motors", async () => {
    api.vibratorSelect.mockResolvedValue(snapshot({ leases: ["vibrator"], vibrator: vibrator() }));
    renderPanel(snapshot({ leases: ["vibrator"] }));
    const options = screen.getAllByRole("option").map((option) => option.textContent);
    expect(options).toEqual(["Choose a device", "Simulated Vibrator · 2 motors"]);
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Use vibrator" })));
    expect(api.vibratorSelect).toHaveBeenCalledWith(1);
  });

  it("raises with the Stop sequence, lowers without it and starts at zero", async () => {
    api.vibratorIntensity.mockResolvedValue(snapshot({ leases: ["vibrator"], vibrator: vibrator({ intensity_percent: 5 }) }));
    renderPanel(snapshot({ leases: ["vibrator"], vibrator: vibrator() }));
    expect(screen.getByText("0% of 40%")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Lower the vibrator intensity" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Test the vibrator" })).toBeDisabled();
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Raise the vibrator intensity" })));
    expect(api.vibratorIntensity).toHaveBeenLastCalledWith(5, 7);
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Lower the vibrator intensity" })));
    expect(api.vibratorIntensity).toHaveBeenLastCalledWith(0, undefined);
  });

  it("tests only while motion is idle and stops raising at the ceiling", async () => {
    api.vibratorTest.mockResolvedValue(snapshot({ leases: ["vibrator"], vibrator: vibrator({ intensity_percent: 40, testing: true }) }));
    const { rerender } = render(<VibratorPanel visible locked={false} canConfigureHost initial={snapshot({ leases: ["vibrator"], vibrator: vibrator({ intensity_percent: 40 }) })} stopSequence={7} playbackActive onChanged={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Raise the vibrator intensity" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Test the vibrator" })).toBeDisabled();
    rerender(<VibratorPanel visible locked={false} canConfigureHost initial={snapshot({ leases: ["vibrator"], vibrator: vibrator({ intensity_percent: 40 }) })} stopSequence={7} onChanged={vi.fn()} />);
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Test the vibrator" })));
    expect(api.vibratorTest).toHaveBeenCalledWith(7);
  });

  it("says when the connection is shared or has no keepalive, leaving simulation to the top bar", () => {
    renderPanel(snapshot({ leases: ["handy", "vibrator"], ping: false, simulated: true, vibrator: vibrator({ effective_ceiling_percent: 35 }) }));
    expect(screen.getByText("Uses the same Intiface connection as The Handy.")).toBeInTheDocument();
    expect(screen.queryByText(/real device/)).toBeNull();
    expect(screen.getByText(/keepalive timeout is off/)).toBeInTheDocument();
    expect(screen.getByText("With these motors' steps, the highest level is 35%.")).toBeInTheDocument();
  });

  it("does not treat a session held only by the Handy as connected", () => {
    renderPanel(snapshot({ leases: ["handy"] }));
    expect(screen.getByRole("button", { name: "Connect vibrator" })).toBeInTheDocument();
    expect(screen.getByText("Vibrator disconnected")).toBeInTheDocument();
  });
});
