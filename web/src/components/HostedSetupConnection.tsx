import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { CloudPlanningStatus, HostedModel, ModelConnection } from "../api/cloud-types";
import { t } from "../i18n";
import { ChatGPTConnection } from "./ChatGPTConnection";
import { connectionSignature } from "../util/model-connections";
import { ModelResponseSettings, modelReadyMessage, selectedModelChange } from "./ModelResponseSettings";

// Catalog discovery is automatic; generation remains an explicit user action.
export function HostedSetupConnection({ connection, locked, patch, onReady }: {
  connection: ModelConnection; locked: boolean; patch: (change: Partial<ModelConnection>) => void; onReady: (ready: boolean) => void;
}) {
  const [key, setKey] = useState("");
  const [keySaved, setKeySaved] = useState(false);
  const [models, setModels] = useState<HostedModel[]>([]);
  const [search, setSearch] = useState("");
  const [operation, setOperation] = useState("");
  const [error, setError] = useState("");
  const [testMessage, setTestMessage] = useState("");
  const [accountReady, setAccountReady] = useState(false);
  const [needsAuthorization, setNeedsAuthorization] = useState(false);
  const accountGeneration = useRef<number | undefined>();
  const readyRef = useRef(onReady); readyRef.current = onReady;
  const patchRef = useRef(patch); patchRef.current = patch;
  const connectionRef = useRef(connection); connectionRef.current = connection;
  const lockedRef = useRef(locked); lockedRef.current = locked;
  const busyRef = useRef(false);
  const catalogAttempt = useRef("");
  const pendingCatalog = useRef<number>();
  const savedKey = useRef(false);
  const keyRevision = useRef(0);
  const signature = connectionSignature(connection);
  const signatureRef = useRef(signature); signatureRef.current = signature;
  const mounted = useRef(true);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  useEffect(() => { setTestMessage(""); readyRef.current(false); }, [signature]);
  useEffect(() => { setKey(""); setKeySaved(false); savedKey.current = false; setModels([]); setSearch(""); setError(""); catalogAttempt.current = ""; }, [connection.id, connection.base_url]);

  async function run(kind: string, work: () => Promise<void>) {
    if (busyRef.current || lockedRef.current) return;
    busyRef.current = true; setOperation(kind); setError("");
    try { await work(); }
    catch (reason) { if (mounted.current) { setError(reason instanceof Error ? reason.message : String(reason)); if (kind !== "catalog") readyRef.current(false); } }
    finally {
      busyRef.current = false;
      if (mounted.current) {
        setOperation("");
        if (pendingCatalog.current !== undefined) { const generation = pendingCatalog.current; pendingCatalog.current = undefined; discover(generation); }
      }
    }
  }
  async function fetchModels() {
    const current = connectionRef.current;
    const snapshot = connectionSignature(current);
    if (key.trim()) {
      keyRevision.current++;
      await api.modelConnectionKey(current, key);
      if (mounted.current) { setKey(""); savedKey.current = true; setKeySaved(true); setTestMessage(""); readyRef.current(false); }
    }
    const result = await api.modelConnectionModels(current);
    if (!mounted.current || signatureRef.current !== snapshot) return;
    setModels(result.models);
    if (current.provider === "chatgpt" && !current.model && result.models[0]) {
      const recommended = result.models.find(model => model.id === "gpt-6-sol") ?? result.models.find(model => model.id === "gpt-6.1-sol") ?? result.models[0];
      patchRef.current(selectedModelChange(recommended, recommended.id, current));
    }
  }
  function discover(generation: number) {
    const current = connectionRef.current;
    const target = `${current.id}:${current.base_url}:${generation}`;
    if (catalogAttempt.current === target || lockedRef.current) return;
    if (busyRef.current) { pendingCatalog.current = generation; return; }
    catalogAttempt.current = target;
    void run("catalog", fetchModels);
  }
  function accountChanged(status: CloudPlanningStatus) {
    const profile = status.connection.profiles.find(item => item.id === status.connection.active);
    const connected = Boolean(profile?.connected && profile.plan_authorized);
    setAccountReady(connected);
    if (!connected) readyRef.current(false);
    if (accountGeneration.current !== status.connection.generation) {
      if (accountGeneration.current !== undefined) { setModels([]); setTestMessage(""); setNeedsAuthorization(false); readyRef.current(false); }
      accountGeneration.current = status.connection.generation;
    }
    if (connected) discover(status.connection.generation);
  }
  useEffect(() => {
    const revision = keyRevision.current;
    void api.cloudPlanningStatus().then(async status => {
      if (!mounted.current || signatureRef.current !== signature) return;
      if (connection.provider === "chatgpt") accountChanged(status);
      const readiness = status.connection_readiness?.[connection.id];
      if (readiness?.ready && readiness.connection && connectionSignature(readiness.connection) === signature) { setTestMessage(modelReadyMessage(readiness.elapsed_ms)); readyRef.current(true); }
      if (connection.provider !== "chatgpt") {
        if (locked) savedKey.current = Boolean(status.connection_keys?.[connection.id]);
        if (!locked && !connection.no_authentication) {
          const binding = await api.modelConnectionKeyStatus(connection);
          if (!mounted.current || signatureRef.current !== signature || keyRevision.current !== revision) return;
          savedKey.current = binding.key_set;
        }
        const saved = savedKey.current;
        setKeySaved(saved);
        if (saved || connection.no_authentication) discover(status.connection.generation);
      }
    }).catch(() => {});
  }, [signature, connection.id, connection.provider, connection.no_authentication, locked]);

  const disabled = locked || Boolean(operation);
  const connected = connection.provider === "chatgpt" ? accountReady : keySaved || Boolean(connection.no_authentication);
  const filtered = models.filter(model => `${model.name} ${model.id}`.toLowerCase().includes(search.toLowerCase())).slice(0, 50);
  const manualModel = <label className="field"><span className="label">{t("Model ID")}</span><input type="text" value={connection.model} disabled={disabled} onChange={event => patch({ model: event.target.value, reasoning_effort: "" })} /></label>;
  return <div className="hosted-setup-connection cloud-motion-settings">
    {connection.provider === "chatgpt" ? <ChatGPTConnection presentation="setup" locked={disabled} needsAuthorization={needsAuthorization} onChange={accountChanged} /> : <>
      <p className="hint-block">{t("API providers use their own key and separate billing. Your key stays on this computer.")}</p>
      {connection.provider === "compatible" && <label className="field"><span className="label">{t("API base URL")}</span><input type="text" value={connection.base_url} disabled={disabled} onChange={event => patch({ base_url: event.target.value })} /></label>}
      {!connection.no_authentication && <label className="field"><span className="label">{t("API key")}</span><input type="password" autoComplete="off" value={key} disabled={disabled} placeholder={keySaved ? t("Saved key will be kept") : undefined} onChange={event => setKey(event.target.value)} /></label>}
      {connection.provider === "compatible" && <label className="field checkbox"><input type="checkbox" checked={Boolean(connection.no_authentication)} disabled={disabled} onChange={event => patch({ no_authentication: event.target.checked })} /><span>{t("This endpoint does not require authentication")}</span></label>}
      {!connected && <button type="button" className="btn btn-secondary" disabled={disabled || !key.trim()} onClick={() => void run("catalog", fetchModels)}>{t("Connect provider")}</button>}
      {keySaved && <details><summary>{t("Manage key")}</summary><button type="button" className="btn btn-quiet" disabled={disabled} onClick={() => void run("key", async () => {
        keyRevision.current++;
        await api.modelConnectionKey(connectionRef.current, "");
        if (mounted.current) { setKey(""); savedKey.current = false; setKeySaved(false); setModels([]); setTestMessage(""); readyRef.current(false); catalogAttempt.current = ""; }
      })}>{t("Remove API key")}</button></details>}
    </>}
    {operation === "catalog" && <p role="status">{t("Fetching models...")}</p>}
    {models.length > 0 && <>
      {models.length > 20 && <label className="field"><span className="label">{t("Search models")}</span><input type="text" value={search} disabled={disabled} onChange={event => setSearch(event.target.value)} /></label>}
      <label className="field"><span className="label">{t("Available models")}</span><select value={connection.model} disabled={disabled} onChange={event => patch(selectedModelChange(models.find(model => model.id === event.target.value), event.target.value, connection))}>
        <option value="">{t("Select an available model")}</option>
        {connection.model && !filtered.some(model => model.id === connection.model) && <option value={connection.model}>{connection.model}</option>}
        {filtered.map(model => <option key={model.id} value={model.id}>{model.name}</option>)}
      </select></label>
    </>}
    {connected && <ModelResponseSettings connection={connection} models={models} disabled={disabled} patch={patch} compact />}
    {(connected || error) && <details className="manual-model"><summary>{t("Enter a model ID")}</summary>{manualModel}
      <button type="button" className="btn btn-quiet" disabled={disabled} onClick={() => { catalogAttempt.current = ""; void run("catalog", fetchModels); }}>{t("Fetch available models")}</button>
    </details>}
    {connected && <div className="button-row">
      <button type="button" className="btn btn-secondary" disabled={disabled || !connection.model.trim()} onClick={() => void run("check", async () => {
        const current = connectionRef.current;
        const snapshot = connectionSignature(current);
        if (key.trim()) { keyRevision.current++; await api.modelConnectionKey(current, key); if (mounted.current) { setKey(""); savedKey.current = true; setKeySaved(true); } }
        const result = await api.modelConnectionTest(current);
        if (!mounted.current || signatureRef.current !== snapshot) return;
        setTestMessage(result.ready ? modelReadyMessage(result.elapsed_ms) : result.message); readyRef.current(result.ready);
        if (current.provider === "chatgpt") setNeedsAuthorization(result.state === "signed_out" || result.state === "permission");
      })}>{operation === "check" ? t("Checking model...") : t("Check model")}</button>
      {testMessage && <span role="status">{testMessage}</span>}
    </div>}
    {connected && !testMessage && <p className="hint-block">{t("Check model uses a text-only request. Nothing moves.")}</p>}
    <p className="hint-block">{t("Accounts and keys are saved immediately. Model choices apply when you continue.")}</p>
    {error && <p className="form-status form-status-error" role="alert">{error}</p>}
  </div>;
}
