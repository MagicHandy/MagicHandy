import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { AppState, CoyoteStatus, IntifaceTransportSnapshot } from "../api/types";
import { DevicesSettings } from "./DevicesSettings";
import { DeviceRuntimeProvider } from "./runtime";

const app = vi.hoisted(() => ({
  state: null as unknown as AppState,
  refresh: vi.fn(async () => undefined),
  show: vi.fn(),
}));

vi.mock("../state/app-state", () => ({
  useAppState: () => ({ state: app.state, backendOnline: true, readOnly: false, refresh: app.refresh, stale: false }),
  useMotionState: () => app.state.motion,
  useToast: () => ({ show: app.show }),
}));

vi.mock("../api/client", () => ({
  api: {
    applyQuick: vi.fn(async () => undefined),
    addHandy: vi.fn(),
    updateHandy: vi.fn(async () => ({ settings: {} })),
    removeHandy: vi.fn(async () => ({ settings: {} })),
    handyConnections: vi.fn(() => new Promise(() => undefined)),
    handyConnect: vi.fn(),
    handyDisconnect: vi.fn(),
    handyDiscover: vi.fn(),
    handySelect: vi.fn(),
    bluetoothStatus: vi.fn(() => new Promise(() => undefined)),
    changeDeviceConnection: vi.fn(async () => ({ settings: {} })),
    connectionCheck: vi.fn(),
    coyoteConnect: vi.fn(),
    coyoteDiscover: vi.fn(),
    coyoteStatus: vi.fn(() => new Promise(() => undefined)),
    intifaceDisconnect: vi.fn(),
    intifaceSelect: vi.fn(),
    intifaceStartScan: vi.fn(),
    intifaceStatus: vi.fn(() => new Promise(() => undefined)),
    intifaceStopScan: vi.fn(),
    saveConnectionKey: vi.fn(async () => ({ settings: {} })),
    saveDeviceOptions: vi.fn(async () => ({ settings: {} })),
    saveSetupPreferences: vi.fn(async () => ({ settings: {} })),
    setEstimSafety: vi.fn(async () => ({ settings: {} })),
    startManualTest: vi.fn(),
    stopMotion: vi.fn(),
    vibratorSelect: vi.fn(),
  },
}));

const coyoteStatus = (patch: Partial<CoyoteStatus> = {}): CoyoteStatus => ({
  connected: false, connecting: false, simulated: false,
  strength_a: 0, strength_b: 0, strength_known: false, strength_source: "unknown", strength_pending: false,
  limits_applied: false, ceiling_a: 30, ceiling_b: 30, playback_state: "idle", reference_pulse: false,
  commanded_a: 0, commanded_b: 0, packets_sent: 0, write_failures: 0, starved_packets: 0, max_send_lateness_ms: 0,
  ...patch,
});

function session(patch: Partial<IntifaceTransportSnapshot> = {}): IntifaceTransportSnapshot {
  return {
    dispatch_owner: "intiface",
    address: "ws://127.0.0.1:12345",
    status: { connected: false, scanning: false, playback_state: "idle", max_ping_time_ms: 0, queue_depth: 0, devices: [] },
    diagnostics: {},
    ...patch,
  };
}

function stateWith(patch: { configured?: string[]; owner?: string; estim?: Record<string, unknown>; intiface?: IntifaceTransportSnapshot; coyote?: Partial<CoyoteStatus> } = {}): AppState {
  return {
    stop_sequence: 7,
    capabilities: { configure_host: true, control: true },
    settings: {
      server: { port: 49717 },
      device: {
        hsp_dispatch_owner: patch.owner ?? "cloud_rest", intiface_server_address: "ws://127.0.0.1:12345", firmware_api_requirement: "firmware_v4_api_v3_required",
        api_application_id_source: "bundled_app_id", connection_key_set: true, linked_failure_policy: "stop_all",
        configured_devices: patch.configured ?? ["handy", "coyote", "vibrator"], playback_devices: ["handy"],
      },
      motion: { speed_min_percent: 20, speed_max_percent: 80, stroke_min_percent: 10, stroke_max_percent: 90, reverse_direction: false, apply_video_speed_limit: false, style: "balanced", handy_model: "handy_original" },
      estim: { safety_acknowledged: true, max_strength_a: 30, max_strength_b: 30, follow: "stroke", frequency_min_hz: 40, frequency_max_hz: 100, balance: 0, swap_channels: false, timing_offset_ms: 0, frequency_balance_a: 160, frequency_balance_b: 160, intensity_balance_a: 0, intensity_balance_b: 0, remembered_device: true, ...patch.estim },
      vibration: { follow: "speed", depth_end: "base", minimum_percent: 0, ceiling_percent: 40, timing_offset_ms: 0 },
      options: { hsp_dispatch_owners: ["cloud_rest", "native_bluetooth", "browser_bluetooth", "intiface"], api_application_id_sources: ["bundled_app_id", "developer_override"] },
    },
    cloud_transport: { connected: false },
    coyote_transport: { output: "handy", available: true, status: coyoteStatus(patch.coyote) },
    native_bluetooth_transport: { available: true, supported: true, remembered_device: false, status: { connected: false, connecting: false, simulated: false, status: "idle", message: "", clock_synced: false, write_bytes: 0, playback_state: "idle" } },
    intiface_transport: patch.intiface ?? session(),
    motion: { available: true, engine: { running: false, paused: false } },
  } as unknown as AppState;
}

function renderPage(page: string) {
  return render(<DeviceRuntimeProvider><DevicesSettings page={page} /></DeviceRuntimeProvider>);
}

beforeEach(() => {
  vi.clearAllMocks();
  app.state = stateWith();
  window.location.hash = "#/settings/devices";
  window.confirm = () => true;
});

describe("Settings > Devices", () => {
  it("lists each device with how it connects and opens its page", () => {
    renderPage("");
    const list = screen.getByRole("heading", { name: "Your devices" }).closest("section")!;
    const handy = within(list).getByRole("link", { name: /The Handy/ });
    expect(handy).toHaveAttribute("href", "#/settings/devices/handy");
    expect(handy).toHaveTextContent("Handy Cloud · Original · 110 mm");
    expect(within(list).getByRole("link", { name: /Coyote/ })).toHaveTextContent("ceilings 30 and 30");
    expect(within(list).getByRole("button", { name: "Add device" })).toBeInTheDocument();
    expect(screen.getByText("Changes save as you make them")).toBeInTheDocument();
  });

  it("adds a device without connecting it and opens its page", async () => {
    app.state = stateWith({ configured: ["handy"] });
    renderPage("add");
    const choice = screen.getByRole("button", { name: /Coyote Two-channel e-stim/ });
    expect(choice).toHaveFocus();
    await act(async () => fireEvent.click(choice));
    expect(api.changeDeviceConnection).toHaveBeenCalledWith("coyote", "add");
    expect(api.coyoteConnect).not.toHaveBeenCalled();
    expect(window.location.hash).toBe("#/settings/devices/coyote");
  });

  it("saves the failure policy and the timing offsets as they change", async () => {
    renderPage("");
    await act(async () => fireEvent.change(screen.getByRole("combobox", { name: "On device failure" }), { target: { value: "continue_healthy" } }));
    expect(api.saveDeviceOptions).toHaveBeenCalledWith({ linked_failure_policy: "continue_healthy" });
    expect(app.refresh).toHaveBeenCalled();
    const vibrator = screen.getByRole("spinbutton", { name: "Vibrator" });
    fireEvent.change(vibrator, { target: { value: "40" } });
    expect(api.saveDeviceOptions).toHaveBeenCalledTimes(1);
    await act(async () => fireEvent.blur(vibrator));
    expect(api.saveDeviceOptions).toHaveBeenLastCalledWith({ vibration: { timing_offset_ms: 40 } });
    const coyote = screen.getByRole("spinbutton", { name: "Coyote" });
    fireEvent.change(coyote, { target: { value: "900" } });
    await act(async () => fireEvent.keyDown(coyote, { key: "Enter" }));
    expect(api.saveDeviceOptions).toHaveBeenLastCalledWith({ estim: { timing_offset_ms: 300 } });
  });

  it("keeps one Intiface Central connection: its address, its users and one Disconnect", async () => {
    app.state = stateWith({ owner: "intiface", intiface: session({ leases: ["handy", "vibrator"], ping_watchdog: true, status: { connected: true, scanning: false, playback_state: "idle", max_ping_time_ms: 500, queue_depth: 0, devices: [] } }) });
    vi.mocked(api.intifaceDisconnect).mockResolvedValue(session());
    const confirm = vi.fn(() => true);
    window.confirm = confirm;
    renderPage("");
    expect(screen.getByText("Connected · used by The Handy, Vibrator")).toBeInTheDocument();
    expect(screen.getByText(/keepalive is on/i)).toBeInTheDocument();
    const address = screen.getByRole("textbox", { name: "Server address" });
    fireEvent.change(address, { target: { value: "ws://192.168.1.20:12345" } });
    await act(async () => fireEvent.keyDown(address, { key: "Enter" }));
    expect(confirm).toHaveBeenCalledOnce();
    expect(api.saveDeviceOptions).toHaveBeenCalledWith({ intiface_server_address: "ws://192.168.1.20:12345" });
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Disconnect" })));
    expect(vi.mocked(api.intifaceDisconnect).mock.calls).toEqual([["handy"], ["vibrator"]]);
  });

  it("names the devices that use Intiface Central before it connects", () => {
    app.state = stateWith({ owner: "intiface" });
    renderPage("");
    expect(screen.getByText("Not connected · used by The Handy, Vibrator")).toBeInTheDocument();
    expect(screen.getByText("A device's Connect opens this connection when it needs it.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Disconnect" })).not.toBeInTheDocument();
  });
});

describe("The Handy's page", () => {
  it("applies a new connection method only through Use this method", async () => {
    renderPage("handy");
    expect(screen.getByRole("heading", { level: 2, name: "The Handy" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("radio", { name: /Bluetooth on this computer/ }));
    expect(api.saveSetupPreferences).not.toHaveBeenCalled();
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Use this method" })));
    expect(api.saveSetupPreferences).toHaveBeenCalledWith({ device_owner: "native_bluetooth" });
  });

  it("keeps the connection key write-only with an explicit Save", async () => {
    const confirm = vi.fn(() => true);
    window.confirm = confirm;
    renderPage("handy");
    const key = screen.getByLabelText("Connection key");
    expect(key).toHaveAttribute("type", "password");
    expect(key).toHaveValue("");
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    fireEvent.change(key, { target: { value: "new-key" } });
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Save" })));
    expect(api.saveConnectionKey).toHaveBeenCalledWith("new-key");
    expect(api.saveDeviceOptions).not.toHaveBeenCalled();
    await waitFor(() => expect(key).toHaveValue(""));
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Forget key" })));
    expect(confirm).toHaveBeenCalledOnce();
    expect(api.saveDeviceOptions).toHaveBeenCalledWith({ clear_connection_key: true });
  });

  it("holds the calibration and the motion test", () => {
    renderPage("handy");
    const calibration = screen.getByRole("heading", { name: "Calibration" }).closest("section")!;
    expect(within(within(calibration).getByRole("radiogroup", { name: "Handy model" })).getByRole("radio", { name: "Original" })).toBeChecked();
    expect(within(calibration).getByRole("switch", { name: "Reverse direction" })).toBeInTheDocument();
    const test = screen.getByRole("heading", { name: "Test" }).closest("section")!;
    expect(within(test).getByRole("button", { name: "Start test" })).toBeEnabled();
  });
});

describe("The Coyote's page", () => {
  it("shows the safety notes before the first connection and records them", async () => {
    app.state = stateWith({ estim: { safety_acknowledged: false } });
    renderPage("coyote");
    expect(screen.getByText("Needs setup")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Find nearby devices" })).toBeDisabled();
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "I have read these notes" })));
    expect(api.setEstimSafety).toHaveBeenCalledWith(true);
  });

  it("finds nearby Coyotes and connects the chosen one by its opaque ID", async () => {
    vi.mocked(api.coyoteDiscover).mockResolvedValue({ candidates: [
      { id: "cand-1", name: "", rssi: -50, identification: "possible" },
      { id: "cand-2", name: "47L121000", rssi: -61, identification: "name_match" },
    ] });
    vi.mocked(api.coyoteConnect).mockResolvedValue({ output: "handy", available: true, status: coyoteStatus({ connected: true }) });
    renderPage("coyote");
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Find nearby devices" })));
    const connect = screen.getByRole("button", { name: "Connect" });
    expect(connect).toBeDisabled();
    fireEvent.click(screen.getByRole("radio", { name: /Likely COYOTE 3.0/ }));
    await act(async () => fireEvent.click(connect));
    expect(api.coyoteConnect).toHaveBeenCalledWith("cand-2");
  });

  it("saves output changes as they happen", async () => {
    renderPage("coyote");
    const output = screen.getByRole("heading", { name: "Output" }).closest("section")!;
    await act(async () => fireEvent.click(within(output).getByRole("radio", { name: "Speed" })));
    expect(api.saveDeviceOptions).toHaveBeenCalledWith({ estim: { follow: "speed" } });
    await act(async () => fireEvent.click(within(output).getByRole("switch", { name: /Swap channels/ })));
    expect(api.saveDeviceOptions).toHaveBeenCalledWith({ estim: { swap_channels: true } });
    fireEvent.change(within(output).getByRole("slider", { name: "Channel A" }), { target: { value: "45" } });
    await waitFor(() => expect(api.saveDeviceOptions).toHaveBeenCalledWith({ estim: { max_strength_a: 45 } }));
  });

  it("forgets the remembered Coyote", async () => {
    renderPage("coyote");
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Forget" })));
    expect(api.saveDeviceOptions).toHaveBeenCalledWith({ estim: { forget_device: true } });
  });

  it("removes a device after confirming and returns to Devices", async () => {
    const confirm = vi.fn(() => true);
    window.confirm = confirm;
    window.location.hash = "#/settings/devices/coyote";
    renderPage("coyote");
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Remove Coyote" })));
    expect(confirm).toHaveBeenCalledOnce();
    expect(api.changeDeviceConnection).toHaveBeenCalledWith("coyote", "remove");
    expect(window.location.hash).toBe("#/settings/devices");
  });
});

describe("The vibrator's page", () => {
  const devices = [
    { device_index: 2, device_name: "Simulated Stroker", linear_actuators: [{ index: 0 }], scalar_actuators: [] },
    { device_index: 3, device_name: "Simulated Vibrator", scalar_actuators: [{ index: 0, actuator_type: "Vibrate", step_count: 20 }, { index: 1, actuator_type: "Vibrate", step_count: 20 }] },
  ];

  it("offers only devices with vibration motors and uses the chosen one", async () => {
    app.state = stateWith({ intiface: session({ leases: ["vibrator"], status: { connected: true, scanning: false, playback_state: "idle", max_ping_time_ms: 0, queue_depth: 0, devices } }) });
    vi.mocked(api.vibratorSelect).mockResolvedValue(session());
    renderPage("vibrator");
    expect(screen.getByRole("radio", { name: /Simulated Stroker/ })).toBeDisabled();
    expect(screen.getByRole("radio", { name: /Simulated Vibrator 2 vibration motors/ })).toBeChecked();
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Use this vibrator" })));
    expect(api.vibratorSelect).toHaveBeenCalledWith(3);
  });

  it("asks to connect before listing devices", () => {
    renderPage("vibrator");
    expect(screen.getByText(/connect the vibrator to list the devices/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Use this vibrator" })).toBeNull();
  });

  it("saves what the vibration follows and shows the depth end only for Depth", async () => {
    // The backend confirms the change in its next snapshot.
    vi.mocked(api.saveDeviceOptions).mockImplementation(async (patch) => {
      Object.assign(app.state.settings!.vibration!, patch.vibration);
      return { settings: app.state.settings! };
    });
    renderPage("vibrator");
    expect(screen.queryByRole("radiogroup", { name: "Strongest at" })).toBeNull();
    await act(async () => fireEvent.click(screen.getByRole("radio", { name: "Depth" })));
    expect(api.saveDeviceOptions).toHaveBeenCalledWith({ vibration: { follow: "depth" } });
    expect(screen.getByRole("radiogroup", { name: "Strongest at" })).toBeInTheDocument();
  });

  it("offers to add a device that is not added yet", async () => {
    app.state = stateWith({ configured: ["handy"] });
    renderPage("vibrator");
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Add Vibrator" })));
    expect(api.changeDeviceConnection).toHaveBeenCalledWith("vibrator", "add");
  });
});


describe("additional Handys", () => {
  const id = "handy-010203040506" as const;
  function extra(method = "cloud_rest") {
    app.state.settings!.device.configured_devices = ["handy", id];
    app.state.settings!.device.handy_connections = [{ id, name: "Partner's Handy", method, handy_model: "handy_2_standard", connection_key_set: true, remembered_device: false }];
    app.state.handy_connections = [{ id, connected: false, connecting: false, status: "disconnected", simulated: false, native_available: true, native_supported: true }];
  }
  it("keeps Add Handy available and navigates using the returned ID without connecting", async () => {
    vi.mocked(api.addHandy).mockResolvedValue({ device_id: id, settings: app.state.settings! });
    renderPage("add");
    await act(async () => fireEvent.click(screen.getByRole("button", { name: /Another Handy/ })));
    expect(api.addHandy).toHaveBeenCalledOnce();
    expect(api.handyConnect).not.toHaveBeenCalled();
    expect(api.changeDeviceConnection).not.toHaveBeenCalled();
    expect(window.location.hash).toBe("#/settings/devices/" + id);
  });
  it("lists the additional instance by name and keeps it excluded", () => {
    extra(); renderPage("");
    const row = screen.getByRole("link", { name: /Partner's Handy/ });
    expect(row).toHaveAttribute("href", "#/settings/devices/" + id);
    expect(row).toHaveTextContent("Not in playback");
    expect(screen.getByText(/one shared motion from one timeline/)).toBeInTheDocument();
  });
  it("edits only this instance's name and key, and clears the key after Save", async () => {
    extra(); renderPage(id);
    const name = screen.getByRole("textbox", { name: "Name" });
    fireEvent.change(name, { target: { value: "Guest Handy" } });
    await act(async () => fireEvent.blur(name));
    expect(api.updateHandy).toHaveBeenCalledWith(id, { name: "Guest Handy" });
    const key = screen.getByLabelText("Connection key");
    fireEvent.change(key, { target: { value: "synthetic-private-key" } });
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Save" })));
    expect(api.updateHandy).toHaveBeenCalledWith(id, { connection_key: "synthetic-private-key" });
    await waitFor(() => expect(key).toHaveValue(""));
    expect(api.saveConnectionKey).not.toHaveBeenCalled();
  });
  it("offers supported methods only and removes exactly this instance", async () => {
    extra(); renderPage(id);
    expect(screen.queryByRole("radio", { name: /Browser Bluetooth/ })).not.toBeInTheDocument();
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Remove Partner's Handy" })));
    expect(api.removeHandy).toHaveBeenCalledWith(id);
    expect(api.changeDeviceConnection).not.toHaveBeenCalled();
  });
  it("disables setup and connection changes for an operator", () => {
    extra(); app.state.capabilities!.configure_host = false; renderPage(id);
    expect(screen.getByRole("textbox", { name: "Name" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Connect Partner's Handy" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Remove Partner's Handy" })).toBeDisabled();
  });
  it("disconnects all named Intiface leases through their own endpoints", async () => {
    extra("intiface");
    app.state.settings!.device.hsp_dispatch_owner = "intiface";
    app.state.intiface_transport = session({ leases: ["handy", id], status: { connected: true, scanning: false, playback_state: "idle", max_ping_time_ms: 500, queue_depth: 0, devices: [] } });
    vi.mocked(api.intifaceDisconnect).mockResolvedValue(session());
    vi.mocked(api.handyDisconnect).mockResolvedValue(app.state.handy_connections![0]);
    renderPage("");
    expect(screen.getByText("Connected · used by The Handy, Partner's Handy")).toBeInTheDocument();
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Disconnect" })));
    expect(api.intifaceDisconnect).toHaveBeenCalledWith("handy");
    expect(api.handyDisconnect).toHaveBeenCalledWith(id);
  });
});
