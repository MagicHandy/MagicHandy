// Streaming chat. Keeps near-bottom scroll stickiness with a jump-to-latest
// affordance and surfaces the malformed-response state. Chat can start, adjust,
// and stop motion through the backend contract; the frontend sends only text.
import { useEffect, useRef, useState } from "react";
import { streamChat } from "../api/client";
import { useAppState, useToast } from "../state/app-state";

interface Msg {
  id: string;
  role: "user" | "assistant";
  text: string;
  streaming?: boolean;
  warning?: boolean;
}

const uid = () => Math.random().toString(36).slice(2, 10);

export function ChatPanel() {
  const { backendOnline, readOnly } = useAppState();
  const { show } = useToast();
  const [messages, setMessages] = useState<Msg[]>([]);
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const [showJump, setShowJump] = useState(false);
  const logRef = useRef<HTMLDivElement>(null);
  const stick = useRef(true);

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
    try {
      // Multi-turn history is a follow-up port; each turn stands alone for now,
      // with the backend supplying system prompt and memory.
      await streamChat(text, [], (ev) => {
        if (ev.event === "delta" || ev.event === "repair_delta") {
          const phase = (ev.data as { phase?: string }).phase;
          const chunk = (ev.data as { text?: string }).text ?? "";
          if (phase === undefined || phase === "" || phase === "reply") {
            setMessages((m) => m.map((x) => (x.id === assistantId ? { ...x, text: x.text + chunk } : x)));
          }
        } else if (ev.event === "malformed") {
          setMessages((m) => m.map((x) => (x.id === assistantId ? { ...x, warning: true } : x)));
        } else if (ev.event === "error") {
          show(String((ev.data as { message?: string }).message ?? "Chat error"), "error");
        }
      });
    } catch (e) {
      show(e instanceof Error ? e.message : "Chat failed.", "error");
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
