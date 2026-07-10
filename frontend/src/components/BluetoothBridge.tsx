// Browser Bluetooth bridge. The browser owns the BLE session and executes only
// backend-issued bridge commands; React never creates motion commands itself.
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { BluetoothBridgeSnapshot, BluetoothCommand } from "../api/types";
import { decodeHandyRPCMessage, encodeHandyRequest } from "../bluetooth/handy-ble-codec";
import { useToast } from "../contexts/ToastContext";

const HANDY_BLE_SERVICE_UUID = "77834d26-40f7-11ee-be56-0242ac120002";
const HANDY_BLE_TX_UUID = "77835032-40f7-11ee-be56-0242ac120002";
const HANDY_BLE_RX_UUID = "77835410-40f7-11ee-be56-0242ac120002";
const HANDY_BLE_NAME_PREFIXES = ["OHD", "Handy", "The Handy"];
const COMMAND_WAIT_SECONDS = 4;
const COMMAND_FETCH_TIMEOUT_MS = (COMMAND_WAIT_SECONDS + 2) * 1000;
const HSP_ADD_CHUNK_POINTS = 20;
const WRITE_WITHOUT_RESPONSE_SETTLE_MS = 20;
const RESPONSE_TIMEOUT_MS = 5000;
const NO_COMPLETION_ID = 2147483647;

interface BluetoothBridgeProps {
  visible: boolean;
  locked: boolean;
  backendOnline: boolean;
  initial?: BluetoothBridgeSnapshot;
}

interface BluetoothCharacteristicLike extends EventTarget {
  properties?: { write?: boolean; writeWithoutResponse?: boolean };
  value?: DataView;
  startNotifications?: () => Promise<BluetoothCharacteristicLike>;
  writeValue?: (bytes: Uint8Array) => Promise<void>;
  writeValueWithResponse?: (bytes: Uint8Array) => Promise<void>;
  writeValueWithoutResponse?: (bytes: Uint8Array) => Promise<void>;
}

interface BluetoothServiceLike {
  getCharacteristic(uuid: string): Promise<BluetoothCharacteristicLike>;
}

interface BluetoothServerLike {
  connected?: boolean;
  getPrimaryService(uuid: string): Promise<BluetoothServiceLike>;
}

interface BluetoothDeviceLike extends EventTarget {
  name?: string;
  gatt?: { connected?: boolean; connect(): Promise<BluetoothServerLike>; disconnect(): void };
}

interface BluetoothNavigator {
  bluetooth?: {
    requestDevice(options: Record<string, unknown>): Promise<BluetoothDeviceLike>;
  };
}

interface PendingResponse {
  resolve: (value: Record<string, unknown>) => void;
  reject: (reason: Error) => void;
  timer: number;
}

export function BluetoothBridge({ visible, locked, backendOnline, initial }: BluetoothBridgeProps) {
  const { notify } = useToast();
  const [bridge, setBridge] = useState<BluetoothBridgeSnapshot>(initial ?? {});
  const [connecting, setConnecting] = useState(false);
  const clientID = useRef(transientClientID("bluetooth-tab"));
  const device = useRef<BluetoothDeviceLike | null>(null);
  const tx = useRef<BluetoothCharacteristicLike | null>(null);
  const rx = useRef<BluetoothCharacteristicLike | null>(null);
  const messageID = useRef(0);
  const pendingResponses = useRef(new Map<number, PendingResponse>());
  const activeStreamID = useRef<number | null>(null);
  const commandLoopActive = useRef(false);

  useEffect(() => {
    if (initial) setBridge(initial);
  }, [initial]);

  useEffect(() => {
    if (!visible || !backendOnline) return;
    void api.bluetoothStatus().then((res) => setBridge(res.bluetooth)).catch(() => undefined);
  }, [backendOnline, visible]);

  useEffect(() => {
    if (!visible) return;
    const id = window.setInterval(() => {
      if (!bluetoothConnected() && !visible) return;
      void postBluetoothStatus({
        status: bluetoothConnected() ? "connected" : "disconnected",
        message: bluetoothConnected() ? "Handy Bluetooth connected." : "Bluetooth disconnected.",
      }).then((res) => setBridge(res.bluetooth)).catch(() => undefined);
      ensureCommandLoop();
    }, 5000);
    return () => window.clearInterval(id);
  });

  function bluetoothSupported() {
    return Boolean((navigator as Navigator & BluetoothNavigator).bluetooth?.requestDevice);
  }

  function bluetoothConnected() {
    return Boolean(device.current?.gatt?.connected && tx.current && rx.current);
  }

  async function postBluetoothStatus(patch: Partial<Parameters<typeof api.postBluetoothStatus>[0]> = {}) {
    return api.postBluetoothStatus({
      client_id: clientID.current,
      connected: bluetoothConnected(),
      supported: bluetoothSupported(),
      device_name: device.current?.name ?? "",
      protocol: bluetoothConnected() ? "hsp_ble" : "",
      ...patch,
    });
  }

  async function connectBluetooth() {
    if (!bluetoothSupported()) {
      const res = await postBluetoothStatus({ connected: false, supported: false, status: "unsupported", message: "Web Bluetooth is not available in this browser." });
      setBridge(res.bluetooth);
      notify("Web Bluetooth is not available in this browser.", "error");
      return;
    }
    setConnecting(true);
    setBridge((b) => ({ ...b, connected: false, status: "connecting", message: "Selecting Handy Bluetooth device." }));
    try {
      const nav = (navigator as Navigator & BluetoothNavigator).bluetooth;
      if (!nav) throw new Error("Web Bluetooth is not available in this browser.");
      const selected = await nav.requestDevice(handyBluetoothRequestOptions());
      device.current = selected;
      selected.addEventListener("gattserverdisconnected", () => void handleBluetoothDisconnect());
      const server = await selected.gatt?.connect();
      if (!server) throw new Error("Bluetooth GATT server is unavailable.");
      const service = await server.getPrimaryService(HANDY_BLE_SERVICE_UUID);
      tx.current = await service.getCharacteristic(HANDY_BLE_TX_UUID);
      rx.current = await service.getCharacteristic(HANDY_BLE_RX_UUID);
      rx.current.addEventListener("characteristicvaluechanged", handleBleMessage as EventListener);
      await rx.current.startNotifications?.();
      try {
        await syncBluetoothClock();
      } catch (e) {
        console.warn("Bluetooth clock sync failed", e);
      }
      const res = await api.bluetoothConnect({
        client_id: clientID.current,
        connected: true,
        supported: true,
        device_name: selected.name || "Handy",
        protocol: "hsp_ble",
        status: "connected",
        message: `Connected to ${selected.name || "Handy"} over local Bluetooth.`,
      });
      setBridge(res.bluetooth);
      ensureCommandLoop();
      notify("Bluetooth connected.");
    } catch (e) {
      clearBluetoothSession({ disconnect: true });
      const message = e instanceof Error ? e.message : "Bluetooth connection failed.";
      const res = await postBluetoothStatus({ connected: false, status: "error", error: message, message: "Bluetooth connection failed." }).catch(() => null);
      if (res) setBridge(res.bluetooth);
      notify(message, "error");
    } finally {
      setConnecting(false);
    }
  }

  async function disconnectBluetooth() {
    try {
      if (bluetoothConnected()) {
        try {
          await sendBleRequest("hsp/stop", {}, { waitForResponse: false });
        } catch {
          // Disconnect should proceed even if the device already disappeared.
        }
      }
      if (device.current?.gatt?.connected) {
        device.current.gatt.disconnect();
        return;
      }
      await handleBluetoothDisconnect();
    } catch (e) {
      notify(e instanceof Error ? e.message : "Bluetooth disconnect failed.", "error");
    }
  }

  async function handleBluetoothDisconnect() {
    const deviceName = device.current?.name ?? "";
    clearBluetoothSession();
    const res = await api.bluetoothDisconnect(clientID.current, deviceName ? `${deviceName} Bluetooth disconnected.` : "Bluetooth disconnected.");
    setBridge(res.bluetooth);
  }

  function clearBluetoothSession({ disconnect = false } = {}) {
    rejectPendingBluetoothResponses("Bluetooth disconnected.");
    if (disconnect && device.current?.gatt?.connected) device.current.gatt.disconnect();
    tx.current = null;
    rx.current = null;
    activeStreamID.current = null;
  }

  function ensureCommandLoop() {
    if (bluetoothConnected() && !commandLoopActive.current) void commandLoop();
  }

  async function commandLoop() {
    if (commandLoopActive.current) return;
    commandLoopActive.current = true;
    try {
      while (bluetoothConnected()) {
        const controller = new AbortController();
        const timeout = window.setTimeout(() => controller.abort(), COMMAND_FETCH_TIMEOUT_MS);
        try {
          const body = await api.bluetoothCommands(clientID.current, COMMAND_WAIT_SECONDS, controller.signal);
          setBridge(body.bluetooth);
          for (const command of body.commands ?? []) {
            await executeBridgeCommand(command);
            if (!bluetoothConnected()) break;
          }
        } catch {
          await delay(1000);
        } finally {
          window.clearTimeout(timeout);
        }
      }
    } finally {
      commandLoopActive.current = false;
    }
  }

  async function executeBridgeCommand(command: BluetoothCommand) {
    const started = performance.now();
    try {
      if (!bluetoothConnected()) throw new Error("Handy Bluetooth is not connected.");
      const response = await runBluetoothCommand(command);
      const ack = await api.bluetoothAck(clientID.current, {
        id: command.id,
        ok: true,
        status: "browser_ack",
        elapsed_ms: performance.now() - started,
        response: response.hsp_state ? { hsp_state: response.hsp_state } : {},
      });
      setBridge(ack.bluetooth);
    } catch (e) {
      const ack = await api.bluetoothAck(clientID.current, {
        id: command.id,
        ok: false,
        status: classifyBluetoothError(e),
        elapsed_ms: performance.now() - started,
        error: e instanceof Error ? e.message : String(e),
      }).catch(() => null);
      if (ack) setBridge(ack.bluetooth);
    }
  }

  async function runBluetoothCommand(command: BluetoothCommand): Promise<Record<string, unknown>> {
    const body = command.body ?? {};
    if (command.path === "hsp/add") return executeHSPAdd(body);
    if (command.path === "hsp/play") {
      await ensureHSPStream(body.stream_id);
      return sendBleRequest("hsp/play", { ...body, server_time: Date.now() }, { waitForResponse: false });
    }
    if (command.path === "hsp/stop") {
      activeStreamID.current = null;
      return sendBleRequest("hsp/stop", {}, { waitForResponse: false });
    }
    if (command.path === "hsp/state") return sendBleRequest("hsp/state");
    if (command.path === "slider/stroke") return sendBleRequest("slider/stroke", body, { waitForResponse: false });
    throw new Error(`Bluetooth command is not implemented: ${command.path}`);
  }

  async function executeHSPAdd(body: Record<string, unknown>) {
    await ensureHSPStream(body.stream_id);
    const points = Array.isArray(body.points) ? body.points : [];
    for (let offset = 0; offset < points.length; offset += HSP_ADD_CHUNK_POINTS) {
      const chunk = points.slice(offset, offset + HSP_ADD_CHUNK_POINTS);
      await sendBleRequest("hsp/add", { points: chunk, flush: offset === 0 ? Boolean(body.flush) : false }, { waitForResponse: false });
    }
    return { ok: true };
  }

  async function ensureHSPStream(streamID: unknown) {
    const nextStreamID = Number.parseInt(String(streamID), 10);
    if (!Number.isFinite(nextStreamID) || nextStreamID < 0) throw new Error("Bluetooth HSP stream ID must be a non-negative integer.");
    if (activeStreamID.current === nextStreamID) return;
    await sendBleRequest("hsp/setup", { stream_id: nextStreamID }, { waitForResponse: false });
    activeStreamID.current = nextStreamID;
  }

  async function sendBleRequest(path: string, body: Record<string, unknown> = {}, options: { waitForResponse?: boolean } = {}) {
    const waitForResponse = options.waitForResponse !== false;
    const id = waitForResponse ? nextBluetoothMessageID() : NO_COMPLETION_ID;
    const bytes = encodeHandyRequest(path, body, id);
    if (!waitForResponse) {
      const writeMode = await writeBluetoothValue(bytes);
      return { ok: true, response_pending: true, write_mode: writeMode };
    }
    const responsePromise = new Promise<Record<string, unknown>>((resolve, reject) => {
      const timer = window.setTimeout(() => {
        pendingResponses.current.delete(id);
        reject(new Error(`Bluetooth response timed out for ${path}.`));
      }, RESPONSE_TIMEOUT_MS);
      pendingResponses.current.set(id, { resolve, reject, timer });
    });
    try {
      await writeBluetoothValue(bytes);
    } catch (e) {
      const pending = pendingResponses.current.get(id);
      if (pending) window.clearTimeout(pending.timer);
      pendingResponses.current.delete(id);
      throw e;
    }
    const response = await responsePromise;
    const error = response.error as { message?: string } | undefined;
    if (error?.message) throw new Error(error.message);
    return response;
  }

  async function writeBluetoothValue(bytes: Uint8Array) {
    const characteristic = tx.current;
    if (!characteristic) throw new Error("Bluetooth TX characteristic is not ready.");
    if (bytes.length > 512) throw new Error(`Bluetooth command is too large (${bytes.length} bytes).`);
    if (characteristic.properties?.write && characteristic.writeValueWithResponse) {
      await characteristic.writeValueWithResponse(bytes);
      return "with-response";
    }
    if (characteristic.properties?.write && characteristic.writeValue) {
      await characteristic.writeValue(bytes);
      return "with-response";
    }
    if (characteristic.properties?.writeWithoutResponse && characteristic.writeValueWithoutResponse) {
      await characteristic.writeValueWithoutResponse(bytes);
      await delay(WRITE_WITHOUT_RESPONSE_SETTLE_MS);
      return "without-response";
    }
    if (characteristic.writeValueWithResponse) {
      await characteristic.writeValueWithResponse(bytes);
      return "with-response";
    }
    if (characteristic.writeValue) {
      await characteristic.writeValue(bytes);
      return "with-response";
    }
    if (characteristic.writeValueWithoutResponse) {
      await characteristic.writeValueWithoutResponse(bytes);
      await delay(WRITE_WITHOUT_RESPONSE_SETTLE_MS);
      return "without-response";
    }
    throw new Error("Bluetooth TX characteristic does not support writes.");
  }

  function handleBleMessage(event: Event) {
    try {
      const view = (event.target as BluetoothCharacteristicLike).value;
      if (!view) return;
      const parsed = decodeHandyRPCMessage(new Uint8Array(view.buffer, view.byteOffset, view.byteLength));
      if (parsed.type === "response") {
        const response = parsed.response as Record<string, unknown> | undefined;
        const id = Number(response?.id ?? 0);
        const pending = pendingResponses.current.get(id);
        if (pending && response) {
          window.clearTimeout(pending.timer);
          pendingResponses.current.delete(id);
          pending.resolve(response);
        }
        const error = response?.error as { message?: string } | undefined;
        if (error?.message) void postBluetoothStatus({ status: "error", error: error.message, message: error.message }).then((res) => setBridge(res.bluetooth));
        return;
      }
      if (parsed.type === "notification") {
        void postBluetoothStatus({ status: "connected", message: "Handy Bluetooth event received." }).then((res) => setBridge(res.bluetooth));
      }
    } catch (e) {
      console.warn("Could not decode Handy Bluetooth message", e);
    }
  }

  async function syncBluetoothClock() {
    const samples: Array<{ offset: number; rtd: number }> = [];
    for (let index = 0; index < 3; index += 1) {
      const before = Date.now();
      const response = await sendBleRequest("clock/offset/get");
      const after = Date.now();
      const clock = response.clock_offset_get as { time?: number } | undefined;
      if (Number.isFinite(clock?.time)) {
        samples.push({ offset: Math.round((before + after) / 2 - Number(clock?.time)), rtd: Math.max(0, after - before) });
      }
      await delay(60);
    }
    if (!samples.length) throw new Error("Handy Bluetooth clock sync did not return usable samples.");
    const offset = Math.round(samples.reduce((sum, sample) => sum + sample.offset, 0) / samples.length);
    const rtd = Math.round(samples.reduce((sum, sample) => sum + sample.rtd, 0) / samples.length);
    await sendBleRequest("clock/offset/set", { clock_offset: offset, rtd }, { waitForResponse: false });
  }

  function nextBluetoothMessageID() {
    messageID.current += 1;
    if (messageID.current > 2147483000) messageID.current = 1;
    return messageID.current;
  }

  function rejectPendingBluetoothResponses(message: string) {
    pendingResponses.current.forEach((pending) => {
      window.clearTimeout(pending.timer);
      pending.reject(new Error(message));
    });
    pendingResponses.current.clear();
  }

  if (!visible) return null;

  const connected = Boolean(bridge.connected || bluetoothConnected());
  const status = bridge.status || (connected ? "connected" : "disconnected");
  const browser = bluetoothSupported() ? "Available" : "Unavailable";
  const deviceName = bridge.device_name || device.current?.name || (connected ? "Handy" : "None");

  return (
    <div className="bluetooth-panel">
      <div className="bluetooth-summary">
        <span className="bluetooth-indicator" data-state={connected ? "connected" : status} aria-hidden="true" />
        <span>{bridge.message || (connected ? "Bluetooth connected" : "Bluetooth disconnected")}</span>
      </div>
      <div className="row-actions">
        <button type="button" className="btn btn-secondary" disabled={locked || connecting || connected} onClick={() => void connectBluetooth()}>
          {connecting ? "Connecting" : "Connect Bluetooth"}
        </button>
        <button type="button" className="btn btn-secondary" disabled={!backendOnline || !connected} onClick={() => void disconnectBluetooth()}>
          Disconnect
        </button>
      </div>
      <dl className="meta-grid">
        <div><dt>Browser</dt><dd>{browser}</dd></div>
        <div><dt>Device</dt><dd>{deviceName}</dd></div>
        <div><dt>Bridge</dt><dd>{bridgeQueueLabel(bridge)}</dd></div>
      </dl>
      {locked && <p className="form-status">{backendOnline ? "Read-only client." : "Core offline."}</p>}
    </div>
  );
}

function handyBluetoothRequestOptions(): Record<string, unknown> {
  return {
    filters: HANDY_BLE_NAME_PREFIXES.map((namePrefix) => ({ namePrefix })),
    optionalServices: [HANDY_BLE_SERVICE_UUID],
  };
}

function bridgeQueueLabel(bridge: BluetoothBridgeSnapshot = {}) {
  const pending = Number(bridge.pending || 0);
  const inflight = Number(bridge.inflight || 0);
  if (pending || inflight) return `${pending} queued / ${inflight} active`;
  if (bridge.last_ack) return bridge.last_ack.ok === false ? "Last failed" : "Last OK";
  return bridge.ready ? "Ready" : "Idle";
}

function classifyBluetoothError(error: unknown) {
  const message = String(error instanceof Error ? error.message : error).toLowerCase();
  if (message.includes("not available")) return "browser_unsupported";
  if (message.includes("not connected") || message.includes("not ready")) return "browser_not_connected";
  if (message.includes("not implemented") || message.includes("too large") || message.includes("encode")) return "browser_encode_error";
  return "device_error";
}

function delay(milliseconds: number) {
  return new Promise((resolve) => window.setTimeout(resolve, milliseconds));
}

function transientClientID(prefix: string) {
  try {
    return `${prefix}-${crypto.randomUUID()}`;
  } catch {
    return `${prefix}-${Date.now()}-${Math.round(Math.random() * 100000)}`;
  }
}
