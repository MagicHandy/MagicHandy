import { t, translateKnown } from "../i18n";
import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { ChatSessionsResponse } from "../api/types";
import { ChatPanel } from "../components/ChatPanel";
import { CloseIcon } from "../shell/icons";
import { useAppState } from "../state/app-state";
import { chatMotionOwner, motionSourceNote, type MotionSource } from "./motionSource";

interface Props {
  /** What moves the device while the video plays. */
  source: MotionSource;
  onClose: () => void;
}

// The active conversation beside the video. It is the same backend session the
// Chat page shows, so switching pages keeps one conversation, and replies are
// spoken by this tab when it holds control, as on the Chat page. Unless the
// chat is the motion source, its replies are words only.
export function VideoChatSide({ source, onClose }: Props) {
  const { state } = useAppState();
  const activeID = state?.chat?.active_session_id ?? "";
  const [workspace, setWorkspace] = useState<ChatSessionsResponse | null>(null);
  const [error, setError] = useState("");
  const generation = useRef(0);

  const load = useCallback(async () => {
    const current = ++generation.current;
    try {
      const response = await api.getChatSessions();
      if (current !== generation.current) return;
      setWorkspace(response);
      setError("");
    } catch (reason) {
      if (current !== generation.current) return;
      setError(reason instanceof Error ? translateKnown(reason.message) : t("Chat session request failed."));
    }
  }, []);

  useEffect(() => {
    void load();
    return () => {
      generation.current += 1;
    };
  }, [activeID, load]);

  const session = workspace?.sessions.find((entry) => entry.id === workspace.active_session_id);
  return (
    <aside className="video-chat-side" aria-label={t("Chat beside the video")}>
      <header className="video-chat-head">
        <div className="video-chat-title">
          <strong>{session?.persona_name || t("Chat")}</strong>
          {session && <span>{session.title}</span>}
        </div>
        <a className="btn btn-secondary compact-command" href="#/chat">{t("Open Chat")}</a>
        <button type="button" className="icon-button" aria-label={t("Close chat")} title={t("Close chat")} onClick={onClose}><CloseIcon size={16} /></button>
      </header>
      <p className="video-chat-note">{motionSourceNote(source)}</p>
      {error ? (
        <div className="video-chat-state" role="alert">
          <span>{error}</span>
          <button type="button" className="btn btn-secondary compact-command" onClick={() => void load()}>{t("Retry")}</button>
        </div>
      ) : session ? (
        <ChatPanel key={session.id} sessionId={session.id} personaName={session.persona_name} motionOwner={chatMotionOwner(source)} onSessionChanged={() => void load()} />
      ) : (
        <div className="video-chat-state" role="status">{t("Loading chats...")}</div>
      )}
    </aside>
  );
}
