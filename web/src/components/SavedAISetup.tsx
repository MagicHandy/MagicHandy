import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { PublicSettings } from "../api/types";
import type { ModelConnection } from "../api/cloud-types";
import { t } from "../i18n";

// Returning users can keep combinations that Easy's two simple splits cannot
// describe. Preserve all roles and policies until they explicitly choose a card.
export function SavedAISetup({ settings, locked, onReady, editCustom }: {
  settings: PublicSettings["llm"]; locked: boolean; onReady: (ready: boolean) => void; editCustom: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const mounted = useRef(true);
  const readyRef = useRef(onReady); readyRef.current = onReady;
  const signature = JSON.stringify(settings);
  const signatureRef = useRef(signature); signatureRef.current = signature;
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  useEffect(() => { setMessage(""); readyRef.current(false); }, [signature]);
  const conversation = settings.connections?.find(connection => connection.id === settings.conversation_connection_id);
  const planner = settings.motion_planner;
  const motion = planner?.provider === "connection" ? settings.connections?.find(connection => connection.id === planner.connection_id) : planner?.provider === "conversation" ? conversation : undefined;
  async function test() {
    if (locked || busy) return;
    setBusy(true); setMessage("");
    try {
      const snapshot = signature;
      const targets = new Map<string, ModelConnection>();
      if (conversation) targets.set(conversation.id, conversation);
      if (motion && settings.motion_generation_mode !== "off") targets.set(motion.id, motion);
      for (const connection of targets.values()) {
        const result = await api.modelConnectionTest(connection);
        if (!result.ready) throw new Error(result.message || t("Request failed"));
      }
      if (settings.motion_generation_mode !== "off" && (planner?.provider === "decisions" || planner?.provider === "chatgpt")) {
        const result = await api.cloudTest(planner.provider, planner.model);
        if (!result.readiness.ready) throw new Error(result.readiness.message || t("Request failed"));
      }
      if (mounted.current && snapshot === signatureRef.current) { setMessage(t("Ready")); readyRef.current(true); }
    } catch (error) {
      if (mounted.current) { setMessage(error instanceof Error ? error.message : t("Request failed")); readyRef.current(false); }
    } finally { if (mounted.current) setBusy(false); }
  }
  const chatModel = conversation ? `${conversation.name} · ${conversation.model}` : settings.model;
  const autopilotModel = motion ? `${motion.name} · ${motion.model}` : planner?.provider === "chatgpt" ? `ChatGPT · ${planner.model}` : planner?.provider === "decisions" ? t("Decisions library selection") : settings.model;
  return <div className="saved-ai-setup">
    <h3>{t("Keep your current AI setup")}</h3>
    <p className="hint-block">{t("Your saved model roles and context sharing are kept. Choosing an AI option above replaces these assignments; Custom setup can edit them individually.")}</p>
    <dl className="setup-summary-rows">
      <div><dt>{t("Chat model")}</dt><dd>{chatModel}</dd></div>
      <div><dt>{t("Autopilot model")}</dt><dd>{autopilotModel}</dd></div>
      <div><dt>{t("Autopilot context sharing")}</dt><dd>{planner?.context_policy === "technical" ? t("Motion details only") : t("Conversation context")}</dd></div>
    </dl>
    <div className="button-row">
      <button type="button" className="btn btn-secondary" disabled={locked || busy} onClick={() => void test()}>{t("Test saved AI setup")}</button>
      <button type="button" className="btn btn-quiet" disabled={locked || busy} onClick={editCustom}>{t("Edit in Custom setup")}</button>
    </div>
    {message && <p className="form-status" role="status">{message}</p>}
  </div>;
}
