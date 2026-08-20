import { t, translateKnown, type MessageKey } from "../i18n";
import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { ChatSession, ChatSessionsResponse } from "../api/types";
import { AutopilotControl } from "../components/AutopilotControl";
import { ChatPanel } from "../components/ChatPanel";
import { ChatSessionDialog } from "../components/ChatSessionDialog";
import { ChatTabs } from "../components/ChatTabs";
import { MotionVisualizer } from "../components/MotionVisualizer";
import { QuickSettings } from "../components/QuickSettings";
import { SegmentedChoice } from "../components/SetpointControls";
import { VoiceQuickControls } from "../components/VoiceQuickControls";
import { useAppState, useToast } from "../state/app-state";

type PendingChange = { action: "new" } | { action: "switch"; target: ChatSession };

const errorMessage = (error: unknown) => error instanceof Error ? translateKnown(error.message) : t("Chat session request failed.");

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
  const [changingMotionMode, setChangingMotionMode] = useState(false);
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
  const backendChat = state?.chat;
  const assistantMood = active && backendChat?.active_session_id === active.id
    ? backendChat.current_mood?.trim() ?? ""
    : "";
  const autopilotActive = state?.modes?.mode === "autopilot" || state?.modes?.active_mode === "autopilot";
  const locked = !backendOnline || readOnly || loading || operation || streaming;
  const savedMotionMode = state?.settings?.llm?.motion_generation_mode;
  const motionCapabilities = state?.settings?.llm?.motion_capabilities;
  const motionMode = savedMotionMode === "dynamic" || savedMotionMode === "pattern" || savedMotionMode === "off"
    ? savedMotionMode
    : motionCapabilities?.motion === false ? "off" : "pattern";

  useEffect(() => {
    if (!workspace || loading || operation || streaming) return;
    const backendActiveID = state?.chat?.active_session_id ?? "";
    const backendLatestSeq = state?.chat?.latest_seq ?? 0;
    const selectionChanged = Boolean(backendActiveID && backendActiveID !== workspace.active_session_id);
    const summaryBehind = backendActiveID === workspace.active_session_id && backendLatestSeq > (active?.latest_seq ?? 0);
    if (selectionChanged || summaryBehind) void loadSessions();
  }, [active?.latest_seq, loadSessions, loading, operation, state?.chat?.active_session_id, state?.chat?.latest_seq, state?.uptime_seconds, streaming, workspace?.active_session_id]);

  async function applyWorkspace(action: () => Promise<ChatSessionsResponse>, success?: MessageKey) {
    if (operation) return;
    loadGeneration.current += 1;
    setOperation(true);
    try {
      const response = await action();
      if (mounted.current) setWorkspace(response);
      if (success) show(translateKnown(success));
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
    if (session.active || locked || !window.confirm(t("Delete {title}? This cannot be undone.", { title: session.title }))) return;
    void applyWorkspace(() => api.deleteChatSession(session.id), "Chat deleted.");
  }

  async function changeMotionMode(mode: "dynamic" | "pattern" | "off") {
    if (mode === motionMode || changingMotionMode || !backendOnline || readOnly) return;
    setChangingMotionMode(true);
    try {
      await api.setLLMMotionMode(mode);
      await refresh();
      show(t("LLM motion mode changed."));
    } catch (error) {
      show(error instanceof Error ? translateKnown(error.message) : t("Request failed"), "error");
    } finally {
      if (mounted.current) setChangingMotionMode(false);
    }
  }

  return (
    <div className="chat-route">
      <div className="chat-workbench">
        <section className="chat-conversation" aria-label={t("Conversation")}>
          <ChatTabs
            sessions={sessions}
            activeId={active?.id ?? ""}
            disabled={locked}
            personaDisabled={locked || autopilotActive}
            onActivate={requestActivate}
            onNew={requestNew}
            onSave={saveSession}
            onDelete={deleteSession}
            assistantMood={assistantMood}
          />
          {loadError ? (
            <div className="chat-session-state" role="alert">
              <strong>{t("Chat tabs unavailable")}</strong>
              <span>{loadError}</span>
              <button type="button" className="btn btn-secondary" onClick={() => void loadSessions()}>{t("Retry")}</button>
            </div>
          ) : active ? (
            <ChatPanel
              key={active.id}
              sessionId={active.id}
              onBusyChange={setStreaming}
              onSessionChanged={loadSessions}
            />
          ) : (
            <div className="chat-session-state" role="status">{loading ? t("Loading chats...") : t("No active chat.")}</div>
          )}
        </section>

        <aside className="chat-sidebar" aria-label={t("Motion controls")}>
          <div className="chat-sidebar-controls">
            <h2 className="section-title">{t("Controls")}</h2>
            <SegmentedChoice
              className="chat-motion-mode"
              label={t("LLM motion")}
              value={motionMode}
              options={[
                { value: "dynamic", label: t("Creative") },
                { value: "pattern", label: t("Pattern library") },
                { value: "off", label: t("Off") },
              ]}
              disabled={!backendOnline || readOnly || changingMotionMode}
              onChange={(mode) => void changeMotionMode(mode)}
            />
            <AutopilotControl />
            <VoiceQuickControls />
            <div className="divider" />
            <h2 className="section-title">{t("Motion style")}</h2>
            <QuickSettings section="style" />
          </div>
          <div className="chat-motion-status">
            <h3 className="group-title">{t("Motion status")}</h3>
            <MotionVisualizer motion={motion} />
          </div>
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
