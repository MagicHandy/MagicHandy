// prompts-memory-ui.js — Phase 10 personalization surface.
//
// Owns the prompt-set editor (built-ins are read-only templates; duplicate to
// edit), the long-term memory manager (immediate-apply toggles, individual
// removal, clear-all), and the settings factory reset. Everything applies
// through its own endpoint; the surrounding Save-settings form is untouched
// except the active-set select, which the form saves normally.

const el = (id) => document.getElementById(id);

const ui = {
  activeSelect: el("llm-prompt-set"),
  editor: {
    select: el("prompt-editor-select"),
    name: el("prompt-editor-name"),
    system: el("prompt-editor-system"),
    badge: el("prompt-editor-badge"),
    save: el("prompt-editor-save"),
    duplicate: el("prompt-editor-duplicate"),
    create: el("prompt-editor-new"),
    remove: el("prompt-editor-delete"),
    status: el("prompt-editor-status"),
  },
  memory: {
    enabled: el("memory-enabled"),
    list: el("memory-list"),
    addText: el("memory-add-text"),
    add: el("memory-add"),
    clear: el("memory-clear"),
    status: el("memory-status"),
  },
  reset: el("settings-reset"),
  resetStatus: el("settings-reset-status"),
};

let promptSets = [];
let editingID = "";
let creatingNew = false;

const CLIENT_ID = (() => {
  try {
    return window.localStorage.getItem("magichandy.controller.client_id") || "";
  } catch {
    return "";
  }
})();

async function api(path, method = "GET", body = undefined) {
  const options = { method, headers: { Accept: "application/json" } };
  if (CLIENT_ID) {
    options.headers["X-MagicHandy-Client-ID"] = CLIENT_ID;
  }
  if (body !== undefined) {
    options.headers["Content-Type"] = "application/json";
    options.body = JSON.stringify(body);
  }
  const response = await fetch(path, options);
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(payload.error || `${path} returned ${response.status}`);
  }
  return payload;
}

// Two-step confirmation for destructive actions: first click arms the button,
// the second within 4s executes; anything else disarms.
function confirmable(button, label, action) {
  let timer = 0;
  button.addEventListener("click", async () => {
    if (button.dataset.armed !== "true") {
      button.dataset.armed = "true";
      button.dataset.label = button.textContent;
      button.textContent = `Confirm: ${label}`;
      timer = window.setTimeout(() => disarm(button), 4000);
      return;
    }
    window.clearTimeout(timer);
    disarm(button);
    await action();
  });
}

function disarm(button) {
  if (button.dataset.armed === "true") {
    button.dataset.armed = "false";
    button.textContent = button.dataset.label || button.textContent;
  }
}

// --- Prompt sets -----------------------------------------------------------------

function renderPromptSets(payload) {
  promptSets = payload.sets || [];

  fillSetSelect(ui.activeSelect);
  const pending = ui.activeSelect.dataset.pendingValue;
  const target = pending || payload.selected;
  if (promptSets.some((set) => set.id === target)) {
    ui.activeSelect.value = target;
    delete ui.activeSelect.dataset.pendingValue;
  }

  fillSetSelect(ui.editor.select);
  if (!creatingNew) {
    if (!promptSets.some((set) => set.id === editingID)) {
      editingID = payload.selected;
    }
    if (promptSets.some((set) => set.id === editingID)) {
      ui.editor.select.value = editingID;
    }
    renderEditorFields();
  }
}

function fillSetSelect(select) {
  const previous = select.value;
  select.replaceChildren(
    ...promptSets.map((set) => {
      const option = document.createElement("option");
      option.value = set.id;
      option.textContent = set.builtin ? `${set.name} — built-in` : set.name;
      return option;
    }),
  );
  if (promptSets.some((set) => set.id === previous)) {
    select.value = previous;
  }
}

function currentEditorSet() {
  return promptSets.find((set) => set.id === ui.editor.select.value);
}

function renderEditorFields() {
  const set = currentEditorSet();
  if (!set) {
    return;
  }
  creatingNew = false;
  editingID = set.id;
  ui.editor.name.value = set.name;
  ui.editor.system.value = set.system;
  ui.editor.badge.hidden = !set.builtin;
  ui.editor.name.readOnly = set.builtin;
  ui.editor.system.readOnly = set.builtin;
  ui.editor.save.disabled = set.builtin;
  ui.editor.remove.disabled = set.builtin;
  setStatus(ui.editor.status, "");
}

function enterNewSetMode(baseName, baseSystem) {
  creatingNew = true;
  ui.editor.name.readOnly = false;
  ui.editor.system.readOnly = false;
  ui.editor.save.disabled = false;
  ui.editor.remove.disabled = true;
  ui.editor.badge.hidden = true;
  ui.editor.name.value = baseName;
  ui.editor.system.value = baseSystem;
  setStatus(ui.editor.status, "New set — Save set to keep it.");
  ui.editor.name.focus();
}

async function savePromptSet() {
  try {
    if (creatingNew) {
      const payload = await api("/api/prompt-sets", "POST", {
        name: ui.editor.name.value,
        system: ui.editor.system.value,
      });
      creatingNew = false;
      editingID = payload.set?.id || editingID;
      renderPromptSets(payload);
      setStatus(ui.editor.status, "Created.");
      return;
    }
    const set = currentEditorSet();
    if (!set || set.builtin) {
      return;
    }
    const payload = await api(`/api/prompt-sets/${encodeURIComponent(set.id)}`, "PUT", {
      name: ui.editor.name.value,
      system: ui.editor.system.value,
    });
    renderPromptSets(payload);
    setStatus(ui.editor.status, "Saved.");
  } catch (error) {
    setStatus(ui.editor.status, error.message);
  }
}

async function deletePromptSet() {
  const set = currentEditorSet();
  if (!set || set.builtin) {
    return;
  }
  try {
    const payload = await api(`/api/prompt-sets/${encodeURIComponent(set.id)}`, "DELETE");
    editingID = payload.selected;
    renderPromptSets(payload);
    setStatus(ui.editor.status, "Deleted.");
  } catch (error) {
    setStatus(ui.editor.status, error.message);
  }
}

// --- Memory ----------------------------------------------------------------------

function renderMemory(snapshot) {
  ui.memory.enabled.checked = Boolean(snapshot.enabled);
  ui.memory.list.replaceChildren(
    ...(snapshot.memories || []).map((item) => memoryRow(item)),
  );
  ui.memory.clear.disabled = !(snapshot.memories || []).length;
}

function memoryRow(item) {
  const row = document.createElement("li");
  row.className = "memory-row";
  row.dataset.memoryId = item.id;

  const toggleLabel = document.createElement("label");
  toggleLabel.className = "toggle-switch";
  const toggle = document.createElement("input");
  toggle.type = "checkbox";
  toggle.setAttribute("role", "switch");
  toggle.checked = item.enabled;
  toggle.setAttribute("aria-label", "Include this memory in chat");
  const track = document.createElement("span");
  track.className = "toggle-track";
  track.setAttribute("aria-hidden", "true");
  toggleLabel.append(toggle, track);
  toggle.addEventListener("change", async () => {
    try {
      renderMemory(await api(`/api/memory/${encodeURIComponent(item.id)}`, "PATCH", {
        enabled: toggle.checked,
      }));
      setStatus(ui.memory.status, "Applied.");
    } catch (error) {
      toggle.checked = !toggle.checked;
      setStatus(ui.memory.status, error.message);
    }
  });

  const text = document.createElement("span");
  text.className = "memory-text";
  text.textContent = item.text;
  if (!item.enabled) {
    row.dataset.disabled = "true";
  }

  const remove = document.createElement("button");
  remove.type = "button";
  remove.className = "secondary-button memory-remove";
  remove.textContent = "Remove";
  remove.addEventListener("click", async () => {
    try {
      renderMemory(await api(`/api/memory/${encodeURIComponent(item.id)}`, "DELETE"));
      setStatus(ui.memory.status, "Removed.");
    } catch (error) {
      setStatus(ui.memory.status, error.message);
    }
  });

  row.append(toggleLabel, text, remove);
  return row;
}

async function addMemory() {
  const text = ui.memory.addText.value.trim();
  if (!text) {
    setStatus(ui.memory.status, "Enter the memory text first.");
    return;
  }
  try {
    renderMemory(await api("/api/memory", "POST", { text }));
    ui.memory.addText.value = "";
    setStatus(ui.memory.status, "Added.");
  } catch (error) {
    setStatus(ui.memory.status, error.message);
  }
}

// --- Shared ----------------------------------------------------------------------

function setStatus(element, message) {
  element.textContent = message;
}

async function refresh() {
  try {
    renderPromptSets(await api("/api/prompt-sets"));
  } catch (error) {
    setStatus(ui.editor.status, error.message);
  }
  try {
    renderMemory(await api("/api/memory"));
  } catch (error) {
    setStatus(ui.memory.status, error.message);
  }
}

// --- Wiring ----------------------------------------------------------------------

ui.editor.select?.addEventListener("change", renderEditorFields);
ui.editor.save?.addEventListener("click", savePromptSet);
ui.editor.create?.addEventListener("click", () => enterNewSetMode("", ""));
ui.editor.duplicate?.addEventListener("click", () => {
  const set = currentEditorSet();
  enterNewSetMode(set ? `${set.name} copy` : "", set ? set.system : "");
});
confirmable(ui.editor.remove, "delete this set", deletePromptSet);

ui.memory.enabled?.addEventListener("change", async () => {
  try {
    renderMemory(await api("/api/memory/enabled", "POST", { enabled: ui.memory.enabled.checked }));
    setStatus(ui.memory.status, ui.memory.enabled.checked ? "Memory on." : "Memory off — chat runs without it.");
  } catch (error) {
    ui.memory.enabled.checked = !ui.memory.enabled.checked;
    setStatus(ui.memory.status, error.message);
  }
});
ui.memory.add?.addEventListener("click", addMemory);
confirmable(ui.memory.clear, "delete every memory", async () => {
  try {
    renderMemory(await api("/api/memory/clear", "POST", {}));
    setStatus(ui.memory.status, "All memories removed.");
  } catch (error) {
    setStatus(ui.memory.status, error.message);
  }
});

confirmable(ui.reset, "reset every setting", async () => {
  try {
    await api("/api/settings/reset", "POST", {});
    setStatus(ui.resetStatus, "Reset. Reloading…");
    window.location.reload();
  } catch (error) {
    setStatus(ui.resetStatus, error.message);
  }
});

refresh();
