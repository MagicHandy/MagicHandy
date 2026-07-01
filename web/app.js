import { encodeHandyRequest, decodeHandyRPCMessage } from "./handy-ble-codec.js";

const statusPill = document.querySelector(".status-pill");
const coreStatus = document.querySelector("#core-status");
const runtimeCore = document.querySelector("#runtime-core");
const runtimeUI = document.querySelector("#runtime-ui");
const runtimeSettings = document.querySelector("#runtime-settings");
const runtimeMotion = document.querySelector("#runtime-motion");
const runtimeChat = document.querySelector("#runtime-chat");
const runtimeLLM = document.querySelector("#runtime-llm");
const runtimeTransport = document.querySelector("#runtime-transport");
const runtimeBluetooth = document.querySelector("#runtime-bluetooth");
const versionValue = document.querySelector("#version-value");
const commitValue = document.querySelector("#commit-value");
const uptimeValue = document.querySelector("#uptime-value");
const healthValue = document.querySelector("#health-value");
const connectionKeyState = document.querySelector("#connection-key-state");
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
const llmCheckButton = document.querySelector("#llm-check");
const llmLoadButton = document.querySelector("#llm-load");
const llmUnloadButton = document.querySelector("#llm-unload");
const llmStatus = document.querySelector("#llm-status");
const cloudCredentialControls = Array.from(document.querySelectorAll(".cloud-credential"));

const fields = {
  serverPort: document.querySelector("#server-port"),
  dispatchOwner: document.querySelector("#dispatch-owner"),
  firmwareRequirement: document.querySelector("#firmware-requirement"),
  appIDSource: document.querySelector("#app-id-source"),
  appIDOverride: document.querySelector("#app-id-override"),
  connectionKey: document.querySelector("#connection-key"),
  clearConnectionKey: document.querySelector("#clear-connection-key"),
  diagnosticsVerbosity: document.querySelector("#diagnostics-verbosity"),
  llmProvider: document.querySelector("#llm-provider"),
  llmMode: document.querySelector("#llm-mode"),
  llmModel: document.querySelector("#llm-model"),
  llmLlamaURL: document.querySelector("#llm-llama-url"),
  llmRunnerPath: document.querySelector("#llm-runner-path"),
  llmModelPath: document.querySelector("#llm-model-path"),
  llmOllamaURL: document.querySelector("#llm-ollama-url"),
  llmPromptSet: document.querySelector("#llm-prompt-set"),
  llmTimeout: document.querySelector("#llm-timeout"),
};

let toastTimer = 0;
let unsupportedStatusPostedAt = 0;
// Motion settings are owned by the motion panel (motion-ui.js, immediate-apply).
// app.js caches them so a connection save preserves them instead of zeroing them.
let currentMotion = null;
let currentLLM = null;
let settingsDirty = false;
let renderingSettings = false;

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
    if (formStatus.textContent === "Not loaded" || formStatus.textContent === "Load failed") {
      formStatus.textContent = "Loaded";
    }
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
    try {
      const fresh = await fetchJSON("/api/state");
      currentMotion = fresh.settings.motion;
      currentLLM = fresh.settings.llm;
    } catch {
      // Fall back to the last rendered settings.
    }
    const payload = settingsPayload();
    const response = await sendJSON("/api/settings", payload);
    renderSettings(response.settings, { force: true });
    settingsDirty = false;
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

async function postEmpty(path) {
  const response = await fetch(path, {
    method: "POST",
    headers: {
      Accept: "application/json",
    },
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
  runtimeChat.textContent = labelFeature(state.features.chat);
  runtimeLLM.textContent = state.llm?.provider ? `${labelFeature(state.llm.provider)} / ${state.llm.model}` : "Not configured";
  runtimeMotion.textContent = labelFeature(state.features.motion);
  runtimeTransport.textContent = labelFeature(state.features.transport);
  renderBluetoothStatus(bluetooth?.bluetooth || state.bluetooth_bridge);
  connectionKeyState.textContent = state.settings.device.connection_key_set ? "Set" : "Not set";
  versionValue.textContent = state.version || "dev";
  commitValue.textContent = state.commit || "unknown";
  uptimeValue.textContent = `${state.uptime_seconds}s`;
  healthValue.textContent = health.status;
}

function renderLLMStatus(status) {
  if (!llmStatus) {
    return;
  }
  const state = status.available ? "Ready" : status.loaded ? "Loaded, not ready" : "Not ready";
  llmStatus.textContent = status.message ? `${state}: ${status.message}` : state;
}

function renderSettings(settings, options = {}) {
  if (settingsDirty && !options.force) {
    return;
  }
  renderingSettings = true;
  fillOptions(fields.dispatchOwner, settings.options.hsp_dispatch_owners);
  fillOptions(fields.appIDSource, settings.options.api_application_id_sources);
  fillOptions(fields.diagnosticsVerbosity, settings.options.diagnostics_verbosities);
  fillOptions(fields.llmProvider, settings.options.llm_providers);
  fillOptions(fields.llmMode, settings.options.llama_cpp_modes);
  fillOptions(fields.llmPromptSet, settings.options.prompt_sets);

  fields.serverPort.value = settings.server.port;
  fields.dispatchOwner.value = settings.device.hsp_dispatch_owner;
  fields.firmwareRequirement.value = settings.device.firmware_api_requirement;
  fields.appIDSource.value = settings.device.api_application_id_source;
  fields.appIDOverride.value = settings.device.api_application_id_override || "";
  fields.connectionKey.value = "";
  fields.connectionKey.placeholder = settings.device.connection_key_set ? "Configured" : "";
  fields.clearConnectionKey.checked = false;
  currentMotion = settings.motion;
  currentLLM = settings.llm;
  fields.llmProvider.value = settings.llm.provider;
  fields.llmMode.value = settings.llm.llama_cpp_mode;
  fields.llmModel.value = settings.llm.model;
  fields.llmLlamaURL.value = settings.llm.llama_cpp_base_url;
  fields.llmRunnerPath.value = settings.llm.llama_cpp_runner_path || "";
  fields.llmModelPath.value = settings.llm.llama_cpp_model_path || "";
  fields.llmOllamaURL.value = settings.llm.ollama_base_url;
  fields.llmPromptSet.value = settings.llm.prompt_set;
  fields.llmTimeout.value = settings.llm.request_timeout_ms;
  fields.diagnosticsVerbosity.value = settings.diagnostics.verbosity;
  updateApplicationIDOverrideState();
  updateTransportVisibility();
  updateLLMVisibility();
  renderingSettings = false;
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
    motion: currentMotion,
    llm: llmPayload(),
    diagnostics: {
      verbosity: fields.diagnosticsVerbosity.value,
    },
  };
}

function llmPayload() {
  const fallback = currentLLM || {};
  return {
    provider: fields.llmProvider.value || fallback.provider,
    llama_cpp_mode: fields.llmMode.value || fallback.llama_cpp_mode,
    llama_cpp_base_url: fields.llmLlamaURL.value.trim() || fallback.llama_cpp_base_url,
    llama_cpp_runner_path: fields.llmRunnerPath.value.trim(),
    llama_cpp_model_path: fields.llmModelPath.value.trim(),
    ollama_base_url: fields.llmOllamaURL.value.trim() || fallback.ollama_base_url,
    model: fields.llmModel.value.trim() || fallback.model,
    prompt_set: fields.llmPromptSet.value || fallback.prompt_set,
    request_timeout_ms: numberValue(fields.llmTimeout) || fallback.request_timeout_ms,
  };
}

function markSettingsDirty() {
  if (!renderingSettings) {
    settingsDirty = true;
  }
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

function updateLLMVisibility() {
  const llamaSelected = fields.llmProvider.value === "llama_cpp";
  const managedSelected = llamaSelected && fields.llmMode.value === "managed";
  fields.llmMode.disabled = !llamaSelected;
  fields.llmLlamaURL.disabled = !llamaSelected;
  fields.llmRunnerPath.disabled = !managedSelected;
  fields.llmModelPath.disabled = !managedSelected;
  fields.llmOllamaURL.disabled = fields.llmProvider.value !== "ollama";
}

async function checkLLM() {
  try {
    llmStatus.textContent = "Checking";
    renderLLMStatus(await fetchJSON("/api/llm/status"));
  } catch (error) {
    llmStatus.textContent = error.message;
  }
}

async function loadLLM() {
  try {
    llmStatus.textContent = "Loading";
    renderLLMStatus(await postEmpty("/api/llm/load"));
  } catch (error) {
    llmStatus.textContent = error.message;
  }
}

async function unloadLLM() {
  try {
    llmStatus.textContent = "Unloading";
    renderLLMStatus(await postEmpty("/api/llm/unload"));
  } catch (error) {
    llmStatus.textContent = error.message;
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


bluetoothConnectButton?.addEventListener("click", connectBluetooth);
bluetoothDisconnectButton?.addEventListener("click", disconnectBluetooth);
llmCheckButton?.addEventListener("click", checkLLM);
llmLoadButton?.addEventListener("click", loadLLM);
llmUnloadButton?.addEventListener("click", unloadLLM);

fields.appIDSource.addEventListener("change", updateApplicationIDOverrideState);
fields.dispatchOwner.addEventListener("change", updateTransportVisibility);
fields.llmProvider.addEventListener("change", updateLLMVisibility);
fields.llmMode.addEventListener("change", updateLLMVisibility);
form.addEventListener("input", markSettingsDirty);
form.addEventListener("change", markSettingsDirty);
form.addEventListener("submit", saveSettings);

startBluetoothHeartbeat();
refreshStatus();
window.setInterval(refreshStatus, 5000);
