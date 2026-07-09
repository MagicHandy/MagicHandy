// Live status + controls for the optional voice workers (ADR 0003). Voice is
// entirely optional: every state here (disabled, not configured, stopped,
// crashed) is a visible readout, never a blocker for the rest of the app.
// Status readouts follow the design guidelines: dot + text, no pills.
import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { VoiceRequestSnapshot, VoiceWorkerStatus } from "../api/types";
import { useToast } from "../state/app-state";

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

export function VoiceWorkers({ locked }: { locked: boolean }) {
  const { show } = useToast();
  const [workers, setWorkers] = useState<Record<string, VoiceWorkerStatus>>({});
  const [requests, setRequests] = useState<VoiceRequestSnapshot[]>([]);
  const [busyRole, setBusyRole] = useState<string | null>(null);
  const alive = useRef(true);

  const refresh = useCallback(async () => {
    try {
      const res = await api.voiceStatus();
      if (!alive.current) return;
      setWorkers(res.voice.workers ?? {});
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

  const roles: ("tts" | "asr")[] = ["tts", "asr"];
  const activeRequests = requests.filter((r) => r.state === "queued" || r.state === "active");

  return (
    <div className="voice-workers">
      {roles.map((role) => {
        const worker = workers[role];
        const state = worker?.state ?? "disabled";
        const canControl = !locked && busyRole !== role && state !== "disabled" && state !== "not_configured";
        const modelLoaded = worker?.model_state === "ready";
        return (
          <div key={role} className="voice-worker-row">
            <div className="voice-worker-head">
              <span className="voice-worker-name">{ROLE_LABEL[role]}</span>
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
              <p className="form-status">No worker command configured. Set a worker path above and save.</p>
            )}
            {worker?.last_error && state !== "running" && (
              <p className="form-status voice-worker-error">{worker.last_error}</p>
            )}
            <div className="row-actions">
              <button type="button" className="btn btn-secondary" disabled={!canControl || state === "running" || state === "starting"} onClick={() => void run(role, () => api.voiceWorkerStart(role), "Start")}>Start</button>
              <button type="button" className="btn btn-secondary" disabled={!canControl || (state !== "running" && state !== "starting" && state !== "crashed")} onClick={() => void run(role, () => api.voiceWorkerStop(role), "Stop")}>Stop</button>
              <button type="button" className="btn btn-secondary" disabled={!canControl} onClick={() => void run(role, () => api.voiceWorkerRestart(role), "Restart")}>Restart</button>
              <button type="button" className="btn btn-secondary" disabled={!canControl || state !== "running"} onClick={() => void run(role, () => api.voiceWorkerModel(role, !modelLoaded), modelLoaded ? "Unload model" : "Load model")}>{modelLoaded ? "Unload model" : "Load model"}</button>
              <button type="button" className="btn btn-secondary" disabled={!canControl || state !== "running" || !modelLoaded} onClick={() => void run(role, () => api.voiceWorkerTest(role, { text: "MagicHandy voice test", delay_ms: 0 }), "Test request")}>Send test</button>
            </div>
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
