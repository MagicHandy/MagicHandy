import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { DeviceOutput, IntifaceTransportSnapshot } from "../api/types";
import { ConnectionManager } from "./ConnectionManager";

const appState = vi.hoisted(() => ({ value: {} as Record<string, unknown> }));
const toast = vi.hoisted(() => vi.fn());
const api = vi.hoisted(() => ({ saveSetupPreferences: vi.fn(), changeDeviceConnection: vi.fn(), nativeBluetoothConnect: vi.fn() }));
const lifecycle = vi.hoisted(() => ({ bluetoothUnmounts: 0, coyoteUnmounts: 0, coyoteProps: {} as Record<string, unknown> }));

vi.mock("../state/app-state", () => ({
  useAppState: () => appState.value,
  useToast: () => ({ show: toast }),
}));
vi.mock("../api/client", () => ({ api }));
vi.mock("../components/IntifacePanel", () => ({ IntifacePanel: () => null }));
vi.mock("../components/VibratorPanel", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../components/VibratorPanel")>();
  return { ...actual, VibratorPanel: (props: { visible: boolean }) => props.visible ? <div data-testid="vibrator-panel" /> : null };
});
vi.mock("../components/QuickSettings", () => ({ QuickSettings: () => null }));
vi.mock("../components/BluetoothBridge", async () => {
  const { useEffect } = await import("react");
  return {
    BluetoothBridge: () => {
      useEffect(() => () => { lifecycle.bluetoothUnmounts += 1; }, []);
      return null;
    },
  };
});
vi.mock("../components/CoyotePanel", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../components/CoyotePanel")>();
  const { useEffect } = await import("react");
  return {
    ...actual,
    CoyotePanel: (props: Record<string, unknown>) => {
      lifecycle.coyoteProps = props;
      useEffect(() => () => { lifecycle.coyoteUnmounts += 1; }, []);
      return props.visible ? <div data-testid="coyote-panel" /> : null;
    },
  };
});

beforeEach(() => {
  toast.mockReset();
  api.saveSetupPreferences.mockReset().mockResolvedValue({ settings: {} });
  api.changeDeviceConnection.mockReset().mockResolvedValue({ settings: {} });
  api.nativeBluetoothConnect.mockReset();
  lifecycle.bluetoothUnmounts = 0;
  lifecycle.coyoteUnmounts = 0;
  lifecycle.coyoteProps = {};
});

function intiface(selected: boolean): IntifaceTransportSnapshot {
  return {
    dispatch_owner: "intiface",
    address: "ws://127.0.0.1:12345",
    status: {
      connected: true,
      scanning: true,
      playback_state: "idle",
      max_ping_time_ms: 0,
      queue_depth: 0,
      devices: [{ device_index: 7, device_name: "Test Linear", linear_actuators: [{ index: 0, step_count: 10000 }] }],
      ...(selected ? { selected_device_index: 7, selected_actuator_index: 0 } : {}),
    },
    diagnostics: {},
  } as unknown as IntifaceTransportSnapshot;
}

function renderWith(snapshot: IntifaceTransportSnapshot) {
  appState.value = {
    backendOnline: true,
    readOnly: false,
    refresh: vi.fn(),
    state: { settings: { device: { hsp_dispatch_owner: "intiface" } }, intiface_transport: snapshot },
  };
  return render(<ConnectionManager open={false} onOpenChange={vi.fn()} />).container.querySelector(".connection-manager");
}

interface ManagerOptions {
  owner?: string;
  output?: DeviceOutput;
  configured?: Array<"handy" | "coyote" | "vibrator">;
  playback?: Array<"handy" | "coyote" | "vibrator">;
  readOnly?: boolean;
  configureHost?: boolean;
  safetyAcknowledged?: boolean;
  coyoteConnected?: boolean;
  methods?: string[];
}

function managerState(options: ManagerOptions = {}) {
  return {
    stop_sequence: 7,
    capabilities: { configure_host: options.configureHost ?? true },
    settings: {
      device: { hsp_dispatch_owner: options.owner ?? "cloud_rest", output: options.output ?? "handy", configured_devices: options.configured, playback_devices: options.playback, connection_key_set: true },
      estim: { safety_acknowledged: options.safetyAcknowledged ?? true },
      options: { hsp_dispatch_owners: options.methods ?? ["cloud_rest", "browser_bluetooth", "intiface"] },
    },
    coyote_transport: {
      output: options.output ?? "handy",
      available: true,
      status: { connected: options.coyoteConnected ?? false, connecting: false, simulated: true, strength_a: 0, strength_b: 0, strength_known: false, strength_source: "unknown", strength_pending: false, limits_applied: false, ceiling_a: 30, ceiling_b: 30, playback_state: "idle", reference_pulse: false },
    },
  };
}

function renderManager(options: ManagerOptions = {}) {
  const refresh = vi.fn();
  const onOpenChange = vi.fn();
  appState.value = { backendOnline: true, readOnly: options.readOnly ?? false, refresh, state: managerState(options) };
  const view = render(<ConnectionManager open onOpenChange={onOpenChange} />);
  const rerenderWith = (next: ManagerOptions) => {
    appState.value = { ...appState.value, readOnly: next.readOnly ?? false, state: managerState(next) };
    view.rerender(<ConnectionManager open onOpenChange={onOpenChange} />);
  };
  return { ...view, refresh, onOpenChange, rerenderWith };
}

describe("Intiface connection status during a Bluetooth scan", () => {
  it("reads as connected once an actuator is selected, even while the scan runs", () => {
    const manager = renderWith(intiface(true));
    expect(manager).toHaveAttribute("data-phase", "connected");
    expect(screen.getAllByText("Intiface connected").length).toBeGreaterThan(0);
    expect(screen.queryByText("Finding a linear device")).not.toBeInTheDocument();
  });

  it("keeps looking while the scan runs and nothing is selected", () => {
    const manager = renderWith(intiface(false));
    expect(manager).toHaveAttribute("data-phase", "connecting");
    expect(screen.getAllByText("Finding a linear device").length).toBeGreaterThan(0);
  });
});

describe("connection method in the manager title", () => {
  it("lists the Handy methods with Setup's explanations and applies only an explicit choice", async () => {
    const { refresh } = renderManager({ owner: "cloud_rest" });
    const trigger = screen.getByRole("button", { name: "The Handy Handy Cloud REST" });
    expect(trigger).toHaveAttribute("aria-haspopup", "menu");

    fireEvent.click(trigger);
    const menu = screen.getByRole("menu", { name: "The Handy" });
    const items = within(menu).getAllByRole("menuitemradio");
    expect(items.map((item) => item.getAttribute("aria-checked"))).toEqual(["true", "false", "false"]);
    expect(within(menu).getByRole("menuitemradio", { name: "Browser Bluetooth" }))
      .toHaveAccessibleDescription("Connect directly from a compatible browser. No cloud connection key is stored.");
    expect(within(menu).getByRole("menuitemradio", { name: "Intiface Central" }))
      .toHaveAccessibleDescription("Use an existing Intiface Central session for local device control.");
    expect(menu).toHaveAccessibleDescription("Changing it stops motion.");

    // Browsing with the keyboard moves focus; it never switches the transport.
    fireEvent.keyDown(menu, { key: "ArrowDown" });
    fireEvent.keyDown(menu, { key: "ArrowDown" });
    expect(within(menu).getByRole("menuitemradio", { name: "Intiface Central" })).toHaveFocus();
    expect(api.saveSetupPreferences).not.toHaveBeenCalled();

    fireEvent.click(within(menu).getByRole("menuitemradio", { name: "Intiface Central" }));
    expect(screen.queryByRole("menu")).toBeNull();
    expect(trigger).toHaveFocus();
    await waitFor(() => expect(api.saveSetupPreferences).toHaveBeenCalledWith({ device_owner: "intiface" }));
    await waitFor(() => expect(refresh).toHaveBeenCalled());
    // The title keeps the saved method until the backend snapshot changes.
    expect(screen.getByRole("button", { name: "The Handy Handy Cloud REST" })).toBeInTheDocument();
  });

  it("closes without a request when the current method is chosen again", () => {
    renderManager({ owner: "browser_bluetooth" });
    fireEvent.click(screen.getByRole("button", { name: "The Handy Browser Bluetooth" }));
    fireEvent.click(screen.getByRole("menuitemradio", { name: "Browser Bluetooth" }));
    expect(screen.queryByRole("menu")).toBeNull();
    expect(api.saveSetupPreferences).not.toHaveBeenCalled();
  });

  it("offers only the methods the backend advertises", () => {
    renderManager({ owner: "cloud_rest", methods: ["cloud_rest", "intiface"] });
    fireEvent.click(screen.getByRole("button", { name: "The Handy Handy Cloud REST" }));
    expect(screen.getAllByRole("menuitemradio").map((item) => item.querySelector("strong")?.textContent)).toEqual(["Handy Cloud REST", "Intiface Central"]);
  });

  it("reports a saved method whose previous device did not stop cleanly", async () => {
    api.saveSetupPreferences.mockResolvedValue({ settings: {}, warning: "preferences were saved, but the selected runtime is not ready yet" });
    renderManager({ owner: "cloud_rest" });
    fireEvent.click(screen.getByRole("button", { name: "The Handy Handy Cloud REST" }));
    fireEvent.click(screen.getByRole("menuitemradio", { name: "Browser Bluetooth" }));
    await waitFor(() => expect(toast).toHaveBeenCalledWith("The setting was saved, but the device runtime could not be updated.", "warning"));
  });

  it("shows the method as text with the reason when this client cannot change it", () => {
    const { container, rerenderWith } = renderManager({ owner: "cloud_rest", readOnly: true });
    const head = container.querySelector<HTMLElement>(".connection-handy .connection-device-head")!;
    expect(within(head).queryByRole("button", { name: /^the handy /i })).toBeNull();
    expect(within(head).getByText("Handy Cloud REST")).toBeInTheDocument();
    expect(screen.getByText("Read-only client.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add connection" })).toHaveAttribute("aria-disabled", "true");

    rerenderWith({ owner: "intiface", configureHost: false });
    expect(within(head).getByText("Intiface Central")).toBeInTheDocument();
    expect(screen.getByText("Host settings and diagnostics are managed by an administrator.")).toBeInTheDocument();
  });

  it("puts the Settings link in the title row and closes the manager on use", () => {
    const { container, onOpenChange } = renderManager();
    const head = container.querySelector<HTMLElement>(".connection-manager-head")!;
    const link = within(head).getByRole("link", { name: "Configure device" });
    expect(link).toHaveAttribute("href", "#/settings/device");
    fireEvent.click(link);
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});

describe("Coyote in the connection manager", () => {
  it("names the Coyote's own Bluetooth link instead of offering Handy methods", () => {
    const { container } = renderManager({ owner: "cloud_rest", output: "coyote" });
    const head = container.querySelector<HTMLElement>(".connection-coyote .connection-device-head")!;
    expect(within(head).queryByRole("button", { name: /^the handy /i })).toBeNull();
    expect(within(head).getByText("Coyote Bluetooth")).toBeInTheDocument();
    expect(container.querySelector(".connection-handy")).toHaveAttribute("hidden");
    expect(screen.queryByRole("img", { name: /the handy wireless connection/i })).toBeNull();
    expect(screen.getByTestId("coyote-panel")).toBeInTheDocument();
    expect(lifecycle.coyoteProps).toMatchObject({ visible: true, safetyAcknowledged: true, stopSequence: 7 });
    expect(container.querySelector(".connection-coyote-route")).toBeInTheDocument();
  });

  it("gives simultaneous connections their own protocol titles", () => {
    const { container } = renderManager({ owner: "browser_bluetooth", output: "handy_coyote", coyoteConnected: true });
    expect(screen.getByRole("button", { name: "The Handy Browser Bluetooth" })).toBeInTheDocument();
    const coyote = container.querySelector<HTMLElement>(".connection-coyote")!;
    expect(within(coyote).getByRole("heading", { name: "Coyote" })).toBeInTheDocument();
    expect(within(coyote).getByText("Coyote Bluetooth")).toBeInTheDocument();
    // Once connected, the route explanation gives way to the controls.
    expect(coyote.querySelector(".connection-coyote-route")).toBeNull();
  });

  it("explains the Coyote route only after the safety notes are accepted", () => {
    const { container } = renderManager({ output: "coyote", safetyAcknowledged: false });
    expect(lifecycle.coyoteProps).toMatchObject({ safetyAcknowledged: false });
    expect(container.querySelector(".connection-coyote-route")).toBeNull();
  });

  it("keeps the device sessions mounted while the output changes", () => {
    const { rerenderWith } = renderManager({ owner: "browser_bluetooth", output: "handy" });
    rerenderWith({ owner: "browser_bluetooth", output: "coyote" });
    rerenderWith({ owner: "browser_bluetooth", output: "handy_coyote" });
    rerenderWith({ owner: "intiface", output: "handy" });
    expect(lifecycle.bluetoothUnmounts).toBe(0);
    expect(lifecycle.coyoteUnmounts).toBe(0);
  });
});

describe("additive connections", () => {
  it("adds only the missing device and keeps the backend's list until it confirms", async () => {
    let confirm!: () => void;
    api.changeDeviceConnection.mockReturnValue(new Promise((resolve) => { confirm = () => resolve({ settings: {} }); }));
    const { refresh, rerenderWith } = renderManager({ output: "handy" });
    expect(screen.queryByRole("radio")).toBeNull();
    expect(screen.getAllByRole("button", { name: /^Remove / })).toHaveLength(1);
    fireEvent.click(screen.getByRole("button", { name: "Add connection" }));
    expect(screen.getAllByRole("menuitem").map((item) => item.textContent)).toEqual(["CoyoteCoyote Bluetooth", "VibratorIntiface Central"]);
    fireEvent.click(screen.getByRole("menuitem", { name: "Coyote Coyote Bluetooth" }));
    // Repeated clicks and a protocol change cannot race the pending save.
    fireEvent.click(screen.getByRole("button", { name: "Add connection" }));
    fireEvent.click(screen.getByRole("button", { name: "The Handy Handy Cloud REST" }));
    expect(screen.queryByRole("menu")).toBeNull();
    expect(api.changeDeviceConnection).toHaveBeenCalledTimes(1);
    expect(api.changeDeviceConnection).toHaveBeenCalledWith("coyote", "add");
    expect(api.saveSetupPreferences).not.toHaveBeenCalled();
    expect(screen.queryByTestId("coyote-panel")).toBeNull();

    await act(async () => confirm());
    expect(refresh).toHaveBeenCalled();
    rerenderWith({ output: "handy", configured: ["handy", "coyote"] });
    expect(screen.getByRole("checkbox", { name: "Use Coyote in playback" })).not.toBeChecked();
    expect(screen.getByTestId("coyote-panel")).toBeInTheDocument();
    // The vibrator is still missing, so adding stays available.
    expect(screen.getByRole("button", { name: "Add connection" })).not.toHaveAttribute("aria-disabled", "true");
    rerenderWith({ output: "handy", configured: ["handy", "coyote", "vibrator"] });
    expect(screen.getByRole("button", { name: "Add connection" })).toHaveAttribute("aria-disabled", "true");
    fireEvent.click(screen.getByRole("button", { name: "Add connection" }));
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("offers the Handy when the Coyote is the sole connection", async () => {
    renderManager({ output: "coyote" });
    fireEvent.click(screen.getByRole("button", { name: "Add connection" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "The Handy Handy Cloud REST" }));
    await waitFor(() => expect(api.changeDeviceConnection).toHaveBeenCalledWith("handy", "add"));
  });

  it.each([
    ["The Handy", "coyote"],
    ["Coyote", "handy"],
  ] as const)("removes %s through the backend and preserves the remaining connection", async (name, remaining) => {
    const { rerenderWith } = renderManager({ output: "handy_coyote" });
    fireEvent.click(screen.getByRole("button", { name: `Remove ${name}` }));
    await waitFor(() => expect(api.changeDeviceConnection).toHaveBeenCalledWith(name === "The Handy" ? "handy" : "coyote", "remove"));
    // No optimistic removal: it is still present until a new snapshot arrives.
    expect(screen.getByRole("button", { name: `Remove ${name}` })).toBeInTheDocument();
    rerenderWith({ output: remaining });
    expect(screen.getAllByRole("button", { name: /^Remove / })).toHaveLength(1);
    expect(screen.getByRole("button", { name: "Add connection" })).toHaveFocus();
  });

  it("keeps both connections on failure and reports the error", async () => {
    api.changeDeviceConnection.mockRejectedValue(new Error("Core unavailable"));
    renderManager({ output: "handy_coyote" });
    fireEvent.click(screen.getByRole("button", { name: "Remove Coyote" }));
    await waitFor(() => expect(toast).toHaveBeenCalledWith("Core unavailable", "error"));
    expect(screen.getByRole("button", { name: "Remove Coyote" })).toBeInTheDocument();
  });
});


describe("playback participation", () => {
  it("places the icon-only add button on the title row and supports no saved devices", () => {
    const { container } = renderManager({ output: "none", configured: [] });
    const add = screen.getByRole("button", { name: "Add connection" });
    expect(container.querySelector(".connection-manager-head")).toContainElement(add);
    expect(add.textContent).toBe("");
    expect(screen.queryByRole("button", { name: /^Remove / })).toBeNull();
    fireEvent.click(add);
    expect(screen.getAllByRole("menuitem")).toHaveLength(3);
  });

  it("allows removing the last connection", async () => {
    const { rerenderWith } = renderManager({ output: "handy" });
    fireEvent.click(screen.getByRole("button", { name: "Remove The Handy" }));
    await waitFor(() => expect(api.changeDeviceConnection).toHaveBeenCalledWith("handy", "remove"));
    rerenderWith({ output: "none", configured: [] });
    expect(screen.queryByRole("button", { name: /^Remove / })).toBeNull();
    expect(screen.getByRole("button", { name: "Add connection" })).toHaveFocus();
  });

  it("changes participation only after backend confirmation and retains excluded connected panels", async () => {
    const { rerenderWith } = renderManager({ output: "handy_coyote", configured: ["handy", "coyote"], coyoteConnected: true });
    fireEvent.click(screen.getByRole("checkbox", { name: "Use Coyote in playback" }));
    await waitFor(() => expect(api.changeDeviceConnection).toHaveBeenCalledWith("coyote", "exclude"));
    expect(screen.getByRole("checkbox", { name: "Use Coyote in playback" })).toBeChecked();
    rerenderWith({ output: "handy", configured: ["handy", "coyote"], coyoteConnected: true });
    expect(screen.getByRole("checkbox", { name: "Use Coyote in playback" })).not.toBeChecked();
    expect(screen.getByTestId("coyote-panel")).toBeInTheDocument();
    expect(lifecycle.coyoteUnmounts).toBe(0);
  });
});

it("offers Native Bluetooth alongside browser Bluetooth and only saves the selection", async () => {
  renderManager({ methods: ["cloud_rest", "browser_bluetooth", "native_bluetooth", "intiface"] });
  fireEvent.click(screen.getByRole("button", { name: "The Handy Handy Cloud REST" }));
  expect(screen.getByRole("menuitemradio", { name: "Browser Bluetooth" })).toBeInTheDocument();
  fireEvent.click(screen.getByRole("menuitemradio", { name: "Native Bluetooth" }));
  await waitFor(() => expect(api.saveSetupPreferences).toHaveBeenCalledWith({ device_owner: "native_bluetooth" }));
  expect(api.nativeBluetoothConnect).not.toHaveBeenCalled();
});

describe("vibrator connection", () => {
  it("adds the vibrator as its own row with a separate playback switch", async () => {
    renderManager({ configured: ["handy", "vibrator"], playback: ["handy"] });
    expect(screen.getByTestId("vibrator-panel")).toBeInTheDocument();
    const playback = screen.getByRole("checkbox", { name: "Use Vibrator in playback" });
    expect(playback).not.toBeChecked();
    fireEvent.click(playback);
    await waitFor(() => expect(api.changeDeviceConnection).toHaveBeenCalledWith("vibrator", "include"));
    fireEvent.click(screen.getByRole("button", { name: "Remove Vibrator" }));
    await waitFor(() => expect(api.changeDeviceConnection).toHaveBeenCalledWith("vibrator", "remove"));
  });

  it("names every saved device on the trigger and supports the vibrator alone", () => {
    const { rerenderWith } = renderManager({ configured: ["handy", "coyote", "vibrator"], playback: ["handy", "coyote", "vibrator"] });
    expect(screen.getByRole("button", { name: /^The Handy \+ Coyote \+ Vibrator / })).toBeInTheDocument();
    rerenderWith({ output: "none", configured: ["vibrator"], playback: ["vibrator"] });
    expect(screen.getByRole("button", { name: /^Vibrator Vibrator disconnected/ })).toBeInTheDocument();
    expect(screen.queryByText("No devices selected for playback.")).toBeNull();
  });
});
