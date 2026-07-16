// Browser Bluetooth bridge. The browser owns the BLE session and executes only
// backend-issued bridge commands; React never creates motion commands itself.
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { BluetoothBridgeSnapshot, BluetoothCommand } from "../api/types";
import { decodeHandyRPCMessage, encodeHandyRequest } from "../bluetooth/handy-ble-codec";
import { useToast } from "../state/app-state";

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

export interface BluetoothBridgeState {
  connected: boolean;
  connecting: boolean;
  status: string;
  deviceName: string;
}

interface BluetoothBridgeProps {
  visible: boolean;
  locked: boolean;
  backendOnline: boolean;
  initial?: BluetoothBridgeSnapshot;
  onStateChange?: (state: BluetoothBridgeState) => void;
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

export function BluetoothBridge({ visible, locked, backendOnline, initial, onStateChange }: BluetoothBridgeProps) {
  const { show } = useToast();
  const [bridge, setBridge] = useState<BluetoothBridgeSnapshot>(initial ?? {});
  const [connecting, setConnecting] = useState(false);
  const clientID = useRef(transientClientID("bluetooth-tab"));
  const device = useRef<BluetoothDeviceLike | null>(null);
  const tx = useRef<BluetoothCharacteristicLike | null>(null);
  const rx = useRef<BluetoothCharacteristicLike | null>(null);
  const messageID = useRef(0);
  const pendingResponses = useRef(new Map<number, PendingResponse>());
  const activeStreamID = useRef<number | null>(null);
  const mounted = useRef(true);
  const backendOnlineRef = useRef(backendOnline);
  const commandGeneration = useRef(0);
  const localStopPending = useRef(false);
  const commandLoopAbort = useRef<AbortController | null>(null);
  const commandRequestAbort = useRef<AbortController | null>(null);
  const disconnectListener = useRef<EventListener | null>(null);
  const notificationListener = useRef<EventListener | null>(null);
  const disconnecting = useRef(false);
  const writeTail = useRef<Promise<void>>(Promise.resolve());
  const lastNotificationStatus = useRef(0);

  backendOnlineRef.current = backendOnline;

  useEffect(() => {
    mounted.current = true;
    const stop = () => void emergencyStopBluetooth(true);
    window.addEventListener("magichandy:emergency-stop", stop);
    return () => {
      mounted.current = false;
      window.removeEventListener("magichandy:emergency-stop", stop);
      commandLoopAbort.current?.abort();
      commandRequestAbort.current?.abort();
      void emergencyStopBluetooth(false).finally(() => clearBluetoothSession({ disconnect: true }));
    };
  }, []);

  useEffect(() => {
    if (initial) setBridge(initial);
  }, [initial]);

  useEffect(() => {
    const connected = Boolean(bridge.connected || (device.current?.gatt?.connected && tx.current && rx.current));
    onStateChange?.({
      connected,
      connecting,
      status: bridge.status || (connected ? "connected" : "disconnected"),
      deviceName: bridge.device_name || device.current?.name || (connected ? "The Handy" : ""),
    });
  }, [bridge, connecting, onStateChange]);

  useEffect(() => {
    if (!visible || !backendOnline) return;
    void api.bluetoothStatus().then((res) => {
      if (mounted.current) setBridge(res.bluetooth);
    }).catch(() => undefined);
  }, [backendOnline, visible]);

  useEffect(() => {
    if (!visible || !backendOnline) {
      commandLoopAbort.current?.abort();
      return;
    }
    ensureCommandLoop();
    const id = window.setInterval(() => {
      void postBluetoothStatus({
        status: bluetoothConnected() ? "connected" : "disconnected",
        message: bluetoothConnected() ? "Handy Bluetooth connected." : "Bluetooth disconnected.",
      }).then((res) => {
        if (mounted.current) setBridge(res.bluetooth);
      }).catch(() => undefined);
      ensureCommandLoop();
    }, 5000);
    return () => window.clearInterval(id);
  }, [backendOnline, visible]);

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
      if (mounted.current) setBridge(res.bluetooth);
      show("Web Bluetooth is not available in this browser.", "error");
      return;
    }
    setConnecting(true);
    setBridge((b) => ({ ...b, connected: false, status: "connecting", message: "Selecting Handy Bluetooth device." }));
    try {
      const nav = (navigator as Navigator & BluetoothNavigator).bluetooth;
      if (!nav) throw new Error("Web Bluetooth is not available in this browser.");
      const selected = await nav.requestDevice(handyBluetoothRequestOptions());
      device.current = selected;
      const onDisconnect: EventListener = () => void handleBluetoothDisconnect();
      disconnectListener.current = onDisconnect;
      selected.addEventListener("gattserverdisconnected", onDisconnect);
      const server = await selected.gatt?.connect();
      if (!server) throw new Error("Bluetooth GATT server is unavailable.");
      const service = await server.getPrimaryService(HANDY_BLE_SERVICE_UUID);
      tx.current = await service.getCharacteristic(HANDY_BLE_TX_UUID);
      rx.current = await service.getCharacteristic(HANDY_BLE_RX_UUID);
      const onNotification: EventListener = (event) => handleBleMessage(event);
      notificationListener.current = onNotification;
      rx.current.addEventListener("characteristicvaluechanged", onNotification);
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
      if (!mounted.current) {
        clearBluetoothSession({ disconnect: true });
        return;
      }
      setBridge(res.bluetooth);
      ensureCommandLoop();
      show("Bluetooth connected.");
    } catch (e) {
      clearBluetoothSession({ disconnect: true });
      const message = e instanceof Error ? e.message : "Bluetooth connection failed.";
      const res = await postBluetoothStatus({ connected: false, status: "error", error: message, message: "Bluetooth connection failed." }).catch(() => null);
      if (res && mounted.current) setBridge(res.bluetooth);
      if (mounted.current) show(message, "error");
    } finally {
      if (mounted.current) setConnecting(false);
    }
  }

  async function disconnectBluetooth() {
    try {
      await emergencyStopBluetooth(false);
      if (device.current?.gatt?.connected) {
        device.current.gatt.disconnect();
        return;
      }
      await handleBluetoothDisconnect();
    } catch (e) {
      show(e instanceof Error ? e.message : "Bluetooth disconnect failed.", "error");
    }
  }

  async function handleBluetoothDisconnect() {
    if (disconnecting.current) return;
    disconnecting.current = true;
    const deviceName = device.current?.name ?? "";
    clearBluetoothSession();
    try {
      const res = await api.bluetoothDisconnect(clientID.current, deviceName ? `${deviceName} Bluetooth disconnected.` : "Bluetooth disconnected.");
      if (mounted.current) setBridge(res.bluetooth);
    } catch {
      if (mounted.current) {
        setBridge((current) => ({
          ...current,
          connected: false,
          ready: false,
          status: "disconnected",
          message: "Bluetooth disconnected; the core could not be notified.",
        }));
      }
    } finally {
      disconnecting.current = false;
    }
  }

  function clearBluetoothSession({ disconnect = false } = {}) {
    commandGeneration.current += 1;
    commandLoopAbort.current?.abort();
    commandLoopAbort.current = null;
    commandRequestAbort.current?.abort();
    commandRequestAbort.current = null;
    rejectPendingBluetoothResponses("Bluetooth disconnected.");
    const currentDevice = device.current;
    const currentRx = rx.current;
    if (currentRx && notificationListener.current) {
      currentRx.removeEventListener("characteristicvaluechanged", notificationListener.current);
    }
    if (currentDevice && disconnectListener.current) {
      currentDevice.removeEventListener("gattserverdisconnected", disconnectListener.current);
    }
    notificationListener.current = null;
    disconnectListener.current = null;
    if (disconnect && currentDevice?.gatt?.connected) currentDevice.gatt.disconnect();
    tx.current = null;
    rx.current = null;
    device.current = null;
    activeStreamID.current = null;
    localStopPending.current = false;
  }

  function ensureCommandLoop() {
    if (!backendOnlineRef.current || !bluetoothConnected() || commandLoopAbort.current) return;
    const controller = new AbortController();
    commandLoopAbort.current = controller;
    void commandLoop(controller);
  }

  async function commandLoop(sessionController: AbortController) {
    try {
      while (!sessionController.signal.aborted && bluetoothConnected() && backendOnlineRef.current) {
        const generation = commandGeneration.current;
        const requestController = new AbortController();
        commandRequestAbort.current = requestController;
        const abortRequest = () => requestController.abort();
        sessionController.signal.addEventListener("abort", abortRequest, { once: true });
        const timeout = window.setTimeout(abortRequest, COMMAND_FETCH_TIMEOUT_MS);
        try {
          const body = await api.bluetoothCommands(clientID.current, COMMAND_WAIT_SECONDS, requestController.signal);
          if (mounted.current) setBridge(body.bluetooth);
          for (const command of body.commands ?? []) {
            await executeBridgeCommand(command, generation);
            if (sessionController.signal.aborted || !bluetoothConnected()) break;
          }
        } catch (error) {
          if (sessionController.signal.aborted) break;
          if (!isAbortError(error)) await delay(1000, sessionController.signal).catch(() => undefined);
        } finally {
          window.clearTimeout(timeout);
          sessionController.signal.removeEventListener("abort", abortRequest);
          if (commandRequestAbort.current === requestController) commandRequestAbort.current = null;
        }
      }
    } finally {
      if (commandLoopAbort.current === sessionController) commandLoopAbort.current = null;
    }
  }

  async function executeBridgeCommand(command: BluetoothCommand, generation: number) {
    const started = performance.now();
    try {
      if (!bluetoothConnected()) throw new Error("Handy Bluetooth is not connected.");
      const response = await runBluetoothCommand(command, generation);
      const ack = await api.bluetoothAck(clientID.current, {
        id: command.id,
        ok: true,
        status: "browser_ack",
        elapsed_ms: performance.now() - started,
        response: response.hsp_state ? { hsp_state: response.hsp_state } : {},
      });
      if (mounted.current) setBridge(ack.bluetooth);
    } catch (e) {
      const ack = await api.bluetoothAck(clientID.current, {
        id: command.id,
        ok: false,
        status: classifyBluetoothError(e),
        elapsed_ms: performance.now() - started,
        error: e instanceof Error ? e.message : String(e),
      }).catch(() => null);
      if (ack && mounted.current) setBridge(ack.bluetooth);
    }
  }

  async function runBluetoothCommand(command: BluetoothCommand, generation: number): Promise<Record<string, unknown>> {
    const body = command.body ?? {};
    if (command.path === "hsp/stop") {
      activeStreamID.current = null;
      const response = await sendBleRequest("hsp/stop", {}, { waitForResponse: false });
      localStopPending.current = false;
      return response;
    }
    if (localStopPending.current) throw new Error("Bluetooth command was invalidated by Emergency Stop.");
    assertCommandGeneration(generation);
    if (command.path === "hsp/add") return executeHSPAdd(body, generation);
    if (command.path === "hsp/play") {
      await ensureHSPStream(body.stream_id, generation);
      assertCommandGeneration(generation);
      return sendBleRequest("hsp/play", { ...body, server_time: Date.now() }, { waitForResponse: false });
    }
    if (command.path === "hsp/state") return sendBleRequest("hsp/state");
    if (command.path === "slider/stroke") return sendBleRequest("slider/stroke", body, { waitForResponse: false });
    throw new Error(`Bluetooth command is not implemented: ${command.path}`);
  }

  async function executeHSPAdd(body: Record<string, unknown>, generation: number) {
    await ensureHSPStream(body.stream_id, generation);
    const points = Array.isArray(body.points) ? body.points : [];
    for (let offset = 0; offset < points.length; offset += HSP_ADD_CHUNK_POINTS) {
      assertCommandGeneration(generation);
      const chunk = points.slice(offset, offset + HSP_ADD_CHUNK_POINTS);
      await sendBleRequest("hsp/add", { points: chunk, flush: offset === 0 ? Boolean(body.flush) : false }, { waitForResponse: false });
    }
    return { ok: true };
  }

  async function ensureHSPStream(streamID: unknown, generation: number) {
    const nextStreamID = Number(streamID);
    if (!Number.isSafeInteger(nextStreamID) || nextStreamID < 0) throw new Error("Bluetooth HSP stream ID must be a non-negative integer.");
    if (activeStreamID.current === nextStreamID) return;
    assertCommandGeneration(generation);
    await sendBleRequest("hsp/setup", { stream_id: nextStreamID }, { waitForResponse: false });
    assertCommandGeneration(generation);
    activeStreamID.current = nextStreamID;
  }

  function assertCommandGeneration(generation: number) {
    if (generation !== commandGeneration.current) {
      throw new Error("Bluetooth command was invalidated by Emergency Stop.");
    }
  }

  async function emergencyStopBluetooth(reportError: boolean) {
    commandGeneration.current += 1;
    localStopPending.current = true;
    commandRequestAbort.current?.abort();
    activeStreamID.current = null;
    if (!bluetoothConnected()) return;
    try {
      await sendBleRequest("hsp/stop", {}, { waitForResponse: false });
    } catch (error) {
      if (reportError && mounted.current) {
        show(error instanceof Error ? `Bluetooth Stop failed: ${error.message}` : "Bluetooth Stop failed.", "error");
      }
    }
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
    const previous = writeTail.current.catch(() => undefined);
    let release!: () => void;
    writeTail.current = new Promise<void>((resolve) => { release = resolve; });
    await previous;
    try {
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
    } finally {
      release();
    }
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
        if (error?.message) {
          void postBluetoothStatus({ status: "error", error: error.message, message: error.message })
            .then((res) => { if (mounted.current) setBridge(res.bluetooth); })
            .catch(() => undefined);
        }
        return;
      }
      if (parsed.type === "notification") {
        const now = Date.now();
        if (now - lastNotificationStatus.current >= 1000) {
          lastNotificationStatus.current = now;
          void postBluetoothStatus({ status: "connected", message: "Handy Bluetooth event received." })
            .then((res) => { if (mounted.current) setBridge(res.bluetooth); })
            .catch(() => undefined);
        }
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
  if (message.includes("invalidated by emergency stop")) return "browser_canceled";
  if (message.includes("not implemented") || message.includes("too large") || message.includes("encode")) return "browser_encode_error";
  return "device_error";
}

function isAbortError(error: unknown) {
  return error instanceof DOMException
    ? error.name === "AbortError"
    : Boolean(error && typeof error === "object" && "name" in error && error.name === "AbortError");
}

function delay(milliseconds: number, signal?: AbortSignal) {
  return new Promise<void>((resolve, reject) => {
    if (signal?.aborted) {
      reject(signal.reason ?? new DOMException("Aborted", "AbortError"));
      return;
    }
    const onAbort = () => {
      window.clearTimeout(timer);
      reject(signal?.reason ?? new DOMException("Aborted", "AbortError"));
    };
    const timer = window.setTimeout(() => {
      signal?.removeEventListener("abort", onAbort);
      resolve();
    }, milliseconds);
    signal?.addEventListener("abort", onAbort, { once: true });
  });
}

function transientClientID(prefix: string) {
  try {
    return `${prefix}-${crypto.randomUUID()}`;
  } catch {
    return `${prefix}-${Date.now()}-${Math.round(Math.random() * 100000)}`;
  }
}
