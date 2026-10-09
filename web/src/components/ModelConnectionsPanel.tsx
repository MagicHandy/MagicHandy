import { useEffect, useRef, useState, type ReactNode } from "react";
import { api } from "../api/client";
import type { CloudPlanningStatus, HostedModel, ModelConnection, MotionPlannerSettings } from "../api/cloud-types";
import type { PublicSettings } from "../api/types";
import { t, translateKnown } from "../i18n";
import { ChatGPTConnection } from "./ChatGPTConnection";
import { connectionSignature, newModelConnection } from "../util/model-connections";
import { ModelResponseSettings, modelReadyMessage, selectedModelChange } from "./ModelResponseSettings";

const providerNames = { chatgpt: "ChatGPT", openrouter: "OpenRouter", openai: "OpenAI API", compatible: "Other compatible provider" } as const;
const defaultPlanner: MotionPlannerSettings = { provider: "conversation", model: "", context_policy: "conversation" };

export function ModelConnectionsPanel({ settings, locked, patch, localEditor }: {
  settings: PublicSettings["llm"];
  locked: boolean;
  patch: (next: Partial<PublicSettings["llm"]>) => void;
  localEditor?: ReactNode;
}) {
  const connections = settings.connections ?? [];
  const planner = settings.motion_planner ?? defaultPlanner;
  const conversationID = settings.conversation_connection_id || "local";
  const [selectedID, setSelectedID] = useState(conversationID);
  const [newProvider, setNewProvider] = useState<ModelConnection["provider"]>("openrouter");
  const [key, setKey] = useState("");
  const [decisionsKey, setDecisionsKey] = useState("");
  const [status, setStatus] = useState<CloudPlanningStatus | null>(null);
  const [keySet, setKeySet] = useState<Record<string, boolean>>({});
  const [models, setModels] = useState<HostedModel[]>([]);
  const [search, setSearch] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [tested, setTested] = useState<Record<string, { ready: boolean; message: string; elapsed_ms?: number }>>({});
  const mounted = useRef(true);
  const busyRef = useRef(false);
  const accountGeneration = useRef<number>();
  const selected = connections.find(connection => connection.id === selectedID);
  const controlsLocked = locked || busy;
  const signature = selected ? connectionSignature(selected) : "";
  const test = tested[signature];
  const plannerID = planner.provider === "connection" ? planner.connection_id : planner.provider === "conversation" ? "conversation" : planner.provider === "chatgpt" ? "legacy-chatgpt" : planner.provider;
  const plannerHosted = planner.provider === "decisions" || planner.provider === "chatgpt" || (planner.provider === "conversation" ? conversationID !== "local" : planner.provider === "connection" && planner.connection_id !== "local");

  function accountChanged(next: CloudPlanningStatus) {
    if (accountGeneration.current !== undefined && accountGeneration.current !== next.connection.generation) { setTested({}); setModels([]); }
    accountGeneration.current = next.connection.generation;
    setStatus(next);
  }

  useEffect(() => {
    mounted.current = true;
    if (typeof api.cloudPlanningStatus === "function") void api.cloudPlanningStatus().then(next => {
      if (mounted.current) { accountChanged(next); setKeySet(next.connection_keys ?? {});
        const restored: Record<string, { ready: boolean; message: string; elapsed_ms?: number }> = {};
        for (const readiness of Object.values(next.connection_readiness ?? {})) {
          if (readiness.connection) restored[connectionSignature(readiness.connection)] = { ready: readiness.ready, message: readiness.message ?? "", elapsed_ms: readiness.elapsed_ms };
        }
        setTested(restored); }
    }).catch(() => {});
    return () => { mounted.current = false; };
  }, []);
  useEffect(() => { setModels([]); setSearch(""); setKey(""); }, [selectedID]);
  useEffect(() => {
    const profile = status?.connection.profiles.find(item => item.id === status.connection.active);
    if (!selected || selected.provider !== "chatgpt" || locked || !profile?.connected || !profile.plan_authorized) return;
    let current = true;
    void api.modelConnectionModels(selected).then(result => { if (current) setModels(result.models); }).catch(() => {});
    return () => { current = false; };
  }, [selectedID, selected?.provider, status?.connection.generation, locked]);
  useEffect(() => {
    if (!selected || selected.provider === "chatgpt" || controlsLocked) return;
    let current = true;
    void api.modelConnectionKeyStatus(selected).then(result => {
      if (current) setKeySet(existing => ({ ...existing, [selected.id]: result.key_set }));
    }).catch(() => {});
    return () => { current = false; };
  }, [signature, controlsLocked]);

  function updateConnection(change: Partial<ModelConnection>) {
    if (selected) patch({ connections: connections.map(connection => connection.id === selected.id ? { ...connection, ...change } : connection) });
  }
  function addConnection() {
    const connection = newModelConnection(newProvider, connections);
    patch({ connections: [...connections, connection] }); setSelectedID(connection.id);
  }
  async function run(work: () => Promise<void>) {
    if (locked || busyRef.current) return; busyRef.current = true; setBusy(true); setError("");
    try { await work(); }
    catch (reason) { if (mounted.current) setError(reason instanceof Error ? reason.message : String(reason)); }
    finally { busyRef.current = false; if (mounted.current) setBusy(false); }
  }
  const filtered = models.filter(model => (model.id + " " + model.name).toLowerCase().includes(search.toLowerCase())).slice(0, 100);
  const ready = Boolean(selected && test?.ready);

  return <>
    <div className="group cloud-motion-settings">
      <h4 className="group-title">{t("Models")}</h4>
      <p className="hint">{t("Model choices apply when you save settings. Accounts and keys are saved immediately.")}</p>
      <label className="field"><span className="label">{t("Chat model")}</span><select value={conversationID} disabled={controlsLocked} onChange={event => patch({ conversation_connection_id: event.target.value })}>
        <option value="local">{t("Local model")}</option>{connections.map(connection => <option value={connection.id} key={connection.id} disabled={!connection.model}>{connection.name + " · " + connection.model}</option>)}
      </select></label>
      {conversationID !== "local" && <details><summary>{t("Context sharing details")}</summary><p className="hint">{t("Conversation context sends the included messages as written, with your enabled persona and memories. Normal context budgets apply. Providers may refuse requests; MagicHandy does not rewrite or reroute refusals.")}</p></details>}
      <label className="field"><span className="label">{t("Autopilot model")}</span><select value={plannerID} disabled={controlsLocked} onChange={event => patch({ motion_planner: { ...planner, provider: event.target.value === "conversation" ? "conversation" : event.target.value === "decisions" ? "decisions" : event.target.value === "local" ? "local" : "connection", connection_id: ["conversation", "decisions", "local"].includes(event.target.value) ? "" : event.target.value } })}>
        <option value="conversation">{t("Same as conversation")}</option><option value="local">{t("Local model")}</option>
        {planner.provider === "chatgpt" && <option value="legacy-chatgpt" disabled>{t("ChatGPT") + " · " + planner.model}</option>}
        {connections.map(connection => <option value={connection.id} key={connection.id} disabled={!connection.model}>{connection.name + " · " + connection.model}</option>)}
        <option value="decisions" disabled={!status?.connection.decisions_key_set}>{t("Decisions library selection")}</option>
      </select></label>
      <p className="hint">{t("Chat requests use the conversation model for any permitted motion. This assignment controls Autopilot.")}</p>
      {plannerHosted && <><label className="field"><span className="label">{t("Autopilot context sharing")}</span><select value={planner.context_policy ?? "conversation"} disabled={controlsLocked} onChange={event => patch({ motion_planner: { ...planner, context_policy: event.target.value as "conversation" | "technical" } })}>
        <option value="conversation">{t("Conversation context")}</option><option value="technical">{t("Motion details only")}</option>
      </select></label>
      <details><summary>{t("Context sharing details")}</summary><p className="hint">{t("Conversation context lets the planner interpret current and standing requests directly. Motion details only sends current semantic motion, limits, and recent motion state; it excludes chat, personas, and memories. Choose full context when conversation continuity matters.")}</p></details></>}
    </div>
    <h3 className="section-title">{t("Model connections")}</h3>

    <div className="group cloud-motion-settings">
      <label className="field"><span className="label">{t("Edit connection")}</span>
        <select value={selectedID} disabled={controlsLocked} onChange={event => setSelectedID(event.target.value)}>
          <option value="local">{t("Local model")}</option>
          {connections.map(connection => <option value={connection.id} key={connection.id}>{connection.name}</option>)}
        </select>
      </label>
      <details><summary>{t("Add connection")}</summary><div className="button-row">
        <select aria-label={t("New connection provider")} disabled={controlsLocked} value={newProvider} onChange={event => setNewProvider(event.target.value as ModelConnection["provider"])}>
          {Object.entries(providerNames).map(([provider, name]) => <option value={provider} key={provider}>{translateKnown(name)}</option>)}
        </select>
        <button className="btn btn-secondary" type="button" disabled={controlsLocked || connections.length >= 16} onClick={addConnection}>{t("Add connection")}</button>
      </div></details>
    </div>
    {selectedID === "local" && localEditor}
    {selected && <div className="group cloud-motion-settings"><h4 className="group-title">{translateKnown(providerNames[selected.provider])}</h4><fieldset className="model-connection-editor" disabled={controlsLocked}>
      <label className="field"><span className="label">{t("Connection name")}</span><input type="text" value={selected.name} onChange={event => updateConnection({ name: event.target.value })} /></label>
      {selected.provider === "chatgpt" ? <ChatGPTConnection locked={controlsLocked} onChange={accountChanged} /> : <>
        <p className="hint">{t("This provider uses its own API key and billing. Credentials stay on the host and are bound to this provider and endpoint.")}</p>
        {selected.provider === "compatible" && <label className="field"><span className="label">{t("API base URL")}</span><input type="text" value={selected.base_url} onChange={event => updateConnection({ base_url: event.target.value })} /></label>}
        <label className="field"><span className="label">{t("API key")}</span><input type="password" autoComplete="off" value={key} onChange={event => setKey(event.target.value)} /></label>
        <div className="button-row">
          <button className="btn btn-secondary" type="button" disabled={!key.trim()} onClick={() => void run(async () => {
            setTested({}); const result = await api.modelConnectionKey(selected, key); if (mounted.current) { setKey(""); setKeySet(current => ({ ...current, [selected.id]: result.key_set })); }
          })}>{t("Save API key")}</button>
          {keySet[selected.id] && <button className="btn btn-secondary" type="button" onClick={() => void run(async () => {
            setTested({}); await api.modelConnectionKey(selected, ""); if (mounted.current) setKeySet(current => ({ ...current, [selected.id]: false }));
          })}>{t("Remove API key")}</button>}
        </div>
        {selected.provider === "compatible" && <label className="field checkbox"><input type="checkbox" checked={Boolean(selected.no_authentication)} onChange={event => updateConnection({ no_authentication: event.target.checked })} /><span>{t("This endpoint does not require authentication")}</span></label>}
      </>}
      <button className="btn btn-secondary" type="button" onClick={() => void run(async () => {
        const result = await api.modelConnectionModels(selected); if (mounted.current) setModels(result.models);
      })}>{t("Fetch available models")}</button>
      {models.length > 0 && <>
        {models.length > 20 && <label className="field"><span className="label">{t("Search models")}</span><input type="text" value={search} onChange={event => setSearch(event.target.value)} /></label>}
        <label className="field"><span className="label">{t("Available models")}</span><select value={selected.model} onChange={event => updateConnection(selectedModelChange(models.find(model => model.id === event.target.value), event.target.value, selected))}>
          <option value="">{t("Select an available model")}</option>
          {filtered.map(model => <option value={model.id} key={model.id}>{model.name}</option>)}
        </select></label>
      </>}
      <ModelResponseSettings connection={selected} models={models} disabled={controlsLocked} patch={updateConnection} />
      <details><summary>{t("Enter a model ID")}</summary>      <label className="field"><span className="label">{t("Model ID")}</span><input type="text" value={selected.model} onChange={event => updateConnection({ model: event.target.value, reasoning_effort: "" })} /></label><p className="hint">{t("A manual model ID is available for endpoints without a model catalog. Test an actual completion before assigning a role.")}</p></details>
      <div className="button-row">
        <button className="btn btn-secondary" type="button" disabled={!selected.model.trim()} onClick={() => void run(async () => {
          const result = await api.modelConnectionTest(selected); if (mounted.current) setTested(current => ({ ...current, [signature]: { ready: result.ready, message: result.message, elapsed_ms: result.elapsed_ms } }));
        })}>{t("Test connection")}</button>
        {test && <span role="status">{test.ready ? modelReadyMessage(test.elapsed_ms) : test.message}</span>}
        <button className="btn btn-secondary" type="button" disabled={!ready} onClick={() => patch({ conversation_connection_id: selected.id, motion_planner: { provider: "conversation", model: "", context_policy: "conversation" } })}>{t("Use for chat and motion")}</button>
      </div>
      <details>
        <summary>{t("Provider capabilities and routing")}</summary>
        <label className="field"><span className="label">{t("Structured output")}</span><select value={selected.output_mode} onChange={event => updateConnection({ output_mode: event.target.value as ModelConnection["output_mode"] })}>
          <option value="auto">{t("Automatic capability discovery")}</option><option value="strict">{t("Strict JSON schema")}</option><option value="json">{t("JSON object")}</option><option value="prompt">{t("Prompt contract and strict parser")}</option>
        </select></label>
        <p className="hint">{t("Motion validation always runs. OpenRouter requires compatible parameter support; generic endpoints use only the capabilities you declare.")}</p>
        {selected.provider === "compatible" && <label className="field checkbox"><input type="checkbox" checked={selected.supported_parameters?.includes("max_tokens") ?? false} onChange={event => updateConnection({ supported_parameters: event.target.checked ? ["max_tokens", "temperature", "top_p"] : [] })} /><span>{t("Endpoint supports standard token and sampling parameters")}</span></label>}
        {selected.provider === "openrouter" && <>
          <label className="field checkbox"><input type="checkbox" checked={selected.allow_fallbacks} onChange={event => updateConnection({ allow_fallbacks: event.target.checked })} /><span>{t("Allow OpenRouter to fall back between compatible hosts")}</span></label>
          <label className="field"><span className="label">{t("Allowed OpenRouter providers")}</span><input type="text" value={selected.allowed_providers?.join(", ") ?? ""} onChange={event => updateConnection({ allowed_providers: event.target.value.split(",").map(value => value.trim()).filter(Boolean) })} /></label>
          <label className="field"><span className="label">{t("Provider data collection")}</span><select value={selected.data_collection} onChange={event => updateConnection({ data_collection: event.target.value as ModelConnection["data_collection"] })}><option value="deny">{t("Deny collection")}</option><option value="allow">{t("Allow collection")}</option></select></label>
          <label className="field checkbox"><input type="checkbox" checked={selected.zero_data_retention} onChange={event => updateConnection({ zero_data_retention: event.target.checked })} /><span>{t("Require advertised zero data retention")}</span></label>
          <p className="hint">{t("Routing restrictions can reduce availability. Provider privacy metadata is not a guarantee about accepted content or physical safety.")}</p>
        </>}
      </details>
    </fieldset></div>}
    <details className="group cloud-motion-settings"><summary>{t("Optional Decisions API")}</summary>
      <p className="hint">{t("Decisions uses its own OpenAI API key and separate billing to choose complete enabled library patterns. ChatGPT sign-in does not authorize this billing path. Select Library motion and explicitly assign the Decisions role to use it.")}</p>
      <label className="field"><span className="label">{t("Decisions API key")}</span><input type="password" autoComplete="off" value={decisionsKey} disabled={controlsLocked} onChange={event => setDecisionsKey(event.target.value)} /></label>
      <div className="button-row">
        <button className="btn btn-secondary" type="button" disabled={controlsLocked || !decisionsKey.trim()} onClick={() => void run(async () => { const next = await api.cloudDecisionsKey(decisionsKey); if (mounted.current) { setStatus(next); setDecisionsKey(""); } })}>{t("Save API key")}</button>
        {status?.connection.decisions_key_set && <><button className="btn btn-secondary" type="button" disabled={controlsLocked} onClick={() => void run(async () => { const next = await api.cloudDecisionsKey(""); if (mounted.current) setStatus(next); })}>{t("Remove API key")}</button>
          <button className="btn btn-secondary" type="button" disabled={controlsLocked} onClick={() => void run(async () => { const next = await api.cloudTest("decisions", ""); if (mounted.current) { setStatus(next); if (!next.readiness.ready) setError(next.readiness.message ?? t("Request failed")); } })}>{t("Test Decisions connection")}</button></>}
      </div>
    </details>
    {error && <p className="error-text" role="alert">{error}</p>}
  </>;
}
