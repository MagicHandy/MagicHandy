const statusPill = document.querySelector(".status-pill");
const coreStatus = document.querySelector("#core-status");
const runtimeCore = document.querySelector("#runtime-core");
const runtimeUI = document.querySelector("#runtime-ui");
const runtimeMotion = document.querySelector("#runtime-motion");
const runtimeTransport = document.querySelector("#runtime-transport");
const versionValue = document.querySelector("#version-value");
const commitValue = document.querySelector("#commit-value");
const uptimeValue = document.querySelector("#uptime-value");
const healthValue = document.querySelector("#health-value");
const stopButton = document.querySelector("#stop-button");
const toast = document.querySelector("#toast");

let toastTimer = 0;

async function refreshStatus() {
  try {
    const [health, status] = await Promise.all([
      fetchJSON("/healthz"),
      fetchJSON("/api/status"),
    ]);

    setCoreState("ok", "Core online");
    runtimeCore.textContent = health.status === "ok" ? "Online" : "Degraded";
    runtimeUI.textContent = capitalize(status.ui);
    runtimeMotion.textContent = labelFeature(status.features.motion);
    runtimeTransport.textContent = labelFeature(status.features.transport);
    versionValue.textContent = status.version || "dev";
    commitValue.textContent = status.commit || "unknown";
    uptimeValue.textContent = `${status.uptime_seconds}s`;
    healthValue.textContent = health.status;
  } catch (error) {
    setCoreState("error", "Core unavailable");
    runtimeCore.textContent = "Unavailable";
    healthValue.textContent = error.message;
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

function setCoreState(state, label) {
  statusPill.dataset.state = state;
  coreStatus.textContent = label;
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

refreshStatus();
window.setInterval(refreshStatus, 5000);
