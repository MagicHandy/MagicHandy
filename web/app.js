const statusPill = document.querySelector(".status-pill");
const coreStatus = document.querySelector("#core-status");
const runtimeCore = document.querySelector("#runtime-core");
const runtimeUI = document.querySelector("#runtime-ui");
const runtimeSettings = document.querySelector("#runtime-settings");
const runtimeMotion = document.querySelector("#runtime-motion");
const runtimeTransport = document.querySelector("#runtime-transport");
const runtimeBluetooth = document.querySelector("#runtime-bluetooth");
const versionValue = document.querySelector("#version-value");
const commitValue = document.querySelector("#commit-value");
const uptimeValue = document.querySelector("#uptime-value");
const healthValue = document.querySelector("#health-value");
const connectionKeyState = document.querySelector("#connection-key-state");
const stopButton = document.querySelector("#stop-button");
const toast = document.querySelector("#toast");
const form = document.querySelector("#settings-form");
const formStatus = document.querySelector("#settings-status");
const bluetoothPanel = document.querySelector("#bluetooth-panel");
const bluetoothIndicator = document.querySelector("#bluetooth-indicator");
const bluetoothStatus = document.querySelector("#bluetooth-status");
const bluetoothBrowser = document.querySelector("#bluetooth-browser");
const bluetoothDevice = document.querySelector("#bluetooth-device");
const bluetoothBridge = document.querySelector("#bluetooth-bridge");
const bluetoothConnectButton = document.querySelector("#bluetooth-connect");
const bluetoothDisconnectButton = document.querySelector("#bluetooth-disconnect");
const cloudCredentialControls = Array.from(document.querySelectorAll(".cloud-credential"));

const fields = {
  serverPort: document.querySelector("#server-port"),
  dispatchOwner: document.querySelector("#dispatch-owner"),
  firmwareRequirement: document.querySelector("#firmware-requirement"),
  appIDSource: document.querySelector("#app-id-source"),
  appIDOverride: document.querySelector("#app-id-override"),
  connectionKey: document.querySelector("#connection-key"),
  clearConnectionKey: document.querySelector("#clear-connection-key"),
  speedMin: document.querySelector("#speed-min"),
  speedMax: document.querySelector("#speed-max"),
  strokeMin: document.querySelector("#stroke-min"),
  strokeMax: document.querySelector("#stroke-max"),
  reverseDirection: document.querySelector("#reverse-direction"),
  diagnosticsVerbosity: document.querySelector("#diagnostics-verbosity"),
};

let toastTimer = 0;
let unsupportedStatusPostedAt = 0;

const DISPATCH_OWNER_CLOUD = "cloud_rest";
const DISPATCH_OWNER_BLUETOOTH = "browser_bluetooth";
const HANDY_BLE_SERVICE_UUID = "77834d26-40f7-11ee-be56-0242ac120002";
const HANDY_BLE_TX_UUID = "77835032-40f7-11ee-be56-0242ac120002";
const HANDY_BLE_RX_UUID = "77835410-40f7-11ee-be56-0242ac120002";
const COMMAND_WAIT_SECONDS = 4;
const HSP_ADD_CHUNK_POINTS = 20;
const WRITE_WITHOUT_RESPONSE_SETTLE_MS = 20;
const RESPONSE_TIMEOUT_MS = 5000;
const NO_COMPLETION_ID = 2147483647;

const bluetoothState = {
  clientID: browserClientID(),
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

async function refreshStatus() {
  try {
    const [health, state, bluetooth] = await Promise.all([
      fetchJSON("/healthz"),
      fetchJSON("/api/state"),
      fetchJSON("/api/transport/bluetooth/status"),
    ]);

    setCoreState("ok", "Core online");
    renderSettings(state.settings);
    renderState(health, state, bluetooth);
  } catch (error) {
    setCoreState("error", "Core unavailable");
    runtimeCore.textContent = "Unavailable";
    healthValue.textContent = error.message;
    formStatus.textContent = "Load failed";
  }
}

async function saveSettings(event) {
  event.preventDefault();
  formStatus.textContent = "Saving";

  try {
    const payload = settingsPayload();
    const response = await sendJSON("/api/settings", payload);
    renderSettings(response.settings);
    runtimeSettings.textContent = labelStatus(response.status.source);
    formStatus.textContent = "Saved";
    showToast("Settings saved.");
  } catch (error) {
    formStatus.textContent = "Save failed";
    showToast(error.message);
  }
}

async function fetchJSON(path) {
  const response = await fetch(path, {
    headers: {
      Accept: "application/json",
    },
  });

  if (!response.ok) {
    throw new Error(`${path} returned ${response.status}`);
  }

  return response.json();
}

async function sendJSON(path, payload) {
  const response = await fetch(path, {
    method: "PUT",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });

  const body = await response.json();
  if (!response.ok) {
    throw new Error(body.error || `${path} returned ${response.status}`);
  }

  return body;
}

async function postJSON(path, payload = {}) {
  const response = await fetch(path, {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });

  const body = await response.json();
  if (!response.ok) {
    throw new Error(body.error || `${path} returned ${response.status}`);
  }

  return body;
}

function renderState(health, state, bluetooth) {
  runtimeCore.textContent = health.status === "ok" ? "Online" : "Degraded";
  runtimeUI.textContent = "Embedded";
  runtimeSettings.textContent = labelStatus(state.settings_status.source);
  runtimeMotion.textContent = labelFeature(state.features.motion);
  runtimeTransport.textContent = labelFeature(state.features.transport);
  renderBluetoothStatus(bluetooth?.bluetooth || state.bluetooth_bridge);
  connectionKeyState.textContent = state.settings.device.connection_key_set ? "Set" : "Not set";
  versionValue.textContent = state.version || "dev";
  commitValue.textContent = state.commit || "unknown";
  uptimeValue.textContent = `${state.uptime_seconds}s`;
  healthValue.textContent = health.status;
}

function renderSettings(settings) {
  fillOptions(fields.dispatchOwner, settings.options.hsp_dispatch_owners);
  fillOptions(fields.appIDSource, settings.options.api_application_id_sources);
  fillOptions(fields.diagnosticsVerbosity, settings.options.diagnostics_verbosities);

  fields.serverPort.value = settings.server.port;
  fields.dispatchOwner.value = settings.device.hsp_dispatch_owner;
  fields.firmwareRequirement.value = settings.device.firmware_api_requirement;
  fields.appIDSource.value = settings.device.api_application_id_source;
  fields.appIDOverride.value = settings.device.api_application_id_override || "";
  fields.connectionKey.value = "";
  fields.connectionKey.placeholder = settings.device.connection_key_set ? "Configured" : "";
  fields.clearConnectionKey.checked = false;
  fields.speedMin.value = settings.motion.speed_min_percent;
  fields.speedMax.value = settings.motion.speed_max_percent;
  fields.strokeMin.value = settings.motion.stroke_min_percent;
  fields.strokeMax.value = settings.motion.stroke_max_percent;
  fields.reverseDirection.checked = settings.motion.reverse_direction;
  fields.diagnosticsVerbosity.value = settings.diagnostics.verbosity;
  updateApplicationIDOverrideState();
  updateTransportVisibility();
}

function settingsPayload() {
  const device = {
    hsp_dispatch_owner: fields.dispatchOwner.value,
    firmware_api_requirement: fields.firmwareRequirement.value,
    api_application_id_source: fields.appIDSource.value,
    api_application_id_override: fields.appIDOverride.value.trim(),
  };

  const key = fields.connectionKey.value.trim();
  if (key !== "") {
    device.handy_connection_key = key;
  }

  return {
    server: {
      port: numberValue(fields.serverPort),
    },
    device,
    clear_connection_key: fields.clearConnectionKey.checked,
    motion: {
      speed_min_percent: numberValue(fields.speedMin),
      speed_max_percent: numberValue(fields.speedMax),
      stroke_min_percent: numberValue(fields.strokeMin),
      stroke_max_percent: numberValue(fields.strokeMax),
      reverse_direction: fields.reverseDirection.checked,
    },
    diagnostics: {
      verbosity: fields.diagnosticsVerbosity.value,
    },
  };
}

function fillOptions(select, values) {
  const current = select.value;
  select.replaceChildren(
    ...values.map((value) => {
      const option = document.createElement("option");
      option.value = value;
      option.textContent = labelFeature(value);
      return option;
    }),
  );
  if (values.includes(current)) {
    select.value = current;
  }
}

function numberValue(input) {
  return Number.parseInt(input.value, 10);
}

function updateApplicationIDOverrideState() {
  const usesOverride = fields.appIDSource.value === "developer_override";
  const bluetoothSelected = fields.dispatchOwner.value === DISPATCH_OWNER_BLUETOOTH;
  fields.appIDSource.disabled = bluetoothSelected;
  fields.appIDOverride.disabled = !usesOverride || bluetoothSelected;
  fields.connectionKey.disabled = bluetoothSelected;
  fields.clearConnectionKey.disabled = bluetoothSelected;
  if (!usesOverride) {
    fields.appIDOverride.value = "";
  }
}

function updateTransportVisibility() {
  const bluetoothSelected = fields.dispatchOwner.value === DISPATCH_OWNER_BLUETOOTH;
  if (bluetoothPanel) {
    bluetoothPanel.hidden = !bluetoothSelected;
  }
  cloudCredentialControls.forEach((control) => {
    control.hidden = bluetoothSelected;
  });
  updateApplicationIDOverrideState();
  if (bluetoothSelected && !bluetoothSupported()) {
    maybePostUnsupportedBluetoothStatus();
  }
}

function renderBluetoothStatus(bridge = {}) {
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
    runtimeBluetooth.textContent = fields.dispatchOwner.value === DISPATCH_OWNER_BLUETOOTH
      ? labelStatus(status)
      : "Hidden";
  }
  if (bluetoothConnectButton) {
    bluetoothConnectButton.disabled = status === "connecting" || bluetoothConnected();
  }
  if (bluetoothDisconnectButton) {
    bluetoothDisconnectButton.disabled = !connected && !bluetoothConnected();
  }
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

function setCoreState(state, label) {
  statusPill.dataset.state = state;
  coreStatus.textContent = label;
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

function browserClientID() {
  const key = "magichandy.bluetooth.client_id";
  try {
    const existing = window.localStorage.getItem(key);
    if (existing) {
      return existing;
    }
    const generated = `browser-${crypto.randomUUID()}`;
    window.localStorage.setItem(key, generated);
    return generated;
  } catch {
    return `browser-${Date.now()}-${Math.round(Math.random() * 100000)}`;
  }
}

function bluetoothSupported() {
  return Boolean(globalThis.navigator?.bluetooth?.requestDevice);
}

function bluetoothConnected() {
  return Boolean(bluetoothState.device?.gatt?.connected && bluetoothState.tx && bluetoothState.rx);
}

async function maybePostUnsupportedBluetoothStatus() {
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
    const device = await navigator.bluetooth.requestDevice({
      filters: [{ services: [HANDY_BLE_SERVICE_UUID] }],
      optionalServices: [HANDY_BLE_SERVICE_UUID],
    });
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
    await sendBleRequest("hsp/state");

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

function startBluetoothHeartbeat() {
  window.clearInterval(bluetoothState.heartbeatTimer);
  bluetoothState.heartbeatTimer = window.setInterval(async () => {
    if (!bluetoothConnected() && fields.dispatchOwner.value !== DISPATCH_OWNER_BLUETOOTH) {
      return;
    }
    try {
      const response = await postBluetoothStatus({
        status: bluetoothConnected() ? "connected" : "disconnected",
        message: bluetoothConnected() ? "Handy Bluetooth connected." : "Bluetooth disconnected.",
      });
      renderBluetoothStatus(response.bluetooth);
    } catch {
      // The next status refresh will surface server availability.
    }
  }, 4000);
}

async function commandLoop() {
  if (bluetoothState.commandLoopActive) {
    return;
  }
  bluetoothState.commandLoopActive = true;
  try {
    while (bluetoothConnected()) {
      const response = await fetch(`/api/transport/bluetooth/commands?client_id=${encodeURIComponent(bluetoothState.clientID)}&wait=${COMMAND_WAIT_SECONDS}`, {
        headers: { Accept: "application/json" },
      });
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
    return sendBleRequest("hsp/play", { ...body, server_time: Date.now() });
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

async function sendSelectedStop() {
  const owner = fields.dispatchOwner.value;
  const endpoint = owner === DISPATCH_OWNER_BLUETOOTH
    ? "/api/transport/bluetooth/stop"
    : owner === DISPATCH_OWNER_CLOUD
      ? "/api/transport/cloud/stop"
      : "";
  if (!endpoint) {
    showToast("No transport owner is selected.");
    return;
  }
  try {
    const response = await postJSON(endpoint, { reason: "ui_stop" });
    if (response.bridge) {
      renderBluetoothStatus(response.bridge);
    }
    showToast("Stop sent.");
  } catch (error) {
    showToast(error.message);
  }
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
    const write_mode = await writeBluetoothValue(bytes);
    return { ok: true, response_pending: true, write_mode };
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

const bluetoothDecoder = new TextDecoder();
const WIRE_VARINT = 0;
const WIRE_LENGTH = 2;
const WIRE_FIXED32 = 5;
const MESSAGE_TYPE_REQUEST = 1;
const MESSAGE_TYPE_RESPONSE = 3;
const MESSAGE_TYPE_NOTIFICATION = 4;
const HSP_POINT_PROTOCOL_MAX = 1000;
const REQUEST_FIELDS = {
  "clock/offset/get": 712,
  "clock/offset/set": 709,
  "slider/stroke": 841,
  "hsp/setup": 860,
  "hsp/add": 861,
  "hsp/play": 863,
  "hsp/stop": 864,
  "hsp/state": 867,
};
const HSP_RESPONSE_FIELDS = new Set([860, 861, 862, 863, 864, 865, 866, 867, 868, 869, 870, 871, 872]);
const HSP_NOTIFICATION_FIELDS = new Set([860, 861, 862, 863, 864, 865]);

function concatBytes(parts) {
  const total = parts.reduce((sum, part) => sum + part.length, 0);
  const output = new Uint8Array(total);
  let offset = 0;
  parts.forEach((part) => {
    output.set(part, offset);
    offset += part.length;
  });
  return output;
}

function encodeVarint(value) {
  let next = typeof value === "bigint" ? value : BigInt(Math.max(0, Number(value) || 0));
  const bytes = [];
  while (next > 0x7fn) {
    bytes.push(Number((next & 0x7fn) | 0x80n));
    next >>= 7n;
  }
  bytes.push(Number(next));
  return Uint8Array.from(bytes);
}

function encodeSignedVarint(value) {
  let next = BigInt(Math.trunc(Number(value) || 0));
  if (next < 0) {
    next = BigInt.asUintN(64, next);
  }
  return encodeVarint(next);
}

function encodeZigZag64(value) {
  const n = BigInt(Math.trunc(Number(value) || 0));
  return encodeVarint((n << 1n) ^ (n >> 63n));
}

function fieldKey(field, wireType) {
  return encodeVarint((BigInt(field) << 3n) | BigInt(wireType));
}

function uintField(field, value) {
  return concatBytes([fieldKey(field, WIRE_VARINT), encodeVarint(value)]);
}

function intField(field, value) {
  return concatBytes([fieldKey(field, WIRE_VARINT), encodeSignedVarint(value)]);
}

function sint64Field(field, value) {
  return concatBytes([fieldKey(field, WIRE_VARINT), encodeZigZag64(value)]);
}

function boolField(field, value) {
  return uintField(field, value ? 1 : 0);
}

function floatField(field, value) {
  const bytes = new Uint8Array(4);
  new DataView(bytes.buffer).setFloat32(0, Number(value) || 0, true);
  return concatBytes([fieldKey(field, WIRE_FIXED32), bytes]);
}

function lengthField(field, bytes) {
  return concatBytes([fieldKey(field, WIRE_LENGTH), encodeVarint(bytes.length), bytes]);
}

function strokePercentValue(value, fallback) {
  const number = Number(value);
  const safe = Number.isFinite(number) ? number : fallback;
  const percent = safe >= 0 && safe <= 1 ? safe * 100 : safe;
  return Math.max(0, Math.min(100, percent));
}

function pointMessage(point = {}) {
  const normalized = Math.max(0, Math.min(100, Number(point.x ?? point.pos ?? 50) || 0));
  return concatBytes([
    uintField(1, Math.max(0, Math.round(Number(point.t ?? 0) || 0))),
    uintField(2, Math.round((normalized / 100) * HSP_POINT_PROTOCOL_MAX)),
  ]);
}

function requestBodyForPath(path, body = {}) {
  if (path === "slider/stroke") {
    return concatBytes([
      floatField(1, strokePercentValue(body.min, 0)),
      floatField(2, strokePercentValue(body.max, 100)),
    ]);
  }
  if (path === "hsp/setup") {
    return uintField(1, body.stream_id ?? 0);
  }
  if (path === "hsp/add") {
    return concatBytes([
      ...(Array.isArray(body.points) ? body.points : []).map((point) => lengthField(1, pointMessage(point))),
      boolField(2, Boolean(body.flush)),
    ]);
  }
  if (path === "hsp/play") {
    return concatBytes([
      intField(1, body.start_time ?? 0),
      uintField(2, body.server_time ?? Date.now()),
      floatField(3, body.playback_rate ?? 1),
      boolField(4, Boolean(body.loop)),
      boolField(5, Boolean(body.pause_on_starving)),
    ]);
  }
  if (path === "clock/offset/set") {
    return concatBytes([
      sint64Field(1, body.clock_offset ?? 0),
      intField(2, body.rtd ?? 0),
    ]);
  }
  if (path === "hsp/stop" || path === "hsp/state" || path === "clock/offset/get") {
    return new Uint8Array();
  }
  throw new Error(`Bluetooth command is not implemented: ${path}`);
}

function encodeHandyRequest(path, body = {}, id = 1) {
  const field = REQUEST_FIELDS[path];
  if (!field) {
    throw new Error(`Bluetooth command is not implemented: ${path}`);
  }
  const payload = requestBodyForPath(path, body);
  const request = concatBytes([
    lengthField(field, payload),
    uintField(2, id),
  ]);
  return concatBytes([
    uintField(1, MESSAGE_TYPE_REQUEST),
    lengthField(2, request),
  ]);
}

function readVarint(bytes, offset) {
  let shift = 0n;
  let value = 0n;
  let index = offset;
  while (index < bytes.length) {
    const byte = BigInt(bytes[index]);
    value |= (byte & 0x7fn) << shift;
    index += 1;
    if ((byte & 0x80n) === 0n) {
      return { value, offset: index };
    }
    shift += 7n;
  }
  throw new Error("Truncated varint");
}

function toNumber(value) {
  const maxSafe = BigInt(Number.MAX_SAFE_INTEGER);
  return Number(value > maxSafe ? maxSafe : value);
}

function toSigned32(value) {
  const unsigned = Number(value & 0xffffffffn);
  return unsigned >= 0x80000000 ? unsigned - 0x100000000 : unsigned;
}

function zigZagToNumber(value) {
  const decoded = (value >> 1n) ^ (-(value & 1n));
  return Number(decoded);
}

function parseFields(bytes) {
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  const fields = [];
  let offset = 0;
  while (offset < bytes.length) {
    const key = readVarint(bytes, offset);
    offset = key.offset;
    const field = Number(key.value >> 3n);
    const wire = Number(key.value & 0x7n);
    let value;
    if (wire === WIRE_VARINT) {
      const parsed = readVarint(bytes, offset);
      value = parsed.value;
      offset = parsed.offset;
    } else if (wire === WIRE_LENGTH) {
      const length = readVarint(bytes, offset);
      offset = length.offset;
      const size = toNumber(length.value);
      value = bytes.slice(offset, offset + size);
      offset += size;
    } else if (wire === WIRE_FIXED32) {
      value = view.getFloat32(offset, true);
      offset += 4;
    } else {
      throw new Error(`Unsupported protobuf wire type ${wire}`);
    }
    fields.push({ field, wire, value });
  }
  return fields;
}

function firstField(fields, number) {
  return fields.find((item) => item.field === number);
}

function stringFromField(item) {
  if (!item || item.wire !== WIRE_LENGTH) {
    return "";
  }
  return bluetoothDecoder.decode(item.value);
}

function hspPlayStateName(value) {
  return {
    0: "not_initialized",
    1: "playing",
    2: "stopped",
    3: "paused",
    4: "starving",
  }[Number(value)] || String(value);
}

function parseHSPState(bytes) {
  const fields = parseFields(bytes);
  const readInt = (number) => {
    const item = firstField(fields, number);
    return item && item.wire === WIRE_VARINT ? toNumber(item.value) : undefined;
  };
  const readFloat = (number) => {
    const item = firstField(fields, number);
    return item && item.wire === WIRE_FIXED32 ? Number(item.value) : undefined;
  };
  const state = {};
  const playState = readInt(1);
  if (playState !== undefined) state.play_state = hspPlayStateName(playState);
  const points = readInt(2);
  if (points !== undefined) state.points = points;
  const maxPoints = readInt(3);
  if (maxPoints !== undefined) state.max_points = maxPoints;
  const currentPoint = firstField(fields, 4);
  if (currentPoint && currentPoint.wire === WIRE_VARINT) state.current_point = toSigned32(currentPoint.value);
  const currentTime = firstField(fields, 5);
  if (currentTime && currentTime.wire === WIRE_VARINT) state.current_time_ms = toSigned32(currentTime.value);
  const loop = readInt(6);
  if (loop !== undefined) state.loop = Boolean(loop);
  const playbackRate = readFloat(7);
  if (playbackRate !== undefined) state.playback_rate = Number(playbackRate.toFixed(4));
  const streamID = readInt(10);
  if (streamID !== undefined) state.stream_id = streamID;
  const tailPoint = firstField(fields, 11);
  if (tailPoint && tailPoint.wire === WIRE_VARINT) state.tail_point_stream_index = toSigned32(tailPoint.value);
  const threshold = readInt(12);
  if (threshold !== undefined) state.tail_point_stream_index_threshold = threshold;
  const pauseOnStarving = readInt(13);
  if (pauseOnStarving !== undefined) state.pause_on_starving = Boolean(pauseOnStarving);
  return state;
}

function parseError(bytes) {
  const fields = parseFields(bytes);
  const code = firstField(fields, 1);
  return {
    code: code && code.wire === WIRE_VARINT ? toNumber(code.value) : 0,
    message: stringFromField(firstField(fields, 2)),
    data: stringFromField(firstField(fields, 3)),
  };
}

function parseHSPResponse(field, bytes) {
  const fields = parseFields(bytes);
  const stateField = firstField(fields, 1);
  const result = { result_field: field };
  if (stateField && stateField.wire === WIRE_LENGTH) {
    result.hsp_state = parseHSPState(stateField.value);
  }
  return result;
}

function parseClockOffsetGet(bytes) {
  const fields = parseFields(bytes);
  const time = firstField(fields, 1);
  const clockOffset = firstField(fields, 2);
  const rtd = firstField(fields, 3);
  return {
    result_field: 712,
    clock_offset_get: {
      time: time && time.wire === WIRE_VARINT ? toNumber(time.value) : 0,
      clock_offset: clockOffset && clockOffset.wire === WIRE_VARINT ? zigZagToNumber(clockOffset.value) : 0,
      rtd: rtd && rtd.wire === WIRE_VARINT ? toNumber(rtd.value) : 0,
    },
  };
}

function parseResponse(bytes) {
  const fields = parseFields(bytes);
  const idField = firstField(fields, 1);
  const response = {
    id: idField && idField.wire === WIRE_VARINT ? toNumber(idField.value) : 0,
    ok: true,
  };
  const errorField = firstField(fields, 2);
  if (errorField && errorField.wire === WIRE_LENGTH) {
    response.error = parseError(errorField.value);
    response.ok = false;
  }
  fields.forEach((field) => {
    if (field.field <= 10 || field.wire !== WIRE_LENGTH) {
      return;
    }
    if (HSP_RESPONSE_FIELDS.has(field.field)) {
      Object.assign(response, parseHSPResponse(field.field, field.value));
    } else if (field.field === 712) {
      Object.assign(response, parseClockOffsetGet(field.value));
    } else {
      response.result_field = field.field;
    }
  });
  return response;
}

function parseNotification(bytes) {
  const fields = parseFields(bytes);
  const idField = firstField(fields, 2);
  const notification = {
    id: idField && idField.wire === WIRE_VARINT ? toNumber(idField.value) : 0,
  };
  fields.forEach((field) => {
    if (field.wire !== WIRE_LENGTH || !HSP_NOTIFICATION_FIELDS.has(field.field)) {
      return;
    }
    const nested = parseFields(field.value);
    const stateField = firstField(nested, 1);
    notification.event_field = field.field;
    if (stateField && stateField.wire === WIRE_LENGTH) {
      notification.hsp_state = parseHSPState(stateField.value);
    }
  });
  return notification;
}

function decodeHandyRPCMessage(buffer) {
  const bytes = buffer instanceof Uint8Array ? buffer : new Uint8Array(buffer);
  const fields = parseFields(bytes);
  const typeField = firstField(fields, 1);
  const type = typeField && typeField.wire === WIRE_VARINT ? toNumber(typeField.value) : 0;
  if (type === MESSAGE_TYPE_RESPONSE) {
    const responseField = firstField(fields, 4);
    return {
      type: "response",
      response: responseField && responseField.wire === WIRE_LENGTH ? parseResponse(responseField.value) : { id: 0, ok: false },
    };
  }
  if (type === MESSAGE_TYPE_NOTIFICATION) {
    const notificationField = firstField(fields, 5);
    return {
      type: "notification",
      notification: notificationField && notificationField.wire === WIRE_LENGTH ? parseNotification(notificationField.value) : {},
    };
  }
  return { type: "unknown", raw_type: type };
}

stopButton.addEventListener("click", sendSelectedStop);
bluetoothConnectButton?.addEventListener("click", connectBluetooth);
bluetoothDisconnectButton?.addEventListener("click", disconnectBluetooth);

fields.appIDSource.addEventListener("change", updateApplicationIDOverrideState);
fields.dispatchOwner.addEventListener("change", updateTransportVisibility);
form.addEventListener("submit", saveSettings);

startBluetoothHeartbeat();
refreshStatus();
window.setInterval(refreshStatus, 5000);
