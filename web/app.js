const statusPill = document.querySelector(".status-pill");
const coreStatus = document.querySelector("#core-status");
const runtimeCore = document.querySelector("#runtime-core");
const runtimeUI = document.querySelector("#runtime-ui");
const runtimeSettings = document.querySelector("#runtime-settings");
const runtimeMotion = document.querySelector("#runtime-motion");
const runtimeTransport = document.querySelector("#runtime-transport");
const versionValue = document.querySelector("#version-value");
const commitValue = document.querySelector("#commit-value");
const uptimeValue = document.querySelector("#uptime-value");
const healthValue = document.querySelector("#health-value");
const connectionKeyState = document.querySelector("#connection-key-state");
const stopButton = document.querySelector("#stop-button");
const toast = document.querySelector("#toast");
const form = document.querySelector("#settings-form");
const formStatus = document.querySelector("#settings-status");

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

async function refreshStatus() {
  try {
    const [health, state] = await Promise.all([
      fetchJSON("/healthz"),
      fetchJSON("/api/state"),
    ]);

    setCoreState("ok", "Core online");
    renderState(health, state);
    renderSettings(state.settings);
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

function renderState(health, state) {
  runtimeCore.textContent = health.status === "ok" ? "Online" : "Degraded";
  runtimeUI.textContent = "Embedded";
  runtimeSettings.textContent = labelStatus(state.settings_status.source);
  runtimeMotion.textContent = labelFeature(state.features.motion);
  runtimeTransport.textContent = labelFeature(state.features.transport);
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
  fields.appIDOverride.disabled = !usesOverride;
  if (!usesOverride) {
    fields.appIDOverride.value = "";
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

stopButton.addEventListener("click", () => {
  showToast("No motion engine is active in this build.");
});

fields.appIDSource.addEventListener("change", updateApplicationIDOverrideState);
form.addEventListener("submit", saveSettings);

refreshStatus();
window.setInterval(refreshStatus, 5000);
