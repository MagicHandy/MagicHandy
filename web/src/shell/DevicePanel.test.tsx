import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { AppState, CoyoteStatus, IntifaceTransportSnapshot } from "../api/types";
import { DeviceRuntimeProvider } from "../devices/runtime";
import { DevicePanel } from "./DevicePanel";

const app = vi.hoisted(() => ({
  state: null as unknown as AppState,
  readOnly: false,
  refresh: vi.fn(async () => undefined),
  show: vi.fn(),
}));

vi.mock("../state/app-state", () => ({
  useAppState: () => ({ state: app.state, backendOnline: true, readOnly: app.readOnly, refresh: app.refresh, stale: false }),
  useMotionState: () => app.state.motion,
  useToast: () => ({ show: app.show }),
}));

vi.mock("../api/client", () => {
  class ApiError extends Error {}
  return {
    ApiError,
    api: {
      applyQuick: vi.fn(async () => undefined),
      changeDeviceConnection: vi.fn(async () => ({ settings: {} })),
      coyoteConnect: vi.fn(),
      coyoteStatus: vi.fn(() => new Promise(() => undefined)),
      coyoteStrength: vi.fn(),
      coyoteTest: vi.fn(),
      coyoteZero: vi.fn(),
      intifaceStatus: vi.fn(() => new Promise(() => undefined)),
      takeControl: vi.fn(async () => ({})),
      bluetoothStatus: vi.fn(() => new Promise(() => undefined)),
    },
  };
});

const coyoteStatus = (patch: Partial<CoyoteStatus> = {}): CoyoteStatus => ({
  connected: true, connecting: false, simulated: true, battery_percent: 82,
  strength_a: 12, strength_b: 8, strength_known: true, strength_source: "reported", strength_pending: false,
  limits_applied: true, ceiling_a: 30, ceiling_b: 30, playback_state: "idle", reference_pulse: false,
  commanded_a: 0, commanded_b: 0, packets_sent: 0, write_failures: 0, starved_packets: 0, max_send_lateness_ms: 0,
  ...patch,
});

const vibratorSession: IntifaceTransportSnapshot = {
  dispatch_owner: "intiface",
  address: "ws://127.0.0.1:12345",
  status: { connected: true, scanning: false, playback_state: "idle", max_ping_time_ms: 0, queue_depth: 0, devices: [] },
  leases: ["vibrator"],
  vibrator: { selected: true, device_index: 3, device_name: "Simulated Vibrator", motors: [{ index: 0, step_count: 20 }], ceiling_percent: 40, effective_ceiling_percent: 40, model_control: true, model_level_percent: 50, level_percent: 0, playback_state: "idle", testing: false, commanded_count: 0, acknowledged_count: 0, rejected_count: 0, timeout_count: 0 },
  diagnostics: {},
};

function stateWith(patch: { configured?: string[]; playing?: string[]; engine?: Record<string, unknown>; coyote?: Partial<CoyoteStatus> } = {}): AppState {
  return {
    stop_sequence: 7,
    capabilities: { configure_host: true, control: true },
    settings: {
      device: {
        hsp_dispatch_owner: "cloud_rest", intiface_server_address: "ws://127.0.0.1:12345", firmware_api_requirement: "firmware_v4_api_v3_required",
        api_application_id_source: "bundled_app_id", connection_key_set: true,
        configured_devices: patch.configured ?? ["handy", "coyote", "vibrator"], playback_devices: patch.playing ?? ["handy", "coyote"],
      },
      motion: { speed_min_percent: 20, speed_max_percent: 80, stroke_min_percent: 10, stroke_max_percent: 90, reverse_direction: false, apply_video_speed_limit: false, style: "balanced", handy_model: "handy_original" },
      estim: { safety_acknowledged: true, max_strength_a: 30, max_strength_b: 30, follow: "stroke", frequency_min_hz: 40, frequency_max_hz: 100, balance: 0, swap_channels: false, timing_offset_ms: 0, frequency_balance_a: 160, frequency_balance_b: 160, intensity_balance_a: 0, intensity_balance_b: 0 },
    },
    cloud_transport: { connected: true, last_latency_ms: 64 },
    coyote_transport: { output: "handy_coyote", available: true, status: coyoteStatus(patch.coyote) },
    intiface_transport: vibratorSession,
    motion: { available: true, engine: { running: false, paused: false, ...patch.engine } },
  } as unknown as AppState;
}

function renderPanel(onOpenChange = vi.fn()) {
  const view = render(<DeviceRuntimeProvider><DevicePanel open onOpenChange={onOpenChange} /></DeviceRuntimeProvider>);
  return { ...view, onOpenChange, panel: screen.getByRole("region", { name: "Devices" }) };
}

beforeEach(() => {
  vi.clearAllMocks();
  app.state = stateWith();
  const snapshot = app.state.coyote_transport;
  vi.mocked(api.coyoteStrength).mockResolvedValue(snapshot!);
  vi.mocked(api.coyoteZero).mockResolvedValue(snapshot!);
  app.readOnly = false;
  window.confirm = () => true;
});

describe("the device panel", () => {
  it("lists the saved devices in run-clock order with one action each", () => {
    const { panel } = renderPanel();
    const rows = within(panel).getAllByRole("group").filter((group) => group.classList.contains("device-row"));
    expect(rows.map((row) => within(row).getByText(/^(The Handy|Coyote|Vibrator)$/).textContent)).toEqual(["The Handy", "Coyote", "Vibrator"]);
    expect(within(rows[0]).getByText("Connected")).toBeInTheDocument();
    expect(within(rows[0]).getByRole("button", { name: "Disconnect The Handy" })).toBeInTheDocument();
    for (const row of rows) {
      expect(within(row).getAllByRole("button").filter((button) => /^(Connect|Disconnect|Reconnect) /.test(button.getAttribute("aria-label") ?? ""))).toHaveLength(1);
    }
    expect(within(panel).getByRole("group", { name: "Coyote strength" })).toBeInTheDocument();
    expect(within(panel).getByRole("group", { name: "Vibrator strength" })).toBeInTheDocument();
    expect(within(panel).queryByRole("textbox")).toBeNull();
    expect(within(panel).queryByRole("combobox")).toBeNull();
  });

  it("changes In playback only after the backend confirms it", async () => {
    const { panel, rerender } = renderPanel();
    const vibrator = within(panel).getByRole("switch", { name: "Use Vibrator in playback" });
    expect(vibrator).not.toBeChecked();
    await act(async () => fireEvent.click(vibrator));
    expect(api.changeDeviceConnection).toHaveBeenCalledWith("vibrator", "include");
    expect(vibrator).not.toBeChecked();
    app.state = stateWith({ playing: ["handy", "coyote", "vibrator"] });
    rerender(<DeviceRuntimeProvider><DevicePanel open onOpenChange={vi.fn()} /></DeviceRuntimeProvider>);
    expect(within(screen.getByRole("region", { name: "Devices" })).getByRole("switch", { name: "Use Vibrator in playback" })).toBeChecked();
  });

  it("steps the Coyote with the Stop sequence and zeroes both channels", async () => {
    const { panel } = renderPanel();
    const strength = within(panel).getByRole("group", { name: "Coyote strength" });
    expect(within(strength).getByText("12 of 30")).toBeInTheDocument();
    await act(async () => fireEvent.click(within(strength).getByRole("button", { name: "Raise Channel A" })));
    expect(api.coyoteStrength).toHaveBeenCalledWith("a", 1, 7);
    await act(async () => fireEvent.click(within(strength).getByRole("button", { name: "Zero both" })));
    expect(api.coyoteZero).toHaveBeenCalledOnce();
    expect(api.coyoteTest).not.toHaveBeenCalled();
  });

  it("offers one Add a device action when no device is added", () => {
    app.state = stateWith({ configured: [], playing: [] });
    const { panel } = renderPanel();
    expect(within(panel).getByText("No devices yet")).toBeInTheDocument();
    expect(within(panel).getByRole("link", { name: "Add a device" })).toHaveAttribute("href", "#/settings/devices/add");
    expect(within(panel).queryByRole("link", { name: "Device settings" })).toBeNull();
    expect(within(panel).queryByRole("slider")).toBeNull();
  });

  it("puts one notice at the top when a run continues without a device", () => {
    app.state = stateWith({ coyote: { connected: false }, engine: { running: true, linked_session: { status: "degraded", continue_healthy: true, retired_devices: ["coyote"] } } });
    const { panel } = renderPanel();
    expect(within(panel).getByText("Continuing without Coyote").closest('[role="status"]')).toHaveTextContent("Stop and start again to rejoin.");
    const coyote = within(panel).getByRole("group", { name: "Coyote" });
    expect(within(coyote).getByText("Connection lost")).toBeInTheDocument();
    expect(within(coyote).getByText(/left this run/)).toBeInTheDocument();
    expect(within(coyote).getByRole("button", { name: "Reconnect Coyote" })).toBeInTheDocument();
  });

  it("lets a watching tab read the status, with one way to take control", () => {
    app.readOnly = true;
    const { panel } = renderPanel();
    expect(within(panel).getByText(/another tab controls magichandy/i)).toBeInTheDocument();
    expect(within(panel).getByRole("button", { name: "Take control" })).toBeInTheDocument();
    expect(within(panel).queryByRole("button", { name: /^(Connect|Disconnect|Reconnect) / })).toBeNull();
    expect(within(panel).queryByRole("group", { name: "Coyote strength" })).toBeNull();
    expect(within(panel).getByText(/A 12 · B 8/)).toBeInTheDocument();
    expect(within(panel).queryByRole("slider")).toBeNull();
    expect(within(panel).getByText("20–80%")).toBeInTheDocument();
    for (const toggle of within(panel).getAllByRole("switch")) expect(toggle).toBeDisabled();
  });

  it("closes as Device settings opens Settings > Devices", () => {
    const { panel, onOpenChange } = renderPanel();
    const [link] = within(panel).getAllByRole("link", { name: "Device settings" });
    expect(link).toHaveAttribute("href", "#/settings/devices");
    fireEvent.click(link);
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("names every device on the chip and keeps its glyphs for narrow screens", () => {
    app.state = stateWith({ coyote: { connected: false, playback_state: "stale" } });
    renderPanel();
    const chip = screen.getByRole("button", { name: /^Devices\. / });
    expect(chip).toHaveAccessibleName("Devices. The Handy connected. Coyote connection lost. Vibrator connected. Close the device panel");
    expect(chip).toHaveTextContent("The Handy + Coyote");
    expect(chip).toHaveTextContent("Coyote connection lost");
    expect(chip.querySelectorAll(".device-chip-glyph")).toHaveLength(3);
  });
});
