import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type {
  LLMModelImport,
  LLMModelManagerSnapshot,
  LLMMotionCapabilities,
  LLMProviderStatus,
  ManagedLlamaRuntimeBuild,
  ManagedLlamaRuntimeStatus,
  ManagedLLMModel,
  OllamaModelCandidate,
  OllamaModelInfo,
  OllamaModelScan,
  PublicSettings,
} from "../api/types";
import { RefreshIcon, TrashIcon, UploadIcon } from "../shell/icons";
import { useToast } from "../state/app-state";
import { formatBytes } from "../util/format";
import { HostPathField } from "./HostPathField";

type LLMSettings = PublicSettings["llm"];

interface ModelSettingsPanelProps {
  settings: LLMSettings;
  saved?: LLMSettings;
  providers: string[];
  llamaModes: string[];
  reasoningModes: string[];
  maxOutputOptions: number[];
  locked: boolean;
  patch: (next: Partial<LLMSettings>) => void;
}

const message = (error: unknown) => (error instanceof Error ? error.message : "Request failed");
const isActiveImport = (job: LLMModelImport) => job.status === "queued" || job.status === "copying";
const isActiveRuntimeBuild = (build?: ManagedLlamaRuntimeBuild) => build?.status === "queued" || build?.status === "building";
const providerLabel = (provider: string) => provider === "llama_cpp" ? "llama.cpp" : provider === "ollama" ? "Ollama" : provider;
const reasoningLabel = (mode: string) => mode === "auto" ? "Automatic / provider default" : mode === "off" ? "Disabled when supported" : mode;

// Absent capabilities resolve to the server defaults: everything but
// experimental patterns (mirrors config.DefaultLLMMotionCapabilities).
const defaultCapabilities: LLMMotionCapabilities = { motion: true, patterns: true, area_focus: true, experimental_patterns: false };

export function ModelSettingsPanel({ settings, saved, providers, llamaModes, reasoningModes, maxOutputOptions, locked, patch }: ModelSettingsPanelProps) {
  const { show } = useToast();
  const [manager, setManager] = useState<LLMModelManagerSnapshot | null>(null);
  const [managerMessage, setManagerMessage] = useState("");
  const [status, setStatus] = useState<LLMProviderStatus | null>(null);
  const [ollamaModels, setOllamaModels] = useState<OllamaModelInfo[]>([]);
  const [ollamaMessage, setOllamaMessage] = useState("");
  const [ollamaError, setOllamaError] = useState("");
  const [scan, setScan] = useState<OllamaModelScan | null>(null);
  const [showOllamaImport, setShowOllamaImport] = useState(false);
  const [showGGUFImport, setShowGGUFImport] = useState(false);
  const [ggufPath, setGGUFPath] = useState("");
  const [ggufName, setGGUFName] = useState("");
  const [scanning, setScanning] = useState(false);
  const [busy, setBusy] = useState("");
  const [confirmRemove, setConfirmRemove] = useState("");
  const [runtimeBackend, setRuntimeBackend] = useState<"auto" | "cpu" | "cuda">("auto");
  const mounted = useRef(true);
  const managerRefresh = useRef<Promise<void> | null>(null);
  const statusGeneration = useRef(0);
  const ollamaGeneration = useRef(0);

  const dirty = JSON.stringify(settings) !== JSON.stringify(saved);
  const ollamaPath = settings.ollama_models_path ?? "";
  const scanPath = ollamaPath.trim() || manager?.suggested_ollama_path || "";
  const activeImports = manager?.imports.some(isActiveImport) ?? false;
  const runtimeBuildActive = isActiveRuntimeBuild(manager?.runtime_build);
  const selectedManagedModel = manager?.models.find((model) => model.id === settings.model);
  const managedConfigured = Boolean(manager?.runtime.installed && selectedManagedModel?.state === "ready");
  const statusProvider = saved?.provider ?? settings.provider;
  const statusModel = saved?.model ?? settings.model;
  const protectedManagedModelID = saved?.provider === "llama_cpp" && saved.llama_cpp_mode === "managed" ? saved.model : "";
  const outputOptions = Array.from(new Set([settings.max_output_tokens, ...(maxOutputOptions.length ? maxOutputOptions : [128, 256, 512, 1024])])).sort((a, b) => a - b);
  const capabilities = settings.motion_capabilities ?? defaultCapabilities;

  function patchCapability(key: keyof LLMMotionCapabilities, value: boolean) {
    patch({ motion_capabilities: { ...capabilities, [key]: value } });
  }

  const refreshManager = useCallback(async () => {
    if (managerRefresh.current) return managerRefresh.current;
    if (mounted.current) setManagerMessage("");
    const request = (async () => {
      try {
        const next = await api.llmModels();
        if (!mounted.current) return;
        setManager(next);
        setManagerMessage("");
      } catch (error) {
        if (mounted.current) setManagerMessage(message(error));
      }
    })();
    managerRefresh.current = request;
    try {
      await request;
    } finally {
      if (managerRefresh.current === request) managerRefresh.current = null;
    }
  }, []);

  const refreshStatus = useCallback(async () => {
    const generation = ++statusGeneration.current;
    try {
      const next = await api.llmStatus();
      if (mounted.current && generation === statusGeneration.current) setStatus(next);
    } catch (error) {
      if (!mounted.current || generation !== statusGeneration.current) return;
      setStatus({
        provider: statusProvider,
        base_url: "",
        model: statusModel,
        available: false,
        message: message(error),
      });
    }
  }, [statusModel, statusProvider]);

  const refreshOllamaModels = useCallback(async () => {
    const generation = ++ollamaGeneration.current;
    try {
      const response = await api.ollamaModels();
      if (!mounted.current || generation !== ollamaGeneration.current) return;
      setOllamaModels(response.models);
      setOllamaMessage(response.message ?? "");
      setOllamaError("");
    } catch (error) {
      if (!mounted.current || generation !== ollamaGeneration.current) return;
      setOllamaError(message(error));
    }
  }, []);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      statusGeneration.current += 1;
      ollamaGeneration.current += 1;
    };
  }, []);

  useEffect(() => {
    void refreshManager();
    void refreshStatus();
  }, [refreshManager, refreshStatus]);

  useEffect(() => {
    if (settings.provider === "ollama" || showOllamaImport) void refreshOllamaModels();
  }, [refreshOllamaModels, settings.provider, showOllamaImport]);

  useEffect(() => {
    if (!activeImports && !runtimeBuildActive) return undefined;
    let canceled = false;
    let timer: number | undefined;
    const poll = async () => {
      await refreshManager();
      if (!canceled) timer = window.setTimeout(() => void poll(), 500);
    };
    timer = window.setTimeout(() => void poll(), 500);
    return () => {
      canceled = true;
      window.clearTimeout(timer);
    };
  }, [activeImports, refreshManager, runtimeBuildActive]);

  useEffect(() => {
    const buildStatus = manager?.runtime_build?.status;
    if (buildStatus && !isActiveRuntimeBuild(manager?.runtime_build)) void refreshStatus();
  }, [manager?.runtime_build, refreshStatus]);

  async function runtimeAction(action: "load" | "unload") {
    setBusy(action);
    try {
      const next = await (action === "load" ? api.llmLoad() : api.llmUnload());
      setStatus(next);
      show(action === "load" ? "Model loaded." : "Model unloaded.");
    } catch (error) {
      show(message(error), "error");
    } finally {
      setBusy("");
    }
  }

  async function buildRuntime() {
    setBusy("runtime-build");
    try {
      const response = await api.buildManagedLlamaRuntime(runtimeBackend);
      setManager((current) => current ? { ...current, runtime_build: response.build } : current);
      show("Managed llama.cpp build started.");
    } catch (error) {
      show(message(error), "error");
    } finally {
      setBusy("");
    }
  }

  async function cancelRuntimeBuild() {
    setBusy("runtime-cancel");
    try {
      await api.cancelManagedLlamaRuntimeBuild();
      await refreshManager();
    } catch (error) {
      show(message(error), "error");
    } finally {
      setBusy("");
    }
  }

  async function refreshModels() {
    setBusy("refresh");
    await Promise.all([refreshManager(), refreshStatus(), settings.provider === "ollama" ? refreshOllamaModels() : Promise.resolve()]);
    setBusy("");
  }

  async function scanOllama() {
    setScanning(true);
    try {
      const result = await api.scanOllamaModels(scanPath);
      setScan(result);
      patch({ ollama_models_path: result.path });
    } catch (error) {
      show(message(error), "error");
    } finally {
      setScanning(false);
    }
  }

  async function importOllama(candidate: OllamaModelCandidate) {
    setBusy(candidate.id);
    try {
      const response = await api.importOllamaModel(scan?.path || scanPath, candidate.id);
      mergeImport(response.import);
      show(`Import started for ${candidate.name}.`);
    } catch (error) {
      show(message(error), "error");
    } finally {
      setBusy("");
    }
  }

  async function importGGUF() {
    setBusy("gguf");
    try {
      const response = await api.importGGUFModel(ggufPath, ggufName);
      mergeImport(response.import);
      setGGUFPath("");
      setGGUFName("");
      show("GGUF import started.");
    } catch (error) {
      show(message(error), "error");
    } finally {
      setBusy("");
    }
  }

  async function cancelImport(job: LLMModelImport) {
    setBusy(job.id);
    try {
      await api.cancelLLMImport(job.id);
      await refreshManager();
    } catch (error) {
      show(message(error), "error");
    } finally {
      setBusy("");
    }
  }

  async function removeModel(model: ManagedLLMModel) {
    setBusy(model.id);
    try {
      await api.deleteLLMModel(model.id);
      setConfirmRemove("");
      await refreshManager();
      show(`${model.display_name} removed.`);
    } catch (error) {
      show(message(error), "error");
    } finally {
      setBusy("");
    }
  }

  function mergeImport(job: LLMModelImport) {
    setManager((current) => {
      if (!current) return current;
      return { ...current, imports: [job, ...current.imports.filter((item) => item.id !== job.id)] };
    });
  }

  function useManagedModel(model: ManagedLLMModel) {
    patch({
      provider: "llama_cpp",
      llama_cpp_mode: "managed",
      model: model.id,
    });
    show("Model selected. Save settings to apply.");
  }

  function useOllamaModel(model: OllamaModelInfo) {
    patch({ provider: "ollama", model: model.name });
    show("Ollama model selected. Save settings to apply.");
  }

  const statusTone = !dirty && status?.available ? "ready" : !dirty && status?.loaded ? "waiting" : "idle";
  const statusMessage = dirty ? "Save settings to check this configuration." : status?.message || "Checking runtime";
  const pathPlaceholder = manager?.suggested_ollama_path || "Ollama models directory";

  return (
    <>
      <div className="model-section-head">
        <h2 className="section-title">Local LLM</h2>
        <div className={`model-health model-health-${statusTone}`} role="status" aria-live="polite">
          <span className="status-dot" aria-hidden="true" />
          <span>{statusMessage}</span>
        </div>
      </div>
      {managerMessage && <p className="form-status form-status-error" role="alert">Model list unavailable: {managerMessage}</p>}

      <div className="model-runtime-grid">
        <label className="field">
          <span className="label">Provider</span>
          <select value={settings.provider} disabled={locked} onChange={(event) => patch({ provider: event.target.value })}>
            {(providers.length ? providers : [settings.provider]).map((provider) => (
              <option key={provider} value={provider}>{providerLabel(provider)}</option>
            ))}
          </select>
        </label>
        {settings.provider === "llama_cpp" && (
          <label className="field">
            <span className="label">llama.cpp mode</span>
            <select value={settings.llama_cpp_mode} disabled={locked} onChange={(event) => patch({ llama_cpp_mode: event.target.value })}>
              {(llamaModes.length ? llamaModes : [settings.llama_cpp_mode]).map((mode) => <option key={mode} value={mode}>{mode}</option>)}
            </select>
          </label>
        )}
      </div>

      {settings.provider === "llama_cpp" && settings.llama_cpp_mode === "managed" && (
        manager ? (
          <ManagedRuntime
            runtime={manager.runtime}
            build={manager.runtime_build}
            selectedModel={selectedManagedModel}
            backend={runtimeBackend}
            locked={locked}
            busy={busy}
            setBackend={setRuntimeBackend}
            onBuild={buildRuntime}
            onCancel={cancelRuntimeBuild}
          />
        ) : !managerMessage ? <p className="form-status" role="status">Checking managed runtime...</p> : null
      )}

      {settings.provider === "llama_cpp" && settings.llama_cpp_mode === "external" && (
        <>
          <div className="model-runtime-grid">
            <label className="field"><span className="label">llama.cpp URL</span><input type="text" value={settings.llama_cpp_base_url} disabled={locked} onChange={(event) => patch({ llama_cpp_base_url: event.target.value })} /></label>
            <label className="field"><span className="label">Model</span><input type="text" list="llama-server-model-options" value={settings.model} disabled={locked} onChange={(event) => patch({ model: event.target.value })} /><datalist id="llama-server-model-options">{status?.models?.map((model) => <option key={model} value={model} />)}</datalist></label>
          </div>
          <LlamaServerModels models={status?.models ?? []} selected={settings.model} locked={locked} onUse={(model) => patch({ model })} />
        </>
      )}

      {settings.provider === "ollama" && (
        <>
          <div className="model-runtime-grid">
            <label className="field"><span className="label">Ollama URL</span><input type="text" value={settings.ollama_base_url} disabled={locked} onChange={(event) => patch({ ollama_base_url: event.target.value })} /></label>
            <label className="field"><span className="label">Model</span><input type="text" list="ollama-model-options" value={settings.model} disabled={locked} onChange={(event) => patch({ model: event.target.value })} /><datalist id="ollama-model-options">{ollamaModels.map((model) => <option key={model.name} value={model.name} />)}</datalist></label>
          </div>
          {ollamaError && <p className="form-status form-status-error" role="alert">Ollama model list unavailable: {ollamaError}</p>}
          {(!ollamaError || ollamaModels.length > 0) && <OllamaDaemonModels models={ollamaModels} selected={settings.model} message={ollamaMessage} locked={locked} onUse={useOllamaModel} />}
        </>
      )}

      <div className="model-generation-settings" aria-label="Generation optimizations">
        <label className="field">
          <span className="label">Maximum output</span>
          <select value={settings.max_output_tokens} disabled={locked} onChange={(event) => patch({ max_output_tokens: Number(event.target.value) })}>
            {outputOptions.map((tokens) => <option key={tokens} value={tokens}>{tokens} tokens</option>)}
          </select>
        </label>
        <label className="field">
          <span className="label">Thinking / reasoning</span>
          <select value={settings.reasoning_mode} disabled={locked} onChange={(event) => patch({ reasoning_mode: event.target.value })}>
            {(reasoningModes.length ? reasoningModes : [settings.reasoning_mode]).map((mode) => <option key={mode} value={mode}>{reasoningLabel(mode)}</option>)}
          </select>
        </label>
        <label className="field model-timeout"><span className="label">Timeout ms</span><input type="number" min={1000} max={300000} value={settings.request_timeout_ms} disabled={locked} onChange={(event) => patch({ request_timeout_ms: Number(event.target.value) })} /></label>
      </div>
      <div className="generation-notes" role="note">
        <p>The selected cap covers reasoning plus visible JSON, so low limits can truncate JSON. The current pinned managed llama.cpp limits automatic reasoning to half that budget; every repair requests reasoning off to leave more budget for JSON.</p>
        <p>{settings.reasoning_mode === "off"
          ? `Requesting disabled reasoning is recommended for compact structured replies from small ${providerLabel(settings.provider)} models. Unsupported models may ignore or reject it.`
          : "Automatic reasoning may improve difficult intent interpretation, but can add hidden tokens and latency before the visible reply."}</p>
      </div>

      <fieldset className="capability-gates">
        <legend className="label">Model motion control</legend>
        <p className="hint-block narrow">
          What the model may do in chat and Autopilot. Disabled methods are never described to the
          model and are ignored if it tries them. Your controls — Stop, limits, manual motion — are
          never affected.
        </p>
        <label className="capability-gate">
          <input
            type="checkbox"
            checked={capabilities.motion}
            disabled={locked}
            onChange={(event) => patchCapability("motion", event.target.checked)}
          />
          <span>Motion control <span className="hint-inline">off makes the model chat-only</span></span>
        </label>
        <label className="capability-gate">
          <input
            type="checkbox"
            checked={capabilities.patterns}
            disabled={locked || !capabilities.motion}
            onChange={(event) => patchCapability("patterns", event.target.checked)}
          />
          <span>Pattern selection <span className="hint-inline">curate enabled library patterns</span></span>
        </label>
        <label className="capability-gate">
          <input
            type="checkbox"
            checked={capabilities.area_focus}
            disabled={locked || !capabilities.motion}
            onChange={(event) => patchCapability("area_focus", event.target.checked)}
          />
          <span>Area focus <span className="hint-inline">tip / shaft / base zones</span></span>
        </label>
        <label className="capability-gate">
          <input
            type="checkbox"
            checked={capabilities.experimental_patterns}
            disabled={locked || !capabilities.motion || !capabilities.patterns}
            onChange={(event) => patchCapability("experimental_patterns", event.target.checked)}
          />
          <span>Experimental patterns <span className="hint-inline">include experimental-tagged catalog entries</span></span>
        </label>
      </fieldset>

      {settings.provider === "llama_cpp" && settings.llama_cpp_mode === "managed" && (
        <div className="row-actions model-runtime-actions">
          <button type="button" className="btn btn-secondary" disabled={locked || dirty || !managedConfigured || runtimeBuildActive || busy !== ""} onClick={() => void runtimeAction("load")}>{busy === "load" ? "Loading..." : "Load"}</button>
          <button type="button" className="btn btn-secondary" disabled={locked || dirty || runtimeBuildActive || busy !== "" || !status?.loaded} onClick={() => void runtimeAction("unload")}>{busy === "unload" ? "Unloading..." : "Unload"}</button>
          {dirty && <span className="form-status">Save settings before runtime actions.</span>}
        </div>
      )}

      <div className="divider" />
      <div className="model-section-head">
        <div>
          <h3 className="model-subtitle">Managed models</h3>
          <p className="model-store-path">{manager?.store_path || (managerMessage ? "Model store unavailable" : "Loading model store")}</p>
        </div>
        <div className="row-actions model-import-actions">
          <button type="button" className="icon-btn model-refresh" aria-label="Refresh model list" title="Refresh model list" disabled={busy === "refresh"} onClick={() => void refreshModels()}><RefreshIcon size={17} /></button>
          <button type="button" className="btn btn-secondary" aria-expanded={showGGUFImport} disabled={locked || !manager} onClick={() => setShowGGUFImport((value) => !value)}><UploadIcon size={16} />Import GGUF</button>
          <button type="button" className="btn btn-secondary" aria-expanded={showOllamaImport} disabled={locked || !manager} onClick={() => setShowOllamaImport((value) => !value)}><UploadIcon size={16} />Import from Ollama</button>
        </div>
      </div>

      {showGGUFImport && (
        <div className="model-import-form" aria-label="Import GGUF model">
          <HostPathField label="GGUF file path" kind="gguf" value={ggufPath} disabled={locked || busy === "gguf"} onChange={setGGUFPath} />
          <label className="field"><span className="label">Display name <span className="hint-inline">optional</span></span><input type="text" value={ggufName} disabled={locked || busy === "gguf"} onChange={(event) => setGGUFName(event.target.value)} /></label>
          <button type="button" className="btn btn-primary" disabled={locked || !ggufPath.trim() || busy === "gguf"} onClick={() => void importGGUF()}>{busy === "gguf" ? "Starting..." : "Import copy"}</button>
        </div>
      )}

      {showOllamaImport && (
        <div className="ollama-import" aria-label="Import models from Ollama">
          <div className="model-import-path">
            <HostPathField label="Ollama models path" kind="directory" value={ollamaPath} placeholder={pathPlaceholder} disabled={locked || scanning} onChange={(ollama_models_path) => { setScan(null); patch({ ollama_models_path }); }} />
            <button type="button" className="btn btn-primary" disabled={locked || scanning} onClick={() => void scanOllama()}>{scanning ? "Scanning..." : "Scan library"}</button>
          </div>
          {scan && <OllamaCandidates candidates={scan.candidates} managed={manager?.models ?? []} locked={locked} busy={busy} onImport={importOllama} />}
        </div>
      )}

      {manager ? (
        <>
          <ImportProgress jobs={manager.imports ?? []} locked={locked} busy={busy} onCancel={cancelImport} />
          <ManagedModels
            models={manager.models ?? []}
            selectedID={settings.provider === "llama_cpp" && settings.llama_cpp_mode === "managed" ? settings.model : ""}
            protectedID={protectedManagedModelID}
            locked={locked}
            busy={busy}
            confirmRemove={confirmRemove}
            setConfirmRemove={setConfirmRemove}
            onUse={useManagedModel}
            onRemove={removeModel}
          />
        </>
      ) : !managerMessage && <p className="form-status" role="status">Loading model list...</p>}
    </>
  );
}

function ManagedRuntime({
  runtime, build, selectedModel, backend, locked, busy, setBackend, onBuild, onCancel,
}: {
  runtime?: ManagedLlamaRuntimeStatus;
  build?: ManagedLlamaRuntimeBuild;
  selectedModel?: ManagedLLMModel;
  backend: "auto" | "cpu" | "cuda";
  locked: boolean;
  busy: string;
  setBackend: (backend: "auto" | "cpu" | "cuda") => void;
  onBuild: () => void;
  onCancel: () => void;
}) {
  const active = isActiveRuntimeBuild(build);
  const backends = runtime?.supported_backends?.length ? runtime.supported_backends : ["auto", "cpu", "cuda"] as const;
  const metadata = [
    runtime?.version || runtime?.expected_version,
    runtime?.backend?.toUpperCase(),
    runtime?.source === "built_from_source" ? "Built from pinned source" : undefined,
  ];
  return (
    <div className="managed-runtime" aria-label="Managed llama.cpp runtime">
      <div className="managed-runtime-summary">
        <ModelIdentity name="Managed llama.cpp runtime" metadata={metadata} />
        <div className={`model-state model-state-${runtime?.state ?? "missing"}`}>{runtime?.message || "Checking managed runtime"}</div>
      </div>
      <div className="managed-runtime-controls">
        <label className="field runtime-backend">
          <span className="label">Build backend</span>
          <select value={backend} disabled={locked || active} onChange={(event) => setBackend(event.target.value as "auto" | "cpu" | "cuda")}>
            {backends.map((option) => <option key={option} value={option}>{option === "auto" ? "Auto-detect" : option.toUpperCase()}</option>)}
          </select>
        </label>
        <button type="button" className="btn btn-secondary" disabled={locked || active || busy !== "" || !runtime?.build_supported} title={runtime?.build_supported ? "Build the pinned app-owned llama.cpp runtime" : "Source builds currently require Windows x64"} onClick={() => void onBuild()}>
          {runtime?.installed ? "Build / switch runtime" : "Build runtime"}
        </button>
        {active && <button type="button" className="btn btn-secondary" disabled={locked || busy === "runtime-cancel"} onClick={() => void onCancel()}>Cancel build</button>}
      </div>
      {active && <progress className="runtime-build-progress" aria-label="Managed llama.cpp build in progress" />}
      {build && <p className={`form-status runtime-build-message${build.status === "failed" ? " form-status-error" : ""}`}>{build.message}</p>}
      <p className="form-status">{selectedModel ? `Selected model: ${selectedModel.display_name}` : "Select a managed model below before loading the runtime."}</p>
    </div>
  );
}

function LlamaServerModels({ models, selected, locked, onUse }: { models: string[]; selected: string; locked: boolean; onUse: (model: string) => void }) {
  if (!models.length) return <p className="form-status">No models reported by llama.cpp.</p>;
  return (
    <div className="provider-model-list" aria-label="Models reported by llama.cpp">
      {models.map((model) => (
        <div className="provider-model-row" key={model}>
          <ModelIdentity name={model} metadata={[]} />
          <button type="button" className="btn btn-secondary" disabled={locked || selected === model} onClick={() => onUse(model)}>{selected === model ? "Selected" : "Use"}</button>
        </div>
      ))}
    </div>
  );
}

function ManagedModels({
  models, selectedID, protectedID, locked, busy, confirmRemove, setConfirmRemove, onUse, onRemove,
}: {
  models: ManagedLLMModel[];
  selectedID: string;
  protectedID: string;
  locked: boolean;
  busy: string;
  confirmRemove: string;
  setConfirmRemove: (id: string) => void;
  onUse: (model: ManagedLLMModel) => void;
  onRemove: (model: ManagedLLMModel) => void;
}) {
  if (!models.length) return <p className="empty-state model-empty">No managed models.</p>;
  return (
    <div className="model-list" aria-label="Managed models">
      {models.map((model) => {
        const selected = selectedID === model.id;
        const protectedModel = selected || protectedID === model.id;
        return (
          <div className={`model-row${selected ? " model-row-selected" : ""}`} key={model.id}>
            <ModelIdentity name={model.display_name} metadata={[model.parameter_size, model.quantization, formatBytes(model.size_bytes), model.source === "ollama" ? "Ollama import" : "GGUF import"]} />
            <div className={`model-state model-state-${model.state}`}>{model.state}{model.message ? `: ${model.message}` : ""}</div>
            <div className="model-row-actions">
              {confirmRemove === model.id ? (
                <>
                  <button type="button" className="btn btn-danger-outline" disabled={locked || protectedModel || busy === model.id} onClick={() => void onRemove(model)}>Remove copy</button>
                  <button type="button" className="btn btn-secondary" disabled={busy === model.id} onClick={() => setConfirmRemove("")}>Cancel</button>
                </>
              ) : (
                <>
                  <button type="button" className="btn btn-secondary" disabled={locked || selected || model.state !== "ready"} onClick={() => onUse(model)}>{selected ? "Selected" : "Use"}</button>
                  <button type="button" className="icon-btn" aria-label={`Remove ${model.display_name}`} title="Remove managed copy" disabled={locked || protectedModel} onClick={() => setConfirmRemove(model.id)}><TrashIcon size={17} /></button>
                </>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
}

function ImportProgress({ jobs, locked, busy, onCancel }: { jobs: LLMModelImport[]; locked: boolean; busy: string; onCancel: (job: LLMModelImport) => void }) {
  const visible = jobs.filter((job) => isActiveImport(job) || job.status === "failed").slice(0, 4);
  if (!visible.length) return null;
  return (
    <div className="model-import-progress" aria-label="Model imports">
      {visible.map((job) => (
        <div className="model-import-job" key={job.id}>
          <div><strong>{job.display_name}</strong><span>{job.error || `${formatBytes(job.bytes_copied)} of ${formatBytes(job.total_bytes)}`}</span></div>
          <progress max={Math.max(1, job.total_bytes)} value={job.bytes_copied} aria-label={`Import progress for ${job.display_name}`} />
          {isActiveImport(job) && <button type="button" className="btn btn-secondary" disabled={locked || busy === job.id} onClick={() => void onCancel(job)}>Cancel</button>}
        </div>
      ))}
    </div>
  );
}

function OllamaCandidates({ candidates, managed, locked, busy, onImport }: { candidates: OllamaModelCandidate[]; managed: ManagedLLMModel[]; locked: boolean; busy: string; onImport: (candidate: OllamaModelCandidate) => void }) {
  const [query, setQuery] = useState("");
  if (!candidates.length) return <p className="empty-state model-empty">No Ollama manifests found.</p>;
  const imported = new Set(managed.map((model) => `sha256:${model.sha256}`));
  const normalizedQuery = query.trim().toLowerCase();
  const visible = normalizedQuery
    ? candidates.filter((candidate) => [candidate.name, candidate.family, candidate.parameter_size, candidate.quantization].some((value) => value?.toLowerCase().includes(normalizedQuery)))
    : candidates;
  return (
    <>
      <label className="field ollama-candidate-filter"><span className="label">Filter models</span><input type="search" value={query} onChange={(event) => setQuery(event.target.value)} /></label>
      <div className="ollama-candidates" aria-label="Ollama models available to import">
        {visible.map((candidate) => {
          const alreadyImported = Boolean(candidate.imported_model_id) || imported.has(candidate.digest ?? "");
          return (
            <div className="ollama-candidate" key={candidate.id}>
              <ModelIdentity name={candidate.name} metadata={[candidate.parameter_size, candidate.quantization, formatBytes(candidate.size_bytes), candidate.license]} />
              <div className="model-candidate-result">{alreadyImported ? "Imported" : candidate.reason || "Ready to copy"}</div>
              <button type="button" className="btn btn-secondary" disabled={locked || alreadyImported || !candidate.importable || busy === candidate.id} onClick={() => void onImport(candidate)}>{busy === candidate.id ? "Starting..." : "Import copy"}</button>
            </div>
          );
        })}
        {!visible.length && <p className="empty-state model-empty">No matching models.</p>}
      </div>
    </>
  );
}

function OllamaDaemonModels({ models, selected, message, locked, onUse }: { models: OllamaModelInfo[]; selected: string; message: string; locked: boolean; onUse: (model: OllamaModelInfo) => void }) {
  if (message) return <p className="form-status">{message}</p>;
  if (!models.length) return <p className="form-status">No models reported by Ollama.</p>;
  return (
    <div className="ollama-daemon-list" aria-label="Models reported by Ollama">
      {models.map((model) => (
        <div className="ollama-daemon-row" key={model.name}>
          <ModelIdentity name={model.name} metadata={[model.parameter_size, model.quantization, formatBytes(model.size_bytes)]} />
          <button type="button" className="btn btn-secondary" disabled={locked || selected === model.name} onClick={() => onUse(model)}>{selected === model.name ? "Selected" : "Use"}</button>
        </div>
      ))}
    </div>
  );
}

function ModelIdentity({ name, metadata }: { name: string; metadata: Array<string | undefined> }) {
  const visible = metadata.filter((value): value is string => Boolean(value));
  return (
    <div className="model-identity">
      <strong>{name}</strong>
      {visible.length > 0 && <span>{visible.join(" | ")}</span>}
    </div>
  );
}
