const chat = {
  form: document.querySelector("#chat-form"),
  input: document.querySelector("#chat-input"),
  send: document.querySelector("#chat-send"),
  log: document.querySelector("#chat-log"),
  jump: document.querySelector("#chat-jump"),
  status: document.querySelector("#chat-status"),
  malformed: document.querySelector("#chat-malformed"),
  provider: document.querySelector("#chat-provider"),
};

const history = [];
let streaming = false;
let backendAvailable = true;
let controllerReadOnly = false;
let stickToBottom = true;
const CONTROLLER_CLIENT_ID = appClientID();

function appendMessage(role, text, state = "", options = {}) {
  const shouldStick = options.forceScroll || shouldStickToBottom();
  const message = document.createElement("div");
  message.className = "chat-message";
  message.dataset.role = role;
  if (state) {
    message.dataset.state = state;
  }
  message.textContent = text;
  chat.log.appendChild(message);
  maybeScrollToLatest(shouldStick);
  return message;
}

function setStatus(message) {
  chat.status.textContent = message;
}

function setMalformed(message, repaired = false) {
  chat.malformed.hidden = !message;
  chat.malformed.textContent = message;
  chat.malformed.dataset.repaired = repaired ? "true" : "false";
}

function rememberTurn(user, assistantContract) {
  history.push({ role: "user", content: user });
  history.push({ role: "assistant", content: JSON.stringify(assistantContract) });
  while (history.length > 12) {
    history.shift();
  }
}

function updateChatAvailability() {
  chat.input.disabled = !backendAvailable || controllerReadOnly;
  chat.send.disabled = streaming || !backendAvailable || controllerReadOnly;
}

function isNearBottom() {
  if (!chat.log) {
    return true;
  }
  const remaining = chat.log.scrollHeight - chat.log.scrollTop - chat.log.clientHeight;
  return remaining < 56;
}

function shouldStickToBottom() {
  stickToBottom = isNearBottom();
  return stickToBottom;
}

function maybeScrollToLatest(shouldStick) {
  if (shouldStick) {
    scrollToLatest();
    return;
  }
  updateJumpVisibility();
}

function scrollToLatest() {
  chat.log.scrollTop = chat.log.scrollHeight;
  stickToBottom = true;
  updateJumpVisibility();
}

function updateJumpVisibility() {
  if (!chat.jump) {
    return;
  }
  chat.jump.hidden = stickToBottom || !chat.log.children.length;
}

async function sendChat(event) {
  event.preventDefault();
  if (streaming) {
    return;
  }

  const text = chat.input.value.trim();
  if (!text) {
    return;
  }

  streaming = true;
  updateChatAvailability();
  chat.input.value = "";
  setMalformed("");
  setStatus("Streaming");
  appendMessage("user", text, "", { forceScroll: true });
  const assistant = appendMessage("assistant", "...", "", { forceScroll: true });

  let raw = "";
  let repairRaw = "";
  let finalReply = "";

  try {
    const response = await fetch("/api/chat/stream", {
      method: "POST",
      headers: {
        Accept: "text/event-stream",
        "Content-Type": "application/json",
        "X-MagicHandy-Client-ID": CONTROLLER_CLIENT_ID,
      },
      body: JSON.stringify({ message: text, history }),
    });
    if (!response.ok) {
      const body = await response.json().catch(() => ({}));
      throw new Error(body.error || `/api/chat/stream returned ${response.status}`);
    }
    if (!response.body) {
      throw new Error("Streaming response body is unavailable.");
    }

    await readEventStream(response.body, (name, payload) => {
      if (name === "status") {
        chat.provider.textContent = `${labelProvider(payload.provider)} / ${payload.model}`;
        setStatus("Streaming");
        return;
      }
      if (name === "delta") {
        raw += payload.text || "";
        renderDraft(assistant, raw);
        return;
      }
      if (name === "repair_delta") {
        repairRaw += payload.text || "";
        renderDraft(assistant, repairRaw);
        return;
      }
      if (name === "malformed") {
        const repaired = Boolean(payload.repaired);
        assistant.dataset.state = "warning";
        setMalformed(repaired ? "Repaired model JSON" : "Malformed model JSON", repaired);
        return;
      }
      if (name === "message") {
        const shouldStick = shouldStickToBottom();
        finalReply = payload.reply || "";
        assistant.textContent = finalReply || "...";
        maybeScrollToLatest(shouldStick);
        if (!payload.initial_malformed) {
          assistant.dataset.state = "";
        }
        rememberTurn(text, {
          reply: finalReply,
          motion: payload.motion || { action: "none" },
        });
        return;
      }
      if (name === "motion") {
        setStatus(motionStatus(payload));
        return;
      }
      if (name === "error") {
        const shouldStick = shouldStickToBottom();
        assistant.dataset.state = "warning";
        assistant.textContent = payload.message || "Chat failed.";
        maybeScrollToLatest(shouldStick);
        setStatus("Failed");
        return;
      }
      if (name === "done") {
        if (!payload.ok && !finalReply) {
          const shouldStick = shouldStickToBottom();
          assistant.dataset.state = "warning";
          assistant.textContent = "Malformed model response.";
          maybeScrollToLatest(shouldStick);
        }
        setStatus(payload.ok ? "Idle" : "Needs attention");
      }
    });
  } catch (error) {
    const shouldStick = shouldStickToBottom();
    assistant.dataset.state = "warning";
    assistant.textContent = error.message;
    maybeScrollToLatest(shouldStick);
    setStatus("Failed");
  } finally {
    streaming = false;
    updateChatAvailability();
    chat.input.focus();
  }
}

async function readEventStream(body, onEvent) {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  for (;;) {
    const { value, done } = await reader.read();
    if (done) {
      break;
    }
    buffer += decoder.decode(value, { stream: true });
    let split = buffer.indexOf("\n\n");
    while (split !== -1) {
      const block = buffer.slice(0, split);
      buffer = buffer.slice(split + 2);
      dispatchEventBlock(block, onEvent);
      split = buffer.indexOf("\n\n");
    }
  }
  buffer += decoder.decode();
  if (buffer.trim()) {
    dispatchEventBlock(buffer, onEvent);
  }
}

function dispatchEventBlock(block, onEvent) {
  let name = "message";
  const data = [];
  for (const line of block.split(/\r?\n/)) {
    if (line.startsWith("event:")) {
      name = line.slice(6).trim();
    } else if (line.startsWith("data:")) {
      data.push(line.slice(5).trimStart());
    }
  }
  if (!data.length) {
    return;
  }
  let payload = {};
  try {
    payload = JSON.parse(data.join("\n"));
  } catch {
    payload = { text: data.join("\n") };
  }
  onEvent(name, payload);
}

function renderDraft(element, raw) {
  const shouldStick = shouldStickToBottom();
  const reply = extractReplyDraft(raw);
  element.textContent = reply || "...";
  maybeScrollToLatest(shouldStick);
}

function extractReplyDraft(raw) {
  const key = raw.indexOf('"reply"');
  if (key === -1) {
    return "";
  }
  const colon = raw.indexOf(":", key + 7);
  if (colon === -1) {
    return "";
  }
  const quote = raw.indexOf('"', colon + 1);
  if (quote === -1) {
    return "";
  }
  let value = "";
  let escaping = false;
  for (let index = quote + 1; index < raw.length; index += 1) {
    const character = raw[index];
    if (escaping) {
      value += decodeEscape(character);
      escaping = false;
      continue;
    }
    if (character === "\\") {
      escaping = true;
      continue;
    }
    if (character === '"') {
      return value;
    }
    value += character;
  }
  return value;
}

function decodeEscape(character) {
  switch (character) {
    case "n":
      return "\n";
    case "r":
      return "\r";
    case "t":
      return "\t";
    case '"':
    case "\\":
    case "/":
      return character;
    default:
      return character;
  }
}

function motionStatus(payload = {}) {
  if (payload.error) {
    return `${labelProvider(payload.action || "motion")} failed`;
  }
  if (!payload.applied) {
    return "Idle";
  }
  return `${labelProvider(payload.action || "motion")} applied`;
}

function labelProvider(value) {
  return String(value || "unknown")
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

async function refreshChatState() {
  try {
    const response = await fetch("/api/state", {
      headers: {
        Accept: "application/json",
        "X-MagicHandy-Client-ID": CONTROLLER_CLIENT_ID,
      },
    });
    if (!response.ok) {
      return;
    }
    const state = await response.json();
    if (state.llm?.provider) {
      chat.provider.textContent = `${labelProvider(state.llm.provider)} / ${state.llm.model}`;
    }
  } catch {
    // The core status poll in app.js owns global availability.
  }
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

chat.form?.addEventListener("submit", sendChat);
chat.log?.addEventListener("scroll", () => {
  stickToBottom = isNearBottom();
  updateJumpVisibility();
});
chat.jump?.addEventListener("click", scrollToLatest);
chat.input?.addEventListener("keydown", (event) => {
  if (event.key === "Enter" && (event.ctrlKey || event.metaKey)) {
    chat.form.requestSubmit();
  }
});
window.addEventListener("magichandy:backend-availability", (event) => {
  backendAvailable = Boolean(event.detail?.available);
  updateChatAvailability();
});
window.addEventListener("magichandy:controller-state", (event) => {
  controllerReadOnly = Boolean(event.detail?.read_only);
  updateChatAvailability();
});

updateChatAvailability();
refreshChatState();
