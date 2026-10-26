// Live status and controls for one optional voice worker (ADR 0003). The
// settings parent owns the shared status poll so ASR, TTS and their request
// queue render one coherent backend snapshot; this component adds only the
// state of the request this tab is waiting on.
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { VoiceModuleStatus, VoiceRequestSnapshot, VoiceWorkerStatus } from "../api/types";
import { formatNumber, t, translateKnown, type MessageKey } from "../i18n";
import { useToast } from "../state/app-state";
import { useVoicePlayback } from "../state/voice-playback";
import { SetupFailureReport } from "./SetupFailureReport";
import type { ParakeetRepair } from "./useParakeetRepair";
import { VoiceWorkerDetails } from "./VoiceWorkerDetails";
import {
  isLoadAction,
  splitWorkerError,
  voicePhaseIsBusy,
  voicePhaseTone,
  voiceWorkerPhase,
  workerHasFailure,
  workerSignature,
  type VoicePendingAction,
  type VoicePhase,
} from "./voice-worker-phase";

type Role = "tts" | "asr";

const message = (error: unknown) => error instanceof Error ? translateKnown(error.message) : t("Request failed");

const PHASE_LABEL: Record<VoicePhase, MessageKey> = {
  checking: "Checking…",
  unavailable: "Unavailable",
  off: "Off",
  unsaved: "Not saved",
  setup: "Needs setup",
  installing: "Installing…",
  setup_failed: "Setup failed",
  stopped: "Stopped",
  starting: "Starting…",
  loading: "Loading…",
  busy: "Busy",
  ready: "Ready",
  unloaded: "Not loaded",
  stopping: "Stopping…",
  failed: "Failed",
};

const STATUS_NAME: Record<Role, MessageKey> = {
  asr: "Speech input status",
  tts: "Speech output status",
};

// Plain words for the two Go context errors. Display only: the raw text stays
// in the technical details, and the phase never depends on an error's wording.
const ERROR_WORDING: Partial<Record<string, MessageKey>> = {
  "context deadline exceeded": "It did not respond in time.",
  "context canceled": "The request was interrupted.",
};

interface Props {
  role: Role;
  locked: boolean;
  /** The settings form has unsaved voice changes; Start and tests use saved settings. */
  dirty?: boolean;
  enabled?: boolean;
  providerSelected?: boolean;
  /** Set when the unsaved draft selects a different provider from the one running. */
  savedProviderLabel?: string;
  showParakeetModule?: boolean;
  showTTSModule?: boolean;
  parakeetRepair?: ParakeetRepair;
  workers: Record<string, VoiceWorkerStatus>;
  requests: VoiceRequestSnapshot[];
  modules: Record<string, VoiceModuleStatus>;
  refresh: () => Promise<void>;
  /** A status snapshot has arrived. */
  loaded?: boolean;
  /** When the shown snapshot was requested. */
  snapshotRequestedAt?: number;
  /** The newest status request failed. */
  statusError?: string;
}

// One request this tab sent and is still waiting on. Every action gets a new
// id; only the newest may clear the pending state or report its outcome.
interface Operation {
  id: number;
  action: VoicePendingAction;
  startedAt: number;
  /** The worker as this tab last saw it when the request began. */
  baseline: string;
}

function newestFinished(requests: VoiceRequestSnapshot[], role: Role) {
  let newest: VoiceRequestSnapshot | undefined;
  for (const request of requests) {
    if (request.role !== role || request.state === "queued" || request.state === "active") continue;
    if (!newest || Date.parse(request.created_at) > Date.parse(newest.created_at)) newest = request;
  }
  return newest;
}

export function VoiceWorkers({
  role,
  locked,
  dirty = false,
  enabled = true,
  providerSelected,
  savedProviderLabel,
  showParakeetModule,
  showTTSModule,
  parakeetRepair,
  workers,
  requests,
  modules,
  refresh,
  loaded = true,
  snapshotRequestedAt,
  statusError = "",
}: Props) {
  const { show } = useToast();
  const { queueSpeech } = useVoicePlayback();
  const [operation, setOperation] = useState<Operation>();
  const [test, setTest] = useState<VoiceRequestSnapshot>();
  const latest = useRef(0);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => { mounted.current = false; };
  }, []);

  if (providerSelected === false) return null;

  const worker = workers[role];
  const module = role === "asr"
    ? (showParakeetModule ? modules.parakeet : undefined)
    : (showTTSModule ? modules.tts : undefined);
  const moduleBlocked = Boolean(module && !module.installed);
  const repair = role === "asr" && showParakeetModule ? parakeetRepair : undefined;
  const repairJob = repair?.job;
  const repairActive = repairJob?.status === "queued" || repairJob?.status === "running";
  const repairFailed = Boolean(repair?.error) || repairJob?.status === "failed";
  const stale = loaded && Boolean(statusError);
  const pending = operation?.action;
  // A crash or error first seen in a snapshot requested after this tab's
  // start began is news about that start; an unchanged older one is not.
  const pendingFailed = Boolean(operation && isLoadAction(operation.action) &&
    (snapshotRequestedAt ?? 0) > operation.startedAt &&
    workerHasFailure(worker) && workerSignature(worker) !== operation.baseline);
  const phase = voiceWorkerPhase({
    worker,
    loaded,
    unavailable: !loaded && Boolean(statusError),
    dirty,
    moduleBlocked,
    repairActive,
    repairFailed,
    pending,
    pendingFailed,
  });
  const tone = voicePhaseTone(phase);
  const { message: errorMessage, output } = splitWorkerError(worker);
  const lastRequest = newestFinished(requests, role);
  const testResult = test ? requests.find((request) => request.id === test.id) ?? test : undefined;

  const blocked = locked || stale || !loaded;
  const canRun = !blocked && !dirty && (pending === undefined || pendingFailed);
  const canStop = !blocked && pending !== "stop" && pending !== "unload";

  const begin = (action: VoicePendingAction) => {
    const next: Operation = { id: ++latest.current, action, startedAt: Date.now(), baseline: workerSignature(worker) };
    setOperation(next);
    return next.id;
  };
  // A later action (Cancel, Stop, another Start) supersedes this one.
  const superseded = (id: number) => id !== latest.current;
  const settle = (id: number) => {
    if (mounted.current) setOperation((current) => (current?.id === id ? undefined : current));
    void refresh();
  };

  async function run(action: VoicePendingAction, request: () => Promise<unknown>, label: MessageKey) {
    const id = begin(action);
    try {
      await request();
    } catch (error) {
      if (!superseded(id)) {
        show(t("{action} failed: {message}", { action: translateKnown(label), message: message(error) }), "error");
      }
    } finally {
      settle(id);
    }
  }

  const start = () => void run("start", () => api.voiceWorkerStart(role), "Start");
  const stop = (label: MessageKey) => void run("stop", () => api.voiceWorkerStop(role), label);
  // A failed process, running or not, gets a fresh one: restart replaces a
  // live worker, start launches a stopped one.
  const retry = () => {
    if (worker?.state === "crashed" || worker?.state === "running") void run("restart", () => api.voiceWorkerRestart(role), "Try again");
    else void run("start", () => api.voiceWorkerStart(role), "Try again");
  };

  async function sendTest() {
    const id = begin("test");
    setTest(undefined);
    try {
      // The TTS preview has to be long enough to be worth listening to. A
      // four-word sample carries barely one intonation contour, so a tone preset
      // that falls apart over a real multi-sentence reply still previews clean --
      // which is exactly how several shipped sounding strained in use.
      // ASR ignores the content and checks a valid silent clip.
      const text = role === "tts"
        ? t("This is how I sound with your current voice settings. Listen to a full sentence or two before deciding.")
        : t("MagicHandy voice test");
      const result = await api.voiceWorkerTest(role, { text, delay_ms: 0 });
      const request = result.request;
      if (request?.id && !superseded(id)) {
        if (mounted.current) setTest(request);
        if (role === "tts") queueSpeech(request.id);
      }
    } catch (error) {
      if (!superseded(id)) show(t("Test request failed: {message}", { message: message(error) }), "error");
    } finally {
      settle(id);
    }
  }

  const lead = phaseMessage(phase, role, { enabled, worker, module, repair });
  const showError = phase === "failed" && errorMessage;
  const errorWording = ERROR_WORDING[errorMessage];
  const detailedError = errorMessage.length > 240 || errorMessage.includes("\n");
  const progressTotal = repairActive ? repairJob?.bytes_total ?? 0 : 0;
  // Leaving for guided setup would discard the unsaved draft.
  const ttsNeedsSetup = role === "tts" && phase === "setup" && !dirty && module?.installed === false &&
    (module.worker_installed === false || module.runtime_installed === false);
  const actionsNeedSave = dirty && !savedProviderLabel && (phase === "stopped" || phase === "failed" || phase === "unloaded" || phase === "ready");
  const hasDetails = loaded && Boolean(worker?.provider || worker?.started_at || errorMessage || output || lastRequest || module?.message);

  return (
    <div className="voice-status" data-tone={tone}>
      <div className="voice-status-head">
        <div className="voice-status-readout" role="status" aria-live="polite" aria-busy={voicePhaseIsBusy(phase) || undefined} aria-label={translateKnown(STATUS_NAME[role])}>
          <span className="voice-status-line">
            <span className="status-dot" data-state={tone} aria-hidden="true" />
            <strong className="voice-status-label">{translateKnown(PHASE_LABEL[phase])}</strong>
          </span>
          {lead && <span className="voice-status-message">{lead}</span>}
        </div>
        <div className="row-actions voice-status-actions">
          {phase === "stopped" && <button type="button" className="btn btn-secondary" disabled={!canRun} onClick={start}>{t("Start")}</button>}
          {phase === "failed" && <button type="button" className="btn btn-secondary" disabled={!canRun} onClick={retry}>{t("Try again")}</button>}
          {phase === "unloaded" && <button type="button" className="btn btn-secondary" disabled={!canRun} onClick={() => void run("load", () => api.voiceWorkerModel(role, true), "Load model")}>{t("Load model")}</button>}
          {phase === "ready" && <button type="button" className="btn btn-secondary" disabled={!canRun} aria-busy={pending === "test" || undefined} onClick={() => void sendTest()}>{pending === "test" ? t("Testing…") : role === "tts" ? t("Play sample") : t("Test input")}</button>}
          {(phase === "starting" || phase === "loading") && <button type="button" className="btn btn-secondary" disabled={!canStop} onClick={() => stop("Cancel")}>{t("Cancel")}</button>}
          {(phase === "ready" || phase === "unloaded" || phase === "busy" || (phase === "failed" && worker?.state === "running")) && <button type="button" className="btn btn-secondary" disabled={!canStop} onClick={() => stop("Stop")}>{t("Stop")}</button>}
          {repair && (phase === "setup" || phase === "setup_failed") && <button type="button" className="btn btn-secondary" disabled={locked || repair.setupBusy} onClick={() => void repair.repair()}>{t("Repair Parakeet")}</button>}
          {repair && repairActive && <button type="button" className="btn btn-secondary" disabled={locked} onClick={() => void repair.cancel()}>{t("Cancel")}</button>}
          {ttsNeedsSetup && <a className="btn btn-secondary" href="#/setup/reconfigure">{t("Run setup again")}</a>}
        </div>
      </div>
      {repairActive && <div className="voice-status-progress">
        {repairJob?.message && <span>{translateKnown(repairJob.message)}</span>}
        {progressTotal > 0 && <progress aria-label={t("Installation progress")} max={progressTotal} value={repairJob?.bytes_completed ?? 0} />}
      </div>}
      {showError && <p className="voice-status-error">{detailedError ? t("Voice ran into a problem. Try again, or open Technical details for the full error.") : errorWording ? translateKnown(errorWording) : translateKnown(errorMessage)}</p>}
      {repair?.error && phase !== "setup_failed" && <p className="voice-status-error" role="alert">{repair.error}</p>}
      {phase === "ready" && testResult && <TestResult role={role} request={testResult} />}
      {savedProviderLabel && <p className="hint">{t("This status is for the saved provider, {provider}. Save settings to use your new choice.", { provider: savedProviderLabel })}</p>}
      {actionsNeedSave && <p className="hint">{t("Save settings to apply your changes before starting or testing.")}</p>}
      <SetupFailureReport job={repairJob} />
      {hasDetails && <VoiceWorkerDetails worker={worker} module={module} lastRequest={lastRequest} error={errorMessage} output={output}>
        {repair && !moduleBlocked && !repairActive && <button type="button" className="btn btn-secondary btn-sm" disabled={locked || repair.setupBusy} onClick={() => void repair.repair()}>{t("Repair Parakeet")}</button>}
        {phase === "ready" && <button type="button" className="btn btn-secondary btn-sm" disabled={!canRun} onClick={() => void run("unload", () => api.voiceWorkerModel(role, false), "Unload model")}>{t("Unload model")}</button>}
      </VoiceWorkerDetails>}
    </div>
  );
}

interface MessageContext {
  enabled: boolean;
  worker?: VoiceWorkerStatus;
  module?: VoiceModuleStatus;
  repair?: ParakeetRepair;
}

function phaseMessage(phase: VoicePhase, role: Role, { enabled, worker, module, repair }: MessageContext): string {
  switch (phase) {
    case "off":
      return enabled ? t("Voice turns on a few seconds after saving.") : t("Voice is off. Turn on Enable voice above, then save.");
    case "unsaved":
      return t("Save settings to apply these changes.");
    case "setup":
      return module && !module.installed && module.message
        ? translateKnown(module.message)
        : t("Complete the provider settings below, then save.");
    case "setup_failed":
      return repair?.error || translateKnown(repair?.job?.message || module?.message || "");
    case "stopped":
      return role === "asr" ? t("Start speech input to use the microphone in Chat.") : t("Start speech output to hear spoken replies.");
    // Start always includes loading the model, so both share one explanation.
    case "starting":
    case "loading":
      return t("Loading the model. This can take a minute or two, especially the first time after MagicHandy starts.");
    case "busy":
      return t("Running, but it has not reported a loaded model yet. This is normal while the model loads. If it lasts more than a few minutes, stop it and start it again.");
    case "ready":
      return role === "asr" ? t("Use the microphone in Chat to talk.") : t("Play a sample to hear the current voice.");
    case "unloaded":
      return t("Running without its model, so it cannot be used yet.");
    case "failed":
      if (worker?.state === "crashed") return t("It stopped unexpectedly.");
      if (worker?.state === "running") return t("It is running but not working.");
      return t("It could not start.");
    default:
      return "";
  }
}

function TestResult({ role, request }: { role: Role; request: VoiceRequestSnapshot }) {
  let text: string;
  let failed = false;
  if (request.state === "queued" || request.state === "active") {
    text = t("Testing…");
  } else if (request.state === "failed") {
    failed = true;
    text = t("Test failed: {message}", { message: request.error?.message ? translateKnown(request.error.message) : t("Request failed") });
  } else if (request.state === "canceled") {
    text = t("Test canceled.");
  } else if (role === "tts") {
    const seconds = request.completion_ms !== undefined ? formatNumber(Math.round(request.completion_ms / 100) / 10) : "";
    text = seconds ? t("Sample generated in {seconds} s.", { seconds }) : t("Sample generated.");
    if (request.audio_truncated) text = `${text} ${t("The audio was cut off at the size limit.")}`;
  } else {
    const heard = request.transcript?.[0]?.text?.trim();
    text = heard ? t("Input test passed. Heard: \"{text}\"", { text: heard }) : t("Input test passed. The model processed a silent test clip.");
  }
  return <p className={failed ? "voice-status-error" : "hint"} role={failed ? "alert" : undefined}>{text}</p>;
}
