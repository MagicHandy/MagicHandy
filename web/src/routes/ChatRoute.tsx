import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { ChatSession, ChatSessionsResponse } from "../api/types";
import { AutopilotControl } from "../components/AutopilotControl";
import { ChatPanel } from "../components/ChatPanel";
import { ChatSessionDialog } from "../components/ChatSessionDialog";
import { ChatTabs } from "../components/ChatTabs";
import { MotionVisualizer } from "../components/MotionVisualizer";
import { QuickSettings } from "../components/QuickSettings";
import { VoiceQuickControls } from "../components/VoiceQuickControls";
import { WorkspaceHead } from "../components/WorkspaceHead";
import { useAppState, useToast } from "../state/app-state";

type PendingChange = { action: "new" } | { action: "switch"; target: ChatSession };

const errorMessage = (error: unknown) => error instanceof Error ? error.message : "Chat session request failed.";

export function ChatRoute() {
  const { backendOnline, readOnly, state, motion, refresh } = useAppState();
  const { show } = useToast();
  const mounted = useRef(true);
  const loadGeneration = useRef(0);
  const [workspace, setWorkspace] = useState<ChatSessionsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [operation, setOperation] = useState(false);
  const [streaming, setStreaming] = useState(false);
  const [pendingChange, setPendingChange] = useState<PendingChange | null>(null);

  const loadSessions = useCallback(async () => {
    const generation = ++loadGeneration.current;
    setLoadError("");
    try {
      const response = await api.getChatSessions();
      if (mounted.current && generation === loadGeneration.current) setWorkspace(response);
    } catch (error) {
      if (mounted.current && generation === loadGeneration.current) setLoadError(errorMessage(error));
    } finally {
      if (mounted.current && generation === loadGeneration.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    mounted.current = true;
    void loadSessions();
    return () => {
      mounted.current = false;
      loadGeneration.current += 1;
    };
  }, [loadSessions]);

  const sessions = workspace?.sessions ?? [];
  const active = sessions.find((session) => session.id === workspace?.active_session_id);
  const autopilotActive = state?.modes?.mode === "autopilot" || state?.modes?.active_mode === "autopilot";
  const locked = !backendOnline || readOnly || loading || operation || streaming;

  useEffect(() => {
    if (!workspace || loading || operation || streaming) return;
    const backendActiveID = state?.chat?.active_session_id ?? "";
    const backendLatestSeq = state?.chat?.latest_seq ?? 0;
    const selectionChanged = Boolean(backendActiveID && backendActiveID !== workspace.active_session_id);
    const summaryBehind = backendActiveID === workspace.active_session_id && backendLatestSeq > (active?.latest_seq ?? 0);
    if (selectionChanged || summaryBehind) void loadSessions();
  }, [active?.latest_seq, loadSessions, loading, operation, state?.chat?.active_session_id, state?.chat?.latest_seq, state?.uptime_seconds, streaming, workspace?.active_session_id]);

  async function applyWorkspace(action: () => Promise<ChatSessionsResponse>, success?: string) {
    if (operation) return;
    loadGeneration.current += 1;
    setOperation(true);
    try {
      const response = await action();
      if (mounted.current) setWorkspace(response);
      if (success) show(success);
      refresh();
    } catch (error) {
      show(errorMessage(error), "error");
    } finally {
      if (mounted.current) setOperation(false);
    }
  }

  function requestNew() {
    if (!active || locked) return;
    setPendingChange({ action: "new" });
  }

  function requestActivate(session: ChatSession) {
    if (!active || session.id === active.id || locked) return;
    if (autopilotActive || !active.saved) {
      setPendingChange({ action: "switch", target: session });
      return;
    }
    void applyWorkspace(() => api.activateChatSession(session.id));
  }

  async function continueChange(saveCurrent: boolean) {
    if (!active || !pendingChange || operation) return;
    const change = pendingChange;
    loadGeneration.current += 1;
    setOperation(true);
    try {
      if (autopilotActive) await api.stopMode();
      if (saveCurrent && !active.saved) await api.saveChatSession(active.id);
      const response = change.action === "new"
        ? await api.createChatSession(!saveCurrent && !active.saved)
        : await api.activateChatSession(change.target.id, !saveCurrent && !active.saved);
      if (mounted.current) {
        setWorkspace(response);
        setPendingChange(null);
      }
      refresh();
    } catch (error) {
      show(errorMessage(error), "error");
    } finally {
      if (mounted.current) setOperation(false);
    }
  }

  function saveSession(session: ChatSession) {
    if (session.saved || locked) return;
    void applyWorkspace(() => api.saveChatSession(session.id), "Chat saved.");
  }

  function deleteSession(session: ChatSession) {
    if (session.active || locked || !window.confirm(`Delete ${session.title}? This cannot be undone.`)) return;
    void applyWorkspace(() => api.deleteChatSession(session.id), "Chat deleted.");
  }

  return (
    <div className="chat-route">
      <WorkspaceHead title="Chat" wide />
      <div className="chat-workbench">
        <section className="chat-conversation" aria-label="Conversation">
          {workspace && active && (
            <ChatTabs
              sessions={sessions}
              activeId={active.id}
              disabled={locked}
              onActivate={requestActivate}
              onNew={requestNew}
              onSave={saveSession}
              onDelete={deleteSession}
            />
          )}
          {loadError ? (
            <div className="chat-session-state" role="alert">
              <strong>Chat tabs unavailable</strong>
              <span>{loadError}</span>
              <button type="button" className="btn btn-secondary" onClick={() => void loadSessions()}>Retry</button>
            </div>
          ) : active ? (
            <ChatPanel
              key={active.id}
              sessionId={active.id}
              onBusyChange={setStreaming}
              onSessionChanged={loadSessions}
            />
          ) : (
            <div className="chat-session-state" role="status">{loading ? "Loading chats..." : "No active chat."}</div>
          )}
        </section>

        <aside className="chat-sidebar panel" aria-label="Motion controls">
          <h2 className="section-title">Controls</h2>
          <AutopilotControl />
          <div className="divider" />
          <VoiceQuickControls />
          <div className="divider" />
          <h2 className="section-title">Motion behavior</h2>
          <QuickSettings section="behavior" />
          <div className="divider" />
          <h3 className="group-title">Live motion</h3>
          <MotionVisualizer motion={motion} />
        </aside>
      </div>

      {pendingChange && active && (
        <ChatSessionDialog
          action={pendingChange.action}
          active={active}
          targetTitle={pendingChange.action === "switch" ? pendingChange.target.title : undefined}
          autopilotActive={autopilotActive}
          pending={operation}
          onCancel={() => setPendingChange(null)}
          onContinue={(saveCurrent) => void continueChange(saveCurrent)}
        />
      )}
    </div>
  );
}
