import { encodeHandyRequest, decodeHandyRPCMessage } from "./handy-ble-codec.js";

const runtimeBluetooth = document.querySelector("#runtime-bluetooth");
const toast = document.querySelector("#toast");
const dispatchOwner = document.querySelector("#dispatch-owner");
const bluetoothIndicator = document.querySelector("#bluetooth-indicator");
const bluetoothStatus = document.querySelector("#bluetooth-status");
const bluetoothBrowser = document.querySelector("#bluetooth-browser");
const bluetoothDevice = document.querySelector("#bluetooth-device");
const bluetoothBridge = document.querySelector("#bluetooth-bridge");
const bluetoothConnectButton = document.querySelector("#bluetooth-connect");
const bluetoothDisconnectButton = document.querySelector("#bluetooth-disconnect");

const DISPATCH_OWNER_BLUETOOTH = "browser_bluetooth";
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
const CONTROLLER_CLIENT_ID = stableClientID("magichandy.controller.client_id", "browser");

let backendAvailable = true;
let unsupportedStatusPostedAt = 0;
let toastTimer = 0;

const bluetoothState = {
  clientID: transientClientID("bluetooth-tab"),
  device: null,
  server: null,
  tx: null,
  rx: null,
  messageID: 0,
  pendingResponses: new Map(),
  activeStreamID: null,
  commandLoopActive: false,
  heartbeatTimer: 0,
  bridge: null,
};

export function bluetoothSupported() {
  return Boolean(globalThis.navigator?.bluetooth?.requestDevice);
}

export function bluetoothConnected() {
  return Boolean(bluetoothState.device?.gatt?.connected && bluetoothState.tx && bluetoothState.rx);
}

export function renderBluetoothStatus(bridge = {}) {
  bluetoothState.bridge = bridge || {};
  const connected = Boolean(bridge?.connected || bluetoothConnected());
  const status = bridge?.status || (connected ? "connected" : "disconnected");
  const state = connected ? "connected" : status === "connecting" ? "connecting" : status === "stale" ? "stale" : status === "error" || status === "unsupported" ? "error" : "disconnected";
  if (bluetoothIndicator) {
    bluetoothIndicator.dataset.state = state;
  }
  if (bluetoothStatus) {
    bluetoothStatus.textContent = bridge?.message || (connected ? "Bluetooth connected" : "Bluetooth disconnected");
  }
  if (bluetoothBrowser) {
    bluetoothBrowser.textContent = bluetoothSupported() ? "Available" : "Unavailable";
  }
  if (bluetoothDevice) {
    bluetoothDevice.textContent = bridge?.device_name || bluetoothState.device?.name || (connected ? "Handy" : "None");
  }
  if (bluetoothBridge) {
    bluetoothBridge.textContent = bridgeQueueLabel(bridge);
  }
  if (runtimeBluetooth) {
    runtimeBluetooth.textContent = dispatchOwner?.value === DISPATCH_OWNER_BLUETOOTH
      ? labelStatus(status)
      : "Hidden";
  }
  if (bluetoothConnectButton) {
    bluetoothConnectButton.disabled = !backendAvailable || status === "connecting" || bluetoothConnected();
  }
  if (bluetoothDisconnectButton) {
    bluetoothDisconnectButton.disabled = !backendAvailable || (!connected && !bluetoothConnected());
  }
}

export async function maybePostUnsupportedBluetoothStatus() {
  const now = Date.now();
  if (bluetoothSupported() || now - unsupportedStatusPostedAt < 10000) {
    return;
  }
  unsupportedStatusPostedAt = now;
  try {
    const response = await postBluetoothStatus({
      connected: false,
      supported: false,
      status: "unsupported",
      message: "Web Bluetooth is not available in this browser.",
    });
    renderBluetoothStatus(response.bluetooth);
  } catch {
    renderBluetoothStatus({
      connected: false,
      supported: false,
      status: "unsupported",
      message: "Web Bluetooth is not available in this browser.",
    });
  }
}

export function startBluetoothHeartbeat() {
  window.clearInterval(bluetoothState.heartbeatTimer);
  bluetoothState.heartbeatTimer = window.setInterval(async () => {
    if (!bluetoothConnected() && dispatchOwner?.value !== DISPATCH_OWNER_BLUETOOTH) {
      return;
    }
    try {
      const response = await postBluetoothStatus({
        status: bluetoothConnected() ? "connected" : "disconnected",
        message: bluetoothConnected() ? "Handy Bluetooth connected." : "Bluetooth disconnected.",
      });
      renderBluetoothStatus(response.bluetooth);
      ensureBluetoothCommandLoop();
    } catch {
      // The next status refresh will surface server availability.
    }
  }, 4000);
}

function handyBluetoothRequestOptions() {
  return {
    filters: [
      { services: [HANDY_BLE_SERVICE_UUID] },
      ...HANDY_BLE_NAME_PREFIXES.map((namePrefix) => ({ namePrefix })),
    ],
    optionalServices: [HANDY_BLE_SERVICE_UUID],
  };
}

async function postBluetoothStatus(payload = {}) {
  return postJSON("/api/transport/bluetooth/status", {
    client_id: bluetoothState.clientID,
    connected: bluetoothConnected(),
    supported: bluetoothSupported(),
    device_name: bluetoothState.device?.name || "",
    protocol: bluetoothConnected() ? "hsp_ble" : "",
    ...payload,
  });
}

async function connectBluetooth() {
  if (!bluetoothSupported()) {
    const response = await postBluetoothStatus({
      connected: false,
      supported: false,
      status: "unsupported",
      message: "Web Bluetooth is not available in this browser.",
    });
    renderBluetoothStatus(response.bluetooth);
    showToast("Web Bluetooth is not available in this browser.");
    return;
  }

  renderBluetoothStatus({
    connected: false,
    status: "connecting",
    message: "Selecting Handy Bluetooth device.",
  });

  try {
    const device = await navigator.bluetooth.requestDevice(handyBluetoothRequestOptions());
    bluetoothState.device = device;
    device.addEventListener("gattserverdisconnected", handleBluetoothDisconnect);

    const server = await device.gatt.connect();
    bluetoothState.server = server;
    const service = await server.getPrimaryService(HANDY_BLE_SERVICE_UUID);
    bluetoothState.tx = await service.getCharacteristic(HANDY_BLE_TX_UUID);
    bluetoothState.rx = await service.getCharacteristic(HANDY_BLE_RX_UUID);
    bluetoothState.rx.addEventListener("characteristicvaluechanged", handleBleMessage);
    await bluetoothState.rx.startNotifications();

    try {
      await syncBluetoothClock();
    } catch (error) {
      console.warn("Bluetooth clock sync failed:", error);
    }

    const response = await postJSON("/api/transport/bluetooth/connect", {
      client_id: bluetoothState.clientID,
      connected: true,
      supported: true,
      device_name: device.name || "Handy",
      protocol: "hsp_ble",
      status: "connected",
      message: `Connected to ${device.name || "Handy"} over local Bluetooth.`,
    });
    renderBluetoothStatus(response.bluetooth);
    startBluetoothHeartbeat();
    commandLoop();
    showToast("Bluetooth connected.");
  } catch (error) {
    clearBluetoothSession({ disconnect: true });
    const response = await postBluetoothStatus({
      connected: false,
      status: "error",
      error: error.message || String(error),
      message: "Bluetooth connection failed.",
    });
    renderBluetoothStatus(response.bluetooth);
    showToast(error.message || "Bluetooth connection failed.");
  }
}

async function disconnectBluetooth() {
  try {
    if (bluetoothConnected()) {
      try {
        await sendBleRequest("hsp/stop", {}, { waitForResponse: false });
      } catch {
        // Disconnect should still proceed if the device has already gone away.
      }
    }
    if (bluetoothState.device?.gatt?.connected) {
      bluetoothState.device.gatt.disconnect();
      return;
    }
    await handleBluetoothDisconnect();
  } catch (error) {
    showToast(error.message || "Bluetooth disconnect failed.");
  }
}

async function handleBluetoothDisconnect() {
  const deviceName = bluetoothState.device?.name || "";
  clearBluetoothSession();
  const response = await postJSON("/api/transport/bluetooth/disconnect", {
    client_id: bluetoothState.clientID,
    message: deviceName ? `${deviceName} Bluetooth disconnected.` : "Bluetooth disconnected.",
  });
  renderBluetoothStatus(response.bluetooth);
}

function clearBluetoothSession({ disconnect = false } = {}) {
  rejectPendingBluetoothResponses("Bluetooth disconnected.");
  if (disconnect && bluetoothState.device?.gatt?.connected) {
    bluetoothState.device.gatt.disconnect();
  }
  bluetoothState.server = null;
  bluetoothState.tx = null;
  bluetoothState.rx = null;
  bluetoothState.activeStreamID = null;
}

async function commandLoop() {
  if (bluetoothState.commandLoopActive) {
    return;
  }
  bluetoothState.commandLoopActive = true;
  try {
    while (bluetoothConnected()) {
      let response;
      const controller = new AbortController();
      const timeout = window.setTimeout(() => controller.abort(), COMMAND_FETCH_TIMEOUT_MS);
      try {
        response = await fetch(`/api/transport/bluetooth/commands?client_id=${encodeURIComponent(bluetoothState.clientID)}&wait=${COMMAND_WAIT_SECONDS}`, {
          headers: { Accept: "application/json" },
          signal: controller.signal,
        });
      } catch {
        await delay(1000);
        continue;
      } finally {
        window.clearTimeout(timeout);
      }
      if (!response.ok) {
        await delay(1000);
        continue;
      }
      const body = await response.json();
      renderBluetoothStatus(body.bluetooth);
      const commands = Array.isArray(body.commands) ? body.commands : [];
      for (const command of commands) {
        await executeBridgeCommand(command);
        if (!bluetoothConnected()) {
          break;
        }
      }
    }
  } finally {
    bluetoothState.commandLoopActive = false;
  }
}

function ensureBluetoothCommandLoop() {
  if (bluetoothConnected() && !bluetoothState.commandLoopActive) {
    commandLoop();
  }
}

async function executeBridgeCommand(command) {
  const started = performance.now();
  try {
    if (!bluetoothConnected()) {
      throw new Error("Handy Bluetooth is not connected.");
    }
    const response = await runBluetoothCommand(command);
    await postBluetoothAck({
      id: command.id,
      ok: true,
      status: "browser_ack",
      elapsed_ms: performance.now() - started,
      response: response?.hsp_state ? { hsp_state: response.hsp_state } : {},
    });
  } catch (error) {
    await postBluetoothAck({
      id: command.id,
      ok: false,
      status: classifyBluetoothError(error),
      elapsed_ms: performance.now() - started,
      error: error.message || String(error),
    });
  }
}

async function postBluetoothAck(payload) {
  const response = await postJSON("/api/transport/bluetooth/ack", {
    client_id: bluetoothState.clientID,
    ...payload,
  });
  renderBluetoothStatus(response.bluetooth);
}

async function runBluetoothCommand(command) {
  const path = command.path;
  const body = command.body || {};
  if (path === "hsp/add") {
    return executeHSPAdd(body);
  }
  if (path === "hsp/play") {
    await ensureHSPStream(body.stream_id);
    return sendBleRequest("hsp/play", { ...body, server_time: Date.now() }, { waitForResponse: false });
  }
  if (path === "hsp/stop") {
    bluetoothState.activeStreamID = null;
    return sendBleRequest("hsp/stop", {}, { waitForResponse: false });
  }
  if (path === "hsp/state") {
    return sendBleRequest("hsp/state");
  }
  if (path === "slider/stroke") {
    return sendBleRequest("slider/stroke", body, { waitForResponse: false });
  }
  throw new Error(`Bluetooth command is not implemented: ${path}`);
}

async function executeHSPAdd(body = {}) {
  await ensureHSPStream(body.stream_id);
  const points = Array.isArray(body.points) ? body.points : [];
  for (let offset = 0; offset < points.length; offset += HSP_ADD_CHUNK_POINTS) {
    const chunk = points.slice(offset, offset + HSP_ADD_CHUNK_POINTS);
    await sendBleRequest("hsp/add", {
      points: chunk,
      flush: offset === 0 ? Boolean(body.flush) : false,
    }, { waitForResponse: false });
  }
  return { ok: true };
}

async function ensureHSPStream(streamID) {
  const nextStreamID = Number.parseInt(streamID, 10);
  if (!Number.isFinite(nextStreamID) || nextStreamID < 0) {
    throw new Error("Bluetooth HSP stream ID must be a non-negative integer.");
  }
  if (bluetoothState.activeStreamID === nextStreamID) {
    return;
  }
  await sendBleRequest("hsp/setup", { stream_id: nextStreamID }, { waitForResponse: false });
  bluetoothState.activeStreamID = nextStreamID;
}

function classifyBluetoothError(error) {
  const message = String(error?.message || error || "").toLowerCase();
  if (message.includes("not available")) {
    return "browser_unsupported";
  }
  if (message.includes("not connected") || message.includes("not ready")) {
    return "browser_not_connected";
  }
  if (message.includes("not implemented") || message.includes("too large") || message.includes("encode")) {
    return "browser_encode_error";
  }
  return "device_error";
}

function delay(milliseconds) {
  return new Promise((resolve) => window.setTimeout(resolve, milliseconds));
}

function nextBluetoothMessageID() {
  bluetoothState.messageID += 1;
  if (bluetoothState.messageID > 2147483000) {
    bluetoothState.messageID = 1;
  }
  return bluetoothState.messageID;
}

async function writeBluetoothValue(bytes) {
  if (!bluetoothState.tx) {
    throw new Error("Bluetooth TX characteristic is not ready.");
  }
  if (bytes.length > 512) {
    throw new Error(`Bluetooth command is too large (${bytes.length} bytes).`);
  }
  const characteristic = bluetoothState.tx;
  if (characteristic.properties?.write && typeof characteristic.writeValueWithResponse === "function") {
    await characteristic.writeValueWithResponse(bytes);
    return "with-response";
  }
  if (characteristic.properties?.write && typeof characteristic.writeValue === "function") {
    await characteristic.writeValue(bytes);
    return "with-response";
  }
  if (characteristic.properties?.writeWithoutResponse && typeof characteristic.writeValueWithoutResponse === "function") {
    await characteristic.writeValueWithoutResponse(bytes);
    await delay(WRITE_WITHOUT_RESPONSE_SETTLE_MS);
    return "without-response";
  }
  if (typeof characteristic.writeValueWithResponse === "function") {
    await characteristic.writeValueWithResponse(bytes);
    return "with-response";
  }
  if (typeof characteristic.writeValue === "function") {
    await characteristic.writeValue(bytes);
    return "with-response";
  }
  if (typeof characteristic.writeValueWithoutResponse === "function") {
    await characteristic.writeValueWithoutResponse(bytes);
    await delay(WRITE_WITHOUT_RESPONSE_SETTLE_MS);
    return "without-response";
  }
  throw new Error("Bluetooth TX characteristic does not support writes.");
}

async function sendBleRequest(path, body = {}, options = {}) {
  const waitForResponse = options.waitForResponse !== false;
  const id = waitForResponse ? nextBluetoothMessageID() : NO_COMPLETION_ID;
  const bytes = encodeHandyRequest(path, body, id);
  if (!waitForResponse) {
    const writeMode = await writeBluetoothValue(bytes);
    return { ok: true, response_pending: true, write_mode: writeMode };
  }

  const responsePromise = new Promise((resolve, reject) => {
    const timer = window.setTimeout(() => {
      bluetoothState.pendingResponses.delete(id);
      reject(new Error(`Bluetooth response timed out for ${path}.`));
    }, RESPONSE_TIMEOUT_MS);
    bluetoothState.pendingResponses.set(id, { resolve, reject, timer });
  });
  try {
    await writeBluetoothValue(bytes);
  } catch (error) {
    const pending = bluetoothState.pendingResponses.get(id);
    if (pending) {
      window.clearTimeout(pending.timer);
      bluetoothState.pendingResponses.delete(id);
    }
    throw error;
  }

  const response = await responsePromise;
  if (response?.error?.message) {
    throw new Error(response.error.message);
  }
  return response || { ok: true };
}

function handleBleMessage(event) {
  let parsed;
  try {
    const view = event.target.value;
    parsed = decodeHandyRPCMessage(new Uint8Array(view.buffer, view.byteOffset, view.byteLength));
  } catch (error) {
    console.warn("Could not decode Handy Bluetooth message:", error);
    return;
  }
  if (parsed.type === "response") {
    const response = parsed.response || {};
    const pending = bluetoothState.pendingResponses.get(response.id);
    if (pending) {
      window.clearTimeout(pending.timer);
      bluetoothState.pendingResponses.delete(response.id);
      pending.resolve(response);
    }
    if (response.error?.message) {
      postBluetoothStatus({
        status: "error",
        error: response.error.message,
        message: response.error.message,
      }).then((body) => renderBluetoothStatus(body.bluetooth)).catch(() => {});
    }
    return;
  }
  if (parsed.type === "notification") {
    postBluetoothStatus({
      status: "connected",
      message: "Handy Bluetooth event received.",
    }).then((body) => renderBluetoothStatus(body.bluetooth)).catch(() => {});
  }
}

async function syncBluetoothClock() {
  const samples = [];
  for (let index = 0; index < 3; index += 1) {
    const before = Date.now();
    const response = await sendBleRequest("clock/offset/get");
    const after = Date.now();
    const machineTime = response?.clock_offset_get?.time;
    if (Number.isFinite(machineTime)) {
      samples.push({
        offset: Math.round((before + after) / 2 - machineTime),
        rtd: Math.max(0, after - before),
      });
    }
    await delay(60);
  }
  if (!samples.length) {
    throw new Error("Handy Bluetooth clock sync did not return usable samples.");
  }
  const offset = Math.round(samples.reduce((sum, sample) => sum + sample.offset, 0) / samples.length);
  const rtd = Math.round(samples.reduce((sum, sample) => sum + sample.rtd, 0) / samples.length);
  await sendBleRequest("clock/offset/set", { clock_offset: offset, rtd }, { waitForResponse: false });
}

function rejectPendingBluetoothResponses(message) {
  bluetoothState.pendingResponses.forEach((pending) => {
    window.clearTimeout(pending.timer);
    pending.reject(new Error(message));
  });
  bluetoothState.pendingResponses.clear();
}

async function postJSON(path, payload = {}) {
  const response = await fetch(path, {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
      ...controllerHeaders(),
    },
    body: JSON.stringify(payload),
  });

  const body = await response.json();
  if (!response.ok) {
    throw new Error(body.error || `${path} returned ${response.status}`);
  }

  return body;
}

function controllerHeaders() {
  return {
    "X-MagicHandy-Client-ID": CONTROLLER_CLIENT_ID,
  };
}

function bridgeQueueLabel(bridge = {}) {
  const pending = Number(bridge?.pending || 0);
  const inflight = Number(bridge?.inflight || 0);
  if (pending || inflight) {
    return `${pending} queued / ${inflight} active`;
  }
  if (bridge?.last_ack) {
    return bridge.last_ack.ok === false ? "Last failed" : "Last OK";
  }
  return bridge?.ready ? "Ready" : "Idle";
}

function stableClientID(key, prefix) {
  try {
    const existing = window.localStorage.getItem(key);
    if (existing) {
      return existing;
    }
    const generated = `${prefix}-${crypto.randomUUID()}`;
    window.localStorage.setItem(key, generated);
    return generated;
  } catch {
    return `${prefix}-${Date.now()}-${Math.round(Math.random() * 100000)}`;
  }
}

function transientClientID(prefix) {
  try {
    return `${prefix}-${crypto.randomUUID()}`;
  } catch {
    return `${prefix}-${Date.now()}-${Math.round(Math.random() * 100000)}`;
  }
}

function labelStatus(value) {
  if (!value) {
    return "Unknown";
  }
  return labelFeature(value);
}

function labelFeature(value) {
  if (!value) {
    return "Unknown";
  }
  return value
    .split("_")
    .map(capitalize)
    .join(" ");
}

function capitalize(value) {
  if (!value) {
    return "";
  }
  return value.charAt(0).toUpperCase() + value.slice(1);
}

function showToast(message) {
  window.clearTimeout(toastTimer);
  toast.textContent = message;
  toast.dataset.visible = "true";
  toastTimer = window.setTimeout(() => {
    toast.dataset.visible = "false";
  }, 2800);
}

window.addEventListener("magichandy:backend-availability", (event) => {
  backendAvailable = Boolean(event.detail?.available);
  renderBluetoothStatus(bluetoothState.bridge);
});

bluetoothConnectButton?.addEventListener("click", connectBluetooth);
bluetoothDisconnectButton?.addEventListener("click", disconnectBluetooth);
