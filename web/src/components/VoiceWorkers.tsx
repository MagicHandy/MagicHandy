// Live status + controls for the optional voice workers (ADR 0003). Voice is
// entirely optional: every state here (disabled, not configured, stopped,
// crashed) is a visible readout, never a blocker for the rest of the app.
// Status readouts follow the design guidelines: dot + text, no pills.
import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { VoiceModuleStatus, VoiceRequestSnapshot, VoiceWorkerStatus } from "../api/types";
import { useToast } from "../state/app-state";
import { audioPlaybackToken, playBlob } from "../util/audio";

const msg = (e: unknown) => (e instanceof Error ? e.message : "Request failed");

const STATE_LABEL: Record<string, string> = {
  disabled: "disabled",
  not_configured: "not configured",
  stopped: "stopped",
  starting: "starting…",
  running: "running",
  crashed: "crashed",
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

const ROLE_LABEL: Record<string, string> = { tts: "Speech output (TTS)", asr: "Speech input (ASR)" };

// dirty means the surrounding settings form has unsaved voice changes; the
// controls act on the *saved* config, so they lock until the form is saved.
export function VoiceWorkers({
  locked, role: selectedRole, dirty, enabled, providerSelected, showParakeetModule, showNeuTTSModule,
}: {
  locked: boolean;
  role?: "tts" | "asr";
  dirty?: boolean;
  enabled?: boolean;
  providerSelected?: boolean;
  showParakeetModule?: boolean;
  showNeuTTSModule?: boolean;
}) {
  const { show } = useToast();
  const [workers, setWorkers] = useState<Record<string, VoiceWorkerStatus>>({});
  const [requests, setRequests] = useState<VoiceRequestSnapshot[]>([]);
  const [modules, setModules] = useState<Record<string, VoiceModuleStatus>>({});
  const [busyRole, setBusyRole] = useState<string | null>(null);
  const alive = useRef(true);

  const refresh = useCallback(async () => {
    try {
      const res = await api.voiceStatus();
      if (!alive.current) return;
      setWorkers(res.voice.workers ?? {});
      setModules(res.voice.modules ?? {});
      setRequests(res.requests ?? []);
    } catch {
      // The settings page surfaces core-offline globally; keep the last view.
    }
  }, []);

  useEffect(() => {
    alive.current = true;
    void refresh();
    const timer = window.setInterval(() => void refresh(), 3000);
    return () => {
      alive.current = false;
      window.clearInterval(timer);
    };
  }, [refresh]);

  async function run(role: "tts" | "asr", action: () => Promise<unknown>, doing: string) {
    setBusyRole(role);
    try {
      await action();
    } catch (e) {
      show(`${doing} failed: ${msg(e)}`, "error");
    } finally {
      setBusyRole(null);
      void refresh();
    }
  }

  async function cancelRequest(id: string) {
    try {
      await api.voiceRequestCancel(id);
    } catch (e) {
      show(msg(e), "error");
    }
    void refresh();
  }

  // A test is only useful with a visible outcome: follow the request to its
  // terminal state and, for TTS, play the produced clip (this tab holds the
  // controller lease whenever the button is enabled).
  async function sendTest(role: "tts" | "asr") {
    setBusyRole(role);
    try {
      const res = await api.voiceWorkerTest(role, { text: "MagicHandy voice test", delay_ms: 0 });
      const id = res.request?.id;
      const deadline = Date.now() + 15000;
      while (id) {
        const poll = await api.voiceRequest(id);
        const state = poll.request?.state;
        if (state === "done") {
          if (role === "tts" && (poll.request?.audio_bytes ?? 0) > 0) {
            const token = audioPlaybackToken();
            await playBlob(await api.voiceRequestAudio(id), token).catch(() => undefined);
          }
          break;
        }
        if (state === "failed" || state === "canceled" || Date.now() > deadline) break;
        await new Promise((resolve) => setTimeout(resolve, 300));
      }
    } catch (e) {
      show(`Test request failed: ${msg(e)}`, "error");
    } finally {
      setBusyRole(null);
      void refresh();
    }
  }

  const roles: ("tts" | "asr")[] = selectedRole ? [selectedRole] : ["tts", "asr"];
  const activeRequests = requests.filter((r) => r.state === "queued" || r.state === "active");
  const parakeetModule = modules.parakeet;
  const neuttsModule = modules.neutts;
  const visibleModule = showParakeetModule ? parakeetModule : showNeuTTSModule ? neuttsModule : undefined;
  const visibleModuleName = showParakeetModule ? "Parakeet" : "NeuTTS";

  return (
    <div className="voice-workers">
      {showParakeetModule && (
        <div className="voice-module-readout" role="status" aria-label="MagicHandy Parakeet module">
          <span className="status-dot" data-state={parakeetModule?.installed ? "ok" : parakeetModule?.state === "incomplete" ? "warn" : "idle"} />
          <span>{parakeetModule?.message || "Checking the MagicHandy Parakeet module."}</span>
        </div>
      )}
      {showNeuTTSModule && (
        <div className="voice-module-readout" role="status" aria-label="NeuTTS module">
          <span className="status-dot" data-state={visibleModule?.installed ? "ok" : visibleModule?.state === "incomplete" ? "warn" : "idle"} />
          <span>{visibleModule?.message || `Checking the ${visibleModuleName} module.`}</span>
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
              <span className="voice-worker-name">{selectedRole ? "Worker" : ROLE_LABEL[role]}</span>
              <span className="status-readout">
                <span className="status-dot" data-state={dotState(state)} />
                <span className="status-text">{STATE_LABEL[state] ?? state}</span>
              </span>
              {worker?.provider && state === "running" && (
                <span className="hint-inline">
                  {worker.provider} v{worker.provider_version} · protocol v{worker.protocol_version} · model {worker.model_state ?? "unknown"} · queue {worker.queue_depth}
                </span>
              )}
            </div>
            {state === "not_configured" && (
              <p className="form-status">{(showParakeetModule || showNeuTTSModule) ? "The selected module is not ready; follow the module status above before starting it." : "The selected worker is not configured. Check its provider fields or installation, then save."}</p>
            )}
            {state === "disabled" && providerSelected && (
              <p className="form-status">{enabled ? "Save these voice settings; Start will appear here once the worker is configured." : "Enable voice workers and save; Start will appear here when the worker is ready."}</p>
            )}
            {worker?.last_error && state !== "running" && (
              <p className="form-status voice-worker-error">{worker.last_error}</p>
            )}
            {lastResult && (
              <p className="form-status voice-last-result">
                {`Last request ${lastResult.state}`}
                {lastResult.state === "done" && lastResult.transcript?.[0]?.text ? ` — “${lastResult.transcript[0].text}”` : ""}
                {lastResult.state === "done" && (lastResult.audio_bytes ?? 0) > 0 ? ` — ${lastResult.audio_bytes} bytes of audio` : ""}
                {lastResult.state === "failed" && lastResult.error ? ` — ${lastResult.error.code}: ${lastResult.error.message}` : ""}
              </p>
            )}
            {state !== "disabled" && state !== "not_configured" && dirty && (
              <p className="form-status">Save settings to apply the selection above before controlling this worker.</p>
            )}
            {state !== "disabled" && state !== "not_configured" && (
              <div className="row-actions">
                {state === "stopped" && <button type="button" className="btn btn-secondary" disabled={!canControl} onClick={() => void run(role, () => api.voiceWorkerStart(role), "Start")}>Start</button>}
                {state === "crashed" && <button type="button" className="btn btn-secondary" disabled={!canControl} onClick={() => void run(role, () => api.voiceWorkerRestart(role), "Restart")}>Restart</button>}
                {(isRunning || isStarting) && <button type="button" className="btn btn-secondary" disabled={!canControl} onClick={() => void run(role, () => api.voiceWorkerStop(role), "Stop")}>Stop</button>}
                {isRunning && <button type="button" className="btn btn-secondary" disabled={!canControl} onClick={() => void run(role, () => api.voiceWorkerModel(role, !modelLoaded), modelLoaded ? "Unload model" : "Load model")}>{modelLoaded ? "Unload model" : "Load model"}</button>}
                {isRunning && modelLoaded && <button type="button" className="btn btn-secondary" disabled={!canControl} onClick={() => void sendTest(role)}>Send test</button>}
              </div>
            )}
          </div>
        );
      })}

      {activeRequests.length > 0 && (
        <div className="voice-requests">
          {activeRequests.map((request) => (
            <div key={request.id} className="voice-request-row">
              <span className="status-readout">
                <span className="status-dot" data-state={request.state === "active" ? "active" : "pending"} />
                <span className="status-text">{request.role} {request.type} #{request.id} — {request.state}</span>
              </span>
              <button type="button" className="btn btn-secondary" disabled={locked} onClick={() => void cancelRequest(request.id)}>Cancel</button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
