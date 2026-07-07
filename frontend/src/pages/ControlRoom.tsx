import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { api } from "../api/client";
import type { ChatMessage, OperationMode } from "../api/types";
import { ChatToolbar } from "../components/ChatToolbar";
import { PersonaAvatar } from "../components/PersonaAvatar";
import { SessionRail } from "../components/SessionRail";
import { TypingIndicator } from "../components/TypingIndicator";
import { useStatus } from "../contexts/StatusContext";
import { useToast } from "../contexts/ToastContext";

export function ControlRoom() {
  const { t } = useTranslation();
  const { snap, error } = useStatus();
  const { notify } = useToast();
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [text, setText] = useState("");
  const [mode, setMode] = useState<OperationMode>("auto");
  const [sending, setSending] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const [awaitingReply, setAwaitingReply] = useState(false);
  const [queueDetail, setQueueDetail] = useState<
    Awaited<ReturnType<typeof api.getQueue>> | null
  >(null);

  const isThinking =
    sending ||
    awaitingReply ||
    Boolean(snap?.chat_pending || snap?.planner_busy);

  const refreshChat = useCallback(async () => {
    try {
      const data = await api.getChatMessages();
      setMessages(data.messages.slice(-50));
    } catch {
      /* */
    }
  }, []);

  useEffect(() => {
    refreshChat();
    api.getStatus().then((s) => setMode(s.operation_mode)).catch(() => {});
    const id = setInterval(refreshChat, 3000);
    return () => clearInterval(id);
  }, [refreshChat]);

  useEffect(() => {
    if (!awaitingReply && !sending) return;
    void refreshChat();
    const id = setInterval(() => void refreshChat(), 700);
    return () => clearInterval(id);
  }, [awaitingReply, sending, refreshChat]);

  useEffect(() => {
    if (!awaitingReply) return;
    const last = messages[messages.length - 1];
    if (last?.role === "assistant") setAwaitingReply(false);
  }, [messages, awaitingReply]);

  useEffect(() => {
    if (!awaitingReply) return;
    const timeoutId = window.setTimeout(() => setAwaitingReply(false), 130_000);
    return () => window.clearTimeout(timeoutId);
  }, [awaitingReply]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, isThinking]);

  useEffect(() => {
    const loadQ = () => {
      api.getQueue().then(setQueueDetail).catch(() => setQueueDetail(null));
    };
    loadQ();
    const id = setInterval(loadQ, 4000);
    return () => clearInterval(id);
  }, [snap?.playback_active, snap?.queue_blocks]);

  const send = async () => {
    const msg = text.trim();
    if (!msg || sending || awaitingReply) return;
    setSending(true);
    setAwaitingReply(true);
    setText("");
    try {
      const res = await api.sendChat(msg);
      if (res.stopped) notify(t("chat.stopWord"), "info");
      else if (res.pending) notify(t("chat.thinking"), "ok");
      else notify(t("chat.sent"), "ok");
      await refreshChat();
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    } finally {
      setSending(false);
    }
  };

  const avatarUrl = snap?.persona_avatar_url ?? null;
  const needsFirstMsg = snap?.auto_running && !snap?.user_session_engaged;
  const personaName = snap?.persona_name ?? t("persona.defaultName");

  if (error) {
    return (
      <div className="page">
        <div className="alert alert-warn">
          {t("layout.apiUnavailable")} <code>Iniciar-MagicHandy.bat</code>
        </div>
      </div>
    );
  }

  return (
    <div className="page control-room page--fill">
      <div className="session-workspace">
        <SessionRail
          snap={snap}
          queueBlocks={queueDetail?.blocks}
          queueEmptyMessage={t("session.queueEmpty")}
        />

        <section className="glass chat-panel" aria-label={t("chat.panelAria")}>
          <div className="chat-header">
            <div className="persona-badge">
              <PersonaAvatar
                url={avatarUrl}
                name={personaName}
                size={44}
                className="persona-badge-avatar"
              />
              <div className="persona-badge-text">
                <strong>{personaName}</strong>
                <span className="hint">
                  {snap?.active_pose && snap.active_pose !== "none"
                    ? `${snap.pose_label ?? snap.active_pose}${snap.pose_detail ? ` · ${snap.pose_detail}` : ""}`
                    : t("chat.ready")}
                </span>
              </div>
            </div>
            <ChatToolbar
              snap={snap}
              mode={mode}
              disabled={sending}
              onModeChange={async (m) => {
                setMode(m);
                await api.setOperationMode(m);
              }}
            />
          </div>

          <div className="messages" role="log" aria-live="polite" aria-relevant="additions">
            {messages.length === 0 && !isThinking && (
              <div className="messages-empty">
                <p className="messages-empty-title">
                  {needsFirstMsg ? t("chat.startSession") : t("chat.noMessages")}
                </p>
                <p className="hint">
                  {needsFirstMsg ? t("chat.startHint") : t("chat.emptyHint")}
                </p>
              </div>
            )}
            {messages.map((m) =>
              m.role === "assistant" ? (
                <div key={m.id} className="bubble-row assistant">
                  <PersonaAvatar url={avatarUrl} name={personaName} size={34} />
                  <div className="bubble assistant">{m.content}</div>
                </div>
              ) : (
                <div key={m.id} className="bubble user">
                  {m.content}
                </div>
              ),
            )}
            {isThinking && (
              <TypingIndicator name={personaName} avatarUrl={avatarUrl} />
            )}
            <div ref={messagesEndRef} className="messages-anchor" />
          </div>

          <div className="chat-compose">
            <textarea
              value={text}
              onChange={(e) => setText(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) send();
              }}
              placeholder={
                needsFirstMsg ? t("chat.placeholderFirst") : t("chat.placeholder")
              }
              rows={2}
              aria-label={t("chat.messageAria")}
            />
            <button
              type="button"
              className="btn btn-primary btn-send"
              disabled={sending || awaitingReply || !text.trim()}
              onClick={send}
            >
              {sending ? "…" : t("common.send")}
            </button>
          </div>
        </section>
      </div>
    </div>
  );
}
