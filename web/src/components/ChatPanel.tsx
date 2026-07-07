// Streaming chat. Keeps near-bottom scroll stickiness with a jump-to-latest
// affordance and surfaces the malformed-response state. Chat can start, adjust,
// and stop motion through the backend contract; the frontend sends only text.
import { useEffect, useRef, useState } from "react";
import { streamChat } from "../api/client";
import type { ChatHistoryMessage } from "../api/types";
import { useAppState, useToast } from "../state/app-state";

interface Msg {
  id: string;
  role: "user" | "assistant";
  text: string;
  streaming?: boolean;
  warning?: boolean;
}

const uid = () => Math.random().toString(36).slice(2, 10);
const MAX_HISTORY = 12;

export function ChatPanel() {
  const { backendOnline, readOnly } = useAppState();
  const { show } = useToast();
  const [messages, setMessages] = useState<Msg[]>([]);
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const [showJump, setShowJump] = useState(false);
  const logRef = useRef<HTMLDivElement>(null);
  const stick = useRef(true);
  const history = useRef<ChatHistoryMessage[]>([]);

  useEffect(() => {
    if (stick.current) {
      const el = logRef.current;
      if (el) el.scrollTop = el.scrollHeight;
    }
  }, [messages]);

  function onScroll() {
    const el = logRef.current;
    if (!el) return;
    const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 60;
    stick.current = nearBottom;
    setShowJump(!nearBottom);
  }
  function jump() {
    const el = logRef.current;
    if (el) el.scrollTop = el.scrollHeight;
    stick.current = true;
    setShowJump(false);
  }

  const locked = !backendOnline || readOnly;

  async function send() {
    const text = draft.trim();
    if (!text || busy || locked) return;
    setDraft("");
    const assistantId = uid();
    setMessages((m) => [
      ...m,
      { id: uid(), role: "user", text },
      { id: assistantId, role: "assistant", text: "", streaming: true },
    ]);
    setBusy(true);
    let raw = "";
    let repairRaw = "";
    let finalReply = "";
    let finalMotion: Record<string, unknown> = { action: "none" };
    try {
      await streamChat(text, history.current, (ev) => {
        if (ev.event === "delta" || ev.event === "repair_delta") {
          const phase = (ev.data as { phase?: string }).phase;
          const chunk = (ev.data as { text?: string }).text ?? "";
          if (ev.event === "repair_delta" || phase === "repair") repairRaw += chunk;
          else raw += chunk;
          const draftReply = extractReplyDraft(ev.event === "repair_delta" ? repairRaw : raw);
          setMessages((m) => m.map((x) => (x.id === assistantId ? { ...x, text: draftReply || x.text || "..." } : x)));
        } else if (ev.event === "message") {
          finalReply = String(ev.data.reply ?? "");
          finalMotion = (ev.data.motion ?? { action: "none" }) as Record<string, unknown>;
          setMessages((m) => m.map((x) => (x.id === assistantId ? { ...x, text: finalReply || "...", warning: Boolean(ev.data.initial_malformed) } : x)));
        } else if (ev.event === "malformed") {
          setMessages((m) => m.map((x) => (x.id === assistantId ? { ...x, warning: true } : x)));
        } else if (ev.event === "error") {
          const message = String((ev.data as { message?: string }).message ?? "Chat error");
          show(message, "error");
          setMessages((m) => m.map((x) => (x.id === assistantId ? { ...x, text: message, warning: true } : x)));
        } else if (ev.event === "done" && ev.data.ok === false && !finalReply) {
          setMessages((m) => m.map((x) => (x.id === assistantId ? { ...x, text: x.text || "Malformed model response.", warning: true } : x)));
        }
      });
      if (finalReply) {
        const nextHistory: ChatHistoryMessage[] = [
          ...history.current,
          { role: "user", content: text },
          { role: "assistant", content: JSON.stringify({ reply: finalReply, motion: finalMotion }) },
        ];
        history.current = nextHistory.slice(-MAX_HISTORY);
      }
    } catch (e) {
      const message = e instanceof Error ? e.message : "Chat failed.";
      show(message, "error");
      setMessages((m) => m.map((x) => (x.id === assistantId ? { ...x, text: message, warning: true } : x)));
    } finally {
      setBusy(false);
      setMessages((m) => m.map((x) => (x.id === assistantId ? { ...x, streaming: false } : x)));
    }
  }

  return (
    <div className="chat">
      <div className="chat-log-shell">
        <div className="chat-log" ref={logRef} onScroll={onScroll} role="log" aria-live="polite" aria-relevant="additions">
          {messages.map((m) => (
            <div key={m.id} className="chat-message" data-role={m.role} data-streaming={m.streaming || undefined} data-state={m.warning ? "warning" : undefined}>
              <span className="chat-avatar" aria-hidden="true">{m.role === "user" ? "You" : "M"}</span>
              <div className="chat-body">
                <span className="chat-speaker">{m.role === "user" ? "You" : "MagicHandy"}</span>
                <div className="chat-bubble">{m.text || (m.warning ? "Malformed model JSON — the reply could not be parsed." : "")}</div>
              </div>
            </div>
          ))}
        </div>
        {showJump && (
          <button type="button" className="btn btn-secondary chat-jump" onClick={jump}>Jump to latest</button>
        )}
      </div>
      <form
        className="chat-form"
        onSubmit={(e) => {
          e.preventDefault();
          void send();
        }}
      >
        <label className="visually-hidden" htmlFor="chat-input">Message</label>
        <textarea
          id="chat-input"
          rows={2}
          maxLength={1000}
          value={draft}
          disabled={locked}
          placeholder={readOnly ? "Read-only client — this tab cannot drive motion." : "Message MagicHandy — chat can start, adjust, and stop motion"}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
              e.preventDefault();
              void send();
            }
          }}
        />
        <div className="chat-actions">
          <button type="submit" className="btn btn-primary" disabled={locked || busy || !draft.trim()}>Send</button>
          <span className="form-status">{busy ? "Streaming…" : locked ? (readOnly ? "Read-only" : "Core offline") : "Idle"}</span>
          <span className="hint-inline" aria-hidden="true">Ctrl+Enter to send</span>
        </div>
      </form>
    </div>
  );
}

function extractReplyDraft(raw: string): string {
  const key = raw.indexOf('"reply"');
  if (key === -1) return "";
  const colon = raw.indexOf(":", key + 7);
  if (colon === -1) return "";
  const quote = raw.indexOf('"', colon + 1);
  if (quote === -1) return "";
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
    if (character === '"') return value;
    value += character;
  }
  return value;
}

function decodeEscape(character: string): string {
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
