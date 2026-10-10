import { t } from "../i18n";
import type { LLMCatalog, LLMCatalogModel } from "../api/catalog-types";
import type {
  LLMModelImport,
  LLMModelManagerSnapshot,
  OllamaModelInfo,
  PublicSettings,
  SetupStatus,
} from "../api/types";
import { formatBytes } from "../util/format";
import { HostPathField } from "./HostPathField";
import { CatalogSourceLine, catalogHardwareLine, catalogModelDetail } from "./ModelCatalog";
import { OllamaLibraryImport } from "./OllamaLibraryImport";
import { SetupChoice } from "./SetupChoice";
import { FieldRow, SetupChoiceGroup, SetupSection } from "./SetupSection";
import { ModelConnectionsPanel } from "./ModelConnectionsPanel";

export type RuntimeChoice = "managed" | "ollama" | "external" | "hosted" | "skip";
// "download:<catalog id>" picks a curated model (downloaded during Install
// unless it is already in the store), "store" picks an imported model, and
// "later" installs only the engine.
export type ManagedModelChoice = `download:${string}` | "store" | "later";
export type RuntimeBackend = "auto" | "cpu" | "cuda";

export function catalogChoiceModel(choice: ManagedModelChoice, catalog: LLMCatalog | null): LLMCatalogModel | undefined {
  return catalog?.models.find((model) => choice === `download:${model.id}`);
}

export interface SetupChatStepProps {
  localOnly?: boolean;
  choice: RuntimeChoice;
  modelChoice: ManagedModelChoice;
  backend: RuntimeBackend;
  settings: PublicSettings["llm"];
  savedSettings?: PublicSettings;
  setup: SetupStatus;
  models: LLMModelManagerSnapshot | null;
  catalog: LLMCatalog | null;
  ollamaModels: OllamaModelInfo[];
  ggufPath: string;
  ggufName: string;
  locked: boolean;
  importLocked: boolean;
  select: (choice: RuntimeChoice) => void;
  selectModel: (choice: ManagedModelChoice) => void;
  setBackend: (backend: RuntimeBackend) => void;
  patchLLM: (patch: Partial<PublicSettings["llm"]>) => void;
  setGGUFPath: (value: string) => void;
  setGGUFName: (value: string) => void;
  importGGUF: () => void;
  mergeImport: (job: LLMModelImport) => void;
  refreshOllama: () => void;
}

export function SetupChatStep(props: SetupChatStepProps) {
  const { choice, setup, catalog, select } = props;
  const engineTitle = props.localOnly ? t("Local model") : t("Chat engine");
  const panel = choice === "managed" ? <ManagedRuntimeRows {...props} />
    : choice === "ollama" ? <>
      <FieldRow id="setup-ollama-url" label={t("Ollama base URL")} stack><input id="setup-ollama-url" type="text" value={props.settings.ollama_base_url} onChange={(event) => props.patchLLM({ ollama_base_url: event.target.value })} /></FieldRow>
      <FieldRow id="setup-ollama-model" label={t("Ollama model")} hint={t("Choose a model exposed by your running Ollama service. Existing Ollama files are not copied for this provider.")}>
        <div className="form-input-action">
          <select id="setup-ollama-model" aria-describedby="setup-ollama-model-hint" value={props.settings.model} onChange={(event) => props.patchLLM({ model: event.target.value })}><option value="">{t("Choose a model")}</option>{props.ollamaModels.map((model) => <option key={model.name} value={model.name}>{model.name} · {formatBytes(model.size_bytes)}</option>)}</select>
          <button type="button" className="btn btn-secondary" disabled={props.locked} onClick={props.refreshOllama}>{t("Refresh Ollama models")}</button>
        </div>
      </FieldRow>
      {!props.ollamaModels.length && <p className="hint form-row-note">{t("No running Ollama service was found. You can finish setup and configure its path later in Settings > Model.")}</p>}
    </>
      : choice === "external" ? <>
        <FieldRow id="setup-external-url" label={t("Server base URL")} stack><input id="setup-external-url" type="text" value={props.settings.llama_cpp_base_url} onChange={(event) => props.patchLLM({ llama_cpp_base_url: event.target.value })} /></FieldRow>
        <FieldRow id="setup-external-model" label={t("Model")} hint={t("Enter the model identifier expected by your compatible llama.cpp server.")}><input id="setup-external-model" aria-describedby="setup-external-model-hint" type="text" value={props.settings.model} onChange={(event) => props.patchLLM({ model: event.target.value })} /></FieldRow>
      </> : undefined;
  return <div className="setup-copy setup-model-library">
    <SetupSection id="setup-engine-title" title={engineTitle}>
      {!props.localOnly && <p className="hint-block">{t("Choose a local model or a hosted connection. Hosted models do not need a local runtime download.")}</p>}
      {choice !== "hosted" && <p className="setup-hardware"><span className="status-dot" data-state={setup.hardware.nvidia ? "ok" : "idle"} />{catalogHardwareLine(catalog, setup.hardware.nvidia, setup.hardware.gpu_name)}</p>}
      <SetupChoiceGroup labelledBy="setup-engine-title" panel={panel}>
        <SetupChoice selected={choice === "managed"} title={props.localOnly ? t("Install a local model") : t("Managed llama.cpp")} detail={t("App-owned, pinned, and checksum-verified. It avoids requiring Ollama or a compiler toolchain.")} badge={setup.hardware.nvidia ? t("Recommended") : undefined} onSelect={() => select("managed")} />
        <SetupChoice selected={choice === "ollama"} title={props.localOnly ? t("Use my Ollama") : t("Use my existing Ollama")} detail={t("Uses no managed runtime disk. MagicHandy uses your existing Ollama service and model library.")} onSelect={() => select("ollama")} />
        <SetupChoice selected={choice === "external"} title={props.localOnly ? t("Use my own server") : t("External llama.cpp server")} detail={t("Use a compatible server you manage. MagicHandy will not install or own that process.")} onSelect={() => select("external")} />
        {!props.localOnly && <SetupChoice selected={choice === "hosted"} title={t("Hosted model")} detail={t("Connect ChatGPT or an API provider. No local model is required.")} onSelect={() => select("hosted")} />}
        {!props.localOnly && <SetupChoice selected={choice === "skip"} title={t("Skip chat model setup")} detail={t("The app remains usable for manual, pattern, and video control.")} onSelect={() => select("skip")} />}
      </SetupChoiceGroup>
    </SetupSection>
    {choice === "managed" && <ManagedChatSetup {...props} />}
    {/* Hosted routing brings its own cards (routing, chat, Autopilot), as in Settings. */}
    {choice === "hosted" && <ModelConnectionsPanel setup settings={props.settings} saved={props.savedSettings?.llm} routing={props.savedSettings?.model_routing} locked={props.locked} patch={props.patchLLM} />}
  </div>;
}

// The managed runtime's options, shown under its choice.
function ManagedRuntimeRows({ backend, setup, models, locked, setBackend }: SetupChatStepProps) {
  const runtimeReady = Boolean(models?.runtime.installed && models.runtime.current);
  return <>
    <p className="setup-selection-state form-row-note" data-ready={runtimeReady}>{runtimeReady ? t("Managed runtime is already installed and verified.") : t("Selected for installation.")}</p>
    <FieldRow id="setup-runtime-backend" label={t("Runtime backend")} hint={<>{setup.llama_runtime.disk_estimate} {t("Official Windows bundles need no compiler or CUDA Toolkit. CUDA requires a compatible NVIDIA driver. License: {license}.", { license: setup.llama_runtime.license })}</>}>
      <select id="setup-runtime-backend" aria-describedby="setup-runtime-backend-hint" value={backend} disabled={locked} onChange={(event) => setBackend(event.target.value as RuntimeBackend)}>{setup.llama_runtime.backends.map((value) => <option key={value} value={value}>{value === "auto" ? t("Automatic") : value.toUpperCase()}</option>)}</select>
    </FieldRow>
    {!setup.hardware.nvidia && <p className="setup-warning form-row-note">{t("Without an NVIDIA card only the CPU backend can run, and replies would be too slow for live chat.")}</p>}
  </>;
}

function ManagedChatSetup({ localOnly, modelChoice, settings, models, catalog, ggufPath, ggufName, importLocked, selectModel, patchLLM, setGGUFPath, setGGUFName, importGGUF, mergeImport }: SetupChatStepProps) {
  const readyModels = models?.models.filter((model) => model.state === "ready") ?? [];
  const catalogIDs = new Set(catalog?.models.map((model) => model.installed_model_id).filter(Boolean));
  const importedModels = readyModels.filter((model) => !catalogIDs.has(model.id));
  const selectedCatalog = catalogChoiceModel(modelChoice, catalog);
  const storePanel = modelChoice === "store" ? <FieldRow id="setup-store-model" label={t("Managed model")}><select id="setup-store-model" value={settings.model} onChange={(event) => patchLLM({ model: event.target.value })}><option value="">{t("Choose a model")}</option>{importedModels.map((model) => <option key={model.id} value={model.id}>{model.display_name} · {formatBytes(model.size_bytes)}</option>)}</select></FieldRow>
    : selectedCatalog ? <div className="form-row form-row-plain"><CatalogSourceLine model={selectedCatalog} /></div> : undefined;
  return <SetupSection id="setup-chat-model-title" title={t("Chat model")}>
    <SetupChoiceGroup label={t("Chat model")} panel={storePanel}>
      {catalog?.models.map((model) => <SetupChoice
        key={model.id}
        selected={modelChoice === `download:${model.id}`}
        title={model.display_name}
        detail={catalogModelDetail(model, catalog)}
        badge={model.installed_model_id ? t("In your store") : model.fit === "recommended" ? t("Recommended") : undefined}
        onSelect={() => {
          selectModel(`download:${model.id}`);
          if (model.installed_model_id) patchLLM({ model: model.installed_model_id });
        }}
      />)}
      {importedModels.length > 0 && <SetupChoice selected={modelChoice === "store"} title={t("Use a model already in MagicHandy")} detail={t("Choose one of the models you imported earlier.")} onSelect={() => selectModel("store")} />}
      {!localOnly && <SetupChoice selected={modelChoice === "later"} title={t("Add a model later")} detail={t("Install the engine now, then add a model from Settings > Chat > Model.")} onSelect={() => selectModel("later")} />}
    </SetupChoiceGroup>
    <details className="setup-import-options">
      <summary>{t("Import a model file instead")}</summary>
      <section className="setup-method" aria-labelledby="setup-gguf-import-title">
        <header className="setup-method-head"><h3 id="setup-gguf-import-title">{t("Import a GGUF file")}</h3></header>
        <div className="setup-method-body">
          <HostPathField label={t("GGUF model file")} value={ggufPath} kind="gguf" disabled={importLocked} onChange={setGGUFPath} />
          <label className="field"><span className="label">{t("Display name")}</span><input type="text" value={ggufName} disabled={importLocked} placeholder={t("Optional model name")} onChange={(event) => setGGUFName(event.target.value)} /></label>
          <button type="button" className="btn btn-secondary" disabled={importLocked || !ggufPath.trim()} onClick={importGGUF}>{t("Import GGUF")}</button>
        </div>
      </section>
      <section className="setup-method" aria-labelledby="setup-ollama-import-title">
        <header className="setup-method-head">
          <h3 id="setup-ollama-import-title">{t("Import from an existing Ollama library")}</h3>
          <p>{t("Choose the Ollama models folder. MagicHandy scans manifests first and copies only the model you select into its verified managed store.")}</p>
        </header>
        <div className="setup-method-body">
          <OllamaLibraryImport
            path={settings.ollama_models_path ?? ""}
            suggestedPath={models?.suggested_ollama_path}
            managedModels={models?.models ?? []}
            locked={importLocked}
            onPathChange={(ollama_models_path) => patchLLM({ ollama_models_path })}
            onImportStarted={mergeImport}
          />
        </div>
      </section>
    </details>
  </SetupSection>;
}
