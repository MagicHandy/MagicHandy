import { useCallback, useEffect, useMemo, useState } from "react";
import { api } from "../api/client";
import type {
  LLMModelImport,
  LLMModelManagerSnapshot,
  LLMProviderStatus,
  ManagedLLMModel,
  OllamaModelCandidate,
  OllamaModelInfo,
  OllamaModelScan,
  PublicSettings,
} from "../api/types";
import { RefreshIcon, TrashIcon, UploadIcon } from "../shell/icons";
import { useToast } from "../state/app-state";
import { formatBytes } from "../util/format";

type LLMSettings = PublicSettings["llm"];

interface ModelSettingsPanelProps {
  settings: LLMSettings;
  saved?: LLMSettings;
  providers: string[];
  llamaModes: string[];
  locked: boolean;
  patch: (next: Partial<LLMSettings>) => void;
}

const message = (error: unknown) => (error instanceof Error ? error.message : "Request failed");
const isActiveImport = (job: LLMModelImport) => job.status === "queued" || job.status === "copying";
const providerLabel = (provider: string) => provider === "llama_cpp" ? "llama.cpp" : provider === "ollama" ? "Ollama" : provider;

export function ModelSettingsPanel({ settings, saved, providers, llamaModes, locked, patch }: ModelSettingsPanelProps) {
  const { show } = useToast();
  const [manager, setManager] = useState<LLMModelManagerSnapshot | null>(null);
  const [managerMessage, setManagerMessage] = useState("");
  const [status, setStatus] = useState<LLMProviderStatus | null>(null);
  const [ollamaModels, setOllamaModels] = useState<OllamaModelInfo[]>([]);
  const [ollamaMessage, setOllamaMessage] = useState("");
  const [scan, setScan] = useState<OllamaModelScan | null>(null);
  const [showOllamaImport, setShowOllamaImport] = useState(false);
  const [showGGUFImport, setShowGGUFImport] = useState(false);
  const [ggufPath, setGGUFPath] = useState("");
  const [ggufName, setGGUFName] = useState("");
  const [scanning, setScanning] = useState(false);
  const [busy, setBusy] = useState("");
  const [confirmRemove, setConfirmRemove] = useState("");

  const dirty = JSON.stringify(settings) !== JSON.stringify(saved);
  const ollamaPath = settings.ollama_models_path ?? "";
  const scanPath = ollamaPath.trim() || manager?.suggested_ollama_path || "";
  const activeImports = manager?.imports.some(isActiveImport) ?? false;
  const managedConfigured = Boolean(settings.llama_cpp_runner_path?.trim() && settings.llama_cpp_model_path?.trim());

  const refreshManager = useCallback(async () => {
    try {
      setManager(await api.llmModels());
      setManagerMessage("");
    } catch (error) {
      setManagerMessage(message(error));
    }
  }, [show]);

  const refreshStatus = useCallback(async () => {
    try {
      setStatus(await api.llmStatus());
    } catch (error) {
      setStatus({
        provider: settings.provider,
        base_url: "",
        model: settings.model,
        available: false,
        message: message(error),
      });
    }
  }, [settings.model, settings.provider]);

  const refreshOllamaModels = useCallback(async () => {
    try {
      const response = await api.ollamaModels();
      setOllamaModels(response.models);
      setOllamaMessage(response.message ?? "");
    } catch (error) {
      setOllamaModels([]);
      setOllamaMessage(message(error));
    }
  }, []);

  useEffect(() => {
    void refreshManager();
    void refreshStatus();
  }, [refreshManager, refreshStatus]);

  useEffect(() => {
    if (settings.provider === "ollama" || showOllamaImport) void refreshOllamaModels();
  }, [refreshOllamaModels, settings.provider, showOllamaImport]);

  useEffect(() => {
    if (!activeImports) return undefined;
    const timer = window.setInterval(() => void refreshManager(), 500);
    return () => window.clearInterval(timer);
  }, [activeImports, refreshManager]);

  const modelOptions = useMemo(() => {
    if (settings.provider === "ollama") return ollamaModels.map((model) => model.name);
    if (settings.llama_cpp_mode === "managed") return manager?.models.map((model) => model.id) ?? [];
    return status?.models ?? [];
  }, [manager?.models, ollamaModels, settings.llama_cpp_mode, settings.provider, status?.models]);

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
      const response = await api.importOllamaModel(scanPath, candidate.id);
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
      llama_cpp_model_path: model.model_path,
    });
    show("Model selected. Save settings to apply.");
  }

  function useOllamaModel(model: OllamaModelInfo) {
    patch({ provider: "ollama", model: model.name });
    show("Ollama model selected. Save settings to apply.");
  }

  const statusTone = status?.available ? "ready" : status?.loaded ? "waiting" : "idle";
  const pathPlaceholder = manager?.suggested_ollama_path || "Ollama models directory";

  return (
    <>
      <div className="model-section-head">
        <h2 className="section-title">Local LLM</h2>
        <div className={`model-health model-health-${statusTone}`} role="status" aria-live="polite">
          <span className="status-dot" aria-hidden="true" />
          <span>{status?.message || "Checking runtime"}</span>
        </div>
      </div>

      <div className="model-runtime-grid">
        <label className="field">
          <span className="label">Provider</span>
          <select value={settings.provider} disabled={locked} onChange={(event) => patch({ provider: event.target.value })}>
            {(providers.length ? providers : [settings.provider]).map((provider) => (
              <option key={provider} value={provider}>{providerLabel(provider)}</option>
            ))}
          </select>
        </label>
        <label className="field">
          <span className="label">Model</span>
          <input
            type="text"
            list="llm-model-options"
            value={settings.model}
            disabled={locked}
            onChange={(event) => patch({ model: event.target.value })}
          />
          <datalist id="llm-model-options">
            {modelOptions.map((model) => <option key={model} value={model} />)}
          </datalist>
        </label>
      </div>

      {settings.provider === "llama_cpp" && (
        <>
          <label className="field">
            <span className="label">llama.cpp mode</span>
            <select value={settings.llama_cpp_mode} disabled={locked} onChange={(event) => patch({ llama_cpp_mode: event.target.value })}>
              {(llamaModes.length ? llamaModes : [settings.llama_cpp_mode]).map((mode) => <option key={mode} value={mode}>{mode}</option>)}
            </select>
          </label>
          {settings.llama_cpp_mode === "external" ? (
            <label className="field"><span className="label">llama.cpp URL</span><input type="text" value={settings.llama_cpp_base_url} disabled={locked} onChange={(event) => patch({ llama_cpp_base_url: event.target.value })} /></label>
          ) : (
            <div className="model-runtime-grid">
              <label className="field"><span className="label">llama-server path</span><input type="text" value={settings.llama_cpp_runner_path ?? ""} disabled={locked} onChange={(event) => patch({ llama_cpp_runner_path: event.target.value })} /></label>
              <label className="field"><span className="label">GGUF model path</span><input type="text" value={settings.llama_cpp_model_path ?? ""} disabled={locked} onChange={(event) => patch({ llama_cpp_model_path: event.target.value })} /></label>
            </div>
          )}
        </>
      )}

      {settings.provider === "ollama" && (
        <>
          <label className="field"><span className="label">Ollama URL</span><input type="text" value={settings.ollama_base_url} disabled={locked} onChange={(event) => patch({ ollama_base_url: event.target.value })} /></label>
          <OllamaDaemonModels models={ollamaModels} selected={settings.model} message={ollamaMessage} locked={locked} onUse={useOllamaModel} />
        </>
      )}

      <label className="field model-timeout"><span className="label">Timeout ms</span><input type="number" min={1000} max={300000} value={settings.request_timeout_ms} disabled={locked} onChange={(event) => patch({ request_timeout_ms: Number(event.target.value) })} /></label>

      {settings.provider === "llama_cpp" && settings.llama_cpp_mode === "managed" && (
        <div className="row-actions model-runtime-actions">
          <button type="button" className="btn btn-secondary" disabled={locked || dirty || !managedConfigured || busy !== ""} onClick={() => void runtimeAction("load")}>{busy === "load" ? "Loading..." : "Load"}</button>
          <button type="button" className="btn btn-secondary" disabled={locked || dirty || busy !== "" || !status?.loaded} onClick={() => void runtimeAction("unload")}>{busy === "unload" ? "Unloading..." : "Unload"}</button>
          {dirty && <span className="form-status">Save settings before runtime actions.</span>}
        </div>
      )}

      <div className="divider" />
      <div className="model-section-head">
        <div>
          <h3 className="model-subtitle">Managed models</h3>
          <p className="model-store-path">{manager?.store_path || "Loading model store"}</p>
        </div>
        <div className="row-actions model-import-actions">
          <button type="button" className="icon-btn model-refresh" aria-label="Refresh model list" title="Refresh model list" disabled={busy === "refresh"} onClick={() => void refreshModels()}><RefreshIcon size={17} /></button>
          <button type="button" className="btn btn-secondary" aria-expanded={showGGUFImport} disabled={locked} onClick={() => setShowGGUFImport((value) => !value)}><UploadIcon size={16} />Import GGUF</button>
          <button type="button" className="btn btn-secondary" aria-expanded={showOllamaImport} disabled={locked} onClick={() => setShowOllamaImport((value) => !value)}><UploadIcon size={16} />Import from Ollama</button>
        </div>
      </div>

      {managerMessage && <p className="form-status">{managerMessage}</p>}

      {showGGUFImport && (
        <div className="model-import-form" aria-label="Import GGUF model">
          <label className="field"><span className="label">GGUF file path</span><input type="text" value={ggufPath} disabled={locked || busy === "gguf"} onChange={(event) => setGGUFPath(event.target.value)} /></label>
          <label className="field"><span className="label">Display name <span className="hint-inline">optional</span></span><input type="text" value={ggufName} disabled={locked || busy === "gguf"} onChange={(event) => setGGUFName(event.target.value)} /></label>
          <button type="button" className="btn btn-primary" disabled={locked || !ggufPath.trim() || busy === "gguf"} onClick={() => void importGGUF()}>{busy === "gguf" ? "Starting..." : "Import copy"}</button>
        </div>
      )}

      {showOllamaImport && (
        <div className="ollama-import" aria-label="Import models from Ollama">
          <div className="model-import-path">
            <label className="field"><span className="label">Ollama models path</span><input type="text" value={ollamaPath} placeholder={pathPlaceholder} disabled={locked || scanning} onChange={(event) => patch({ ollama_models_path: event.target.value })} /></label>
            <button type="button" className="btn btn-primary" disabled={locked || scanning} onClick={() => void scanOllama()}>{scanning ? "Scanning..." : "Scan library"}</button>
          </div>
          {scan && <OllamaCandidates candidates={scan.candidates} managed={manager?.models ?? []} locked={locked} busy={busy} onImport={importOllama} />}
        </div>
      )}

      <ImportProgress jobs={manager?.imports ?? []} locked={locked} busy={busy} onCancel={cancelImport} />
      <ManagedModels
        models={manager?.models ?? []}
        selectedPath={settings.llama_cpp_model_path ?? ""}
        locked={locked}
        busy={busy}
        confirmRemove={confirmRemove}
        setConfirmRemove={setConfirmRemove}
        onUse={useManagedModel}
        onRemove={removeModel}
      />
    </>
  );
}

function ManagedModels({
  models, selectedPath, locked, busy, confirmRemove, setConfirmRemove, onUse, onRemove,
}: {
  models: ManagedLLMModel[];
  selectedPath: string;
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
        const selected = sameDisplayedPath(selectedPath, model.model_path);
        return (
          <div className={`model-row${selected ? " model-row-selected" : ""}`} key={model.id}>
            <ModelIdentity name={model.display_name} metadata={[model.parameter_size, model.quantization, formatBytes(model.size_bytes), model.source === "ollama" ? "Ollama import" : "GGUF import"]} />
            <div className={`model-state model-state-${model.state}`}>{model.state}{model.message ? `: ${model.message}` : ""}</div>
            <div className="model-row-actions">
              {confirmRemove === model.id ? (
                <>
                  <button type="button" className="btn btn-danger-outline" disabled={busy === model.id} onClick={() => void onRemove(model)}>Remove copy</button>
                  <button type="button" className="btn btn-secondary" disabled={busy === model.id} onClick={() => setConfirmRemove("")}>Cancel</button>
                </>
              ) : (
                <>
                  <button type="button" className="btn btn-secondary" disabled={locked || selected || model.state !== "ready"} onClick={() => onUse(model)}>{selected ? "Selected" : "Use"}</button>
                  <button type="button" className="icon-btn" aria-label={`Remove ${model.display_name}`} title="Remove managed copy" disabled={locked || selected} onClick={() => setConfirmRemove(model.id)}><TrashIcon size={17} /></button>
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

function sameDisplayedPath(left: string, right: string): boolean {
  if (left === right) return true;
  const windowsPath = /^[a-z]:/i.test(left) || left.includes("\\") || right.includes("\\");
  return windowsPath && left.toLowerCase() === right.toLowerCase();
}
