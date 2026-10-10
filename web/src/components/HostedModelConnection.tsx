import { DismissibleNotice } from "./DismissibleNotice";
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { CloudPlanningStatus, HostedModel, ModelConnection } from "../api/cloud-types";
import { t, translateKnown } from "../i18n";
import { ChatGPTConnection } from "./ChatGPTConnection";
import { connectionSignature, modelProviderNames } from "../util/model-connections";
import { ModelResponseSettings, modelReadyMessage, selectedModelChange } from "./ModelResponseSettings";

export function HostedModelConnection({ connection: selected, locked, patch: updateConnection }: {
  connection: ModelConnection; locked: boolean; patch: (change: Partial<ModelConnection>) => void;
}) {
  const [key, setKey] = useState("");
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
  const controlsLocked = locked || busy;
  const signature = connectionSignature(selected);
  const currentSignature = useRef(signature);
  currentSignature.current = signature;
  const test = tested[signature];

  function accountChanged(next: CloudPlanningStatus) {
    if (accountGeneration.current !== undefined && next.connection.generation < accountGeneration.current) return false;
    if (selected.provider === "chatgpt" && accountGeneration.current !== undefined && accountGeneration.current !== next.connection.generation) { setTested({}); setModels([]); }
    accountGeneration.current = next.connection.generation;
    setStatus(next);
    return true;
  }
  useEffect(() => {
    mounted.current = true;
    let current = true;
    void api.cloudPlanningStatus().then(next => {
      if (!current) return;
      if (!accountChanged(next)) return;
      const restored: typeof tested = {};
      for (const readiness of Object.values(next.connection_readiness ?? {})) {
        if (readiness.connection) restored[connectionSignature(readiness.connection)] = { ready: readiness.ready, message: readiness.message ?? "", elapsed_ms: readiness.elapsed_ms };
      }
      setTested(current => ({ ...restored, ...current }));
    }).catch(() => {});
    return () => { current = false; mounted.current = false; };
  }, []);
  useEffect(() => { setModels([]); setSearch(""); setKey(""); setError(""); }, [selected.id, selected.base_url]);
  useEffect(() => {
    const profile = status?.connection.profiles.find(item => item.id === status.connection.active);
    if (selected.provider !== "chatgpt" || locked || !profile?.connected || !profile.plan_authorized) return;
    let current = true;
    void api.modelConnectionModels(selected).then(result => { if (current) setModels(result.models); }).catch(() => {});
    return () => { current = false; };
  }, [selected.id, selected.provider, status?.connection.generation, locked]);
  useEffect(() => {
    if (selected.provider === "chatgpt" || controlsLocked) return;
    let current = true;
    void api.modelConnectionKeyStatus(selected).then(result => {
      if (current) setKeySet(existing => ({ ...existing, [selected.id]: result.key_set }));
    }).catch(() => {});
    return () => { current = false; };
  }, [signature, controlsLocked]);
  async function run(work: () => Promise<void>) {
    if (locked || busyRef.current) return;
    busyRef.current = true; setBusy(true); setError("");
    try { await work(); }
    catch (reason) { if (mounted.current && currentSignature.current === signature) setError(reason instanceof Error ? reason.message : String(reason)); }
    finally { busyRef.current = false; if (mounted.current) setBusy(false); }
  }
  const filtered = models.filter(model => (model.id + " " + model.name).toLowerCase().includes(search.toLowerCase())).slice(0, 100);
  const currentRequest = () => mounted.current && currentSignature.current === signature;
  return <div className="cloud-motion-settings">
    <h4 className="group-title">{translateKnown(modelProviderNames[selected.provider])}</h4>
    <div className="model-connection-editor">
      {selected.provider === "chatgpt" ? <ChatGPTConnection locked={controlsLocked} onChange={accountChanged} /> : <>
        <DismissibleNotice id="model-api-credentials" className="model-help"><p className="hint">{t("This provider uses its own API key and billing. Credentials stay on the host and are bound to this provider and endpoint.")}</p></DismissibleNotice>
        {selected.provider === "compatible" && <label className="field"><span className="label">{t("API base URL")}</span><input disabled={controlsLocked} type="text" value={selected.base_url} onChange={event => updateConnection({ base_url: event.target.value })} /></label>}
        <label className="field"><span className="label">{t("API key")}</span><input disabled={controlsLocked} type="password" autoComplete="off" value={key} onChange={event => setKey(event.target.value)} /></label>
        <div className="button-row">
          <button className="btn btn-secondary" type="button" disabled={controlsLocked || !key.trim()} onClick={() => void run(async () => {
            setTested({}); setModels([]); const result = await api.modelConnectionKey(selected, key); if (currentRequest()) { setKey(""); setKeySet(current => ({ ...current, [selected.id]: result.key_set })); }
          })}>{t("Save API key")}</button>
          {keySet[selected.id] && <button className="btn btn-secondary" type="button" disabled={controlsLocked} onClick={() => void run(async () => {
            setTested({}); setModels([]); await api.modelConnectionKey(selected, ""); if (currentRequest()) setKeySet(current => ({ ...current, [selected.id]: false }));
          })}>{t("Remove API key")}</button>}
        </div>
        {selected.provider === "compatible" && <label className="toggle-line"><span className="toggle"><input disabled={controlsLocked} type="checkbox" checked={Boolean(selected.no_authentication)} onChange={event => updateConnection({ no_authentication: event.target.checked })} /><span className="track" aria-hidden="true" /></span><span>{t("This endpoint does not require authentication")}</span></label>}
      </>}
      <button className="btn btn-secondary" type="button" disabled={controlsLocked} onClick={() => void run(async () => {
        const result = await api.modelConnectionModels(selected); if (currentRequest()) setModels(result.models);
      })}>{t("Fetch available models")}</button>
      {models.length > 0 && <>
        {models.length > 20 && <label className="field"><span className="label">{t("Search models")}</span><input disabled={controlsLocked} type="text" value={search} onChange={event => setSearch(event.target.value)} /></label>}
        <label className="field"><span className="label">{t("Available models")}</span><select disabled={controlsLocked} value={selected.model} onChange={event => updateConnection(selectedModelChange(models.find(model => model.id === event.target.value), event.target.value, selected))}>
          <option value="">{t("Select an available model")}</option>
          {selected.model && !filtered.some(model => model.id === selected.model) && <option value={selected.model}>{selected.model}</option>}
          {filtered.map(model => <option value={model.id} key={model.id}>{model.name}</option>)}
        </select></label>
      </>}
      <ModelResponseSettings connection={selected} models={models} disabled={controlsLocked} patch={updateConnection} />
      <details><summary>{t("Enter a model ID")}</summary>      <label className="field"><span className="label">{t("Model ID")}</span><input disabled={controlsLocked} type="text" value={selected.model} onChange={event => updateConnection({ model: event.target.value, reasoning_effort: "" })} /></label><DismissibleNotice id="model-manual-id" className="model-help"><p className="hint">{t("A manual model ID is available for endpoints without a model catalog. Test an actual completion before assigning a role.")}</p></DismissibleNotice></details>
      <div className="button-row">
        <button className="btn btn-secondary" type="button" disabled={controlsLocked || !selected.model.trim()} onClick={() => void run(async () => {
          const result = await api.modelConnectionTest(selected); if (currentRequest()) setTested(current => ({ ...current, [signature]: { ready: result.ready, message: result.message, elapsed_ms: result.elapsed_ms } }));
        })}>{t("Test connection")}</button>
        {test && <span role="status">{test.ready ? modelReadyMessage(test.elapsed_ms) : test.message}</span>}
      </div>
      {(selected.provider === "openrouter" || selected.provider === "compatible") && <details>
        <summary>{t("Provider capabilities and routing")}</summary>
        <label className="field"><span className="label">{t("Structured output")}</span><select disabled={controlsLocked} value={selected.output_mode} onChange={event => updateConnection({ output_mode: event.target.value as ModelConnection["output_mode"] })}>
          <option value="auto">{t("Automatic capability discovery")}</option><option value="strict">{t("Strict JSON schema")}</option><option value="json">{t("JSON object")}</option><option value="prompt">{t("Prompt contract and strict parser")}</option>
        </select></label>
        <DismissibleNotice id="model-provider-routing" className="model-help"><p className="hint">{t("Motion validation always runs. OpenRouter requires compatible parameter support; generic endpoints use only the capabilities you declare.")}</p></DismissibleNotice>
        {selected.provider === "compatible" && <label className="toggle-line"><span className="toggle"><input disabled={controlsLocked} type="checkbox" checked={selected.supported_parameters?.includes("max_tokens") ?? false} onChange={event => updateConnection({ supported_parameters: event.target.checked ? ["max_tokens", "temperature", "top_p"] : [] })} /><span className="track" aria-hidden="true" /></span><span>{t("Endpoint supports standard token and sampling parameters")}</span></label>}
        {selected.provider === "openrouter" && <>
          <label className="toggle-line"><span className="toggle"><input disabled={controlsLocked} type="checkbox" checked={selected.allow_fallbacks} onChange={event => updateConnection({ allow_fallbacks: event.target.checked })} /><span className="track" aria-hidden="true" /></span><span>{t("Allow OpenRouter to fall back between compatible hosts")}</span></label>
          <label className="field"><span className="label">{t("Allowed OpenRouter providers")}</span><input disabled={controlsLocked} type="text" value={selected.allowed_providers?.join(", ") ?? ""} onChange={event => updateConnection({ allowed_providers: event.target.value.split(",").map(value => value.trim()).filter(Boolean) })} /></label>
          <label className="field"><span className="label">{t("Provider data collection")}</span><select disabled={controlsLocked} value={selected.data_collection} onChange={event => updateConnection({ data_collection: event.target.value as ModelConnection["data_collection"] })}><option value="deny">{t("Deny collection")}</option><option value="allow">{t("Allow collection")}</option></select></label>
          <label className="toggle-line"><span className="toggle"><input disabled={controlsLocked} type="checkbox" checked={selected.zero_data_retention} onChange={event => updateConnection({ zero_data_retention: event.target.checked })} /><span className="track" aria-hidden="true" /></span><span>{t("Require advertised zero data retention")}</span></label>
          <DismissibleNotice id="model-provider-privacy" className="model-help"><p className="hint">{t("Routing restrictions can reduce availability. Provider privacy metadata is not a guarantee about accepted content or physical safety.")}</p></DismissibleNotice>
        </>}
      </details>}
    </div>
    {error && <p className="error-text" role="alert">{error}</p>}
  </div>;
}
