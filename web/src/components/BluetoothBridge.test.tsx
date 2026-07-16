import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { encodeHandyRequest } from "../bluetooth/handy-ble-codec";
import { BluetoothBridge } from "./BluetoothBridge";

interface EncodedRequest {
  path: string;
  body: Record<string, unknown>;
  id: number;
}

const codec = vi.hoisted(() => ({
  requests: new WeakMap<object, EncodedRequest>(),
  decoded: null as Record<string, unknown> | null,
}));

const show = vi.hoisted(() => vi.fn());

vi.mock("../api/client", () => ({
  api: {
    bluetoothStatus: vi.fn(),
    postBluetoothStatus: vi.fn(),
    bluetoothConnect: vi.fn(),
    bluetoothDisconnect: vi.fn(),
    bluetoothCommands: vi.fn(),
    bluetoothAck: vi.fn(),
  },
}));

vi.mock("../bluetooth/handy-ble-codec", () => ({
  encodeHandyRequest: vi.fn((path: string, body: Record<string, unknown>, id: number) => {
    const bytes = Uint8Array.of(id & 0xff);
    codec.requests.set(bytes, { path, body, id });
    return bytes;
  }),
  decodeHandyRPCMessage: vi.fn(() => codec.decoded ?? { type: "unknown" }),
}));

vi.mock("../state/app-state", () => ({
  useToast: () => ({ show }),
}));

const bluetoothStatus = vi.mocked(api.bluetoothStatus);
const postBluetoothStatus = vi.mocked(api.postBluetoothStatus);
const bluetoothConnect = vi.mocked(api.bluetoothConnect);
const bluetoothDisconnect = vi.mocked(api.bluetoothDisconnect);
const bluetoothCommands = vi.mocked(api.bluetoothCommands);
const bluetoothAck = vi.mocked(api.bluetoothAck);
const encodeRequest = vi.mocked(encodeHandyRequest);

const connectedSnapshot = {
  connected: true,
  ready: true,
  supported: true,
  status: "connected",
  device_name: "OHD_test",
};

function statusResponse(bluetooth: typeof connectedSnapshot | Record<string, unknown>) {
  return { status: "ok", dispatch_owner: "browser_bluetooth", bluetooth };
}

class FakeCharacteristic extends EventTarget {
  properties = { write: true, writeWithoutResponse: false };
  value?: DataView;
  writes: EncodedRequest[] = [];
  peer?: FakeCharacteristic;

  startNotifications = vi.fn(async () => this);
  writeValueWithResponse = vi.fn(async (bytes: Uint8Array) => {
    const request = codec.requests.get(bytes);
    if (!request) throw new Error("unrecognized encoded request");
    this.writes.push(request);
    if (request.path === "clock/offset/get") {
      codec.decoded = {
        type: "response",
        response: {
          id: request.id,
          ok: true,
          clock_offset_get: { time: Date.now(), clock_offset: 0, rtd: 1 },
        },
      };
      this.peer?.emitValue();
    }
  });

  emitValue() {
    this.value = new DataView(Uint8Array.of(1).buffer);
    this.dispatchEvent(new Event("characteristicvaluechanged"));
  }
}

class FakeDevice extends EventTarget {
  name = "OHD_test";
  readonly tx = new FakeCharacteristic();
  readonly rx = new FakeCharacteristic();
  readonly gatt = {
    connected: false,
    connect: vi.fn(async () => {
      this.gatt.connected = true;
      return {
        connected: true,
        getPrimaryService: async () => ({
          getCharacteristic: async (uuid: string) => uuid.includes("5032") ? this.tx : this.rx,
        }),
      };
    }),
    disconnect: vi.fn(() => {
      this.gatt.connected = false;
      this.dispatchEvent(new Event("gattserverdisconnected"));
    }),
  };

  constructor() {
    super();
    this.tx.peer = this.rx;
  }
}

describe("BluetoothBridge", () => {
  let device: FakeDevice;
  let commandSignal: AbortSignal | undefined;

  beforeEach(() => {
    show.mockReset();
    encodeRequest.mockClear();
    codec.decoded = null;
    device = new FakeDevice();
    commandSignal = undefined;
    Object.defineProperty(navigator, "bluetooth", {
      configurable: true,
      value: { requestDevice: vi.fn(async () => device) },
    });
    bluetoothStatus.mockResolvedValue(statusResponse({}));
    postBluetoothStatus.mockResolvedValue(statusResponse(connectedSnapshot));
    bluetoothConnect.mockResolvedValue(statusResponse(connectedSnapshot));
    bluetoothDisconnect.mockResolvedValue(statusResponse({ connected: false, ready: false, status: "disconnected" }));
    bluetoothAck.mockResolvedValue({ status: "ok", bluetooth: connectedSnapshot });
    bluetoothCommands.mockImplementation((_clientID, _wait, signal) => new Promise((_resolve, reject) => {
      commandSignal = signal;
      signal?.addEventListener("abort", () => reject(new DOMException("Aborted", "AbortError")), { once: true });
    }));
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  async function connect() {
    fireEvent.click(screen.getByRole("button", { name: "Connect Bluetooth" }));
    await waitFor(() => expect(bluetoothConnect).toHaveBeenCalledOnce());
    await waitFor(() => expect(screen.getByRole("button", { name: "Disconnect" })).toBeEnabled());
  }

  it("writes Emergency Stop directly to the device when the backend goes offline", async () => {
    const result = render(<BluetoothBridge visible locked={false} backendOnline />);
    await connect();
    result.rerender(<BluetoothBridge visible locked={false} backendOnline={false} />);
    await waitFor(() => expect(commandSignal?.aborted).toBe(true));

    await act(async () => window.dispatchEvent(new Event("magichandy:emergency-stop")));

    await waitFor(() => expect(device.tx.writes.some((request) => request.path === "hsp/stop")).toBe(true));
    expect(show).not.toHaveBeenCalledWith(expect.stringMatching(/stop failed/i), "error");
  });

  it("aborts command polling, removes listeners, and disconnects on unmount", async () => {
    const result = render(<BluetoothBridge visible locked={false} backendOnline />);
    await connect();
    expect(commandSignal?.aborted).toBe(false);

    result.unmount();

    await waitFor(() => expect(commandSignal?.aborted).toBe(true));
    await waitFor(() => expect(device.gatt.disconnect).toHaveBeenCalledOnce());
    expect(device.tx.writes.some((request) => request.path === "hsp/stop")).toBe(true);
  });
});
