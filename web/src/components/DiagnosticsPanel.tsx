// Diagnostics: a read-only status grid from backend state, a one-click copyable
// summary for bug reports, trace export, and a double-confirm settings reset
// (memories and prompt sets are deliberately untouched by reset).
import { useState } from "react";
import { api } from "../api/client";
import { useAppState, useToast } from "../state/app-state";

const msg = (e: unknown) => (e instanceof Error ? e.message : "Request failed");

function download(name: string, content: string) {
  const blob = new Blob([content], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

export function DiagnosticsPanel({ locked = false }: { locked?: boolean }) {
  const { state, backendOnline, refresh } = useAppState();
  const { show } = useToast();
  const [confirmReset, setConfirmReset] = useState(false);
  const engine = state?.motion?.engine;
  const intiface = state?.intiface_transport?.status;

  const rows: Array<[string, string]> = [
    ["Version", String(state?.version ?? "dev")],
    ["Commit", String(state?.commit ?? "unknown")],
    ["Uptime", `${state?.uptime_seconds ?? 0}s`],
    ["Engine", engine?.running ? "running" : engine?.paused ? "paused" : "idle"],
    ["Estimated position", engine?.last_sample ? `${Math.round(engine.last_sample.position_percent)}%` : "—"],
    ["Data dir", String(state?.data_dir ?? "—")],
    ["Datastore", String(state?.datastore_path ?? "—")],
  ];
  if (intiface) {
    rows.push(
      ["Intiface playback", intiface.playback_state || "idle"],
      ["Intiface buffer", `${intiface.queue_depth} queued / ${intiface.queue_coverage_ms ?? 0}ms`],
      ["Intiface pending ACKs", String(intiface.pending_acks ?? 0)],
      ["Intiface ACK latency", `${intiface.last_ack_latency_ms ?? 0}ms last / ${intiface.max_ack_latency_ms ?? 0}ms max`],
      ["Intiface send lateness", `${intiface.last_send_lateness_ms ?? 0}ms last / ${intiface.max_send_lateness_ms ?? 0}ms max`],
      ["Intiface resolution", intiface.selected_resolution_percent ? `${intiface.selected_resolution_percent.toFixed(3)}%` : "—"],
    );
  }

  async function copy() {
    const bundle = JSON.stringify(
      {
        version: state?.version,
        commit: state?.commit,
        uptime_seconds: state?.uptime_seconds,
        motion: state?.motion,
        transport: state?.transport,
        intiface_transport: state?.intiface_transport,
        controller: state?.controller,
        settings_status: state?.settings_status,
      },
      null,
      2,
    );
    try {
      await navigator.clipboard.writeText(bundle);
      show("Diagnostics summary copied.");
    } catch {
      show("Clipboard unavailable.", "error");
    }
  }
  async function exportTrace() {
    try {
      const data = await api.exportTrace();
      download("magichandy-trace.json", JSON.stringify(data, null, 2));
    } catch (e) {
      show(msg(e), "error");
    }
  }
  async function reset() {
    if (!confirmReset) {
      setConfirmReset(true);
      return;
    }
    setConfirmReset(false);
    try {
      await api.resetSettings();
      show("Settings reset to defaults.");
      refresh();
    } catch (e) {
      show(msg(e), "error");
    }
  }

  return (
    <>
      <div className="row-actions hint-block">
        <button type="button" className="btn btn-secondary" onClick={() => void copy()}>Copy summary</button>
        <button type="button" className="btn btn-secondary" disabled={!backendOnline} onClick={() => void exportTrace()}>Export trace</button>
      </div>
      <dl className="meta-grid">
        {rows.map(([k, v]) => (
          <div key={k}>
            <dt>{k}</dt>
            <dd>{v}</dd>
          </div>
        ))}
      </dl>
      <div className="divider" />
      <div className="group">
        <h3 className="group-title">Reset</h3>
        <p className="hint-block">
          Restores every setting to factory defaults, including the connection key. Saved memories and prompt
          sets are not touched.
        </p>
        <button type="button" className="btn btn-danger-outline" disabled={locked} onClick={() => void reset()}>
          {confirmReset ? "Confirm reset all settings" : "Reset all settings"}
        </button>
      </div>
    </>
  );
}
