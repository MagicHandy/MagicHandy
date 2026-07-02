import {
  bluetoothSupported,
  maybePostUnsupportedBluetoothStatus,
  renderBluetoothStatus,
  startBluetoothHeartbeat,
} from "./bluetooth-ui.js";

const statusPill = document.querySelector(".status-pill");
const transportPill = document.querySelector(".transport-pill");
const controllerPill = document.querySelector(".controller-pill");
const coreStatus = document.querySelector("#core-status");
const transportStatus = document.querySelector("#transport-status");
const controllerStatus = document.querySelector("#controller-status");
const backendBanner = document.querySelector("#backend-banner");
const backendBannerMessage = document.querySelector("#backend-banner-message");
const runtimeCore = document.querySelector("#runtime-core");
const runtimeUI = document.querySelector("#runtime-ui");
const runtimeSettings = document.querySelector("#runtime-settings");
const runtimeMotion = document.querySelector("#runtime-motion");
const runtimeChat = document.querySelector("#runtime-chat");
const runtimeLLM = document.querySelector("#runtime-llm");
const runtimeTransport = document.querySelector("#runtime-transport");
const versionValue = document.querySelector("#version-value");
const commitValue = document.querySelector("#commit-value");
const uptimeValue = document.querySelector("#uptime-value");
const healthValue = document.querySelector("#health-value");
const connectionKeyState = document.querySelector("#connection-key-state");
const toast = document.querySelector("#toast");
const form = document.querySelector("#settings-form");
const formStatus = document.querySelector("#settings-status");
const bluetoothPanel = document.querySelector("#bluetooth-panel");
const connectionCheckButton = document.querySelector("#connection-check");
const connectionCheckStatus = document.querySelector("#connection-check-status");
const llmCheckButton = document.querySelector("#llm-check");
const llmLoadButton = document.querySelector("#llm-load");
const llmUnloadButton = document.querySelector("#llm-unload");
const llmStatus = document.querySelector("#llm-status");
const diagnosticsCopyButton = document.querySelector("#diagnostics-copy");
const diagnosticsCopyStatus = document.querySelector("#diagnostics-copy-status");
const cloudCredentialControls = Array.from(document.querySelectorAll(".cloud-credential"));
const backendRequiredControls = Array.from(document.querySelectorAll(
  "[data-requires-backend] button, [data-requires-backend] input, [data-requires-backend] select, [data-requires-backend] textarea",
)).filter((control) => !control.hasAttribute("data-allow-backend-offline"));
const controllerRequiredControls = Array.from(document.querySelectorAll(
  "[data-requires-backend] button, [data-requires-backend] input, [data-requires-backend] select, [data-requires-backend] textarea",
)).filter((control) => !control.hasAttribute("data-allow-backend-offline") && !control.hasAttribute("data-allow-readonly"));

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
// Motion settings are owned by the motion panel (motion-ui.js, immediate-apply).
// app.js caches them so a connection save preserves them instead of zeroing them.
let currentMotion = null;
let currentLLM = null;
let settingsDirty = false;
let renderingSettings = false;
let backendAvailable = true;
let controllerReadOnly = false;
let controllerReason = "";
let lastState = null;

const DISPATCH_OWNER_CLOUD = "cloud_rest";
const DISPATCH_OWNER_BLUETOOTH = "browser_bluetooth";
const CONTROLLER_CLIENT_ID = appClientID();

async function refreshStatus() {
  try {
    const [health, state, bluetooth] = await Promise.all([
      fetchJSON("/healthz"),
      fetchJSON("/api/state"),
      fetchJSON("/api/transport/bluetooth/status"),
    ]);

    setCoreState("ok", "Core online");
    setBackendAvailability(true);
    renderSettings(state.settings);
    renderState(health, state, bluetooth);
    if (formStatus.textContent === "Not loaded" || formStatus.textContent === "Load failed") {
      formStatus.textContent = "Loaded";
    }
  } catch (error) {
    setCoreState("error", "Core unavailable");
    setBackendAvailability(false, error.message);
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
      ...controllerHeaders(),
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

async function postEmpty(path) {
  const response = await fetch(path, {
    method: "POST",
    headers: {
      Accept: "application/json",
      ...controllerHeaders(),
    },
  });

  const body = await response.json();
  if (!response.ok) {
    throw new Error(body.error || `${path} returned ${response.status}`);
  }

  return body;
}

function renderState(health, state, bluetooth) {
  lastState = state;
  setControllerState(state.controller);
  runtimeCore.textContent = health.status === "ok" ? "Online" : "Degraded";
  runtimeUI.textContent = "Embedded";
  runtimeSettings.textContent = labelStatus(state.settings_status.source);
  runtimeChat.textContent = labelFeature(state.features.chat);
  runtimeLLM.textContent = state.llm?.provider ? `${labelFeature(state.llm.provider)} / ${state.llm.model}` : "Not configured";
  runtimeMotion.textContent = labelFeature(state.features.motion);
  runtimeTransport.textContent = labelFeature(state.features.transport);
  renderTransportStatus(state, bluetooth);
  renderBluetoothStatus(bluetooth?.bluetooth || state.bluetooth_bridge);
  connectionKeyState.textContent = state.settings.device.connection_key_set ? "Set" : "Not set";
  versionValue.textContent = state.version || "dev";
  commitValue.textContent = state.commit || "unknown";
  uptimeValue.textContent = `${state.uptime_seconds}s`;
  healthValue.textContent = health.status;
}

function setBackendAvailability(available, reason = "") {
  backendAvailable = Boolean(available);
  document.body.dataset.backend = backendAvailable ? "online" : "offline";
  if (backendBanner) {
    backendBanner.hidden = backendAvailable;
  }
  if (backendBannerMessage && !backendAvailable) {
    backendBannerMessage.textContent = reason || "Backend-required controls are locked until the core responds.";
  }
  if (!backendAvailable && transportPill && transportStatus) {
    transportPill.dataset.state = "error";
    transportStatus.textContent = "Transport unavailable";
  }
  for (const control of backendRequiredControls) {
    setControlBackendDisabled(control, !backendAvailable, reason);
  }
  if (backendAvailable) {
    updateApplicationIDOverrideState();
    updateTransportVisibility();
    updateLLMVisibility();
    renderBluetoothStatus(lastState?.bluetooth_bridge);
  }
  applyControllerLock();
  window.dispatchEvent(new CustomEvent("magichandy:backend-availability", {
    detail: { available: backendAvailable, reason },
  }));
}

function setControlBackendDisabled(control, disabled, reason) {
  if (disabled) {
    if (!control.disabled && !("backendTitle" in control.dataset)) {
      control.dataset.backendTitle = control.getAttribute("title") || "";
      control.dataset.disabledByBackend = "true";
    }
    control.disabled = true;
    control.title = reason ? `Core unavailable: ${reason}` : "Core unavailable";
    return;
  }
  if (control.dataset.disabledByBackend === "true") {
    control.disabled = false;
    if (control.dataset.backendTitle) {
      control.title = control.dataset.backendTitle;
    } else {
      control.removeAttribute("title");
    }
  }
  delete control.dataset.backendTitle;
  delete control.dataset.disabledByBackend;
}

function setControllerState(controller = {}) {
  controllerReadOnly = Boolean(controller.read_only);
  controllerReason = controller.reason || "";
  document.body.dataset.controller = controllerReadOnly ? "readonly" : "active";
  if (controllerPill) {
    controllerPill.dataset.state = controllerReadOnly ? "pending" : "ok";
  }
  if (controllerStatus) {
    controllerStatus.textContent = controllerReadOnly
      ? `Read-only: ${controllerReason || "another tab controls motion"}`
      : "Controller active";
  }
  applyControllerLock();
  window.dispatchEvent(new CustomEvent("magichandy:controller-state", {
    detail: controller,
  }));
}

function applyControllerLock() {
  for (const control of controllerRequiredControls) {
    setControlControllerDisabled(control, controllerReadOnly, controllerReason);
  }
  if (!controllerReadOnly) {
    updateApplicationIDOverrideState();
    updateTransportVisibility();
    updateLLMVisibility();
    renderBluetoothStatus(lastState?.bluetooth_bridge);
  }
}

function setControlControllerDisabled(control, disabled, reason) {
  if (disabled) {
    if (control.dataset.disabledByController !== "true") {
      control.dataset.controllerTitle = control.getAttribute("title") || "";
      control.dataset.controllerWasDisabled = control.disabled ? "true" : "false";
      control.dataset.disabledByController = "true";
    }
    control.disabled = true;
    if (control.dataset.disabledByBackend !== "true") {
      control.title = reason ? `Read-only: ${reason}` : "Read-only client";
    }
    return;
  }
  if (control.dataset.disabledByController === "true") {
    const wasDisabled = control.dataset.controllerWasDisabled === "true";
    delete control.dataset.disabledByController;
    delete control.dataset.controllerWasDisabled;
    if (control.dataset.disabledByBackend !== "true" && !wasDisabled) {
      control.disabled = false;
      if (control.dataset.controllerTitle) {
        control.title = control.dataset.controllerTitle;
      } else {
        control.removeAttribute("title");
      }
    }
  }
  delete control.dataset.controllerTitle;
}

function renderTransportStatus(state, bluetooth) {
  if (!transportStatus || !transportPill) {
    return;
  }
  const owner = state?.settings?.device?.hsp_dispatch_owner;
  if (owner === DISPATCH_OWNER_BLUETOOTH) {
    const bridge = bluetooth?.bluetooth || state?.bluetooth_bridge || {};
    const status = bridge.status || (bridge.connected ? "connected" : "disconnected");
    transportPill.dataset.state = bridge.connected ? "ok" : status === "error" || status === "stale" ? "error" : "pending";
    transportStatus.textContent = `Bluetooth: ${labelStatus(status)}`;
    return;
  }

  const cloud = state?.cloud_transport || {};
  const playback = cloud.playback_state || state?.transport?.playback_state || "unknown";
  transportPill.dataset.state = cloud.last_error ? "error" : playback === "unknown" ? "pending" : "ok";
  transportStatus.textContent = `Cloud: ${labelStatus(playback)}`;
}

async function checkConnection() {
  if (!connectionCheckStatus) {
    return;
  }
  connectionCheckStatus.textContent = "Checking";
  const owner = fields.dispatchOwner.value;
  const path = owner === DISPATCH_OWNER_BLUETOOTH
    ? "/api/transport/bluetooth/check"
    : "/api/transport/cloud/check";
  try {
    const check = await postEmpty(path);
    renderConnectionCheck(owner, check);
  } catch (error) {
    connectionCheckStatus.textContent = error.message;
    if (transportPill) {
      transportPill.dataset.state = "error";
    }
    if (transportStatus) {
      transportStatus.textContent = `${labelFeature(owner)}: Check failed`;
    }
  }
}

function renderConnectionCheck(owner, check = {}) {
  const hspState = check.hsp_available ? "HSP ready" : "HSP unavailable";
  const playback = check.playback_state ? ` / ${labelStatus(check.playback_state)}` : "";
  const latency = Number.isFinite(check.latency_ms) ? ` / ${check.latency_ms} ms` : "";
  const label = check.ok ? "Connected" : "Not ready";
  connectionCheckStatus.textContent = `${label}: ${hspState}${playback}${latency}`;
  if (transportPill) {
    transportPill.dataset.state = check.ok && check.hsp_available ? "ok" : "error";
  }
  if (transportStatus) {
    transportStatus.textContent = `${labelFeature(owner)}: ${label}`;
  }
}

async function copyDiagnosticsSummary() {
  if (!diagnosticsCopyStatus) {
    return;
  }
  diagnosticsCopyStatus.textContent = "Collecting";
  const summary = {
    copied_at: new Date().toISOString(),
    backend_available: backendAvailable,
    page: window.location.href,
    health: await collectJSON("/healthz"),
    state: await collectJSON("/api/state"),
    transport: await collectJSON("/api/transport/diagnostics"),
    cloud_transport: await collectJSON("/api/transport/cloud/diagnostics"),
    bluetooth_transport: await collectJSON("/api/transport/bluetooth/diagnostics"),
    traces: await collectJSON("/api/traces"),
  };
  try {
    await writeClipboard(JSON.stringify(summary, null, 2));
    diagnosticsCopyStatus.textContent = "Copied";
    showToast("Diagnostics copied.");
  } catch (error) {
    diagnosticsCopyStatus.textContent = error.message;
  }
}

async function collectJSON(path) {
  try {
    return await fetchJSON(path);
  } catch (error) {
    return { error: error.message };
  }
}

async function writeClipboard(text) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text);
    return;
  }
  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "");
  textarea.style.position = "fixed";
  textarea.style.left = "-9999px";
  document.body.appendChild(textarea);
  textarea.select();
  try {
    if (!document.execCommand("copy")) {
      throw new Error("Clipboard copy was blocked.");
    }
  } finally {
    textarea.remove();
  }
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
  applyControllerLock();
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

function controllerHeaders() {
  return {
    "X-MagicHandy-Client-ID": CONTROLLER_CLIENT_ID,
  };
}

function appClientID() {
  return stableClientID("magichandy.controller.client_id", "browser");
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

connectionCheckButton?.addEventListener("click", checkConnection);
llmCheckButton?.addEventListener("click", checkLLM);
llmLoadButton?.addEventListener("click", loadLLM);
llmUnloadButton?.addEventListener("click", unloadLLM);
diagnosticsCopyButton?.addEventListener("click", copyDiagnosticsSummary);

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
