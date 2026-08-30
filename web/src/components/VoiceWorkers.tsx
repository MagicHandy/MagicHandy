import { t, translateKnown, type MessageKey } from "../i18n";
// Live status and controls for one optional voice worker (ADR 0003). The
// settings parent owns the shared status poll so ASR, TTS, and their request
// queue render one coherent backend snapshot.
import { useState } from "react";
import { api } from "../api/client";
import type { SetupJob, VoiceModuleStatus, VoiceRequestSnapshot, VoiceWorkerStatus } from "../api/types";
import { useToast } from "../state/app-state";
import { useVoicePlayback } from "../state/voice-playback";

const message = (error: unknown) => error instanceof Error ? translateKnown(error.message) : t("Request failed");

const STATE_LABEL: Partial<Record<string, MessageKey>> = {
  disabled: "Disabled",
  not_configured: "Not configured",
  stopped: "Stopped",
  starting: "Starting...",
  running: "Running",
  crashed: "Crashed",
};

function dotState(state: string): string {
  switch (state) {
    case "running":
      return "ok";
    case "starting":
      return "pending";
    case "crashed":
      return "error";
    case "not_configured":
      return "warn";
    default:
      return "idle";
  }
}

const ROLE_LABEL: Partial<Record<string, MessageKey>> = {
  tts: "Speech output (TTS)",
  asr: "Speech input (ASR)",
};

interface Props {
  locked: boolean;
  role?: "tts" | "asr";
  dirty?: boolean;
  enabled?: boolean;
  providerSelected?: boolean;
  showParakeetModule?: boolean;
  showTTSModule?: boolean;
  ttsModuleName?: string;
  parakeetRepair?: {
    job?: SetupJob;
    setupBusy: boolean;
    error: string;
    repair: () => Promise<void>;
    cancel: () => Promise<void>;
  };
  workers: Record<string, VoiceWorkerStatus>;
  requests: VoiceRequestSnapshot[];
  modules: Record<string, VoiceModuleStatus>;
  refresh: () => Promise<void>;
}

// dirty means the surrounding settings form has unsaved voice changes; these
// controls act on saved config and therefore lock until that config is saved.
export function VoiceWorkers({
  locked,
  role: selectedRole,
  dirty,
  enabled,
  providerSelected,
  showParakeetModule,
  showTTSModule,
  ttsModuleName,
  parakeetRepair,
  workers,
  requests,
  modules,
  refresh,
}: Props) {
  const { show } = useToast();
  const { queueSpeech } = useVoicePlayback();
  const [busyRole, setBusyRole] = useState<string | null>(null);

  async function run(role: "tts" | "asr", action: () => Promise<unknown>, doing: MessageKey) {
    setBusyRole(role);
    try {
      await action();
    } catch (error) {
      show(t("{action} failed: {message}", { action: translateKnown(doing), message: message(error) }), "error");
    } finally {
      setBusyRole(null);
      void refresh();
    }
  }

  async function sendTest(role: "tts" | "asr") {
    setBusyRole(role);
    try {
      // The TTS preview has to be long enough to be worth listening to. A
      // four-word sample carries barely one intonation contour, so a tone preset
      // that falls apart over a real multi-sentence reply still previews clean --
      // which is exactly how several shipped sounding strained in use.
      // ASR ignores the content and only checks that the text is non-empty.
      const text = role === "tts"
        ? t("This is how I sound with your current voice settings. Listen to a full sentence or two before deciding.")
        : t("MagicHandy voice test");
      const result = await api.voiceWorkerTest(role, { text, delay_ms: 0 });
      if (role === "tts" && result.request?.id) queueSpeech(result.request.id);
    } catch (error) {
      show(t("Test request failed: {message}", { message: message(error) }), "error");
    } finally {
      setBusyRole(null);
      void refresh();
    }
  }

  const roles: ("tts" | "asr")[] = selectedRole ? [selectedRole] : ["tts", "asr"];
  const parakeetModule = modules.parakeet;
  const ttsModule = modules.tts;
  const ttsNeedsSetup = !dirty && ttsModule?.installed === false &&
    (ttsModule.worker_installed === false || ttsModule.runtime_installed === false);
  const parakeetRepairActive = parakeetRepair?.job?.status === "queued" || parakeetRepair?.job?.status === "running";
  const parakeetRepairMessage = parakeetModule?.installed
    ? parakeetModule.message
    : parakeetRepair?.error || (parakeetRepair?.job && parakeetRepair.job.status !== "complete" ? parakeetRepair.job.message : "") || parakeetModule?.message;

  if (selectedRole && providerSelected === false) return null;

  return (
    <div className="voice-workers">
      {showParakeetModule && (
        <div className="voice-module-readout" role="status" aria-live="polite" aria-busy={parakeetRepairActive || undefined} aria-label={t("MagicHandy Parakeet module")}>
          <span className="status-dot" data-state={parakeetRepair?.error ? "error" : parakeetRepairActive ? "pending" : parakeetModule?.installed ? "ok" : parakeetModule?.state === "incomplete" ? "warn" : "idle"} />
          <span className="voice-module-message">
            <span>{parakeetRepairMessage ? translateKnown(parakeetRepairMessage) : t("Checking the MagicHandy Parakeet module.")}</span>
            {parakeetRepairActive && (parakeetRepair?.job?.bytes_total ?? 0) > 0 && <progress aria-label={t("Installation progress")} max={parakeetRepair?.job?.bytes_total} value={parakeetRepair?.job?.bytes_completed ?? 0} />}
          </span>
          {parakeetRepair && <span className="voice-module-actions">
            {parakeetRepairActive && <button type="button" className="btn btn-secondary" disabled={locked} onClick={() => void parakeetRepair.cancel()}>{t("Cancel")}</button>}
            <button type="button" className="btn btn-secondary" disabled={locked || parakeetRepair.setupBusy} onClick={() => void parakeetRepair.repair()}>{t("Repair")} {t("Parakeet")}</button>
          </span>}
        </div>
      )}
      {showTTSModule && (
        <div className="voice-module-readout" role="status" aria-label={t("Checking the {module} module.", { module: ttsModuleName ?? "TTS" })}>
          <span className="status-dot" data-state={dirty ? "idle" : ttsModule?.installed ? "ok" : ttsModule?.state === "incomplete" ? "warn" : "idle"} />
          <span>{dirty ? t("Save settings before runtime actions.") : ttsModule?.message ? translateKnown(ttsModule.message) : t("Checking the {module} module.", { module: ttsModuleName ?? "TTS" })}</span>
          {ttsNeedsSetup && <a className="voice-module-setup-link" href="#/setup/reconfigure">{t("Run setup again")}</a>}
        </div>
      )}
      {roles.map((role) => {
        const worker = workers[role];
        const state = worker?.state ?? "disabled";
        const managedModuleUnavailable = role === "asr"
          ? Boolean(showParakeetModule && parakeetModule && !parakeetModule.installed)
          : Boolean(showTTSModule && ttsModule && !ttsModule.installed);
        if (managedModuleUnavailable || (dirty && state === "disabled")) return null;
        const canControl = !locked && !dirty && busyRole !== role && state !== "disabled" && state !== "not_configured";
        const modelLoaded = worker?.model_state === "ready";
        const isRunning = state === "running";
        const isStarting = state === "starting";
        const modelFailed = isRunning && !modelLoaded && Boolean(worker?.last_error);
        const lastResult = requests.find(
          (request) => request.role === role && (request.state === "done" || request.state === "failed" || request.state === "canceled"),
        );
        return (
          <div key={role} className="voice-worker-row">
            <div className="voice-worker-head">
              <span className="voice-worker-name">{selectedRole ? t("Status") : translateKnown(ROLE_LABEL[role] ?? role)}</span>
              <span className="status-readout">
                <span className="status-dot" data-state={modelFailed ? "error" : dotState(state)} />
                <span className="status-text">{modelFailed ? t("Not ready") : translateKnown(STATE_LABEL[state] ?? state)}</span>
              </span>
              {worker?.provider && state === "running" && (
                <span className="hint-inline">
                  {t("{provider} v{providerVersion} / protocol v{protocolVersion} / model {modelState} / queue {queueDepth}", { provider: worker.provider, providerVersion: worker.provider_version ?? "unknown", protocolVersion: worker.protocol_version ?? "unknown", modelState: worker.model_state ?? "unknown", queueDepth: worker.queue_depth ?? 0 })}
                </span>
              )}
            </div>
            {state === "not_configured" && (
              <p className="form-status">{(showParakeetModule || showTTSModule) ? t("The selected module is not ready; follow the module status above before starting it.") : t("The selected worker is not configured. Check its provider fields or installation, then save.")}</p>
            )}
            {state === "disabled" && providerSelected && (
              <p className="form-status">{enabled ? t("Save these voice settings; Start will appear here once the worker is configured.") : t("Enable voice workers and save; Start will appear here when the worker is ready.")}</p>
            )}
            {worker?.last_error && (state !== "running" || modelFailed) && (
              <p className="form-status voice-worker-error">{worker.last_error}</p>
            )}
            {lastResult && (
              <p className="form-status voice-last-result">
                {lastResult.state === "done" && lastResult.transcript?.[0]?.text
                  ? t("Last request completed: \"{text}\"", { text: lastResult.transcript[0].text })
                  : lastResult.state === "done" && (lastResult.audio_bytes ?? 0) > 0
                    ? t("Last request completed with {bytes} bytes of audio.", { bytes: lastResult.audio_bytes ?? 0 })
                    : lastResult.state === "failed" && lastResult.error
                      ? t("Last request failed ({code}): {message}", { code: lastResult.error.code, message: lastResult.error.message })
                      : t("Last request: {state}", { state: lastResult.state })}
              </p>
            )}
            {state !== "disabled" && state !== "not_configured" && dirty && (
              <p className="form-status">{t("Save settings to apply the selection above before controlling this worker.")}</p>
            )}
            {state !== "disabled" && state !== "not_configured" && (
              <div className="row-actions">
                {state === "stopped" && <button type="button" className="btn btn-secondary" disabled={!canControl} onClick={() => void run(role, () => api.voiceWorkerStart(role), "Start")}>{t("Start")}</button>}
                {state === "crashed" && <button type="button" className="btn btn-secondary" disabled={!canControl} onClick={() => void run(role, () => api.voiceWorkerRestart(role), "Restart")}>{t("Restart")}</button>}
                {(isRunning || isStarting) && <button type="button" className="btn btn-secondary" disabled={!canControl} onClick={() => void run(role, () => api.voiceWorkerStop(role), "Stop")}>{t("Stop")}</button>}
                {isRunning && <button type="button" className="btn btn-secondary" disabled={!canControl} onClick={() => void run(role, () => api.voiceWorkerModel(role, !modelLoaded), modelLoaded ? "Unload model" : "Load model")}>{modelLoaded ? t("Unload model") : t("Load model")}</button>}
                {isRunning && modelLoaded && <button type="button" className="btn btn-secondary" disabled={!canControl} onClick={() => void sendTest(role)}>{t("Send test")}</button>}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
