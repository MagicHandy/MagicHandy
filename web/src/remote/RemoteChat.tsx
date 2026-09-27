import { t, translateKnown } from "../i18n";
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { RemoteChatPresence, RemoteState } from "../api/remote-types";
import type { ChatLogMessage } from "../api/types";
import { outcomeFor, type RemoteSend } from "./useRemoteCommands";

const SHOWN_MESSAGES = 40;
// Matches the desktop composer and remote.MaxChatRunes.
const MAX_MESSAGE_CHARACTERS = 1000;

interface Props {
  state: RemoteState;
  send: RemoteSend;
  /** The newest committed message, from the shared state poll. */
  latestSeq?: number;
}

// The desktop's open conversation. The phone reads the shared log and asks the
// desktop to send; the desktop's chat composes, streams and speaks the reply.
export function RemoteChat({ state, send, latestSeq }: Props) {
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
  return <RemoteChatSession chat={chat} state={state} send={send} latestSeq={latestSeq} />;
}

function RemoteChatSession({ chat, state, send, latestSeq }: Props & { chat: RemoteChatPresence }) {
  const [messages, setMessages] = useState<ChatLogMessage[]>([]);
  const [loadError, setLoadError] = useState("");
  const [draft, setDraft] = useState("");
  const [sent, setSent] = useState<{ id: string; text: string } | null>(null);
  const logRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const abort = new AbortController();
    api.getChatMessages(chat.session_id, 0, { signal: abort.signal }).then((page) => {
      setMessages(page.messages.slice(-SHOWN_MESSAGES));
      setLoadError("");
    }, (reason: unknown) => {
      if (!abort.signal.aborted) setLoadError(reason instanceof Error ? translateKnown(reason.message) : t("Conversation history request failed."));
    });
    return () => abort.abort();
  }, [chat.session_id, chat.busy, latestSeq]);

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
    if (!text || chat.busy || sent) return;
    setDraft("");
    const id = await send({ target: "chat", action: "send", text });
    if (id) setSent({ id, text });
    else setDraft((current) => current || text);
  }

  const blocked = chat.busy || !chat.ready || Boolean(sent);
  return (
    <section className="remote-chat" aria-label={t("Desktop chat")}>
      <div className="remote-chat-log" ref={logRef} role="log" aria-live="polite">
        {loadError && <p className="form-status" role="alert">{loadError}</p>}
        {!loadError && messages.length === 0 && <p className="form-status">{t("No messages yet")}</p>}
        {messages.map((message) => (
          <div key={message.seq} className="remote-chat-message" data-role={message.role}>
            <span className="chat-speaker">{message.role === "user" ? t("You") : chat.persona_name || "MagicHandy"}</span>
            <div className="chat-bubble">{message.content}</div>
          </div>
        ))}
        {chat.busy && <p className="remote-chat-busy" role="status">{t("The desktop is answering…")}</p>}
      </div>
      <form
        className="remote-chat-form"
        onSubmit={(event) => {
          event.preventDefault();
          void submit();
        }}
      >
        <label className="visually-hidden" htmlFor="remote-chat-input">{t("Message")}</label>
        <textarea
          id="remote-chat-input"
          rows={2}
          maxLength={MAX_MESSAGE_CHARACTERS}
          value={draft}
          placeholder={chat.ready ? t("Message the desktop chat…") : t("The desktop chat cannot send right now.")}
          onChange={(event) => setDraft(event.target.value)}
        />
        <button type="submit" className="btn btn-primary" disabled={blocked || !draft.trim()}>{t("Send")}</button>
      </form>
    </section>
  );
}
