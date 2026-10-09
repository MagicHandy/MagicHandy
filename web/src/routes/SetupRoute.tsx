import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { LLMCatalog } from "../api/catalog-types";
import type {
  ConnectionCheckResult,
  LLMModelImport,
  LLMModelManagerSnapshot,
  OllamaModelInfo,
  PublicSettings,
  SetupInstallPlan,
  SetupStatus,
  NetworkStatus,
} from "../api/types";
import { SetupFailureReport } from "../components/SetupFailureReport";
import { networkScope, type NetworkScope } from "../components/NetworkSetupFields";
import { SetupChatStep, catalogChoiceModel, type ManagedModelChoice, type RuntimeBackend, type RuntimeChoice } from "../components/SetupChatStep";
import { SetupBackupStep } from "../components/SetupBackupStep";
import { EasySetupStep, SetupModeChoice } from "../components/EasySetupStep";
import { EasyAISetup, initialEasyAIChoice, setupCloudConnectionID, type EasyAIChoice } from "../components/EasyAISetup";
import type { ModelConnection } from "../api/cloud-types";
import { newModelConnection } from "../util/model-connections";
import { setupRequiredBytes } from "../util/setup-space";
import {
  AccessStep,
  DeviceStep,
  FinishStep,
  InstallStep,
  VoiceStep,
  WelcomeStep,
  activeSetupJob,
  promptLocale,
  type AccessChoice,
  type HandyModel,
  type VoiceChoice,
} from "../components/SetupSteps";
import { t, translateKnown } from "../i18n";
import { useAppState, useToast } from "../state/app-state";
import { formatBytes } from "../util/format";
import { passwordMeetsMinimum } from "../util/password";
import { useAuth } from "../state/auth";

const CUSTOM_STEPS = ["welcome", "access", "device", "chat", "backup", "voice", "install", "finish"] as const;
// Easy Setup assesses the computer and asks its questions on one page.
const EASY_STEPS = ["welcome", "easy", "backup", "install", "finish"] as const;
type SetupStep = (typeof CUSTOM_STEPS)[number] | (typeof EASY_STEPS)[number];
type SetupMode = "easy" | "custom";

function setupStepLabel(step: SetupStep): string {
  if (step === "welcome") return t("Welcome");
  if (step === "easy") return t("Your setup");
  if (step === "access") return t("Access");
  if (step === "device") return t("Device");
  if (step === "chat") return t("Chat AI");
  if (step === "backup") return t("Local backup (optional)");
  if (step === "voice") return t("Voice (optional)");
  if (step === "install") return t("Install selected features");
  return t("Finish");
}


const message = (error: unknown) => error instanceof Error ? translateKnown(error.message) : t("Request failed");
const activeJob = activeSetupJob;
const activeModelImport = (job: LLMModelImport) => job.status === "queued" || job.status === "copying" || job.status === "downloading";

function initialRuntimeChoice(settings?: PublicSettings["llm"]): RuntimeChoice {
  if (!settings) return "skip";
  if (settings.conversation_connection_id && settings.conversation_connection_id !== "local") return "hosted";
  if (settings.provider === "ollama") return "ollama";
  if (settings.provider === "llama_cpp" && settings.llama_cpp_mode === "managed") return "managed";
  if (settings.provider === "llama_cpp") return "external";
  return "skip";
}

function initialHandyModel(settings?: PublicSettings | null): HandyModel {
  const saved = settings?.motion?.handy_model;
  return saved === "handy_2_standard" || saved === "handy_2_pro" ? saved : "handy_original";
}

export function SetupRoute() {
  const auth = useAuth();
  const { state, backendOnline, readOnly, refresh } = useAppState();
  const { show } = useToast();
  const [step, setStep] = useState(0);
  const [mode, setMode] = useState<SetupMode>("easy");
  const easyDefaultsApplied = useRef(false);
  const [setup, setSetup] = useState<SetupStatus | null>(null);
  const [settings, setSettings] = useState<PublicSettings | null>(state?.settings ?? null);
  const [models, setModels] = useState<LLMModelManagerSnapshot | null>(null);
  const [catalog, setCatalog] = useState<LLMCatalog | null>(null);
  const [ollamaModels, setOllamaModels] = useState<OllamaModelInfo[]>([]);
  const [runtimeChoice, setRuntimeChoice] = useState<RuntimeChoice>(() => initialRuntimeChoice(state?.settings?.llm));
  const [easyAIChoice, setEasyAIChoice] = useState<EasyAIChoice>(() => initialEasyAIChoice(state?.settings?.llm));
  const [backupChoice, setBackupChoice] = useState<boolean | null>(() => state?.settings?.llm.retry_refusal_locally ? true : null);
  const [backupRuntime, setBackupRuntime] = useState<RuntimeChoice>(() => state?.settings?.llm.provider === "ollama" ? "ollama" : state?.settings?.llm.llama_cpp_mode === "external" ? "external" : "managed");
  const [backupChecked, setBackupChecked] = useState(false);
  const [easyCombine, setEasyCombine] = useState(() => Boolean(state?.settings?.llm.motion_planner?.provider === "connection" && (!state.settings.llm.conversation_connection_id || state.settings.llm.conversation_connection_id === "local")));
  const [easyHostedReady, setEasyHostedReady] = useState(false);
  const [easyConnectionID, setEasyConnectionID] = useState(() => setupCloudConnectionID(state?.settings?.llm));
  const [modelChoice, setModelChoice] = useState<ManagedModelChoice>("later");
  const chatDefaultsApplied = useRef(false);
  const [pendingImportID, setPendingImportID] = useState("");
  const [runtimeBackend, setRuntimeBackend] = useState<RuntimeBackend>("auto");
  const [handyModel, setHandyModel] = useState<HandyModel>(() => initialHandyModel(state?.settings));
  const [voiceChoice, setVoiceChoice] = useState<VoiceChoice>("none");
  const [voiceDevice, setVoiceDevice] = useState<"cpu" | "cuda">("cpu");
  const [voiceAutoLaunch, setVoiceAutoLaunch] = useState(true);
  const [voiceEnableAfterInstall, setVoiceEnableAfterInstall] = useState(true);
  const [qwenWAV, setQwenWAV] = useState(state?.settings?.voice?.tts_reference_wav ?? "");
  const [qwenTranscript, setQwenTranscript] = useState(state?.settings?.voice?.tts_reference_text ?? "");
  const [qwenLater, setQwenLater] = useState(false);
  const [parakeetSelected, setParakeetSelected] = useState(false);
  const parakeetSelectionInitialized = useRef(false);
  const [accessChoice, setAccessChoice] = useState<AccessChoice>(auth.status?.initialized ? "protected" : "local");
  const [administratorUsername, setAdministratorUsername] = useState("");
  const [administratorPassword, setAdministratorPassword] = useState("");
  const [administratorConfirmation, setAdministratorConfirmation] = useState("");
  const [createdAdministrator, setCreatedAdministrator] = useState(false);
  const [accessScope, setAccessScope] = useState<NetworkScope>("local");
  const [originalScope, setOriginalScope] = useState<NetworkScope>("local");
  const [networkReady, setNetworkReady] = useState(false);
  const [networkLoaded, setNetworkLoaded] = useState(false);
  const [setupNetworkStatus, setSetupNetworkStatus] = useState<NetworkStatus | null>(null);
  const [connectionKey, setConnectionKey] = useState("");
  const [connectionResult, setConnectionResult] = useState<ConnectionCheckResult | null>(null);
  const [ggufPath, setGGUFPath] = useState("");
  const [ggufName, setGGUFName] = useState("");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [installJobID, setInstallJobID] = useState("");
  const [installSubmitted, setInstallSubmitted] = useState(false);
  const mounted = useRef(true);
  const setupBody = useRef<HTMLDivElement>(null);

  const hostedChat = Boolean(settings?.llm.conversation_connection_id && settings.llm.conversation_connection_id !== "local");
  const needsBackup = hostedChat && backupChoice === true;
  const localRuntime = needsBackup ? backupRuntime : runtimeChoice;
  const steps: readonly SetupStep[] = (mode === "easy" ? EASY_STEPS : CUSTOM_STEPS).filter(item => item !== "backup" || hostedChat);
  const stepsRef = useRef(steps);
  stepsRef.current = steps;
  const locked = !backendOnline || readOnly || !settings || !setup || Boolean(busy);
  const installationActive = activeJob(setup?.installation);
  const installJob = setup?.installation?.id === installJobID ? setup.installation : undefined;
  const activeImport = models?.imports.find(activeModelImport);
  const currentStep = steps[step];
  const networkRequired = accessScope !== "local" || originalScope !== "local";
  useEffect(() => {
    if (currentStep !== "access" || networkLoaded) return;
    const abort = new AbortController();
    void api.networkStatus(abort.signal).then((status) => {
      if (abort.signal.aborted) return;
      const scope = networkScope(status.saved ?? status.active);
      setSetupNetworkStatus(status);
      setAccessScope(scope); setOriginalScope(scope); setNetworkLoaded(true);
      if (scope !== "local") setAccessChoice("protected");
    }).catch((reason: unknown) => { if (!abort.signal.aborted) setError(message(reason)); });
    return () => abort.abort();
  }, [currentStep, networkLoaded]);

  const loadCatalog = useCallback(async () => {
    try {
      const next = await api.llmCatalog();
      if (!Array.isArray(next?.models)) throw new Error("catalog unavailable");
      if (mounted.current) setCatalog(next);
    } catch {
      // The curated list is optional: imports and "add a model later" still
      // work, and an empty list still lets the chat defaults settle.
      if (mounted.current) setCatalog({ models: [], hardware: { nvidia: false } });
    }
  }, []);

  const load = useCallback(async () => {
    try {
      const [setupStatus, modelStatus] = await Promise.all([api.setupStatus(), api.llmModels()]);
      if (!mounted.current) return;
      setSetup(setupStatus);
      setModels(modelStatus);
      if (setupStatus.installation?.kind === "install_plan" && (setupStatus.required || activeJob(setupStatus.installation))) {
        setInstallJobID(setupStatus.installation.id);
        setInstallSubmitted(true);
        setStep(stepsRef.current.indexOf("install"));
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
    void loadCatalog();
    return () => { mounted.current = false; };
  }, [load, loadCatalog]);

  useEffect(() => {
    if (!state?.settings || settings) return;
    setSettings(state.settings);
    setRuntimeChoice(initialRuntimeChoice(state.settings.llm));
    setEasyAIChoice(initialEasyAIChoice(state.settings.llm));
    setBackupChoice(state.settings.llm.retry_refusal_locally ? true : null);
    setBackupRuntime(state.settings.llm.provider === "ollama" ? "ollama" : state.settings.llm.llama_cpp_mode === "external" ? "external" : "managed");
    setEasyConnectionID(setupCloudConnectionID(state.settings.llm));
    setHandyModel(initialHandyModel(state.settings));
    setQwenWAV(state.settings.voice?.tts_reference_wav ?? "");
    setQwenTranscript(state.settings.voice?.tts_reference_text ?? "");
  }, [settings, state?.settings]);

  useEffect(() => {
    if (!setup?.hardware.nvidia) return;
    setVoiceDevice("cuda");
  }, [setup?.hardware.nvidia]);

  useEffect(() => {
    if (!setup || parakeetSelectionInitialized.current) return;
    parakeetSelectionInitialized.current = true;
    setParakeetSelected(setup.parakeet.preselected);
  }, [setup]);

  // Pick the chat defaults once everything they depend on has loaded: keep a
  // model the user already has, otherwise offer the tested download when the
  // GPU can hold it. Without an NVIDIA card, chat setup starts skipped.
  useEffect(() => {
    if (chatDefaultsApplied.current || !settings || !setup || !models || !catalog) return;
    chatDefaultsApplied.current = true;
    const ready = models.models.filter((model) => model.state === "ready");
    const installedCatalog = catalog.models.find((model) => model.installed_model_id && model.installed_model_id === settings.llm.model);
    if (installedCatalog) {
      setModelChoice(`download:${installedCatalog.id}`);
      return;
    }
    if (ready.some((model) => model.id === settings.llm.model)) {
      setModelChoice("store");
      return;
    }
    if (!setup.hardware.nvidia && ready.length === 0 && initialRuntimeChoice(settings.llm) === "managed") {
      setRuntimeChoice("skip");
      return;
    }
    // The backend marks the first curated model this GPU holds as recommended;
    // the tuned default covers a card whose memory could not be read.
    const preferred = catalog.models.find((model) => model.fit === "recommended") ?? catalog.models.find((model) => model.default);
    // Choosing an installed catalog entry never rewrites the saved model here;
    // the Chat AI step's save applies its store ID only for a managed engine.
    if (preferred && (preferred.installed_model_id || preferred.fit === "recommended" || preferred.fit === "unknown_vram")) {
      setModelChoice(`download:${preferred.id}`);
    }
  }, [catalog, models, settings, setup]);

  // Easy Setup takes the assessment's choices: the recommended model (unless a
  // ready model is already selected), the matching runtime, and voice off
  // until the user turns it on.
  useEffect(() => {
    const assessment = setup?.assessment;
    if (mode !== "easy" || easyDefaultsApplied.current || !assessment || !models || !catalog || !settings) return;
    easyDefaultsApplied.current = true;
    setRuntimeBackend("auto");
    setVoiceChoice("none");
    setParakeetSelected(false);
    setVoiceDevice(assessment.voice_device === "cuda" ? "cuda" : "cpu");
    setVoiceAutoLaunch(true);
    setVoiceEnableAfterInstall(true);
    if (initialEasyAIChoice(settings.llm) !== "local") return;
    if (settings.llm.provider === "ollama" || settings.llm.llama_cpp_mode === "external") return;
    if ((assessment.chat.status !== "unmet" && assessment.model_id) || assessment.model_installed_id) {
      setRuntimeChoice("managed");
      patchLLM({ provider: "llama_cpp", llama_cpp_mode: "managed", conversation_connection_id: "local", ...(assessment.model_installed_id ? { model: assessment.model_installed_id } : {}) });
      setModelChoice(assessment.model_installed_id ? "store" : `download:${assessment.model_id}`);
    } else {
      setRuntimeChoice("skip");
    }
  }, [catalog, mode, models, settings, setup?.assessment]);

  const changeMode = (next: SetupMode) => {
    if (next === "easy") easyDefaultsApplied.current = false;
    setMode(next);
  };

  useEffect(() => {
    if (!installationActive && !activeImport) return;
    const timer = window.setInterval(() => {
      void load();
      if (activeImport) void api.llmModels().then((snapshot) => mounted.current && setModels(snapshot));
    }, 1000);
    return () => window.clearInterval(timer);
  }, [activeImport, installationActive, load]);

  // A finished plan may have downloaded and selected a model on the backend.
  // Adopt the saved settings so a later Continue cannot write stale choices.
  const installStatus = installJob?.status;
  useEffect(() => {
    if (installStatus !== "complete") return;
    void api.getSettings().then((response) => { if (mounted.current) setSettings(response.settings); }).catch(() => undefined);
    void api.llmModels().then((snapshot) => { if (mounted.current) setModels(snapshot); }).catch(() => undefined);
    void loadCatalog();
  }, [installStatus, loadCatalog]);

  // Select a model the moment its import finishes.
  useEffect(() => {
    if (!pendingImportID) return;
    const job = models?.imports.find((item) => item.id === pendingImportID);
    if (!job || activeModelImport(job)) return;
    setPendingImportID("");
    if (job.status === "complete" && job.model_id) {
      patchLLM({ model: job.model_id });
      setModelChoice("store");
      void loadCatalog();
    }
  }, [loadCatalog, models, pendingImportID]);

  useEffect(() => {
    if (runtimeChoice === "ollama" || (currentStep === "backup" && backupChoice && backupRuntime === "ollama")) void loadOllama();
  }, [loadOllama, runtimeChoice, currentStep, backupChoice, backupRuntime]);

  useEffect(() => {
    for (const element of [document.getElementById("workspace"), setupBody.current]) {
      if (element) { element.scrollTop = 0; element.scrollLeft = 0; }
    }
  }, [step]);

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

  function patchLLM(patch: Partial<PublicSettings["llm"]>) {
    setBackupChecked(false);
    setSettings((current) => current ? { ...current, llm: { ...current.llm, ...patch } } : current);
  }

  function selectEasyAI(choice: EasyAIChoice, combine?: boolean, provider?: ModelConnection["provider"]) {
    if (!settings || !setup?.assessment) return;
    const existingLocal = settings.llm.provider === "ollama" || settings.llm.llama_cpp_mode === "external";
    const localAvailable = setup.assessment.chat.status !== "unmet" || existingLocal;
    const nextCombine = choice !== "local" && localAvailable && (combine ?? false);
    const connections = settings.llm.connections ?? [];
    const nextProvider = choice === "chatgpt" ? "chatgpt" : provider ?? "openrouter";
    const connection = choice === "local" ? undefined : connections.find(item => item.id === easyConnectionID && item.provider === nextProvider) ?? connections.find(item => item.provider === nextProvider) ?? newModelConnection(nextProvider, connections);
    setEasyAIChoice(choice); setEasyCombine(nextCombine);
    if (connection?.id !== easyConnectionID || choice === "local") setEasyHostedReady(false);
    setEasyConnectionID(connection?.id ?? "");
    const useLocal = choice === "local" || nextCombine;
    const localRuntime = settings.llm.provider === "ollama" ? "ollama" : settings.llm.llama_cpp_mode === "external" ? "external" : "managed";
    setRuntimeChoice(useLocal ? localAvailable ? localRuntime : "skip" : "hosted");
    const assessment = setup.assessment;
    if (useLocal && localRuntime === "managed" && assessment.model_id) setModelChoice(assessment.model_installed_id ? "store" : `download:${assessment.model_id}`);
    patchLLM({
      ...(connection && !connections.some(item => item.id === connection.id) ? { connections: [...connections, connection] } : {}),
      ...(useLocal && localRuntime === "managed" ? { provider: "llama_cpp", llama_cpp_mode: "managed", ...(assessment.model_installed_id ? { model: assessment.model_installed_id } : {}) } : {}),
      conversation_connection_id: useLocal ? "local" : connection?.id,
      motion_planner: nextCombine ? { provider: "connection", connection_id: connection?.id, model: "", context_policy: "technical" } : { provider: "conversation", model: "", context_policy: "conversation" },
    });
  }

  const saveCurrentStep = async () => {
    if (!settings) return;
    if (currentStep === "welcome") {
      await savePreferences({
        ui_locale: settings.ui?.locale ?? "en",
        chat_locale: promptLocale(settings.llm.prompt_set, settings.ui?.locale ?? "en"),
      });
    } else if (currentStep === "access" && accessChoice === "protected" && !auth.status?.initialized && !createdAdministrator) {
      if (!passwordMeetsMinimum(administratorPassword)) {
        throw new Error(t("Use a password or passphrase of at least 15 characters."));
      }
      if (administratorPassword !== administratorConfirmation) {
        throw new Error(t("The passwords do not match."));
      }
      await auth.bootstrap(administratorUsername.trim(), administratorPassword);
      setCreatedAdministrator(true);
      setAdministratorPassword("");
      setAdministratorConfirmation("");
    } else if (currentStep === "device") {
      await savePreferences({
        device_owner: settings.device.hsp_dispatch_owner,
        handy_model: handyModel,
        ...(connectionKey.trim() ? { connection_key: connectionKey.trim() } : {}),
      });
      setConnectionKey("");
    } else if (currentStep === "chat" && runtimeChoice !== "skip") {
      // A catalog model already in the store is used as is, not downloaded.
      const installed = runtimeChoice === "managed" ? selectedCatalogModel?.installed_model_id : undefined;
      await savePreferences({ llm: installed ? { ...settings.llm, model: installed } : settings.llm });
    } else if (currentStep === "backup") {
      const installed = backupChoice && backupRuntime === "managed" ? selectedCatalogModel?.installed_model_id : undefined;
      await savePreferences({ llm: { ...settings.llm, retry_refusal_locally: backupChoice === true, ...(installed ? { model: installed } : {}) } });
    } else if (currentStep === "easy") {
      const installed = runtimeChoice === "managed" ? selectedCatalogModel?.installed_model_id : undefined;
      await savePreferences({ llm: installed ? { ...settings.llm, model: installed } : settings.llm });
      if (connectionKey.trim()) {
        await savePreferences({ device_owner: "cloud_rest", handy_model: handyModel, connection_key: connectionKey.trim() });
        setConnectionKey("");
      }
    }
  };

  const continueStep = () => void run("continue", async () => {
    await saveCurrentStep();
    if (currentStep === "access" && networkRequired && !networkReady) return;
    if (currentStep === "easy" && hostedChat) { setStep(steps.indexOf("backup")); return; }
    if (currentStep === "voice" || currentStep === "easy" || (currentStep === "backup" && mode === "easy")) {
      await beginInstall();
      const plan = installPlan();
      if (mode === "easy" && !plan.llama && !plan.model && !plan.voice && !plan.parakeet) {
        setStep(steps.indexOf("finish"));
        return;
      }
    }
    setStep((current) => Math.min(steps.length - 1, current + 1));
  });

  // Skipping the chat step skips chat entirely. Keeping the engine while
  // adding a model later is an explicit choice inside the step, so a skip can
  // no longer drop a runtime the user selected.
  const skipStep = () => {
    setError("");
    if (currentStep === "chat") {
      setRuntimeChoice("skip");
      patchLLM({ conversation_connection_id: "local", motion_planner: { provider: "conversation", model: "", context_policy: "conversation" } });
      setStep(steps.filter(item => item !== "backup").indexOf("voice"));
      return;
    }
    if (currentStep === "voice") {
      setVoiceChoice("none");
      setParakeetSelected(false);
      void run("continue", async () => {
        await beginInstall("none", false);
        setStep(steps.indexOf("install"));
      });
      return;
    }
    setStep((current) => Math.min(steps.length - 1, current + 1));
  };

  const selectRuntime = (choice: RuntimeChoice) => {
    setRuntimeChoice(choice);
    if (choice !== "hosted") patchLLM({ conversation_connection_id: "local", motion_planner: { provider: "conversation", model: "", context_policy: "conversation" } });
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
    const response = await api.importGGUFModel(ggufPath.trim(), ggufName.trim());
    setPendingImportID(response.import.id);
    setGGUFPath("");
    setGGUFName("");
    setModels(await api.llmModels());
  });

  const mergeImport = (job: LLMModelImport) => {
    setPendingImportID(job.id);
    setModels((current) => current ? {
      ...current,
      imports: [job, ...current.imports.filter((item) => item.id !== job.id)],
    } : current);
  };

  const selectedCatalogModel = localRuntime === "managed" ? catalogChoiceModel(modelChoice, catalog) : undefined;

  function selectEasyVoice(choice: VoiceChoice) {
    setVoiceChoice(choice);
    const option = setup?.assessment?.voice_options?.find((item) => item.module === choice);
    if (option) setVoiceDevice(option.device);
  }

  function installPlan(nextVoiceChoice = voiceChoice, nextParakeet = parakeetSelected): SetupInstallPlan {
    const plan: SetupInstallPlan = { parakeet: nextParakeet };
    if (localRuntime === "managed" && !(models?.runtime.installed && models.runtime.current)) {
      plan.llama = { backend: runtimeBackend };
    }
    if (selectedCatalogModel && !selectedCatalogModel.installed_model_id) {
      plan.model = { catalog_id: selectedCatalogModel.id };
    }
    const module = setup?.voice_modules.find((item) => item.id === nextVoiceChoice);
    if (module) {
      plan.voice = {
        module: module.id,
        device: module.id === "faster-qwen3-tts" ? "cuda" : voiceDevice,
        auto_launch: voiceAutoLaunch,
        ...(module.id === "faster-qwen3-tts" && !qwenLater ? { reference: { wav: qwenWAV.trim(), transcript: qwenTranscript.trim() } } : {}),
      };
    }
    if (voiceEnableAfterInstall && (module?.ready_after_install || plan.voice?.reference || nextParakeet)) plan.enable_voice = true;
    return plan;
  }

  async function beginInstall(nextVoiceChoice = voiceChoice, nextParakeet = parakeetSelected) {
    const plan = installPlan(nextVoiceChoice, nextParakeet);
    setInstallSubmitted(true);
    if (!plan.llama && !plan.model && !plan.voice && !plan.parakeet) {
      setInstallJobID("");
      return;
    }
    const response = await api.installSetupPlan(plan);
    setInstallJobID(response.installation.id);
    setSetup((current) => current ? { ...current, installation: response.installation } : current);
  }

  const retryInstall = () => void run("retry", async () => {
    const response = await api.retrySetupPlan();
    setInstallSubmitted(true);
    setInstallJobID(response.installation.id);
    setSetup(current => current ? { ...current, installation: response.installation } : current);
  });

  const verifyCloud = () => void run("verify", async () => {
    if (!settings) return;
    await savePreferences({
      device_owner: "cloud_rest",
      ...(connectionKey.trim() ? { connection_key: connectionKey.trim() } : {}),
    });
    setConnectionKey("");
    setConnectionResult(await api.connectionCheck("cloud"));
  });

  const managedModels = models?.models.filter((model) => model.state === "ready") ?? [];
  const managedModelReady = managedModels.some((model) => model.id === settings?.llm.model);
  const runtimeInstalled = Boolean(models?.runtime.installed && models.runtime.current);
  const managedModelPending = runtimeChoice === "managed" && !managedModelReady && (modelChoice === "later" || Boolean(selectedCatalogModel));
  const chatChoiceReady = runtimeChoice === "skip"
    || (runtimeChoice === "hosted" && Boolean(settings?.llm.connections?.some(connection => connection.id === settings.llm.conversation_connection_id && connection.model.trim())))
    || (runtimeChoice === "managed" && (modelChoice === "later" || Boolean(selectedCatalogModel) || managedModelReady))
    || ((runtimeChoice === "ollama" || runtimeChoice === "external") && Boolean(settings?.llm.model.trim()));
  const installationReady = installSubmitted && (!installJob || installJob.status === "complete");
  const accessReady = accessChoice === "local" || createdAdministrator || Boolean(auth.status?.initialized) || (
    Boolean(administratorUsername.trim()) &&
    passwordMeetsMinimum(administratorPassword) &&
    administratorPassword === administratorConfirmation
  );
  const qwenReferenceReady = voiceChoice !== "faster-qwen3-tts" || qwenLater || Boolean(qwenWAV.trim() && qwenTranscript.trim());
  const easyAIReady = easyAIChoice === "local" || easyHostedReady;
  const existingLocal = Boolean(settings?.llm.model && (settings.llm.provider === "ollama" || settings.llm.llama_cpp_mode === "external"));
  const localAvailable = existingLocal || Boolean(setup?.assessment && setup.assessment.chat.status !== "unmet");
  const easyBytes = setup?.assessment ? setupRequiredBytes(setup.assessment, runtimeChoice === "managed", voiceChoice, parakeetSelected) : 0;
  const easySpaceReady = !setup?.assessment?.free_disk_bytes || easyBytes <= setup.assessment.free_disk_bytes;
  const easyPlan = installPlan();
  const easyNeedsInstall = Boolean(easyPlan.llama || easyPlan.model || easyPlan.voice || easyPlan.parakeet);
  const backupModelReady = backupRuntime === "managed" ? Boolean(selectedCatalogModel) || managedModelReady : backupRuntime === "ollama" ? ollamaModels.some(model => model.name === settings?.llm.model) : Boolean(settings?.llm.model.trim());
  const backupBytes = easyBytes + (backupRuntime === "managed" && selectedCatalogModel && !selectedCatalogModel.installed_model_id ? Math.max(selectedCatalogModel.size_bytes + 512 * 2 ** 20, setup?.assessment?.chat.bytes ?? 0) : 0);
  const backupSpaceReady = !setup?.assessment?.free_disk_bytes || backupBytes <= setup.assessment.free_disk_bytes;
  const backupStepReady = backupChoice === false || (backupChoice === true && backupModelReady && backupSpaceReady);
  const currentStepReady = (currentStep !== "backup" || backupStepReady) && (currentStep !== "easy" || (easyAIReady && Boolean(setup?.assessment) && easySpaceReady)) && ((currentStep !== "easy" && currentStep !== "voice") || qwenReferenceReady) && (currentStep !== "chat" || chatChoiceReady) && (currentStep !== "access" || (accessReady && networkLoaded && (!networkRequired || (!auth.status?.initialized && !createdAdministrator) || networkReady)));
  const canFinish = (!needsBackup || backupChecked) && (runtimeChoice === "skip" || (runtimeChoice === "managed"
    ? runtimeInstalled && (managedModelReady || modelChoice === "later")
    : chatChoiceReady)) && (mode !== "easy" || easyAIReady);
  const requiresSignInAfterSetup = createdAdministrator || Boolean(
    auth.status?.authentication_required && auth.status.authenticated && !settings?.ui?.setup_completed,
  );

  const checkBackup = () => void run("backup-check", async () => {
    if (currentStep === "backup") await saveCurrentStep();
    const result = await api.checkSetupLocalModel();
    if (!result.ready) throw new Error(result.message);
    setBackupChecked(true);
  });

  const skipBackup = () => void run("skip-backup", async () => {
    if (!settings) return;
    await savePreferences({ llm: { ...settings.llm, retry_refusal_locally: false } });
    setBackupChoice(false);
  });

  const finish = () => void run("finish", async () => {
    const result = await api.completeSetup(runtimeChoice === "skip" || (runtimeChoice === "managed" && !managedModelReady));
    window.location.hash = "#/chat";
    if (result.signed_out) await auth.refresh();
    else await refresh();
  });

  const titles: Record<SetupStep, string> = {
    welcome: t("Set up MagicHandy"),
    access: t("Choose who can open MagicHandy"),
    device: t("Choose how MagicHandy reaches your device"),
    chat: t("Set up the chat AI"),
    voice: t("Add voice features"),
    backup: t("Local backup (optional)"),
    easy: t("Easy setup"),
    install: t("Installing selected features"),
    finish: t("Setup is ready"),
  };
  const title = titles[currentStep];
  const easyConnection = settings?.llm.connections?.find(item => item.id === easyConnectionID);

  if (!settings || !setup) {
    return <section className="setup-loading" aria-live="polite"><span className="startup-progress" /><p>{error || t("Loading setup...")}</p></section>;
  }

  return (
    <section className="setup-layout" aria-labelledby="setup-title">
      <aside className="setup-progress" aria-label={t("Setup progress")}>
        <div className="setup-brand"><span aria-hidden="true">M</span><strong>{t("MagicHandy")}</strong></div>
        <ol>
          {steps.map((item, index) => (
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
          <p className="eyebrow">{t("Step {current} of {total}", { current: step + 1, total: steps.length })}</p>
          <h1 id="setup-title">{title}</h1>
        </header>

        <div className="setup-body" ref={setupBody}>
          {setup.installation?.status === "failed" && (currentStep !== "install" || installJob?.id !== setup.installation.id) && <section className="setup-notice">
            <strong>{translateKnown(setup.installation.message)}</strong>
            <SetupFailureReport job={setup.installation} />
          </section>}
          {currentStep === "welcome" && <WelcomeStep locked={locked} settings={settings} patch={(patch) => setSettings({ ...settings, ...patch })} />}
          {currentStep === "welcome" && <SetupModeChoice mode={mode} setMode={changeMode} />}
          {currentStep === "easy" && <EasySetupStep
            setup={setup}
            settings={settings}
            catalog={catalog}
            voiceChoice={voiceChoice}
            voiceInput={parakeetSelected}
            connectionKey={connectionKey}
            locked={locked || installationActive}
            qwenReference={{ wav: qwenWAV, transcript: qwenTranscript, later: qwenLater, locked: locked || installationActive, setWAV: setQwenWAV, setTranscript: setQwenTranscript, setLater: setQwenLater }}
            useLocalAI={easyAIChoice === "local" || easyCombine}
            chatEnabled={easyAIChoice !== "local" || localAvailable}
            aiSetup={<EasyAISetup choice={easyAIChoice} combine={easyCombine} localAvailable={localAvailable} connection={easyConnection} settings={settings.llm} locked={locked || installationActive}
              editCustom={() => { setMode("custom"); setStep(CUSTOM_STEPS.indexOf("chat")); }}
              select={selectEasyAI}
              setCombine={combine => selectEasyAI(easyAIChoice, combine, easyConnection?.provider)}
              setProvider={provider => selectEasyAI(easyAIChoice, easyCombine, provider)}
              patchConnection={change => patchLLM({ connections: settings.llm.connections?.map(connection => connection.id === easyConnectionID ? { ...connection, ...change } : connection) })}
              onReady={setEasyHostedReady}
            />}
            setChatVoice={(chat_voice) => patchLLM({ chat_voice })}
            setVoiceOutput={(enabled) => selectEasyVoice(enabled ? (setup.assessment?.voice_module ?? "chatterbox") as VoiceChoice : "none")}
            setVoiceChoice={selectEasyVoice}
            setVoiceInput={setParakeetSelected}
            setConnectionKey={setConnectionKey}
          />}
          {currentStep === "access" && <AccessStep
            choice={accessChoice}
            initialized={Boolean(auth.status?.initialized) || createdAdministrator}
            scope={accessScope}
            setScope={(scope) => { setAccessScope(scope); setNetworkReady(false); if (scope !== "local") setAccessChoice("protected"); }}
            networkRequired={networkRequired}
            networkLoaded={networkLoaded}
            networkStatus={setupNetworkStatus}
            onNetworkReady={setNetworkReady}
            backendOnline={backendOnline}
            username={administratorUsername}
            password={administratorPassword}
            confirmation={administratorConfirmation}
            locked={locked}
            setChoice={setAccessChoice}
            setUsername={setAdministratorUsername}
            setPassword={setAdministratorPassword}
            setConfirmation={setAdministratorConfirmation}
          />}
          {currentStep === "device" && <DeviceStep
            settings={settings}
            handyModel={handyModel}
            connectionKey={connectionKey}
            connectionResult={connectionResult}
            locked={locked}
            setHandyModel={setHandyModel}
            setConnectionKey={setConnectionKey}
            patchOwner={(owner) => setSettings({ ...settings, device: { ...settings.device, hsp_dispatch_owner: owner } })}
            verifyCloud={verifyCloud}
          />}
          {(currentStep === "chat" || currentStep === "backup") && <SetupBackupOrChat
            backup={currentStep === "backup"}
            enabled={backupChoice}
            choose={enabled => { setBackupChoice(enabled); patchLLM({ retry_refusal_locally: enabled, ...(enabled ? backupRuntime === "ollama" ? { provider: "ollama" } : { provider: "llama_cpp", llama_cpp_mode: backupRuntime === "managed" ? "managed" : "external" } : {}) }); }}
            savedSettings={state?.settings}
            choice={currentStep === "backup" ? backupRuntime : runtimeChoice}
            modelChoice={modelChoice}
            backend={runtimeBackend}
            settings={settings.llm}
            setup={setup}
            models={models}
            catalog={catalog}
            ollamaModels={ollamaModels}
            ggufPath={ggufPath}
            ggufName={ggufName}
            locked={locked || installationActive}
            importLocked={locked || Boolean(activeImport)}
            select={choice => {
              if (currentStep !== "backup") { selectRuntime(choice); return; }
              if (choice === backupRuntime) return;
              setBackupRuntime(choice);
              setModelChoice("later");
              patchLLM({ model: "", ...(choice === "ollama" ? { provider: "ollama" } : { provider: "llama_cpp", llama_cpp_mode: choice === "managed" ? "managed" : "external" }) });
            }}
            selectModel={choice => { setModelChoice(choice); setBackupChecked(false); }}
            setBackend={setRuntimeBackend}
            patchLLM={patch => {
              if (currentStep === "backup" && patch.ollama_base_url !== undefined && patch.ollama_base_url !== settings.llm.ollama_base_url) { setOllamaModels([]); patchLLM({ ...patch, model: "" }); }
              else patchLLM(patch);
            }}
            setGGUFPath={setGGUFPath}
            setGGUFName={setGGUFName}
            importGGUF={importGGUF}
            mergeImport={mergeImport}
            refreshOllama={() => void run("refresh-ollama", async () => { if (currentStep === "backup") await savePreferences({ llm: settings.llm }); await loadOllama(); })}
          />}
          {currentStep === "backup" && backupChoice && backupRuntime !== "managed" && backupModelReady && <button type="button" className="btn btn-secondary" disabled={locked || backupChecked} onClick={checkBackup}>{busy === "backup-check" ? t("Checking local backup...") : backupChecked ? t("Local backup checked") : t("Check local backup")}</button>}
          {currentStep === "voice" && <VoiceStep
            setup={setup}
            choice={voiceChoice}
            device={voiceDevice}
            autoLaunch={voiceAutoLaunch}
            enableAfterInstall={voiceEnableAfterInstall}
            parakeetSelected={parakeetSelected}
            locked={locked || installationActive}
            qwenReference={{ wav: qwenWAV, transcript: qwenTranscript, later: qwenLater, locked: locked || installationActive, setWAV: setQwenWAV, setTranscript: setQwenTranscript, setLater: setQwenLater }}
            setChoice={setVoiceChoice}
            setDevice={setVoiceDevice}
            setAutoLaunch={setVoiceAutoLaunch}
            setEnableAfterInstall={setVoiceEnableAfterInstall}
            setParakeetSelected={setParakeetSelected}
          />}
          {currentStep === "install" && <InstallStep
            job={installJob}
            submitted={installSubmitted}
            runtimeChoice={runtimeChoice}
            voiceChoice={voiceChoice}
            parakeetSelected={parakeetSelected}
            cancel={cancelInstall}
            retry={retryInstall}
          />}
          {currentStep === "finish" && needsBackup && <div className="setup-notice">
            <strong>{t("Local backup")}: {settings.llm.model}</strong>
            <p>{t("Check that the local model can generate a reply before finishing setup.")}</p>
            <div className="button-row"><button type="button" className="btn btn-secondary" disabled={locked || backupChecked} onClick={checkBackup}>{busy === "backup-check" ? t("Checking local backup...") : backupChecked ? t("Local backup checked") : t("Check local backup")}</button>
            {!backupChecked && <button type="button" className="btn btn-quiet" disabled={locked} onClick={skipBackup}>{t("Continue without a backup")}</button>}</div>
          </div>}
          {currentStep === "backup" && backupChoice && !backupSpaceReady && <p role="alert">{t("Installing these needs about {needed} of free space, but only {free} is free. Free up space before continuing.", { needed: formatBytes(backupBytes), free: formatBytes(setup.assessment?.free_disk_bytes ?? 0) })}</p>}
          {currentStep === "finish" && <FinishStep
            setup={setup}
            settings={settings}
            models={models}
            runtimeChoice={runtimeChoice}
            modelPending={managedModelPending}
            voiceChoice={voiceChoice}
            parakeetSelected={parakeetSelected}
            requiresSignIn={requiresSignInAfterSetup}
          />}

          {activeImport && <p className="setup-inline-status" role="status">{t("Importing {name}: {copied} of {total}", {
            name: activeImport.display_name,
            copied: formatBytes(activeImport.bytes_copied),
            total: formatBytes(activeImport.total_bytes),
          })}</p>}
          {currentStep === "chat" && !chatChoiceReady && <p className="setup-inline-status" role="status">{t("Choose a model to download or import, or pick Add a model later.")}</p>}
          {error && <p className="form-status setup-error" role="alert">{error}</p>}
        </div>

        <div className="setup-action-reason" role="status">
          {readOnly ? t("Take control to change setup.") : !backendOnline ? t("The backend is offline. Setup changes are unavailable.") : currentStep === "easy" && !easyAIReady ? t("Check a model to continue.") : currentStep === "backup" && backupChoice === null ? t("Choose whether to set up a local backup.") : currentStep === "backup" && backupChoice && !backupModelReady ? t("Choose a local model to continue.") : null}
        </div>
        <footer className="setup-actions">
          <button type="button" className="btn btn-secondary" disabled={step === 0 || installationActive || Boolean(busy)} onClick={() => setStep((current) => current - 1)}>{t("Back")}</button>
          <span className="setup-action-spacer" />
          {step < steps.length - 1 && currentStep !== "install" && currentStep !== "access" && currentStep !== "easy" && currentStep !== "welcome" && currentStep !== "backup" && <button type="button" className="btn btn-quiet" disabled={installationActive || Boolean(busy)} onClick={skipStep}>{t("Skip for now")}</button>}
          {step < steps.length - 1 ? (
            <button type="button" className="btn btn-primary" disabled={locked || installationActive || !currentStepReady || (currentStep === "install" && !installationReady)} onClick={continueStep}>{busy === "continue" ? t("Saving...") : ((currentStep === "easy" && !hostedChat) || (currentStep === "backup" && mode === "easy")) && easyNeedsInstall ? t("Install and continue") : t("Continue")}</button>
          ) : (
            <button type="button" className="btn btn-primary" disabled={locked || !canFinish} onClick={finish}>{busy === "finish" ? t("Finishing setup...") : requiresSignInAfterSetup ? t("Finish and sign in") : t("Open MagicHandy")}</button>
          )}
        </footer>
      </div>
    </section>
  );
}

function SetupBackupOrChat({ backup, enabled, choose, ...props }: React.ComponentProps<typeof SetupBackupStep> & { backup: boolean }) {
  return backup ? <SetupBackupStep {...props} enabled={enabled} choose={choose} /> : <SetupChatStep {...props} />;
}
