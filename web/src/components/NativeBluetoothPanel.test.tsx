import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { NativeBluetoothPanel } from "./NativeBluetoothPanel";
import type { NativeBluetoothSnapshot } from "../api/native-bluetooth-types";

const api = vi.hoisted(() => ({ nativeBluetoothConnect: vi.fn(), nativeBluetoothDisconnect: vi.fn(), nativeBluetoothForget: vi.fn() }));
vi.mock("../api/client", () => ({ api }));
vi.mock("../state/app-state", () => ({ useToast: () => ({ show: vi.fn() }) }));
const initial: NativeBluetoothSnapshot = { available: true, supported: true, remembered_device: true, status: { connected: false, connecting: false, simulated: true, status: "disconnected", message: "", clock_synced: false, write_bytes: 0, playback_state: "unknown" } };
beforeEach(() => { for (const fn of Object.values(api)) fn.mockReset().mockResolvedValue(initial); });

describe("native Handy connection", () => {
  it("only connects on an explicit action and waits for the backend snapshot", async () => {
    const refresh = vi.fn();
    const view = render(<NativeBluetoothPanel snapshot={initial} locked={false} canConfigureHost onChanged={refresh} />);
    expect(api.nativeBluetoothConnect).not.toHaveBeenCalled();
    expect(screen.getByText(/computer running MagicHandy/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Connect Handy" }));
    await waitFor(() => expect(refresh).toHaveBeenCalledOnce());
    expect(api.nativeBluetoothConnect).toHaveBeenCalledOnce();
    expect(screen.getByRole("button", { name: "Connect Handy" })).toBeInTheDocument();
    view.rerender(<NativeBluetoothPanel snapshot={{ ...initial, status: { ...initial.status, connected: true } }} locked={false} canConfigureHost onChanged={refresh} />);
    expect(screen.getByRole("button", { name: "Disconnect" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Forget device" })).not.toBeInTheDocument();
  });
  it("disables connection when the host helper is absent", () => {
    render(<NativeBluetoothPanel snapshot={{ ...initial, available: false }} locked={false} canConfigureHost onChanged={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Connect Handy" })).toBeDisabled();
    expect(screen.getByRole("status")).toHaveTextContent("helper is missing");
  });
  it("keeps native host configuration unavailable to a read-only client", () => {
    render(<NativeBluetoothPanel snapshot={initial} locked canConfigureHost={false} onChanged={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Connect Handy" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Forget device" })).toBeDisabled();
  });
});
