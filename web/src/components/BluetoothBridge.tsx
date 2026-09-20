import { t, translateKnown } from "../i18n";
// Browser Bluetooth bridge. The browser owns the BLE session and executes only
// backend-issued bridge commands; React never creates motion commands itself.
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { BluetoothBridgeSnapshot, BluetoothCommand, BluetoothGatewaySnapshot } from "../api/types";
import { HandyBleSession, type BluetoothCharacteristicLike } from "../bluetooth/handy-ble-session";
import { syncHandyBleClock } from "../bluetooth/handy-ble-clock";
import { bluetoothPlaybackFeedback } from "../bluetooth/playback-feedback";
import { useToast } from "../state/app-state";

const HANDY_BLE_SERVICE_UUID = "77834d26-40f7-11ee-be56-0242ac120002";
const HANDY_BLE_TX_UUID = "77835032-40f7-11ee-be56-0242ac120002";
const HANDY_BLE_RX_UUID = "77835410-40f7-11ee-be56-0242ac120002";
const HANDY_BLE_NAME_PREFIXES = ["OHD", "Handy", "The Handy"];
const COMMAND_WAIT_SECONDS = 4;
const COMMAND_FETCH_TIMEOUT_MS = (COMMAND_WAIT_SECONDS + 2) * 1000;
const HSP_ADD_CHUNK_POINTS = 20;

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
  canConfigureHost?: boolean;
  initial?: BluetoothBridgeSnapshot;
  onStateChange?: (state: BluetoothBridgeState) => void;
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

export function BluetoothBridge({ visible, locked, backendOnline, canConfigureHost = true, initial, onStateChange }: BluetoothBridgeProps) {
  const { show } = useToast();
  const [bridge, setBridge] = useState<BluetoothBridgeSnapshot>(initial ?? {});
  const [connecting, setConnecting] = useState(false);
  const clientID = useRef(transientClientID("bluetooth-tab"));
  const gateway = useRef<BluetoothGatewaySnapshot>();
  const registered = useRef(false);
  const device = useRef<BluetoothDeviceLike | null>(null);
  const tx = useRef<BluetoothCharacteristicLike | null>(null);
  const rx = useRef<BluetoothCharacteristicLike | null>(null);
  const bleSession = useRef<HandyBleSession | null>(null);
  const clockSynced = useRef(false);
  const connectionAttempt = useRef<AbortController | null>(null);
  const activeStreamID = useRef<number | null>(null);
  const mounted = useRef(true);
  const commandGeneration = useRef(0);
  const localStopPending = useRef(false);
  const commandLoopAbort = useRef<AbortController | null>(null);
  const commandRequestAbort = useRef<AbortController | null>(null);
  const disconnectListener = useRef<EventListener | null>(null);
  const disconnecting = useRef(false);
  const lastNotificationStatus = useRef(0);
  const feedbackSequence = useRef(0);
  const feedbackCommandID = useRef<string | null>(null);
  const lastReportedPlayState = useRef("");


  useEffect(() => {
    mounted.current = true;
    const stop = () => void emergencyStopBluetooth(true);
    const hide = () => { if (document.visibilityState === "hidden") void loseBluetoothGateway(); };
    window.addEventListener("magichandy:emergency-stop", stop);
    document.addEventListener("visibilitychange", hide);
    return () => {
      mounted.current = false;
      window.removeEventListener("magichandy:emergency-stop", stop);
      document.removeEventListener("visibilitychange", hide);
      commandLoopAbort.current?.abort();
      commandRequestAbort.current?.abort();
      void loseBluetoothGateway();
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
    if (!visible) {
      void loseBluetoothGateway();
      return;
    }
    ensureCommandLoop();
    const id = window.setInterval(() => {
      if (!registered.current || !bluetoothConnected()) return;
      void postBluetoothStatus({
        status: bluetoothConnected() ? "connected" : "disconnected",
        message: bluetoothConnected() ? "Handy Bluetooth connected." : "Bluetooth disconnected.",
      }).then((res) => {
        if (mounted.current) setBridge(res.bluetooth);
      }).catch(() => undefined);
      ensureCommandLoop();
    }, 5000);
    return () => window.clearInterval(id);
  }, [visible]);

  function bluetoothSupported() {
    return Boolean((navigator as Navigator & BluetoothNavigator).bluetooth?.requestDevice);
  }

  function bluetoothConnected() {
    return Boolean(device.current?.gatt?.connected && tx.current && rx.current);
  }

  async function postBluetoothStatus(patch: Partial<Parameters<typeof api.postBluetoothStatus>[0]> = {}) {
    if (!registered.current) return api.bluetoothStatus();
    const admittedGateway = gateway.current;
    const admittedDevice = device.current;
    const result = await api.postBluetoothStatus({
      client_id: clientID.current,
      connected: bluetoothConnected(),
      supported: bluetoothSupported(),
      device_name: device.current?.name ?? "",
      protocol: bluetoothConnected() ? "hsp_ble" : "",
      ...patch,
    }, admittedGateway);
    if (!registered.current || gateway.current !== admittedGateway || device.current !== admittedDevice) throw new DOMException("Obsolete gateway status", "AbortError");
    return result;
  }

  async function connectBluetooth() {
    if (!bluetoothSupported()) {
      const res = await postBluetoothStatus({ connected: false, supported: false, status: "unsupported", message: "Web Bluetooth is not available in this browser." });
      if (mounted.current) setBridge(res.bluetooth);
      show(t("Web Bluetooth is not available in this browser."), "error");
      return;
    }
    const attempt = new AbortController();
    connectionAttempt.current = attempt;
    const checkAttempt = () => { if (attempt.signal.aborted || !mounted.current || !bluetoothPageVisible()) throw new Error("Bluetooth connection was canceled."); };
    setConnecting(true);
    setBridge((b) => ({ ...b, connected: false, status: "connecting", message: "Selecting Handy Bluetooth device." }));
    try {
      const nav = (navigator as Navigator & BluetoothNavigator).bluetooth;
      if (!nav) throw new Error("Web Bluetooth is not available in this browser.");
      const selected = await nav.requestDevice(handyBluetoothRequestOptions());
      checkAttempt();
      device.current = selected;
      const onDisconnect: EventListener = () => { if (device.current === selected) void handleBluetoothDisconnect(); };
      disconnectListener.current = onDisconnect;
      selected.addEventListener("gattserverdisconnected", onDisconnect);
      const server = await selected.gatt?.connect();
      checkAttempt();
      if (!server) throw new Error("Bluetooth GATT server is unavailable.");
      const service = await server.getPrimaryService(HANDY_BLE_SERVICE_UUID);
      checkAttempt();
      const nextTx = await service.getCharacteristic(HANDY_BLE_TX_UUID);
      checkAttempt();
      const nextRx = await service.getCharacteristic(HANDY_BLE_RX_UUID);
      checkAttempt();
      tx.current = nextTx;
      rx.current = nextRx;
      const session = new HandyBleSession(nextTx, nextRx, handleBleNotification, (error) => {
        if (mounted.current) show(translateKnown(error.message), "error");
        void loseBluetoothGateway();
      });
      bleSession.current = session;
      await nextRx.startNotifications?.();
      checkAttempt();
      try {
        await syncHandyBleClock(session);
        clockSynced.current = true;
      } catch (e) {
        console.warn("Bluetooth clock sync failed", e);
      }
      if (attempt.signal.aborted || !session.active || !mounted.current || !bluetoothPageVisible() || device.current !== selected || !selected.gatt?.connected) {
        throw new Error("Bluetooth connection was canceled.");
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
      if (!res.gateway || typeof res.gateway.required !== "boolean" ||
        (res.gateway.required && (!res.gateway.owned || !res.gateway.epoch || !Number.isSafeInteger(res.gateway.generation) || res.gateway.generation! <= 0))) {
        throw new Error("The core did not authorize this Bluetooth gateway. Reconnect from the device browser.");
      }
      if (attempt.signal.aborted || !session.active || !mounted.current || !bluetoothPageVisible() || device.current !== selected || !selected.gatt?.connected) {
        void api.bluetoothDisconnect(clientID.current, "Bluetooth connection was canceled.", res.gateway).catch(() => undefined);
        clearBluetoothSession({ disconnect: true });
        return;
      }
      gateway.current = res.gateway;
      registered.current = true;
      setBridge(res.bluetooth);
      ensureCommandLoop();
      show(t("Bluetooth connected."));
    } catch (e) {
      clearBluetoothSession({ disconnect: true });
      const message = e instanceof Error ? e.message : "Bluetooth connection failed.";
      const res = await postBluetoothStatus({ connected: false, status: "error", error: message, message: "Bluetooth connection failed." }).catch(() => null);
      if (res && mounted.current) setBridge(res.bluetooth);
      if (mounted.current) show(translateKnown(message), "error");
    } finally {
      if (connectionAttempt.current === attempt) connectionAttempt.current = null;
      if (mounted.current) setConnecting(false);
    }
  }

  async function disconnectBluetooth() {
    try {
      if (!bluetoothConnected()) {
        const res = await api.bluetoothDisconnect(bridge.client_id ?? "", "Bluetooth gateway disconnected by its administrator.");
        if (mounted.current) setBridge(res.bluetooth);
        return;
      }
      await emergencyStopBluetooth(false);
      if (device.current?.gatt?.connected) {
        device.current.gatt.disconnect();
        return;
      }
      await handleBluetoothDisconnect();
    } catch (e) {
      show(e instanceof Error ? translateKnown(e.message) : t("Bluetooth disconnect failed."), "error");
    }
  }

  async function handleBluetoothDisconnect() {
    if (disconnecting.current) return;
    disconnecting.current = true;
    const deviceName = device.current?.name ?? "";
    const previousGateway = gateway.current;
    clearBluetoothSession();
    try {
      const res = await api.bluetoothDisconnect(clientID.current, deviceName ? `${deviceName} Bluetooth disconnected.` : "Bluetooth disconnected.", previousGateway);
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
    registered.current = false;
    gateway.current = undefined;
    commandGeneration.current += 1;
    commandLoopAbort.current?.abort();
    commandLoopAbort.current = null;
    commandRequestAbort.current?.abort();
    commandRequestAbort.current = null;
    connectionAttempt.current?.abort();
    bleSession.current?.close();
    bleSession.current = null;
    clockSynced.current = false;
    const currentDevice = device.current;
    if (currentDevice && disconnectListener.current) {
      currentDevice.removeEventListener("gattserverdisconnected", disconnectListener.current);
    }
    disconnectListener.current = null;
    if (disconnect && currentDevice?.gatt?.connected) currentDevice.gatt.disconnect();
    tx.current = null;
    rx.current = null;
    device.current = null;
    activeStreamID.current = null;
    localStopPending.current = false;
    lastReportedPlayState.current = "";
    feedbackCommandID.current = null;
    feedbackSequence.current = 0;
  }

  function ensureCommandLoop() {
    if (!registered.current || !bluetoothConnected() || commandLoopAbort.current || document.visibilityState === "hidden") return;
    const controller = new AbortController();
    commandLoopAbort.current = controller;
    void commandLoop(controller);
  }

  async function commandLoop(sessionController: AbortController) {
    try {
      while (!sessionController.signal.aborted && registered.current && bluetoothConnected()) {
        const generation = commandGeneration.current;
        const admittedGateway = gateway.current;
        const requestController = new AbortController();
        commandRequestAbort.current = requestController;
        const abortRequest = () => requestController.abort();
        sessionController.signal.addEventListener("abort", abortRequest, { once: true });
        const timeout = window.setTimeout(abortRequest, COMMAND_FETCH_TIMEOUT_MS);
        try {
          const body = await api.bluetoothCommands(clientID.current, COMMAND_WAIT_SECONDS, requestController.signal, admittedGateway);
          if (sessionController.signal.aborted || gateway.current !== admittedGateway) break;
          if (requestController.signal.aborted) {
            if (generation !== commandGeneration.current) continue;
            await loseBluetoothGateway();
            break;
          }
          if (mounted.current) setBridge(body.bluetooth);
          for (const command of body.commands ?? []) {
            await executeBridgeCommand(command, generation);
            if (sessionController.signal.aborted || !bluetoothConnected()) break;
          }
        } catch {
          if (sessionController.signal.aborted) break;
          if (requestController.signal.aborted && generation !== commandGeneration.current) continue;
          // A failed device channel needs a local stop and explicit reconnect.
          // It must not execute the rest of a batch using obsolete authority.
          await loseBluetoothGateway();
          break;
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
    const admittedGateway = gateway.current;
    let payload: Parameters<typeof api.bluetoothAck>[1];
    try {
      if (!bluetoothConnected()) throw new Error("Handy Bluetooth is not connected.");
      const response = await runBluetoothCommand(command, generation);
      payload = {
        id: command.id,
        ok: true,
        status: "browser_ack",
        elapsed_ms: performance.now() - started,
        response: response.hsp_state ? { hsp_state: response.hsp_state } : {},
      };
    } catch (e) {
      payload = {
        id: command.id,
        ok: false,
        status: classifyBluetoothError(e),
        elapsed_ms: performance.now() - started,
        error: e instanceof Error ? e.message : String(e),
      };
    }
    if (!registered.current || gateway.current !== admittedGateway) return;
    try {
      const ack = await api.bluetoothAck(clientID.current, payload, admittedGateway);
      if (mounted.current && gateway.current === admittedGateway) setBridge(ack.bluetooth);
    } catch {
      await loseBluetoothGateway();
    }
  }

  async function runBluetoothCommand(command: BluetoothCommand, generation: number): Promise<Record<string, unknown>> {
    const body = command.body ?? {};
    if (command.path === "hsp/stop") {
      activeStreamID.current = null;
      feedbackCommandID.current = null;
      const response = await sendBleRequest("hsp/stop", {}, { waitForResponse: false });
      localStopPending.current = false;
      return response;
    }
    if (localStopPending.current) throw new Error("Bluetooth command was invalidated by Emergency Stop.");
    if (command.path === "hsp/add" || command.path === "hsp/play") feedbackCommandID.current = command.id;
    assertCommandGeneration(generation);
    if (command.path === "hsp/add") return executeHSPAdd(body, generation);
    if (command.path === "hsp/play") {
      await ensureHSPStream(body.stream_id, generation);
      assertCommandGeneration(generation);
      return sendBleRequest("hsp/play", () => ({ ...body, server_time: clockSynced.current ? Date.now() : undefined }), { waitForResponse: false, generation });
    }
    if (command.path === "hsp/state") return sendBleRequest("hsp/state", {}, { generation });
    if (command.path === "slider/stroke") return sendBleRequest("slider/stroke", body, { waitForResponse: false, generation });
    throw new Error(`Bluetooth command is not implemented: ${command.path}`);
  }

  async function executeHSPAdd(body: Record<string, unknown>, generation: number) {
    await ensureHSPStream(body.stream_id, generation);
    const points = Array.isArray(body.points) ? body.points : [];
    for (let offset = 0; offset < points.length; offset += HSP_ADD_CHUNK_POINTS) {
      assertCommandGeneration(generation);
      const chunk = points.slice(offset, offset + HSP_ADD_CHUNK_POINTS);
      const tail = typeof body.tail_point_stream_index === "number" ? body.tail_point_stream_index - points.length + offset + chunk.length : undefined;
      await sendBleRequest("hsp/add", { points: chunk, flush: offset === 0 ? Boolean(body.flush) : false, tail_point_stream_index: tail }, { waitForResponse: false, generation });
    }
    return { ok: true };
  }

  async function ensureHSPStream(streamID: unknown, generation: number) {
    const nextStreamID = Number(streamID);
    if (!Number.isSafeInteger(nextStreamID) || nextStreamID < 0 || nextStreamID > 0xffffffff) throw new Error("Bluetooth HSP stream ID must be a non-negative integer.");
    if (activeStreamID.current === nextStreamID) return;
    assertCommandGeneration(generation);
    await sendBleRequest("hsp/setup", { stream_id: nextStreamID }, { waitForResponse: false, generation });
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
    feedbackCommandID.current = null;
    if (!bluetoothConnected()) return;
    try {
      await sendBleRequest("hsp/stop", {}, { waitForResponse: false });
    } catch (error) {
      if (reportError && mounted.current) {
        show(error instanceof Error ? t("Bluetooth Stop failed: {message}", { message: error.message }) : t("Bluetooth Stop failed."), "error");
      }
    }
  }

  async function loseBluetoothGateway() {
    connectionAttempt.current?.abort();
    if (disconnecting.current) return;
    if (!registered.current && !bluetoothConnected()) {
      clearBluetoothSession({ disconnect: true });
      return;
    }
    disconnecting.current = true;
    const previousGateway = gateway.current;
    const wasRegistered = registered.current;
    registered.current = false;
    commandLoopAbort.current?.abort();
    // Notify while this browser session is still mounted; local Stop remains
    // independent of the HTTP result and teardown is bounded below.
    if (wasRegistered) void api.postBluetoothStatus({
      client_id: clientID.current, connected: false, supported: bluetoothSupported(),
      status: "disconnected", message: "Bluetooth device channel ended.",
    }, previousGateway).catch(() => undefined);
    let timer: number | undefined;
    try {
      await Promise.race([
        emergencyStopBluetooth(false),
        new Promise<void>((resolve) => { timer = window.setTimeout(resolve, 1000); }),
      ]);
    } finally {
      window.clearTimeout(timer);
      clearBluetoothSession({ disconnect: true });
      if (mounted.current) setBridge((current) => ({ ...current, connected: false, ready: false, status: "disconnected", message: "Bluetooth gateway connection ended. Reconnect explicitly from the device browser." }));
      disconnecting.current = false;
    }
  }

  function sendBleRequest(path: string, body: Record<string, unknown> | (() => Record<string, unknown>) = {}, options: { waitForResponse?: boolean; generation?: number } = {}) {
    const session = bleSession.current;
    if (!session) return Promise.reject(new Error("Bluetooth TX characteristic is not ready."));
    return session.request(path, body, {
      waitForResponse: options.waitForResponse,
      check: options.generation === undefined ? undefined : () => assertCommandGeneration(options.generation!),
    });
  }

  function handleBleNotification(notification: Record<string, unknown>) {
    const feedback = bluetoothPlaybackFeedback(notification.hsp_state, activeStreamID.current, ++feedbackSequence.current, feedbackCommandID.current);
    const now = Date.now();
    // State changes bypass the progress throttle. The backend orders reports.
    if (feedback && (feedback.play_state !== lastReportedPlayState.current || now - lastNotificationStatus.current >= 250)) {
      lastNotificationStatus.current = now;
      lastReportedPlayState.current = feedback.play_state;
      void postBluetoothStatus({ status: "connected", hsp_state: feedback })
        .then((res) => { if (mounted.current) setBridge(res.bluetooth); })
        .catch(() => undefined);
    }
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
        <span>{bridge.message ? translateKnown(bridge.message) : connected ? t("Bluetooth connected") : t("Bluetooth disconnected")}</span>
      </div>
      <div className="row-actions">
        <button type="button" className="btn btn-secondary" disabled={locked || !canConfigureHost || connecting || connected} onClick={() => void connectBluetooth()}>
          {connecting ? t("Connecting") : t("Connect Bluetooth")}
        </button>
        <button type="button" className="btn btn-secondary" disabled={!connected || (!bluetoothConnected() && (locked || !backendOnline || !canConfigureHost))} onClick={() => void disconnectBluetooth()}>{t("Disconnect")}</button>
      </div>
      <dl className="meta-grid">
        <div><dt>{t("Browser")}</dt><dd>{translateKnown(browser)}</dd></div>
        <div><dt>{t("Device")}</dt><dd>{deviceName}</dd></div>
        <div><dt>{t("Bridge")}</dt><dd>{bridgeQueueLabel(bridge)}</dd></div>
        <div><dt>{t("Device connection")}</dt><dd>{bluetoothConnected() ? t("This browser") : connected ? t("Another browser") : t("Disconnected")}</dd></div>
      </dl>
      {locked && <p className="form-status">{backendOnline ? t("Read-only client.") : t("Core offline.")}</p>}
    </div>
  );
}

function handyBluetoothRequestOptions(): Record<string, unknown> {
  return {
    filters: HANDY_BLE_NAME_PREFIXES.map((namePrefix) => ({ namePrefix })),
    optionalServices: [HANDY_BLE_SERVICE_UUID],
  };
}

function bluetoothPageVisible() { return document.visibilityState !== "hidden"; }

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

function transientClientID(prefix: string) {
  try {
    return `${prefix}-${crypto.randomUUID()}`;
  } catch {
    return `${prefix}-${Date.now()}-${Math.round(Math.random() * 100000)}`;
  }
}
