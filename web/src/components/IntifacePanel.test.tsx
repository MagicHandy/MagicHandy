import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { IntifaceTransportSnapshot } from "../api/types";
import { IntifacePanel } from "./IntifacePanel";

vi.mock("../api/client", () => ({
  api: { intifaceStatus: vi.fn(() => new Promise(() => undefined)) },
}));
vi.mock("../state/app-state", () => ({ useToast: () => ({ show: vi.fn() }) }));

function snapshot(devices: IntifaceTransportSnapshot["status"]["devices"]): IntifaceTransportSnapshot {
  return {
    dispatch_owner: "intiface",
    address: "ws://127.0.0.1:12345",
    status: { connected: true, scanning: false, playback_state: "idle", max_ping_time_ms: 0, queue_depth: 0, devices },
    diagnostics: {},
  } as IntifaceTransportSnapshot;
}

describe("IntifacePanel", () => {
  it("lists only linear actuators and tolerates devices without any", () => {
    render(<IntifacePanel visible locked={false} dirty={false} initial={snapshot([
      { device_index: 3, device_name: "Test Vibrator", scalar_actuators: [{ index: 0, actuator_type: "Vibrate", step_count: 20 }] },
      { device_index: 7, device_name: "Test Linear", linear_actuators: [{ index: 0, feature_descriptor: "Position" }], scalar_actuators: [] },
    ])} />);
    const options = screen.getAllByRole("option").map((option) => option.textContent);
    expect(options).toContain("Test Linear - Position");
    expect(options.some((label) => label?.includes("Test Vibrator"))).toBe(false);
  });
});
