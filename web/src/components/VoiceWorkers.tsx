import { t, translateKnown, type MessageKey } from "../i18n";
// Live status and controls for one optional voice worker (ADR 0003). The
// settings parent owns the shared status poll so ASR, TTS, and their request
// queue render one coherent backend snapshot.
import { useState } from "react";
import { api } from "../api/client";
import type { VoiceModuleStatus, VoiceRequestSnapshot, VoiceWorkerStatus } from "../api/types";
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
      const result = await api.voiceWorkerTest(role, { text: t("MagicHandy voice test"), delay_ms: 0 });
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

  return (
    <div className="voice-workers">
      {showParakeetModule && (
        <div className="voice-module-readout" role="status" aria-label={t("MagicHandy Parakeet module")}>
          <span className="status-dot" data-state={parakeetModule?.installed ? "ok" : parakeetModule?.state === "incomplete" ? "warn" : "idle"} />
          <span>{parakeetModule?.message ? translateKnown(parakeetModule.message) : t("Checking the MagicHandy Parakeet module.")}</span>
        </div>
      )}
      {showTTSModule && (
        <div className="voice-module-readout" role="status" aria-label={t("Checking the {module} module.", { module: ttsModuleName ?? "TTS" })}>
          <span className="status-dot" data-state={ttsModule?.installed ? "ok" : ttsModule?.state === "incomplete" ? "warn" : "idle"} />
          <span>{ttsModule?.message ? translateKnown(ttsModule.message) : t("Checking the {module} module.", { module: ttsModuleName ?? "TTS" })}</span>
        </div>
      )}
      {roles.map((role) => {
        const worker = workers[role];
        const state = worker?.state ?? "disabled";
        const canControl = !locked && !dirty && busyRole !== role && state !== "disabled" && state !== "not_configured";
        const modelLoaded = worker?.model_state === "ready";
        const isRunning = state === "running";
        const isStarting = state === "starting";
        const lastResult = requests.find(
          (request) => request.role === role && (request.state === "done" || request.state === "failed" || request.state === "canceled"),
        );
        return (
          <div key={role} className="voice-worker-row">
            <div className="voice-worker-head">
              <span className="voice-worker-name">{selectedRole ? t("Worker") : translateKnown(ROLE_LABEL[role] ?? role)}</span>
              <span className="status-readout">
                <span className="status-dot" data-state={dotState(state)} />
                <span className="status-text">{translateKnown(STATE_LABEL[state] ?? state)}</span>
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
            {worker?.last_error && state !== "running" && (
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
