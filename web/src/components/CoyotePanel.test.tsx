import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { CoyoteStatus, CoyoteTransportSnapshot } from "../api/types";
import { CoyotePanel, coyoteStrengthText } from "./CoyotePanel";

const toast = vi.hoisted(() => ({ show: vi.fn() }));

vi.mock("../api/client", () => ({
  api: {
    coyoteConnect: vi.fn(),
    coyoteDiscover: vi.fn(),
    coyoteDisconnect: vi.fn(),
    coyoteStatus: vi.fn(),
    coyoteStrength: vi.fn(),
    coyoteTest: vi.fn(),
    coyoteZero: vi.fn(),
    setEstimSafety: vi.fn(),
  },
}));

vi.mock("../state/app-state", () => ({
  useToast: () => toast,
}));

function status(patch: Partial<CoyoteStatus> = {}): CoyoteStatus {
  return {
    connected: true,
    connecting: false,
    simulated: false,
    device_name: "47L121000",
    battery_percent: 87,
    strength_a: 4,
    strength_b: 30,
    strength_known: true,
    strength_source: "reported",
    strength_pending: false,
    limits_applied: true,
    ceiling_a: 30,
    ceiling_b: 30,
    playback_state: "idle",
    reference_pulse: false,
    commanded_a: 0,
    commanded_b: 0,
    packets_sent: 0,
    write_failures: 0,
    starved_packets: 0,
    max_send_lateness_ms: 0,
    ...patch,
  };
}

function snapshot(patch: Partial<CoyoteStatus> = {}): CoyoteTransportSnapshot {
  return { output: "coyote", available: true, status: status(patch) };
}

function renderPanel(props: Partial<Parameters<typeof CoyotePanel>[0]> = {}) {
  const onChanged = vi.fn();
  render(
    <CoyotePanel
      visible
      stopSequence={3}
      locked={false}
      canConfigureHost
      safetyAcknowledged
      initial={snapshot()}
      onChanged={onChanged}
      {...props}
    />,
  );
  return { onChanged };
}

describe("CoyotePanel", () => {
  beforeEach(() => {
    // Poll only when the scenario advances time. A busy full-suite run must
    // not inject an unrelated baseline response into an action assertion.
    vi.useFakeTimers();
    vi.clearAllMocks();
    vi.mocked(api.coyoteStatus).mockResolvedValue(snapshot());
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("shows the safety notes until the host accepts them", async () => {
    vi.mocked(api.setEstimSafety).mockResolvedValue({ settings: {} as never });
    const { onChanged } = renderPanel({ safetyAcknowledged: false });
    expect(screen.getByText(/below the waist only/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Connect Coyote/ })).not.toBeInTheDocument();
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "I have read these notes" })));
    expect(api.setEstimSafety).toHaveBeenCalledWith(true);
    expect(onChanged).toHaveBeenCalled();
  });

  it("renders the device's own strength and ceilings", () => {
    renderPanel();
    expect(screen.getByText("4 of 30")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Raise Channel B" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Raise Channel A" })).toBeEnabled();
    expect(screen.getByText("Battery 87%")).toBeInTheDocument();
  });

  it("sends one-step requests and adopts the backend's reply", async () => {
    vi.mocked(api.coyoteStrength).mockResolvedValue(snapshot({ strength_a: 3 }));
    renderPanel();
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Lower Channel A" })));
    expect(api.coyoteStrength).toHaveBeenCalledWith("a", -1, 3);
    expect(api.coyoteTest).not.toHaveBeenCalled();
    expect(screen.getByText("3 of 30")).toBeInTheDocument();
  });

  it("blocks raising until the strength is confirmed, but always allows lowering", () => {
    renderPanel({ initial: snapshot({ strength_known: false }) });
    expect(screen.getByRole("button", { name: "Raise Channel A" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Lower Channel A" })).toBeEnabled();
    expect(screen.getAllByText("Confirming strength…")).toHaveLength(2);
  });

  it("keeps raising for the host but lowering for everyone", () => {
    renderPanel({ canConfigureHost: false });
    expect(screen.getByRole("button", { name: "Raise Channel A" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Lower Channel A" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Set both to zero" })).toBeEnabled();
  });

  it("connects through the backend", async () => {
    vi.mocked(api.coyoteConnect).mockResolvedValue(snapshot());
    const { onChanged } = renderPanel({ initial: snapshot({ connected: false, strength_known: false }) });
    expect(screen.getByText(/briefly press a wheel/)).toBeInTheDocument();
    await act(async () => fireEvent.click(screen.getByRole("button", { name: /Connect Coyote/ })));
    expect(api.coyoteConnect).toHaveBeenCalled();
    expect(onChanged).toHaveBeenCalled();
    expect(toast.show).toHaveBeenCalledWith("Coyote connected. Check the strength readout before starting.");
  });

  it("requires an explicit choice from nearby devices and connects by opaque ID", async () => {
    vi.mocked(api.coyoteDiscover).mockResolvedValue({ candidates: [
      { id: "chosen", name: "", rssi: -24, identification: "possible" }, { id: "other", name: "47L121000", rssi: -61, identification: "name_match" },
    ] });
    vi.mocked(api.coyoteConnect).mockResolvedValue(snapshot({ strength_known: false }));
    renderPanel({ initial: snapshot({ connected: false, strength_known: false }) });
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Find nearby devices" })));
    expect(api.coyoteConnect).not.toHaveBeenCalled();
    expect(screen.getByRole("combobox")).toHaveValue("");
    expect(screen.getByRole("option", { name: "Possible COYOTE 1 · -24 dBm" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Likely COYOTE 3.0 · -61 dBm" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Connect Coyote/ })).toBeDisabled();
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "chosen" } });
    await act(async () => fireEvent.click(screen.getByRole("button", { name: /Connect Coyote/ })));
    expect(api.coyoteConnect).toHaveBeenCalledWith("chosen");
    expect(screen.getByRole("button", { name: "Raise Channel A" })).toBeDisabled();
  });

  it("formats a compact readout only while connected", () => {
    expect(coyoteStrengthText(status())).toBe("A 4 · B 30");
    expect(coyoteStrengthText(status({ connected: false }))).toBe("");
  });

  it("plays a channel test only on an explicit click with the current Stop sequence", async () => {
    vi.mocked(api.coyoteTest).mockResolvedValue(snapshot({ reference_pulse: true }));
    renderPanel();
    expect(api.coyoteTest).not.toHaveBeenCalled();
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Test Channel A" })));
    expect(api.coyoteTest).toHaveBeenCalledWith("a", 3);
    expect(api.coyoteStrength).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Test Channel A" })).toBeDisabled();
  });

  it.each([
    { strength_pending: true },
    { limits_applied: false },
    { strength_known: false },
    { playback_state: "stopping" },
  ])("blocks increases and tests for an unready device: %j", (patch) => {
    renderPanel({ initial: snapshot(patch) });
    expect(screen.getByRole("button", { name: "Raise Channel A" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Test Channel A" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Lower Channel A" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Set both to zero" })).toBeEnabled();
  });

  it("distinguishes commanded zero from a device report", () => {
    renderPanel({ initial: snapshot({ strength_a: 0, strength_b: 0, strength_source: "zero_command" }) });
    expect(screen.getByText("Zero command accepted. The device has not reported a strength change.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Test Channel A" })).toBeDisabled();
  });

  it.each([
    { locked: true },
    { canConfigureHost: false },
    { stopSequence: undefined },
    { initial: snapshot({ playback_state: "playing" }) },
  ])("rejects a test without host authority, fresh state and idle output: %j", (props) => {
    renderPanel(props);
    expect(screen.getByRole("button", { name: "Test Channel A" })).toBeDisabled();
  });
});
