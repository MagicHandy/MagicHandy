import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { IntifaceTransportSnapshot } from "../api/types";
import { ConnectionManager } from "./ConnectionManager";

const appState = vi.hoisted(() => ({ value: {} as Record<string, unknown> }));
vi.mock("../state/app-state", () => ({
  useAppState: () => appState.value,
  useToast: () => ({ show: vi.fn() }),
}));
vi.mock("../components/IntifacePanel", () => ({ IntifacePanel: () => null }));
vi.mock("../components/BluetoothBridge", () => ({ BluetoothBridge: () => null }));
vi.mock("../components/QuickSettings", () => ({ QuickSettings: () => null }));

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
