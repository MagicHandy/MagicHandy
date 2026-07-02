// motion-ui.js — Phase 8 motion control surface.
//
// Owns the live visualizer, Start/Stop, immediate-apply quick controls, the
// persistent Stop button, transport/engine diagnostics, and trace export.
// State is read from the backend motion engine (docs/ui-design.md: the
// visualizer reflects engine state, never guessed client state).

const el = (id) => document.getElementById(id);

const ui = {
  visualizer: document.querySelector(".visualizer"),
  range: document.querySelector(".visualizer-range"),
  position: document.querySelector(".visualizer-position"),
  state: el("motion-state"),
  substate: el("motion-substate"),
  stop: el("stop-button"),
  start: el("motion-start"),
  stopMotion: el("motion-stop"),
  runReadout: el("motion-run-readout"),
  pattern: el("motion-pattern"),
  speed: el("motion-speed"),
  speedValue: el("motion-speed-value"),
  quick: {
    speedMin: el("quick-speed-min"),
    speedMax: el("quick-speed-max"),
    strokeMin: el("quick-stroke-min"),
    strokeMax: el("quick-stroke-max"),
    reverse: el("quick-reverse"),
  },
  quickOut: {
    speedMin: el("quick-speed-min-value"),
    speedMax: el("quick-speed-max-value"),
    strokeMin: el("quick-stroke-min-value"),
    strokeMax: el("quick-stroke-max-value"),
  },
  quickStatus: el("quick-status"),
  traceExport: el("trace-export"),
  diag: {
    engineState: el("engine-state"),
    enginePosition: el("engine-position"),
    engineError: el("engine-error"),
    transportPlayback: el("transport-playback"),
    transportCommand: el("transport-command"),
    transportLatency: el("transport-latency"),
  },
};

let running = false;
let pollTimer = 0;
let quickTimer = 0;
let backendAvailable = true;
let controllerReadOnly = false;
let motionEvents = null;
const CONTROLLER_CLIENT_ID = appClientID();

async function getJSON(path) {
  const response = await fetch(path, { headers: { Accept: "application/json", ...controllerHeaders() } });
  if (!response.ok) {
    throw new Error(`${path} returned ${response.status}`);
  }
  return response.json();
}

async function postJSON(path, body) {
  const response = await fetch(path, {
    method: "POST",
    headers: { Accept: "application/json", "Content-Type": "application/json", ...controllerHeaders() },
    body: JSON.stringify(body || {}),
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(payload.error || `${path} returned ${response.status}`);
  }
  return payload;
}

function titleCase(value) {
  return String(value || "")
    .split("_")
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

// --- Visualizer + engine state ------------------------------------------------

function renderMotion(motion) {
  const available = Boolean(motion?.available);
  const engine = motion?.engine || {};
  running = available && Boolean(engine.running);

  ui.start.disabled = controllerReadOnly || !backendAvailable || !available || running;
  ui.stopMotion.disabled = !running;
  setQuickControlsDisabled(controllerReadOnly || !backendAvailable || !available);

  const stateName = !available ? "unavailable" : running ? "running" : "idle";
  ui.state.textContent = available ? (running ? "Running" : "Idle") : "Unavailable";
  ui.state.dataset.state = stateName;
  if (ui.visualizer) {
    ui.visualizer.dataset.state = stateName;
  }

  const settings = engine.settings || {};
  const strokeMin = clampPercent(settings.stroke_min_percent, 0);
  const strokeMax = clampPercent(settings.stroke_max_percent, 100);
  if (ui.range) {
    ui.range.style.left = `${strokeMin}%`;
    ui.range.style.width = `${Math.max(0, strokeMax - strokeMin)}%`;
  }

  const sample = engine.last_sample;
  const positionPercent = sample ? clampPercent(sample.position_percent, 50) : 50;
  if (ui.position) {
    ui.position.style.left = `${positionPercent}%`;
    ui.position.dataset.active = running ? "true" : "false";
    ui.position.title = "Commanded position estimate";
  }

  const target = engine.target || {};
  if (running) {
    ui.runReadout.textContent = `${titleCase(target.pattern_id || "stroke")} · speed ${target.speed_percent ?? "—"}%`;
    ui.substate.textContent = engine.last_error ? "recovering estimate" : "estimate";
  } else {
    ui.runReadout.textContent = available ? "Idle" : "Motion unavailable";
    ui.substate.textContent = sample ? "estimate" : "";
  }
}

function clampPercent(value, fallback) {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    return fallback;
  }
  return Math.max(0, Math.min(100, number));
}

function renderDiagnostics(state) {
  const engine = state?.motion?.engine || {};
  ui.diag.engineState.textContent = state?.motion?.available
    ? engine.running
      ? "Running"
      : "Idle"
    : "Unavailable";
  const sample = engine.last_sample;
  ui.diag.enginePosition.textContent = sample ? `${sample.position_percent}% estimate` : "—";
  ui.diag.engineError.textContent = engine.last_error || "None";

  const transport = state?.transport || {};
  ui.diag.transportPlayback.textContent = titleCase(transport.playback_state) || "—";
  const lastCommand = transport.last_command;
  ui.diag.transportCommand.textContent = lastCommand ? titleCase(lastCommand.kind) : "—";
  ui.diag.transportLatency.textContent = Number.isFinite(transport.last_latency_ms)
    ? `${transport.last_latency_ms} ms`
    : "—";
}

async function poll() {
  try {
    const [motion, state] = await Promise.all([getJSON("/api/motion/state"), getJSON("/api/state")]);
    renderMotion(motion);
    renderDiagnostics(state);
  } catch {
    running = false;
    ui.state.textContent = "Unavailable";
    ui.state.dataset.state = "unavailable";
    ui.start.disabled = true;
    setQuickControlsDisabled(true);
  }
  schedulePoll();
}

function schedulePoll() {
  window.clearTimeout(pollTimer);
  pollTimer = window.setTimeout(poll, running ? 250 : 1500);
}

// --- Controls -----------------------------------------------------------------

async function startMotion() {
  setQuickStatus("");
  try {
    const motion = await postJSON("/api/motion/start", {
      pattern: ui.pattern.value,
      speed_percent: Number.parseInt(ui.speed.value, 10),
    });
    renderMotion(motion);
    schedulePoll();
  } catch (error) {
    setQuickStatus(error.message);
  }
}

async function stopMotion() {
  try {
    const motion = await postJSON("/api/motion/stop", {});
    renderMotion(motion);
    schedulePoll();
  } catch (error) {
    setQuickStatus(error.message);
  }
}

async function applySpeed() {
  ui.speedValue.textContent = `${ui.speed.value}%`;
  if (!running) {
    return;
  }
  try {
    const motion = await postJSON("/api/motion/target", {
      pattern: ui.pattern.value,
      speed_percent: Number.parseInt(ui.speed.value, 10),
    });
    renderMotion(motion);
  } catch (error) {
    setQuickStatus(error.message);
  }
}

function quickPayload() {
  const values = normalizeQuickControls();
  return {
    speed_min_percent: values.speedMin,
    speed_max_percent: values.speedMax,
    stroke_min_percent: values.strokeMin,
    stroke_max_percent: values.strokeMax,
    reverse_direction: ui.quick.reverse.checked,
  };
}

function readQuickControls() {
  return {
    speedMin: sliderInt(ui.quick.speedMin, 1),
    speedMax: sliderInt(ui.quick.speedMax, 100),
    strokeMin: sliderInt(ui.quick.strokeMin, 0),
    strokeMax: sliderInt(ui.quick.strokeMax, 100),
  };
}

function sliderInt(control, fallback) {
  const value = Number.parseInt(control.value, 10);
  return Number.isFinite(value) ? value : fallback;
}

function normalizeQuickValues(values) {
  const normalized = { ...values };
  if (normalized.speedMin > normalized.speedMax) {
    normalized.speedMax = normalized.speedMin;
  }
  if (normalized.strokeMin >= normalized.strokeMax) {
    normalized.strokeMax = Math.min(100, normalized.strokeMin + 1);
    if (normalized.strokeMin >= normalized.strokeMax) {
      normalized.strokeMin = Math.max(0, normalized.strokeMax - 1);
    }
  }
  return normalized;
}

function normalizeQuickControls() {
  const values = normalizeQuickValues(readQuickControls());
  ui.quick.speedMin.value = values.speedMin;
  ui.quick.speedMax.value = values.speedMax;
  ui.quick.strokeMin.value = values.strokeMin;
  ui.quick.strokeMax.value = values.strokeMax;
  return values;
}

function updateQuickOutputs() {
  const values = normalizeQuickControls();
  ui.quickOut.speedMin.textContent = `${values.speedMin}%`;
  ui.quickOut.speedMax.textContent = `${values.speedMax}%`;
  ui.quickOut.strokeMin.textContent = `${values.strokeMin}%`;
  ui.quickOut.strokeMax.textContent = `${values.strokeMax}%`;
}

async function applyQuick() {
  window.clearTimeout(quickTimer);
  setQuickStatus("Applying…");
  try {
    const result = await postJSON("/api/motion/quick", quickPayload());
    if (result.motion) {
      applyMotionSettings(result.motion);
    }
    setQuickStatus("Applied");
    if (result.engine) {
      renderMotion({ available: true, engine: result.engine });
    }
  } catch (error) {
    setQuickStatus(error.message);
  }
}

function scheduleQuick() {
  window.clearTimeout(quickTimer);
  quickTimer = window.setTimeout(applyQuick, 150);
}

function setQuickStatus(message) {
  ui.quickStatus.textContent = message;
}

function setQuickControlsDisabled(disabled) {
  for (const control of Object.values(ui.quick)) {
    control.disabled = disabled;
  }
}

function controllerHeaders() {
  return {
    "X-MagicHandy-Client-ID": CONTROLLER_CLIENT_ID,
  };
}

function appClientID() {
  const key = "magichandy.controller.client_id";
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

function applyMotionSettings(motion) {
  ui.quick.speedMin.value = motion.speed_min_percent;
  ui.quick.speedMax.value = motion.speed_max_percent;
  ui.quick.strokeMin.value = motion.stroke_min_percent;
  ui.quick.strokeMax.value = motion.stroke_max_percent;
  ui.quick.reverse.checked = Boolean(motion.reverse_direction);
  updateQuickOutputs();
  const mid = Math.round((motion.speed_min_percent + motion.speed_max_percent) / 2);
  if (!ui.speed.dataset.touched) {
    ui.speed.value = mid;
    ui.speedValue.textContent = `${mid}%`;
  }
}

async function exportTrace() {
  try {
    const trace = await getJSON("/api/traces");
    const blob = new Blob([JSON.stringify(trace, null, 2)], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `magichandy-trace-${Date.now()}.json`;
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(url);
  } catch (error) {
    setQuickStatus(error.message);
  }
}

// --- Wiring -------------------------------------------------------------------

ui.start.addEventListener("click", startMotion);
ui.stopMotion.addEventListener("click", stopMotion);
ui.stop.addEventListener("click", stopMotion);
ui.speed.addEventListener("input", () => {
  ui.speed.dataset.touched = "true";
  ui.speedValue.textContent = `${ui.speed.value}%`;
});
ui.speed.addEventListener("change", applySpeed);
ui.pattern.addEventListener("change", () => {
  if (running) {
    applySpeed();
  }
});

for (const control of Object.values(ui.quick)) {
  control.addEventListener("input", updateQuickOutputs);
  control.addEventListener("change", scheduleQuick);
}

ui.traceExport.addEventListener("click", exportTrace);

window.addEventListener("magichandy:backend-availability", (event) => {
  backendAvailable = Boolean(event.detail?.available);
  if (!backendAvailable) {
    ui.start.disabled = true;
    setQuickControlsDisabled(true);
    ui.state.textContent = "Unavailable";
    ui.state.dataset.state = "unavailable";
    if (ui.visualizer) {
      ui.visualizer.dataset.state = "unavailable";
    }
    return;
  }
  setQuickControlsDisabled(controllerReadOnly);
});

window.addEventListener("magichandy:controller-state", (event) => {
  controllerReadOnly = Boolean(event.detail?.read_only);
  if (controllerReadOnly) {
    ui.start.disabled = true;
    setQuickControlsDisabled(true);
    return;
  }
  setQuickControlsDisabled(!backendAvailable);
});

document.addEventListener("keydown", (event) => {
  if (event.key === "Escape") {
    stopMotion();
  }
});

function startMotionEvents() {
  if (typeof EventSource === "undefined") {
    return;
  }
  const url = `/api/motion/events?client_id=${encodeURIComponent(CONTROLLER_CLIENT_ID)}`;
  motionEvents = new EventSource(url);
  motionEvents.addEventListener("motion", (event) => {
    try {
      renderMotion(JSON.parse(event.data));
    } catch {
      // The polling path will recover if a malformed event is ever observed.
    }
  });
  motionEvents.addEventListener("error", () => {
    if (motionEvents) {
      motionEvents.close();
      motionEvents = null;
    }
    window.setTimeout(startMotionEvents, 2000);
  });
}

async function init() {
  try {
    const state = await getJSON("/api/state");
    if (state?.settings?.motion) {
      applyMotionSettings(state.settings.motion);
    }
  } catch {
    // The poll loop will retry and surface unavailability.
  }
  startMotionEvents();
  poll();
}

init();
