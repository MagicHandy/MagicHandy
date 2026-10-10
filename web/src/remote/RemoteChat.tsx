import { t, translateKnown } from "../i18n";
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { RemoteChatPresence, RemoteState } from "../api/remote-types";
import type { RemoteDisplayMessage } from "../api/remote-types";
import { outcomeFor, type RemoteSend } from "./useRemoteCommands";

const SHOWN_MESSAGES = 40;
// Matches the desktop composer and remote.MaxChatRunes.
const MAX_MESSAGE_CHARACTERS = 1000;

interface Props {
  state: RemoteState;
  send: RemoteSend;
}

// The desktop's open conversation. The phone reads the shared log and asks the
// desktop to send; the desktop's chat composes, streams and speaks the reply.
export function RemoteChat({ state, send }: Props) {
  const chat = state.chat;
  if (!chat) {
    return (
      <section className="remote-chat remote-chat-empty" aria-label={t("Desktop chat")}>
        <p>{t("No chat is open on the desktop.")}</p>
        <button type="button" className="btn btn-primary" onClick={() => void send({ target: "chat", action: "open" })}>
          {t("Open chat on the desktop")}
        </button>
      </section>
    );
  }
  return <RemoteChatSession chat={chat} state={state} send={send} />;
}

function RemoteChatSession({ chat, state, send }: Props & { chat: RemoteChatPresence }) {
  const [messages, setMessages] = useState<RemoteDisplayMessage[]>([]);
  const [loadError, setLoadError] = useState("");
  const [draft, setDraft] = useState("");
  const [sent, setSent] = useState<{ id: string; text: string } | null>(null);
  const [sending, setSending] = useState(false);
  const submitting = useRef(false);
  const logRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const abort = new AbortController();
    api.remoteMessages(chat.session_id, abort.signal).then((page) => {
      if (abort.signal.aborted) return;
      setMessages(page.messages.slice(-SHOWN_MESSAGES));
      setLoadError("");
    }, (reason: unknown) => {
      if (!abort.signal.aborted) setLoadError(reason instanceof Error ? translateKnown(reason.message) : t("Conversation history request failed."));
    });
    return () => abort.abort();
  }, [chat.session_id, chat.busy, chat.latest_seq, chat.revision]);

  useEffect(() => {
    const log = logRef.current;
    if (log) log.scrollTop = log.scrollHeight;
  }, [messages]);

  // A message the desktop could not send comes back to the composer.
  const outcome = sent ? outcomeFor(state, sent.id) : undefined;
  useEffect(() => {
    if (!sent || !outcome) return;
    if (!outcome.ok) setDraft((current) => current || sent.text);
    setSent(null);
  }, [outcome, sent]);

  async function submit() {
    const text = draft.trim();
    if (!text || chat.busy || !chat.ready || sent || submitting.current) return;
    submitting.current = true;
    setSending(true);
    setDraft("");
    const id = await send({ target: "chat", action: "send", text });
    if (id) setSent({ id, text });
    else setDraft((current) => current || text);
    submitting.current = false;
    setSending(false);
  }

  const blocked = chat.busy || !chat.ready || sending || Boolean(sent);
  return (
    <section className="remote-chat" aria-label={t("Desktop chat")}>
      <div className="chat-log remote-chat-log" ref={logRef} role="log" aria-live="polite">
        {loadError && <p className="form-status" role="alert">{loadError}</p>}
        {!loadError && messages.length === 0 && <p className="form-status">{t("No messages yet")}</p>}
        {messages.map((message) => {
          const speaker = message.role === "user" ? t("You") : chat.persona_name || "MagicHandy";
          return (
            <div key={message.seq} className="chat-message" data-role={message.role}>
              <span className="chat-avatar" aria-hidden="true">{speaker.slice(0, 1).toUpperCase()}</span>
              <div className="chat-body">
                <span className="chat-speaker">{speaker}</span>
                <div className="chat-bubble">{message.content}{message.truncated && <span>…</span>}</div>
              </div>
            </div>
          );
        })}
        {chat.busy && <p className="remote-chat-busy" role="status">{t("The desktop is answering…")}</p>}
      </div>
      <form
        className="chat-form remote-chat-form"
        onSubmit={(event) => {
          event.preventDefault();
          void submit();
        }}
      >
        <div className="chat-compose-row">
        <label className="visually-hidden" htmlFor="remote-chat-input">{t("Message")}</label>
        <textarea
          id="remote-chat-input"
          rows={2}
          maxLength={MAX_MESSAGE_CHARACTERS}
          value={draft}
          placeholder={chat.ready ? t("Message the desktop chat…") : t("The desktop chat cannot send right now.")}
          onChange={(event) => setDraft(event.target.value)}
        />
        <button type="submit" className="btn btn-primary chat-send" disabled={blocked || !draft.trim()}>{t("Send")}</button>
        </div>
      </form>
    </section>
  );
}
