import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type {
  ConnectionCheckResult,
  LLMModelImport,
  LLMModelManagerSnapshot,
  OllamaModelInfo,
  PublicSettings,
  SetupInstallPlan,
  SetupJob,
  SetupStatus,
} from "../api/types";
import { HostPathField } from "../components/HostPathField";
import { OllamaLibraryImport } from "../components/OllamaLibraryImport";
import { LOCALE_OPTIONS, t, translateKnown } from "../i18n";
import { useAppState, useToast } from "../state/app-state";
import { formatBytes } from "../util/format";

const STEPS = ["welcome", "device", "runtime", "model", "voice", "install", "finish"] as const;
type SetupStep = (typeof STEPS)[number];

function setupStepLabel(step: SetupStep): string {
  if (step === "welcome") return t("Welcome");
  if (step === "device") return t("Device");
  if (step === "runtime") return t("Model runtime");
  if (step === "model") return t("Model library");
  if (step === "voice") return t("Voice (optional)");
  if (step === "install") return t("Install selected features");
  return t("Finish");
}

type RuntimeChoice = "managed" | "ollama" | "external" | "skip";
type VoiceChoice = "none" | "faster-qwen3-tts" | "chatterbox" | "external";

const message = (error: unknown) => error instanceof Error ? translateKnown(error.message) : t("Request failed");
const activeJob = (job?: SetupJob) => job?.status === "queued" || job?.status === "running";

function initialRuntimeChoice(settings?: PublicSettings["llm"]): RuntimeChoice {
  if (!settings) return "skip";
  if (settings.provider === "ollama") return "ollama";
  if (settings.provider === "llama_cpp" && settings.llama_cpp_mode === "managed") return "managed";
  if (settings.provider === "llama_cpp") return "external";
  return "skip";
}

export function SetupRoute() {
  const { state, backendOnline, readOnly, refresh } = useAppState();
  const { show } = useToast();
  const [step, setStep] = useState(0);
  const [setup, setSetup] = useState<SetupStatus | null>(null);
  const [settings, setSettings] = useState<PublicSettings | null>(state?.settings ?? null);
  const [models, setModels] = useState<LLMModelManagerSnapshot | null>(null);
  const [ollamaModels, setOllamaModels] = useState<OllamaModelInfo[]>([]);
  const [runtimeChoice, setRuntimeChoice] = useState<RuntimeChoice>(() => initialRuntimeChoice(state?.settings?.llm));
  const [runtimeBackend, setRuntimeBackend] = useState<"auto" | "cpu" | "cuda">("auto");
  const [voiceChoice, setVoiceChoice] = useState<VoiceChoice>("none");
  const [voiceDevice, setVoiceDevice] = useState<"cpu" | "cuda">("cpu");
  const [voiceAutoLaunch, setVoiceAutoLaunch] = useState(true);
  const [parakeetSelected, setParakeetSelected] = useState(false);
  const [connectionKey, setConnectionKey] = useState("");
  const [connectionResult, setConnectionResult] = useState<ConnectionCheckResult | null>(null);
  const [ggufPath, setGGUFPath] = useState("");
  const [ggufName, setGGUFName] = useState("");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [installJobID, setInstallJobID] = useState("");
  const [installSubmitted, setInstallSubmitted] = useState(false);
  const mounted = useRef(true);

  const locked = !backendOnline || readOnly || !settings || !setup || Boolean(busy);
  const installationActive = activeJob(setup?.installation);
  const installJob = setup?.installation?.id === installJobID ? setup.installation : undefined;
  const activeImport = models?.imports.find((job) => job.status === "queued" || job.status === "copying");

  const load = useCallback(async () => {
    try {
      const [setupStatus, modelStatus] = await Promise.all([api.setupStatus(), api.llmModels()]);
      if (!mounted.current) return;
      setSetup(setupStatus);
      setModels(modelStatus);
      if (setupStatus.installation?.kind === "install_plan" && (setupStatus.required || activeJob(setupStatus.installation))) {
        setInstallJobID(setupStatus.installation.id);
        setInstallSubmitted(true);
        setStep(STEPS.indexOf("install"));
      }
      setError("");
    } catch (reason) {
      if (mounted.current) setError(message(reason));
    }
  }, []);

  const loadOllama = useCallback(async () => {
    try {
      const response = await api.ollamaModels();
      if (mounted.current) setOllamaModels(response.models ?? []);
    } catch (reason) {
      if (mounted.current) setError(message(reason));
    }
  }, []);

  useEffect(() => {
    mounted.current = true;
    void load();
    return () => { mounted.current = false; };
  }, [load]);

  useEffect(() => {
    if (!state?.settings || settings) return;
    setSettings(state.settings);
    setRuntimeChoice(initialRuntimeChoice(state.settings.llm));
  }, [settings, state?.settings]);

  useEffect(() => {
    if (!setup?.hardware.nvidia) return;
    setVoiceDevice("cuda");
  }, [setup?.hardware.nvidia]);

  useEffect(() => {
    if (!installationActive && !activeImport) return;
    const timer = window.setInterval(() => {
      void load();
      if (activeImport) void api.llmModels().then((snapshot) => mounted.current && setModels(snapshot));
    }, 1000);
    return () => window.clearInterval(timer);
  }, [activeImport, installationActive, load]);

  useEffect(() => {
    if (runtimeChoice === "ollama") void loadOllama();
  }, [loadOllama, runtimeChoice]);

  const run = async (name: string, action: () => Promise<void>) => {
    if (busy) return;
    setBusy(name);
    setError("");
    try {
      await action();
    } catch (reason) {
      const detail = message(reason);
      setError(detail);
      show(detail, "error");
    } finally {
      if (mounted.current) setBusy("");
    }
  };

  const savePreferences = async (body: Parameters<typeof api.saveSetupPreferences>[0]) => {
    const response = await api.saveSetupPreferences(body);
    setSettings(response.settings);
    await refresh();
  };

  const patchLLM = (patch: Partial<PublicSettings["llm"]>) => {
    setSettings((current) => current ? { ...current, llm: { ...current.llm, ...patch } } : current);
  };

  const saveCurrentStep = async () => {
    if (!settings) return;
    if (step === 0) {
      await savePreferences({
        ui_locale: settings.ui?.locale ?? "en",
        chat_locale: promptLocale(settings.llm.prompt_set, settings.ui?.locale ?? "en"),
      });
    } else if (step === 1) {
      await savePreferences({
        device_owner: settings.device.hsp_dispatch_owner,
        ...(connectionKey.trim() ? { connection_key: connectionKey.trim() } : {}),
      });
      setConnectionKey("");
    } else if ((step === 2 || step === 3) && runtimeChoice !== "skip") {
      await savePreferences({ llm: settings.llm });
    }
  };

  const continueStep = () => void run("continue", async () => {
    await saveCurrentStep();
    if (step === STEPS.indexOf("voice")) {
      await beginInstall();
    }
    setStep((current) => Math.min(STEPS.length - 1, current + 1));
  });

  const skipStep = () => {
    setError("");
    if (step === STEPS.indexOf("runtime")) {
      setRuntimeChoice("skip");
      setStep(STEPS.indexOf("voice"));
      return;
    }
    if (step === STEPS.indexOf("model")) {
      setRuntimeChoice("skip");
    }
    if (step === STEPS.indexOf("voice")) {
      setVoiceChoice("none");
      setParakeetSelected(false);
      void run("continue", async () => {
        await beginInstall("none", false);
        setStep(STEPS.indexOf("install"));
      });
      return;
    }
    setStep((current) => Math.min(STEPS.length - 1, current + 1));
  };

  const selectRuntime = (choice: RuntimeChoice) => {
    setRuntimeChoice(choice);
    if (choice === "managed") patchLLM({ provider: "llama_cpp", llama_cpp_mode: "managed" });
    if (choice === "ollama") patchLLM({ provider: "ollama" });
    if (choice === "external") patchLLM({ provider: "llama_cpp", llama_cpp_mode: "external" });
  };

  const cancelInstall = () => void run("cancel", async () => {
    const response = await api.cancelSetupInstall();
    setSetup((current) => current ? { ...current, installation: response.installation } : current);
  });

  const importGGUF = () => void run("import", async () => {
    if (!ggufPath.trim()) return;
    await api.importGGUFModel(ggufPath.trim(), ggufName.trim());
    setGGUFPath("");
    setGGUFName("");
    setModels(await api.llmModels());
  });

  const mergeImport = (job: LLMModelImport) => {
    setModels((current) => current ? {
      ...current,
      imports: [job, ...current.imports.filter((item) => item.id !== job.id)],
    } : current);
  };

  function installPlan(nextVoiceChoice = voiceChoice, nextParakeet = parakeetSelected): SetupInstallPlan {
    const plan: SetupInstallPlan = { parakeet: nextParakeet };
    if (runtimeChoice === "managed" && !(models?.runtime.installed && models.runtime.current)) {
      plan.llama = { backend: runtimeBackend };
    }
    const module = setup?.voice_modules.find((item) => item.id === nextVoiceChoice);
    if (module) {
      plan.voice = {
        module: module.id,
        device: module.id === "faster-qwen3-tts" ? "cuda" : voiceDevice,
        auto_launch: voiceAutoLaunch,
      };
    }
    return plan;
  }

  async function beginInstall(nextVoiceChoice = voiceChoice, nextParakeet = parakeetSelected) {
    const plan = installPlan(nextVoiceChoice, nextParakeet);
    setInstallSubmitted(true);
    if (!plan.llama && !plan.voice && !plan.parakeet) {
      setInstallJobID("");
      return;
    }
    const response = await api.installSetupPlan(plan);
    setInstallJobID(response.installation.id);
    setSetup((current) => current ? { ...current, installation: response.installation } : current);
  }

  const verifyCloud = () => void run("verify", async () => {
    if (!settings) return;
    await savePreferences({
      device_owner: "cloud_rest",
      ...(connectionKey.trim() ? { connection_key: connectionKey.trim() } : {}),
    });
    setConnectionKey("");
    setConnectionResult(await api.connectionCheck("cloud"));
  });

  const finish = () => void run("finish", async () => {
    await api.completeSetup(runtimeChoice === "skip");
    await refresh();
    window.location.hash = "#/chat";
  });

  const managedModels = models?.models.filter((model) => model.state === "ready") ?? [];
  const managedModelReady = managedModels.some((model) => model.id === settings?.llm.model);
  const modelChoiceReady = runtimeChoice === "skip"
    || (runtimeChoice === "managed" && managedModelReady)
    || ((runtimeChoice === "ollama" || runtimeChoice === "external") && Boolean(settings?.llm.model.trim()));
  const installationReady = installSubmitted && (!installJob || installJob.status === "complete");
  const currentStepReady = step !== STEPS.indexOf("model") || modelChoiceReady;
  const canFinish = runtimeChoice === "skip" || (
    modelChoiceReady && (runtimeChoice !== "managed" || Boolean(models?.runtime.installed && models.runtime.current))
  );

  const title = [
    t("Set up MagicHandy"),
    t("Choose how MagicHandy reaches your device"),
    t("Choose your model runtime"),
    t("Choose a chat model"),
    t("Add voice features"),
    t("Installing selected features"),
    t("Setup is ready"),
  ][step];

  if (!settings || !setup) {
    return <section className="setup-loading" aria-live="polite"><span className="startup-progress" /><p>{error || t("Loading setup...")}</p></section>;
  }

  return (
    <section className="setup-layout" aria-labelledby="setup-title">
      <aside className="setup-progress" aria-label={t("Setup progress")}>
        <div className="setup-brand"><span aria-hidden="true">M</span><strong>{t("MagicHandy")}</strong></div>
        <ol>
          {STEPS.map((item, index) => (
            <li key={item} data-state={index < step ? "complete" : index === step ? "current" : "pending"}>
              <button type="button" disabled={index > step || installationActive} onClick={() => setStep(index)} aria-current={index === step ? "step" : undefined}>
                <span aria-hidden="true">{index < step ? null : index + 1}</span>{setupStepLabel(item)}
              </button>
            </li>
          ))}
        </ol>
        <p>{t("Every optional feature can be added later from Settings.")}</p>
      </aside>

      <div className="setup-main">
        <header className="setup-head">
          <p className="eyebrow">{t("Step {current} of {total}", { current: step + 1, total: STEPS.length })}</p>
          <h1 id="setup-title">{title}</h1>
        </header>

        <div className="setup-body">
          {step === 0 && <WelcomeStep settings={settings} patch={(patch) => setSettings({ ...settings, ...patch })} />}
          {step === 1 && <DeviceStep
            settings={settings}
            connectionKey={connectionKey}
            connectionResult={connectionResult}
            locked={locked}
            setConnectionKey={setConnectionKey}
            patchOwner={(owner) => setSettings({ ...settings, device: { ...settings.device, hsp_dispatch_owner: owner } })}
            verifyCloud={verifyCloud}
          />}
          {step === 2 && <RuntimeStep
            choice={runtimeChoice}
            backend={runtimeBackend}
            settings={settings.llm}
            setup={setup}
            models={models}
            locked={locked || installationActive}
            select={selectRuntime}
            setBackend={setRuntimeBackend}
            patchLLM={patchLLM}
          />}
          {step === 3 && <ModelStep
            choice={runtimeChoice}
            settings={settings.llm}
            models={models}
            ollamaModels={ollamaModels}
            ggufPath={ggufPath}
            ggufName={ggufName}
            locked={locked || Boolean(activeImport)}
            patch={patchLLM}
            setGGUFPath={setGGUFPath}
            setGGUFName={setGGUFName}
            importGGUF={importGGUF}
            mergeImport={mergeImport}
            refreshOllama={() => void loadOllama()}
          />}
          {step === 4 && <VoiceStep
            setup={setup}
            choice={voiceChoice}
            device={voiceDevice}
            autoLaunch={voiceAutoLaunch}
            parakeetSelected={parakeetSelected}
            locked={locked || installationActive}
            setChoice={setVoiceChoice}
            setDevice={setVoiceDevice}
            setAutoLaunch={setVoiceAutoLaunch}
            setParakeetSelected={setParakeetSelected}
          />}
          {step === 5 && <InstallStep
            job={installJob}
            submitted={installSubmitted}
            runtimeChoice={runtimeChoice}
            voiceChoice={voiceChoice}
            parakeetSelected={parakeetSelected}
            cancel={cancelInstall}
            retry={() => void run("retry", beginInstall)}
          />}
          {step === 6 && <FinishStep setup={setup} settings={settings} models={models} runtimeChoice={runtimeChoice} voiceChoice={voiceChoice} parakeetSelected={parakeetSelected} />}

          {activeImport && <p className="setup-inline-status" role="status">{t("Importing {name}: {copied} of {total}", {
            name: activeImport.display_name,
            copied: formatBytes(activeImport.bytes_copied),
            total: formatBytes(activeImport.total_bytes),
          })}</p>}
          {error && <p className="form-status setup-error" role="alert">{error}</p>}
        </div>

        <footer className="setup-actions">
          <button type="button" className="btn btn-secondary" disabled={step === 0 || installationActive || Boolean(busy)} onClick={() => setStep((current) => current - 1)}>{t("Back")}</button>
          <span className="setup-action-spacer" />
          {step < STEPS.length - 1 && step !== STEPS.indexOf("install") && <button type="button" className="btn btn-quiet" disabled={installationActive || Boolean(busy)} onClick={skipStep}>{t("Skip for now")}</button>}
          {step < STEPS.length - 1 ? (
            <button type="button" className="btn btn-primary" disabled={locked || installationActive || !currentStepReady || (step === STEPS.indexOf("install") && !installationReady)} onClick={continueStep}>{busy === "continue" ? t("Saving...") : t("Continue")}</button>
          ) : (
            <button type="button" className="btn btn-primary" disabled={locked || !canFinish} onClick={finish}>{busy === "finish" ? t("Finishing setup...") : t("Open MagicHandy")}</button>
          )}
        </footer>
      </div>
    </section>
  );
}

function WelcomeStep({ settings, patch }: { settings: PublicSettings; patch: (patch: Partial<PublicSettings>) => void }) {
  const locale = settings.ui?.locale ?? "en";
  const chatLocale = promptLocale(settings.llm.prompt_set, locale);
  return <div className="setup-copy">
    <p>{t("Setup configures local services and optional models. Nothing downloads, builds, connects, or moves the device without a separate action.")}</p>
    <div className="setup-fields two-columns">
      <label className="field"><span className="label">{t("App language")}</span><select value={locale} onChange={(event) => patch({ ui: { ...settings.ui, locale: event.target.value } })}>{LOCALE_OPTIONS.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></label>
      <label className="field"><span className="label">{t("Chat reply language")}</span><select value={chatLocale} onChange={(event) => patch({ llm: { ...settings.llm, prompt_set: promptSet(event.target.value) } })}>{LOCALE_OPTIONS.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></label>
    </div>
    <div className="setup-notice"><strong>{t("Device safety remains active during setup.")}</strong><span>{t("Emergency Stop stays available. Connection checks never command motion.")}</span></div>
  </div>;
}

function DeviceStep({ settings, connectionKey, connectionResult, locked, setConnectionKey, patchOwner, verifyCloud }: {
  settings: PublicSettings; connectionKey: string; connectionResult: ConnectionCheckResult | null; locked: boolean;
  setConnectionKey: (value: string) => void; patchOwner: (owner: string) => void; verifyCloud: () => void;
}) {
  const owner = settings.device.hsp_dispatch_owner;
  return <div className="setup-copy">
    <p>{t("Choose one connection owner. You can switch later from the connection manager in the top bar.")}</p>
    <div className="setup-choices">
      <Choice selected={owner === "cloud_rest"} title={t("Handy Cloud REST")} detail={t("Recommended for firmware v4. Requires your private connection key and internet access.")} onSelect={() => patchOwner("cloud_rest")} />
      <Choice selected={owner === "browser_bluetooth"} title={t("Browser Bluetooth")} detail={t("Connect directly from a compatible browser. No cloud connection key is stored.")} onSelect={() => patchOwner("browser_bluetooth")} />
      <Choice selected={owner === "intiface"} title={t("Intiface Central")} detail={t("Use an existing Intiface Central session for local device control.")} onSelect={() => patchOwner("intiface")} />
    </div>
    {owner === "cloud_rest" && <div className="setup-subsection">
      <label className="field"><span className="label">{t("Handy connection key")}</span><input type="password" autoComplete="off" value={connectionKey} placeholder={settings.device.connection_key_set ? t("Saved key will be kept") : t("Enter connection key")} onChange={(event) => setConnectionKey(event.target.value)} /></label>
      <div className="row-actions"><button type="button" className="btn btn-secondary" disabled={locked || (!connectionKey.trim() && !settings.device.connection_key_set)} onClick={verifyCloud}>{t("Save and check connection")}</button>{connectionResult && <span className="form-status" data-ok={connectionResult.ok}>{connectionResult.ok ? t("Connection verified") : translateKnown(connectionResult.message || connectionResult.status)}</span>}</div>
    </div>}
    {owner !== "cloud_rest" && <p className="hint-block">{t("Finish setup, then use the connection manager in the top bar to authorize and select the device.")}</p>}
  </div>;
}

function RuntimeStep({ choice, backend, settings, setup, models, locked, select, setBackend, patchLLM }: {
  choice: RuntimeChoice; backend: "auto" | "cpu" | "cuda"; settings: PublicSettings["llm"];
  setup: SetupStatus; models: LLMModelManagerSnapshot | null; locked: boolean;
  select: (choice: RuntimeChoice) => void; setBackend: (backend: "auto" | "cpu" | "cuda") => void;
  patchLLM: (patch: Partial<PublicSettings["llm"]>) => void;
}) {
  const runtimeReady = models?.runtime.installed && models.runtime.current;
  return <div className="setup-copy">
    <p>{t("The runtime generates chat replies and motion decisions. Voice models are configured separately.")}</p>
    <div className="setup-hardware"><span className="status-dot" data-state={setup.hardware.nvidia ? "ok" : "idle"} />{setup.hardware.nvidia ? t("Detected {gpu}", { gpu: setup.hardware.gpu_name ?? "NVIDIA GPU" }) : t("No NVIDIA GPU detected; CPU is the compatible managed option.")}</div>
    <div className="setup-choices">
      <Choice selected={choice === "managed"} title={t("Managed llama.cpp")} detail={t("App-owned, pinned, and checksum-verified. It avoids requiring Ollama or a compiler toolchain.")} badge={t("Recommended")} onSelect={() => select("managed")} />
      <Choice selected={choice === "ollama"} title={t("Use my existing Ollama")} detail={t("Uses no managed runtime disk. MagicHandy uses your existing Ollama service and model library.")} onSelect={() => select("ollama")} />
      <Choice selected={choice === "external"} title={t("External llama.cpp server")} detail={t("Use a compatible server you manage. MagicHandy will not install or own that process.")} onSelect={() => select("external")} />
      <Choice selected={choice === "skip"} title={t("Skip chat model setup")} detail={t("The app remains usable for manual, pattern, and video control.")} onSelect={() => select("skip")} />
    </div>
    {choice === "managed" && <div className="setup-subsection">
      <label className="field"><span className="label">{t("Runtime backend")}</span><select value={backend} disabled={locked} onChange={(event) => setBackend(event.target.value as typeof backend)}>{setup.llama_runtime.backends.map((value) => <option key={value} value={value}>{value === "auto" ? t("Automatic") : value.toUpperCase()}</option>)}</select></label>
      <p className="hint-block">{setup.llama_runtime.disk_estimate} {t("Official Windows bundles need no compiler or CUDA Toolkit. CUDA requires a compatible NVIDIA driver. License: {license}.", { license: setup.llama_runtime.license })}</p>
      <p className="setup-selection-state" data-ready={runtimeReady}>{runtimeReady ? t("Managed runtime is already installed and verified.") : t("Selected for installation after the voice step.")}</p>
    </div>}
    {choice === "ollama" && <div className="setup-subsection"><label className="field"><span className="label">{t("Ollama base URL")}</span><input value={settings.ollama_base_url} onChange={(event) => patchLLM({ ollama_base_url: event.target.value })} /></label></div>}
    {choice === "external" && <div className="setup-subsection"><label className="field"><span className="label">{t("Server base URL")}</span><input value={settings.llama_cpp_base_url} onChange={(event) => patchLLM({ llama_cpp_base_url: event.target.value })} /></label></div>}
  </div>;
}

function ModelStep({ choice, settings, models, ollamaModels, ggufPath, ggufName, locked, patch, setGGUFPath, setGGUFName, importGGUF, mergeImport, refreshOllama }: {
  choice: RuntimeChoice; settings: PublicSettings["llm"]; models: LLMModelManagerSnapshot | null; ollamaModels: OllamaModelInfo[];
  ggufPath: string; ggufName: string; locked: boolean; patch: (patch: Partial<PublicSettings["llm"]>) => void;
  setGGUFPath: (value: string) => void; setGGUFName: (value: string) => void; importGGUF: () => void;
  mergeImport: (job: LLMModelImport) => void; refreshOllama: () => void;
}) {
  if (choice === "skip") return <div className="setup-copy"><p>{t("No model will be configured. You can open Settings > Model at any time.")}</p></div>;
  if (choice === "managed") return <div className="setup-copy setup-model-library">
    <p>{t("Managed llama.cpp reads GGUF models copied into MagicHandy's checksummed model store.")}</p>
    <section className="setup-method" aria-labelledby="setup-managed-model-title">
      <header className="setup-method-head"><h2 id="setup-managed-model-title">{t("Managed model")}</h2></header>
      <div className="setup-method-body">
        {models?.models.filter((model) => model.state === "ready").length ? <label className="field"><span className="visually-hidden">{t("Managed model")}</span><select aria-label={t("Managed model")} value={settings.model} onChange={(event) => patch({ model: event.target.value })}><option value="">{t("Choose a model")}</option>{models.models.filter((model) => model.state === "ready").map((model) => <option key={model.id} value={model.id}>{model.display_name} · {formatBytes(model.size_bytes)}</option>)}</select></label> : <p className="setup-empty">{t("No managed models have been imported yet.")}</p>}
      </div>
    </section>
    <section className="setup-method" aria-labelledby="setup-gguf-import-title">
      <header className="setup-method-head"><h2 id="setup-gguf-import-title">{t("Import a GGUF file")}</h2></header>
      <div className="setup-method-body">
        <HostPathField label={t("GGUF model file")} value={ggufPath} kind="gguf" disabled={locked} onChange={setGGUFPath} />
        <label className="field"><span className="label">{t("Display name")}</span><input value={ggufName} disabled={locked} placeholder={t("Optional model name")} onChange={(event) => setGGUFName(event.target.value)} /></label>
        <button type="button" className="btn btn-secondary" disabled={locked || !ggufPath.trim()} onClick={importGGUF}>{t("Import GGUF")}</button>
      </div>
    </section>
    <section className="setup-method" aria-labelledby="setup-ollama-import-title">
      <header className="setup-method-head">
        <h2 id="setup-ollama-import-title">{t("Import from an existing Ollama library")}</h2>
        <p>{t("Choose the Ollama models folder. MagicHandy scans manifests first and copies only the model you select into its verified managed store.")}</p>
      </header>
      <div className="setup-method-body">
        <OllamaLibraryImport
          path={settings.ollama_models_path ?? ""}
          suggestedPath={models?.suggested_ollama_path}
          managedModels={models?.models ?? []}
          locked={locked}
          onPathChange={(ollama_models_path) => patch({ ollama_models_path })}
          onImportStarted={mergeImport}
        />
      </div>
    </section>
  </div>;
  if (choice === "ollama") return <div className="setup-copy">
    <p>{t("Choose a model exposed by your running Ollama service. Existing Ollama files are not copied for this provider.")}</p>
    <label className="field"><span className="label">{t("Ollama model")}</span><select value={settings.model} onChange={(event) => patch({ model: event.target.value })}><option value="">{t("Choose a model")}</option>{ollamaModels.map((model) => <option key={model.name} value={model.name}>{model.name} · {formatBytes(model.size_bytes)}</option>)}</select></label>
    <button type="button" className="btn btn-secondary" disabled={locked} onClick={refreshOllama}>{t("Refresh Ollama models")}</button>
    {!ollamaModels.length && <p className="hint-block">{t("No running Ollama service was found. You can finish setup and configure its path later in Settings > Model.")}</p>}
  </div>;
  return <div className="setup-copy">
    <p>{t("Enter the model identifier expected by your compatible llama.cpp server.")}</p>
    <label className="field"><span className="label">{t("Model")}</span><input value={settings.model} onChange={(event) => patch({ model: event.target.value })} /></label>
  </div>;
}

function VoiceStep({ setup, choice, device, autoLaunch, parakeetSelected, locked, setChoice, setDevice, setAutoLaunch, setParakeetSelected }: {
  setup: SetupStatus; choice: VoiceChoice; device: "cpu" | "cuda"; autoLaunch: boolean; parakeetSelected: boolean; locked: boolean;
  setChoice: (choice: VoiceChoice) => void; setDevice: (device: "cpu" | "cuda") => void; setAutoLaunch: (enabled: boolean) => void;
  setParakeetSelected: (selected: boolean) => void;
}) {
  const module = setup.voice_modules.find((item) => item.id === choice);
  return <div className="setup-copy">
    <p>{t("Voice is optional and stays disabled after installation until you configure a voice and press Start in Settings.")}</p>
    <h2>{t("Speech output")}</h2>
    <div className="setup-choices">
      <Choice selected={choice === "none"} title={t("No speech output")} detail={t("Use text chat only. This uses no model storage or VRAM.")} onSelect={() => setChoice("none")} />
      {setup.voice_modules.map((item) => <Choice key={item.id} selected={choice === item.id} title={item.name} detail={`${item.summary} ${item.reference_requirement}`} badge={item.recommended_for_nvidia ? t("Recommended for NVIDIA") : undefined} disabled={item.id === "faster-qwen3-tts" && !setup.hardware.nvidia} onSelect={() => setChoice(item.id as VoiceChoice)} />)}
      <Choice selected={choice === "external"} title={t("Existing compatible voice server")} detail={t("Configure its URL, model, and optional key later in Settings > Voice.")} onSelect={() => setChoice("external")} />
    </div>
    {module && <div className="setup-subsection">
      {module.supported_devices.length > 1 && <label className="field"><span className="label">{t("Execution device")}</span><select value={device} disabled={locked} onChange={(event) => setDevice(event.target.value as typeof device)}>{module.supported_devices.map((value) => <option key={value} value={value}>{value.toUpperCase()}</option>)}</select></label>}
      <label className="toggle-line"><span className="toggle"><input type="checkbox" checked={autoLaunch} disabled={locked} onChange={(event) => setAutoLaunch(event.target.checked)} /><span className="track" aria-hidden="true" /></span><span>{t("Launch the local voice server with MagicHandy")}<small>{module.disk_estimate}</small></span></label>
      <p className="hint-block">{t("Code license: {code}. Model license: {model}.", { code: module.license, model: module.model_license })}</p>
      <p className="setup-selection-state">{t("Selected for installation on the next step.")}</p>
    </div>}
    <div className="setup-divider" />
    <h2>{t("Speech input")}</h2>
    <div className="setup-choices">
      <Choice selected={!parakeetSelected} title={t("No speech input")} detail={t("Keep microphone transcription off. It can be added later in Voice settings.")} onSelect={() => setParakeetSelected(false)} />
      <Choice selected={parakeetSelected} title={setup.parakeet.name} detail={`${setup.parakeet.summary} ${t("Download: {size}.", { size: setup.parakeet.download_size })}`} onSelect={() => setParakeetSelected(true)} />
    </div>
    {parakeetSelected && <p className="hint-block">{t("Runner license: {runner}; model license: {model}.", { runner: setup.parakeet.runner_license, model: setup.parakeet.model_license })}</p>}
  </div>;
}

function InstallStep({ job, submitted, runtimeChoice, voiceChoice, parakeetSelected, cancel, retry }: {
  job?: SetupJob; submitted: boolean; runtimeChoice: RuntimeChoice; voiceChoice: VoiceChoice; parakeetSelected: boolean;
  cancel: () => void; retry: () => void;
}) {
  if (!submitted) return <div className="setup-copy"><p>{t("Preparing the installation plan...")}</p></div>;
  if (!job) return <div className="setup-copy">
    <p>{t("No selected component needs installation. Existing services and skipped features were left unchanged.")}</p>
    <dl className="setup-summary">
      <div><dt>{t("Model runtime")}</dt><dd>{translateKnown(runtimeChoice)}</dd></div>
      <div><dt>{t("Speech output")}</dt><dd>{translateKnown(voiceChoice)}</dd></div>
      <div><dt>{t("Speech input")}</dt><dd>{parakeetSelected ? t("Parakeet") : t("Not selected")}</dd></div>
    </dl>
  </div>;
  return <div className="setup-copy">
    <p>{t("MagicHandy is installing and verifying the selected local components. You can leave this page open; the backend owns the queue.")}</p>
    <SetupJobPanel job={job} cancel={cancel} />
    {(job.status === "failed" || job.status === "cancelled") && <button type="button" className="btn btn-primary" onClick={retry}>{t("Retry installation")}</button>}
  </div>;
}

function FinishStep({ setup, settings, models, runtimeChoice, voiceChoice, parakeetSelected }: {
  setup: SetupStatus; settings: PublicSettings; models: LLMModelManagerSnapshot | null;
  runtimeChoice: RuntimeChoice; voiceChoice: VoiceChoice; parakeetSelected: boolean;
}) {
  const selectedModel = models?.models.find((model) => model.id === settings.llm.model);
  const runtimeSummary = runtimeChoice === "skip"
    ? t("Skipped; chat and Autopilot remain unavailable")
    : runtimeChoice === "managed"
      ? t("Managed llama.cpp, verified with {model}", { model: selectedModel?.display_name || settings.llm.model })
      : `${runtimeChoice === "ollama" ? "Ollama" : "External llama.cpp"} | ${settings.llm.model}`;
  return <div className="setup-copy">
    <p>{t("Your choices are saved. Skipped features remain available from Settings without rerunning the Windows installer.")}</p>
    <dl className="setup-summary">
      <div><dt>{t("Data folder")}</dt><dd>{setup.data_dir}</dd></div>
      <div><dt>{t("Model runtime")}</dt><dd>{runtimeSummary}</dd></div>
      <div><dt>{t("Speech output")}</dt><dd>{translateKnown(voiceChoice)}</dd></div>
      <div><dt>{t("Speech input")}</dt><dd>{parakeetSelected ? t("Parakeet installed") : t("Not selected")}</dd></div>
      <div><dt>{t("Local address")}</dt><dd>{window.location.origin}</dd></div>
    </dl>
    <div className="setup-notice"><strong>{t("Before commanding motion")}</strong><span>{t("Connect The Handy, confirm the active transport, and review speed and stroke limits in the top-bar connection manager.")}</span></div>
  </div>;
}

function Choice({ selected, title, detail, badge, disabled, onSelect }: { selected: boolean; title: string; detail: string; badge?: string; disabled?: boolean; onSelect: () => void }) {
  return <label className="setup-choice" data-selected={selected} data-disabled={disabled || undefined}>
    <input type="radio" checked={selected} disabled={disabled} onChange={onSelect} />
    <span className="setup-choice-copy"><strong>{title}</strong><small>{detail}</small></span>
    {badge && <span className="setup-badge">{badge}</span>}
  </label>;
}

function SetupJobPanel({ job, cancel }: { job: SetupJob; cancel: () => void }) {
  const active = activeJob(job);
  const completed = job.completed_steps ?? 0;
  const total = Math.max(job.total_steps ?? job.steps?.length ?? 0, 1);
  return <section className="setup-job" aria-live="polite" aria-busy={active}>
    <div><span className="status-dot" data-state={job.status === "complete" ? "ok" : job.status === "failed" ? "error" : active ? "working" : "idle"} /><strong>{translateKnown(job.message)}</strong></div>
    <progress className="setup-install-progress" max={total} value={Math.min(completed, total)} aria-label={t("Installation progress")} />
    {job.steps && <ol className="setup-install-steps">{job.steps.map((item) => <li key={item.id} data-state={item.status}><span className="status-dot" data-state={item.status === "complete" ? "ok" : item.status === "failed" ? "error" : item.status === "running" ? "working" : "idle"} /><span><strong>{item.label}</strong>{item.message && <small>{translateKnown(item.message)}</small>}</span></li>)}</ol>}
    <div className="setup-terminal" role="log" aria-label={t("Installation terminal output")}><pre>{job.output || t("Waiting for installer output...")}</pre></div>
    {active && <button type="button" className="btn btn-secondary" onClick={cancel}>{t("Cancel installation")}</button>}
  </section>;
}

function promptSet(locale: string): string {
  return ({
    en: "magichandy_motion_v1",
    es: "magichandy_motion_v1_es",
    "pt-BR": "magichandy_motion_v1_pt_br",
    "zh-Hans": "magichandy_motion_v1_zh_hans",
    ja: "magichandy_motion_v1_ja",
  } as Record<string, string>)[locale] ?? "magichandy_motion_v1";
}

function promptLocale(value: string, fallback: string): string {
  return ({
    magichandy_motion_v1: "en",
    magichandy_motion_v1_es: "es",
    magichandy_motion_v1_pt_br: "pt-BR",
    magichandy_motion_v1_zh_hans: "zh-Hans",
    magichandy_motion_v1_ja: "ja",
  } as Record<string, string>)[value] ?? fallback;
}
