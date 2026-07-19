// Preset Modes: the autonomous-motion workspace (renamed Hands-free). Both
// autonomous modes are engine clients — no separate motion pathway. Autopilot
// is Freestyle's loop with the segment choice curated by the local model;
// every decision failure falls back to the deterministic planner and says so.
import { useRef, useState } from "react";
import { api } from "../api/client";
import { WorkspaceHead } from "../components/WorkspaceHead";
import { useAppState, useToast } from "../state/app-state";

const STYLES = ["gentle", "balanced", "intense"] as const;
const cap = (s: string) => s[0].toUpperCase() + s.slice(1);
const msg = (e: unknown) => (e instanceof Error ? e.message : "Request failed");

const decisionSourceCopy: Record<string, string> = {
  model: "Model-curated",
  fallback: "Deterministic fallback (model unavailable)",
  hold: "Holding the current segment",
};

export function PresetModesRoute() {
  const { state, backendOnline, readOnly, motion, refresh } = useAppState();
  const { show } = useToast();
  const locked = !backendOnline || readOnly;
  const modes = state?.modes;
  const freestyleActive = modes?.mode === "freestyle" || modes?.active_mode === "freestyle";
  const autopilotActive = modes?.mode === "autopilot" || modes?.active_mode === "autopilot";
  const style = state?.settings?.motion?.style ?? "balanced";
  const [pending, setPending] = useState(false);
  const pendingRef = useRef(false);
  const [stylePending, setStylePending] = useState(false);
  const stylePendingRef = useRef(false);

  async function startMode(mode: "freestyle" | "autopilot") {
    if (pendingRef.current || locked) return;
    pendingRef.current = true;
    setPending(true);
    try {
      await api.startMode(mode);
      show(`${cap(mode)} started.`);
    } catch (e) {
      show(msg(e), "error");
    } finally {
      pendingRef.current = false;
      setPending(false);
      refresh();
    }
  }
  async function stopModes() {
    if (pendingRef.current || locked) return;
    pendingRef.current = true;
    setPending(true);
    try {
      await api.stopMode();
      show("Stopped.");
    } catch (e) {
      show(msg(e), "error");
    } finally {
      pendingRef.current = false;
      setPending(false);
      refresh();
    }
  }
  async function setStyle(s: string) {
    if (stylePendingRef.current || locked) return;
    stylePendingRef.current = true;
    setStylePending(true);
    try {
      await api.applyQuick({ style: s });
    } catch (e) {
      show(msg(e), "error");
    } finally {
      stylePendingRef.current = false;
      setStylePending(false);
      refresh();
    }
  }

  return (
    <>
      <WorkspaceHead title="Preset modes" lede="Autonomous motion — every mode is a client of the one motion engine." />

      <section className="panel autopilot">
        <p className="eyebrow">Autonomous motion</p>
        <div className="headline">
          <div>
            <h2 className="section-title">Autopilot</h2>
            <p className="hint-block narrow">
              Hands the wheel to the assistant: at every segment boundary it curates an enabled pattern
              and intensity from your library — bounded by your limits, fully traced, and interruptible
              by Stop and Pause. If the model is unavailable, the deterministic planner takes the segment
              and the status says so.
            </p>
          </div>
        </div>
        <div className="row-actions hint-block">
          {autopilotActive ? (
            <button type="button" className="btn btn-secondary" onClick={() => void stopModes()} disabled={locked || pending}>
              Stop Autopilot
            </button>
          ) : (
            <button type="button" className="btn btn-start" onClick={() => void startMode("autopilot")} disabled={locked || pending}>
              Start Autopilot
            </button>
          )}
          {autopilotActive && motion?.engine?.paused && <span className="form-status">Paused</span>}
        </div>
        {autopilotActive && (
          <div className="autopilot-status" role="status">
            <p className="form-status">
              Segment {modes?.segment_index ?? 0}
              {modes?.decision_source ? ` — ${decisionSourceCopy[modes.decision_source] ?? modes.decision_source}` : ""}
            </p>
            {modes?.last_say && <p className="hint-block narrow autopilot-say">“{modes.last_say}”</p>}
          </div>
        )}
      </section>

      <section className="panel">
        <h2 className="section-title">Freestyle</h2>
        <p className="hint-block">
          Deterministic autonomous motion across bounded arrangement segments.
        </p>
        <div className="row-actions hint-block">
          {freestyleActive ? (
            <button type="button" className="btn btn-secondary" onClick={() => void stopModes()} disabled={locked || pending}>
              Stop Freestyle
            </button>
          ) : (
            <button type="button" className="btn btn-start" onClick={() => void startMode("freestyle")} disabled={locked || pending}>
              Start Freestyle
            </button>
          )}
          {freestyleActive && motion?.engine?.paused && <span className="form-status">Paused</span>}
        </div>
        <div className="field">
          <span className="label">Style <span className="hint-inline">biases pacing</span></span>
          <div className="segmented" role="group" aria-label="Motion style">
            {STYLES.map((s) => (
              <button key={s} type="button" aria-pressed={style === s} disabled={locked || stylePending} onClick={() => void setStyle(s)}>
                {cap(s)}
              </button>
            ))}
          </div>
        </div>
        {locked && <p className="form-status">{readOnly ? "Read-only client." : "Core offline."}</p>}
      </section>

      <section className="panel">
        <h2 className="section-title">Preset arrangements</h2>
        <p className="coming-soon">
          Saved arrangements are not available yet.
        </p>
        <div className="chip-row" aria-hidden="true">
          {["Slow build", "Waves", "Edge", "Steady", "Cooldown"].map((c) => (
            <span key={c} className="chip chip-placeholder">{c}</span>
          ))}
        </div>
      </section>
    </>
  );
}
