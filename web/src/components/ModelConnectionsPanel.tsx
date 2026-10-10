import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { api } from "../api/client";
import type { CloudPlanningStatus, ModelConnection, ModelRoute, ModelRoutingSnapshot, MotionPlannerSettings } from "../api/cloud-types";
import type { PublicSettings } from "../api/types";
import { t, translateKnown } from "../i18n";
import { connectionRoles, connectionSignature, modelConnectionLabel, modelProviderNames, modelSettingsPatch, modelSettingsSignature, newModelConnection, removeConnection, type ModelRole } from "../util/model-connections";
import { DecisionsConnection } from "./DecisionsConnection";
import { ModelConnectionEditor } from "./ModelConnectionEditor";
import { FieldRow } from "./SetupSection";

const defaultPlanner: MotionPlannerSettings = { provider: "conversation", model: "", context_policy: "conversation" };
const MAX_CONNECTIONS = 16;
type Provider = ModelConnection["provider"];

// Role choices list only connections that exist. Adding a provider happens in
// External providers, so picking a role never creates one.
function ConnectionOptions({ connections }: { connections: ModelConnection[] }) {
  return <>
    <option value="local">{t("Local model")}</option>
    {connections.map(connection => <option key={connection.id} value={connection.id}>{modelConnectionLabel(connection) + (connection.model ? "" : " · " + t("Choose a model"))}</option>)}
  </>;
}

function providerLabel(route: ModelRoute): string {
  return route.provider === "ollama" ? "Ollama" : route.provider === "llama_cpp" ? "llama.cpp" : translateKnown(modelProviderNames[route.provider as keyof typeof modelProviderNames] ?? route.provider);
}

// What the saved settings do, from the backend's resolver. A draft never
// changes this text until it is saved.
function chatRouteText(routing?: ModelRoutingSnapshot): string {
  if (!routing) return t("Loading saved model routing…");
  if (routing.chat.state === "unavailable") return t("Chat cannot use this connection. Choose another model and save settings.");
  return routing.chat.kind === "local" ? t("Chat messages go to your configured local-model endpoint.") : t("Chat messages go to {provider}, including enabled persona and memory context.", { provider: providerLabel(routing.chat) });
}

function autopilotRouteText(routing?: ModelRoutingSnapshot): string {
  if (!routing || routing.autopilot.kind === "local" || routing.autopilot.state === "unavailable") return t("Autopilot plans motion between messages. Choosing its model does not change where chat messages go.");
  return routing.autopilot.context_policy === "technical"
    ? t("Autopilot receives motion details only. Chat messages, personas and memories are excluded from its requests.")
    : t("Autopilot also receives included conversation context.");
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
  const [status, setStatus] = useState<CloudPlanningStatus | null>(null);
  const [editRetryModel, setEditRetryModel] = useState(false);
  const statusRevision = useRef(0);
  const refreshStatus = useCallback(() => {
    const revision = ++statusRevision.current;
    void api.cloudPlanningStatus().then(next => { if (revision === statusRevision.current) setStatus(next); }).catch(() => {});
  }, []);
  // A local-only install has nothing hosted to report, so it makes no cloud call.
  const hostedInUse = connections.length > 0 || planner.provider === "decisions";
  useEffect(() => { if (hostedInUse) refreshStatus(); }, [hostedInUse, connections.length, refreshStatus]);
  const decisionsReady = Boolean(status?.connection.decisions_key_set);

  function chooseConnection(id: string, role: ModelRole) {
    const nextChat = role === "chat" ? id : conversationID;
    const special = ["conversation", "local", "decisions"].includes(id);
    const nextPlanner = role === "autopilot" ? { ...planner, provider: special ? id : "connection", connection_id: special ? "" : id } as MotionPlannerSettings : planner;
    if (role === "autopilot" && id !== nextChat && !special) nextPlanner.context_policy = "technical";
    patch({ conversation_connection_id: nextChat, motion_planner: nextPlanner });
  }
  const draftChanged = Boolean(saved && modelSettingsSignature(saved) !== modelSettingsSignature(settings));
  const usesLocal = conversationID === "local" || planner.provider === "local" || planner.connection_id === "local";

  return <>
    {draftChanged && !setup && <div className="model-routing-draft"><p className="hint" role="status">{t("Unsaved model changes. Chat still uses the saved routing until you save settings.")}</p><button type="button" className="btn btn-secondary" disabled={locked} onClick={() => { if (saved) patch(modelSettingsPatch(saved)); }}>{t("Discard model changes")}</button><p className="hint">{t("Saved accounts and API keys are not changed by Discard.")}</p></div>}
    <section className="group cloud-motion-settings model-roles" aria-label={t("Model roles")}>
      <h3 className="group-title">{t("Model roles")}</h3>
      <div className="form-rows">
        <FieldRow id="model-role-chat" label={t("Chat model")} hint={setup ? t("This model receives your chat messages and handles any motion requests made in chat.") : chatRouteText(routing)}>
          <select id="model-role-chat" aria-describedby="model-role-chat-hint" value={conversationID} disabled={locked} onChange={event => chooseConnection(event.target.value, "chat")}>
            {conversationID !== "local" && !conversation && <option value={conversationID} disabled>{t("Unavailable connection")}</option>}
            <ConnectionOptions connections={connections} />
          </select>
        </FieldRow>
        <FieldRow id="model-role-autopilot" label={t("Autopilot model")} hint={autopilotRouteText(setup ? undefined : routing)}>
          <select id="model-role-autopilot" aria-describedby="model-role-autopilot-hint" value={plannerID || "conversation"} disabled={locked} onChange={event => chooseConnection(event.target.value, "autopilot")}>
            <option value="conversation">{t("Same as conversation")}</option>
            {planner.provider === "connection" && planner.connection_id !== "local" && !planningConnection && <option value={planner.connection_id} disabled>{t("Unavailable connection")}</option>}
            <ConnectionOptions connections={connections} />
            {planner.provider === "chatgpt" && <option value="legacy-chatgpt" disabled>{t("ChatGPT") + " · " + planner.model}</option>}
            <option value="decisions" disabled={!decisionsReady && planner.provider !== "decisions"}>{t("Decisions library selection")}</option>
          </select>
        </FieldRow>
        {plannerHosted && planner.provider !== "decisions" && <FieldRow id="model-role-context" label={t("Autopilot context sharing")} hint={planner.context_policy === "technical" ? t("Only semantic motion, limits and recent motion state are shared. Chat messages, personas and memories are excluded; this planner does not interpret the conversation.") : t("Included chat messages, enabled persona and memories also go to the Autopilot model.")}>
          <select id="model-role-context" aria-describedby="model-role-context-hint" value={planner.context_policy ?? "conversation"} disabled={locked} onChange={event => patch({ motion_planner: { ...planner, context_policy: event.target.value as "conversation" | "technical" } })}>
            <option value="conversation">{t("Conversation context")}</option><option value="technical">{t("Motion details only")}</option>
          </select>
        </FieldRow>}
        {conversation && !setup && <label className="toggle-line form-row"><span className="toggle"><input type="checkbox" disabled={locked} checked={Boolean(settings.retry_refusal_locally)} onChange={event => patch({ retry_refusal_locally: event.target.checked })} /><span className="track" aria-hidden="true" /></span><span>{t("Retry declined chat requests with the local model")}<small>{settings.retry_refusal_locally
          ? t("Local retry model: {provider} · {model}", { provider: settings.provider === "ollama" ? "Ollama" : "llama.cpp", model: settings.model || t("Choose a model") })
          : t("One local attempt after an explicit provider refusal, using your local prompt and the original conversation. Errors, incomplete output and refusal wording in an otherwise valid reply do not trigger a retry. Autopilot is unchanged.")}</small></span></label>}
        {conversation && !setup && settings.retry_refusal_locally && localEditor && !usesLocal && <details className="form-row-details" onToggle={event => setEditRetryModel(event.currentTarget.open)}><summary>{t("Configure local retry model")}</summary>{editRetryModel && localEditor}</details>}
      </div>
    </section>
    <ExternalProviders settings={settings} status={status} locked={locked} setup={setup} patch={patch} refreshStatus={refreshStatus} />
    {usesLocal && localEditor}
  </>;
}

// One list of every provider that isn't the built-in local model: its status,
// the roles that use it, and Edit and Remove. The open row's editor sits
// directly under it.
function ExternalProviders({ settings, status, locked, setup, patch, refreshStatus }: {
  settings: PublicSettings["llm"]; status: CloudPlanningStatus | null; locked: boolean; setup: boolean;
  patch: (next: Partial<PublicSettings["llm"]>) => void; refreshStatus: () => void;
}) {
  const connections = settings.connections ?? [];
  const [openID, setOpenID] = useState("");
  const [confirmID, setConfirmID] = useState("");
  const [adding, setAdding] = useState(false);
  const [decisionsBusy, setDecisionsBusy] = useState(false);
  const decisionsSet = Boolean(status?.connection.decisions_key_set);
  const planner = settings.motion_planner ?? defaultPlanner;
  const showDecisions = decisionsSet || openID === "decisions";
  const full = connections.length >= MAX_CONNECTIONS;

  function toggle(id: string) { setConfirmID(""); setOpenID(current => current === id ? "" : id); if (openID === id) refreshStatus(); }
  function add(provider: Provider | "decisions") {
    setAdding(false); setConfirmID("");
    if (provider === "decisions") { setOpenID("decisions"); return; }
    if (full) return;
    const connection = newModelConnection(provider, connections);
    const change: Partial<PublicSettings["llm"]> = { connections: [...connections, connection] };
    // Setup's Hosted model choice means chat should use the first provider added.
    if (setup && (settings.conversation_connection_id || "local") === "local") change.conversation_connection_id = connection.id;
    patch(change); setOpenID(connection.id);
  }
  function remove(id: string) {
    setConfirmID(""); if (openID === id) setOpenID("");
    patch(removeConnection(settings, id));
  }
  async function removeDecisions() {
    setConfirmID(""); setDecisionsBusy(true);
    try {
      await api.cloudDecisionsKey("");
      if (planner.provider === "decisions") patch({ motion_planner: { ...planner, provider: "conversation", connection_id: "", context_policy: "conversation" } });
      if (openID === "decisions") setOpenID("");
    } finally { setDecisionsBusy(false); refreshStatus(); }
  }
  const roleNames = (roles: ModelRole[]) => roles.map(role => role === "chat" ? t("Chat") : t("Autopilot"));

  return <section className="group cloud-motion-settings external-providers" aria-label={t("External providers")}>
    <h3 className="group-title">{t("External providers")}<span className="hint-inline">{t("{count} of {max}", { count: connections.length, max: MAX_CONNECTIONS })}</span></h3>
    {connections.length > 0 || showDecisions ? <div className="provider-list">
      {connections.map(connection => {
        const roles = connectionRoles(settings, connection.id);
        const open = openID === connection.id;
        return <ProviderItem key={connection.id}
          title={connection.name.trim() || translateKnown(modelProviderNames[connection.provider])}
          provider={translateKnown(modelProviderNames[connection.provider]) + (connection.provider === "compatible" ? " · " + connection.base_url.replace(/^https?:\/\//, "").replace(/\/v1\/?$/, "") : "")}
          state={connectionState(connection, status)}
          roles={roleNames(roles)}
          open={open} locked={locked}
          onToggle={() => toggle(connection.id)}
          onRemove={() => { setOpenID(""); setConfirmID(connection.id); }}
          confirm={confirmID === connection.id && <RemoveConfirm
            text={roles.length ? t("{roles} will switch to the local model. The saved API key is deleted when you save settings.", { roles: roleNames(roles).join(" · ") }) : t("Its saved API key is deleted when you save settings.")}
            name={connection.name.trim() || translateKnown(modelProviderNames[connection.provider])}
            onCancel={() => setConfirmID("")} onConfirm={() => remove(connection.id)} />}>
          {open && <ModelConnectionEditor key={connection.id} full={!setup} connection={connection} locked={locked} patch={change => patch({ connections: connections.map(item => item.id === connection.id ? { ...item, ...change } : item) })} />}
        </ProviderItem>;
      })}
      {showDecisions && <ProviderItem
        title={t("Decisions API")} provider={t("OpenAI API · library selection")}
        state={decisionsSet ? { tone: "ok", text: t("Key saved") } : { tone: "idle", text: t("API key needed") }}
        roles={planner.provider === "decisions" ? [t("Autopilot")] : []}
        open={openID === "decisions"} locked={locked || decisionsBusy}
        onToggle={() => toggle("decisions")}
        onRemove={decisionsSet ? () => { setOpenID(""); setConfirmID("decisions"); } : undefined}
        confirm={confirmID === "decisions" && <RemoveConfirm name={t("Decisions API")}
          text={planner.provider === "decisions" ? t("Autopilot will switch to the chat model. The Decisions API key is deleted now.") : t("The Decisions API key is deleted now.")}
          onCancel={() => setConfirmID("")} onConfirm={() => void removeDecisions()} />}>
        {openID === "decisions" && <DecisionsConnection locked={locked} onChange={refreshStatus} />}
      </ProviderItem>}
    </div> : <p className="hint">{t("No external providers yet. Add one to use a hosted model or your own compatible server.")}</p>}
    <div className="provider-add">
      <button type="button" className="btn btn-secondary" aria-expanded={adding} disabled={locked} onClick={() => { if (!adding) refreshStatus(); setAdding(value => !value); }}>{t("Add provider")}</button>
      <small className="hint">{setup ? t("Accounts and keys are saved immediately. Model choices apply when you continue.") : t("New providers start unassigned. Choose them in Model roles. Accounts and keys are saved immediately.")}</small>
    </div>
    {adding && <div className="provider-menu" role="group" aria-label={t("Add provider")}>
      {(Object.keys(modelProviderNames) as Provider[]).map(provider => <button key={provider} type="button" disabled={locked || full} onClick={() => add(provider)}>
        <strong>{translateKnown(modelProviderNames[provider])}</strong><small>{providerDetail(provider)}</small>
      </button>)}
      {!showDecisions && !setup && <button type="button" disabled={locked} onClick={() => add("decisions")}><strong>{t("Decisions API")}</strong><small>{t("Library pattern selection for Autopilot, with its own OpenAI key.")}</small></button>}
      {full && <p className="hint">{t("You can add up to {max} providers. Remove one to add another.", { max: MAX_CONNECTIONS })}</p>}
    </div>}
  </section>;
}

function providerDetail(provider: Provider): string {
  return provider === "chatgpt" ? t("Sign in and use your ChatGPT plan.")
    : provider === "openrouter" ? t("One API key for many hosted models.")
      : provider === "openai" ? t("Your OpenAI API key and billing.")
        : t("LM Studio, vLLM or another OpenAI-compatible server.");
}

type ProviderState = { tone: "ok" | "warn" | "idle"; text: string };

// Readiness counts only when the backend checked this exact configuration.
function connectionState(connection: ModelConnection, status: CloudPlanningStatus | null): ProviderState {
  if (!connection.model.trim()) return { tone: "warn", text: t("Choose a model") };
  const readiness = status?.connection_readiness?.[connection.id];
  if (readiness?.ready && readiness.connection && connectionSignature(readiness.connection) === connectionSignature(connection)) return { tone: "ok", text: connection.model + " · " + t("Ready") };
  if (connection.provider !== "chatgpt" && !connection.no_authentication && status && !status.connection_keys?.[connection.id]) return { tone: "warn", text: connection.model + " · " + t("API key needed") };
  return { tone: "idle", text: connection.model + " · " + t("Not checked") };
}

function ProviderItem({ title, provider, state, roles, open, locked, onToggle, onRemove, confirm, children }: {
  title: string; provider: string; state: ProviderState; roles: string[]; open: boolean; locked: boolean;
  onToggle: () => void; onRemove?: () => void; confirm: ReactNode; children: ReactNode;
}) {
  return <>
    <div className="provider-row" data-open={open || undefined}>
      <span className="provider-id">
        <strong>{title}<span className="provider-kind">{provider}</span></strong>
        <small><span className="provider-state"><span className="status-dot" data-state={state.tone === "ok" ? "ok" : state.tone === "warn" ? "warn" : "idle"} aria-hidden="true" />{state.text}</span>
          {roles.length > 0 ? <span className="provider-roles">{t("Used by")} {roles.map(role => <span key={role} className="setup-badge">{role}</span>)}</span> : <span>{t("Not used")}</span>}</small>
      </span>
      <span className="provider-actions">
        <button type="button" className="btn btn-quiet" aria-expanded={open} aria-label={(open ? t("Close") : t("Edit")) + " · " + title} onClick={onToggle}>{open ? t("Close") : t("Edit")}</button>
        {onRemove && <button type="button" className="btn btn-quiet" aria-label={t("Remove") + " · " + title} disabled={locked} onClick={onRemove}>{t("Remove")}</button>}
      </span>
    </div>
    {confirm}
    {open && <div className="provider-editor">{children}</div>}
  </>;
}

function RemoveConfirm({ name, text, onCancel, onConfirm }: { name: string; text: string; onCancel: () => void; onConfirm: () => void }) {
  return <div className="provider-confirm" role="alertdialog" aria-label={t("Remove {name}?", { name })}>
    <p><strong>{t("Remove {name}?", { name })}</strong> {text}</p>
    <span className="provider-actions">
      <button type="button" className="btn btn-quiet" onClick={onCancel}>{t("Cancel")}</button>
      <button type="button" className="btn btn-danger-outline" onClick={onConfirm}>{t("Remove")}</button>
    </span>
  </div>;
}
