import { DismissibleNotice } from "./DismissibleNotice";
import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { api } from "../api/client";
import type { ModelConnection, MotionPlannerSettings } from "../api/cloud-types";
import type { PublicSettings } from "../api/types";
import { t, translateKnown } from "../i18n";
import { modelSettingsPatch, modelSettingsSignature, modelConnectionLabel, modelProviderNames, newModelConnection } from "../util/model-connections";
import { HostedModelConnection } from "./HostedModelConnection";
import { DecisionsConnection } from "./DecisionsConnection";
import { ModelRoutingSummary } from "./ModelRoutingSummary";

const defaultPlanner: MotionPlannerSettings = { provider: "conversation", model: "", context_policy: "conversation" };

function ConnectionOptions({ connections, local = true }: { connections: ModelConnection[]; local?: boolean }) {
  return <>
    {local && <option value="local">{t("Local model")}</option>}
    {Object.entries(modelProviderNames).map(([provider, name]) => {
      const matches = connections.filter(connection => connection.provider === provider);
      return matches.length ? matches.map(connection => <option key={connection.id} value={connection.id}>{modelConnectionLabel(connection) + (connection.model ? "" : " · " + t("Choose a model"))}</option>)
        : <option key={provider} value={`new:${provider}`} disabled={connections.length >= 16}>{translateKnown(name)}</option>;
    })}
  </>;
}

export function ModelConnectionsPanel({ settings, saved, routing, locked, patch, localEditor, setup = false }: {
  settings: PublicSettings["llm"]; saved: PublicSettings["llm"] | undefined; routing?: PublicSettings["model_routing"];
  locked: boolean; patch: (next: Partial<PublicSettings["llm"]>) => void; localEditor?: ReactNode; setup?: boolean;
}) {
  const connections = settings.connections ?? [];
  const planner = settings.motion_planner ?? defaultPlanner;
  const conversationID = settings.conversation_connection_id || "local";
  const conversation = connections.find(connection => connection.id === conversationID);
  const plannerID = planner.provider === "connection" ? planner.connection_id : planner.provider === "chatgpt" ? "legacy-chatgpt" : planner.provider;
  const planningConnection = planner.provider === "connection" ? connections.find(connection => connection.id === planner.connection_id) : undefined;
  const plannerHosted = planner.provider === "decisions" || planner.provider === "chatgpt" || (planner.provider === "conversation" ? conversationID !== "local" : Boolean(planningConnection));
  const [managedID, setManagedID] = useState("");
  const [editRetryModel, setEditRetryModel] = useState(false);
  const managed = connections.find(item => item.id === managedID);
  const [decisionsReady, setDecisionsReady] = useState(false);
  const decisionsRevision = useRef(0);
  const decisionsChanged = useCallback((ready: boolean) => { decisionsRevision.current++; setDecisionsReady(ready); }, []);
  useEffect(() => {
    if (!connections.length && planner.provider !== "decisions") return;
    let current = true;
    const revision = decisionsRevision.current;
    void api.cloudPlanningStatus().then(next => { if (current && revision === decisionsRevision.current) setDecisionsReady(next.connection.decisions_key_set); }).catch(() => {});
    return () => { current = false; };
  }, [connections.length, planner.provider]);

  function chooseConnection(value: string, role: "chat" | "autopilot") {
    let nextConnections = connections;
    let id = value;
    if (value.startsWith("new:")) {
      if (connections.length >= 16) return;
      const connection = newModelConnection(value.slice(4) as ModelConnection["provider"], connections);
      nextConnections = [...connections, connection]; id = connection.id;
    }
    const nextChat = role === "chat" ? id : conversationID;
    const nextPlanner = role === "autopilot" ? { ...planner, provider: ["conversation", "local", "decisions"].includes(id) ? id : "connection", connection_id: ["conversation", "local", "decisions"].includes(id) ? "" : id } as MotionPlannerSettings : planner;
    if (role === "autopilot" && id !== nextChat && !["conversation", "local", "decisions"].includes(id)) nextPlanner.context_policy = "technical";
    patch({ connections: nextConnections, conversation_connection_id: nextChat, motion_planner: nextPlanner });
  }
  function updateConnection(connection: ModelConnection, change: Partial<ModelConnection>) {
    patch({ connections: connections.map(item => item.id === connection.id ? { ...item, ...change } : item) });
  }
  function editor(connection: ModelConnection) {
    return <HostedModelConnection key={connection.id} connection={connection} locked={locked} patch={change => updateConnection(connection, change)} />;
  }
  const draftChanged = Boolean(saved && modelSettingsSignature(saved) !== modelSettingsSignature(settings));
  function addConnection(provider: ModelConnection["provider"]) {
    const connection = newModelConnection(provider, connections);
    patch({ connections: [...connections, connection] }); setManagedID(connection.id);
  }


  return <>
    <ModelRoutingSummary routing={routing} />
    {draftChanged && !setup && <div className="model-routing-draft"><p className="hint" role="status">{t("Unsaved model changes. Chat still uses the saved routing above until you save settings.")}</p><button type="button" className="btn btn-secondary" disabled={locked} onClick={() => { if (saved) patch(modelSettingsPatch(saved)); setManagedID(""); }}>{t("Discard model changes")}</button><p className="hint">{t("Saved accounts and API keys are not changed by Discard.")}</p></div>}
    <section className="model-role-section cloud-motion-settings" aria-label={t("Chat model settings")}>
      <h3 className="group-title">{t("Chat model")}</h3>
      <label className="field"><span className="visually-hidden">{t("Chat model")}</span><select value={conversationID} disabled={locked} onChange={event => chooseConnection(event.target.value, "chat")}>
        {conversationID !== "local" && !conversation && <option value={conversationID} disabled>{t("Unavailable connection")}</option>}
        <ConnectionOptions connections={connections} />
      </select></label>
      <DismissibleNotice id="model-chat-role" className="model-help"><p className="hint">{t("This model receives your chat messages and handles any motion requests made in chat.")}</p></DismissibleNotice>
      {conversation && (planner.provider === "conversation" || planningConnection?.id === conversationID) && <p className="hint">{t("This connection is also used by Autopilot.")}</p>}
      {conversation ? <>
        {editor(conversation)}
        {!setup && <><label className="toggle-line"><span className="toggle"><input type="checkbox" disabled={locked} checked={Boolean(settings.retry_refusal_locally)} onChange={event => patch({ retry_refusal_locally: event.target.checked })} /><span className="track" aria-hidden="true" /></span><span>{t("Retry declined chat requests with the local model")}</span></label>
        {settings.retry_refusal_locally && <>
          <DismissibleNotice id="model-local-retry" className="model-help"><p className="hint">{t("One local attempt after an explicit provider refusal, using your local prompt and the original conversation. Errors, incomplete output and refusal wording in an otherwise valid reply do not trigger a retry. Autopilot is unchanged.")}</p><p className="hint">{t("The local endpoint must be available. A cold model may take longer to load. No model is downloaded automatically.")}</p></DismissibleNotice>
          <p className="hint">{t("Local retry model: {provider} · {model}", { provider: settings.provider === "ollama" ? "Ollama" : "llama.cpp", model: settings.model || t("Choose a model") })}</p>
          {localEditor && (planner.provider === "local" || planner.connection_id === "local" ? <p className="hint">{t("Local model configuration is shared with Autopilot below.")}</p> : <details onToggle={event => setEditRetryModel(event.currentTarget.open)}><summary>{t("Configure local retry model")}</summary>{editRetryModel && localEditor}</details>)}

        </>}</>}
        <DismissibleNotice id="model-chat-context" className="model-help"><details><summary>{t("Chat context and refusals")}</summary><p className="hint">{setup ? t("Included chat messages, enabled personas, and memories are sent to your selected provider. You can choose an optional local backup in the next step.") : t("Included messages, enabled persona and memories are sent as written. Provider content rules still apply. A refusal stays with the selected provider unless you enable the local retry above.")}</p></details></DismissibleNotice>
      </> : conversationID === "local" ? localEditor : <p role="alert">{t("The selected connection is unavailable. Choose a model before saving.")}</p>}
    </section>
    <section className="model-role-section cloud-motion-settings" aria-label={t("Autopilot model settings")}>
      <h3 className="group-title">{t("Autopilot model")}</h3>
      <DismissibleNotice id="model-autopilot-role" className="model-help"><p className="hint">{t("Autopilot plans motion between messages. Choosing its model does not change where chat messages go.")}</p></DismissibleNotice>
      <label className="field"><span className="visually-hidden">{t("Autopilot model")}</span><select value={plannerID || "conversation"} disabled={locked} onChange={event => chooseConnection(event.target.value, "autopilot")}>
        <option value="conversation">{t("Same as conversation")}</option>
        {planner.provider === "connection" && planner.connection_id !== "local" && !planningConnection && <option value={planner.connection_id} disabled>{t("Unavailable connection")}</option>}
        <ConnectionOptions connections={connections} />
        {planner.provider === "chatgpt" && <option value="legacy-chatgpt" disabled>{t("ChatGPT") + " · " + planner.model}</option>}
        <option value="decisions" disabled={!decisionsReady && planner.provider !== "decisions"}>{t("Decisions library selection")}</option>
      </select></label>
      {plannerHosted && <>{planner.provider !== "decisions" && <label className="field"><span className="label">{t("Autopilot context sharing")}</span><select value={planner.context_policy ?? "conversation"} disabled={locked} onChange={event => patch({ motion_planner: { ...planner, context_policy: event.target.value as "conversation" | "technical" } })}>
        <option value="conversation">{t("Conversation context")}</option><option value="technical">{t("Motion details only")}</option>
      </select></label>}
      <DismissibleNotice id="model-autopilot-context" className="model-help"><p className="hint">{planner.context_policy === "technical" || planner.provider === "decisions" ? t("Only semantic motion, limits and recent motion state are shared. Chat messages, personas and memories are excluded; this planner does not interpret the conversation.") : t("Included chat messages, enabled persona and memories also go to the Autopilot model.")}</p></DismissibleNotice></>}
      {planningConnection && planningConnection.id !== conversationID && editor(planningConnection)}
      {(planner.provider === "local" || planner.connection_id === "local") && conversationID !== "local" && localEditor}
    </section>
    <details className="group cloud-motion-settings"><summary>{t("Named connections")}</summary>
      <DismissibleNotice id="model-named-connections" className="model-help"><p className="hint">{t("Add another configuration for a provider, or rename an existing connection. The provider identity always stays visible in model choices.")}</p></DismissibleNotice>
      {connections.map(connection => <label className="field" key={connection.id}><span className="label">{translateKnown(modelProviderNames[connection.provider]) + " · " + connection.id}</span><input aria-label={t("Connection name") + " · " + connection.id} disabled={locked} value={connection.name} onChange={event => updateConnection(connection, { name: event.target.value })} /></label>)}
      <label className="field"><span className="label">{t("Configure named connection")}</span><select value={managedID} disabled={locked} onChange={event => setManagedID(event.target.value)}><option value="">{t("Choose a connection")}</option>{connections.map(connection => <option key={connection.id} value={connection.id}>{modelConnectionLabel(connection)}</option>)}</select></label>
      {managed && managed.id !== conversationID && managed.id !== planningConnection?.id ? editor(managed) : managed && <p className="hint">{t("This connection is configured in its model section above.")}</p>}
      <div className="button-row">{Object.keys(modelProviderNames).map(provider => <button type="button" className="btn btn-secondary" key={provider} disabled={locked || connections.length >= 16} onClick={() => addConnection(provider as ModelConnection["provider"])}>{t("Add {provider}", { provider: translateKnown(modelProviderNames[provider as ModelConnection["provider"]]) })}</button>)}</div>
      <DismissibleNotice id="model-save-guidance" className="model-help"><p className="hint">{setup ? t("Accounts and keys are saved immediately. Model choices apply when you continue.") : t("Model choices apply when you save settings. Accounts and keys are saved immediately.")}</p></DismissibleNotice>
    </details>
    <DecisionsConnection locked={locked} onReadyChange={decisionsChanged} />
  </>;
}
