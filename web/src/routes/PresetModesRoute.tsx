// Preset Modes: the autonomous-motion workspace (renamed Hands-free). Freestyle
// is live; Autopilot is staged coming-soon until the backend planner exists
// (docs/react-ui-implementation-handoff.md, step 4). All modes are engine
// clients — no separate motion pathway.
import { useState } from "react";
import { api } from "../api/client";
import { WorkspaceHead } from "../components/WorkspaceHead";
import { useAppState, useToast } from "../state/app-state";

const STYLES = ["gentle", "balanced", "intense"] as const;
const cap = (s: string) => s[0].toUpperCase() + s.slice(1);
const msg = (e: unknown) => (e instanceof Error ? e.message : "Request failed");

export function PresetModesRoute() {
  const { state, backendOnline, readOnly, motion, refresh } = useAppState();
  const { show } = useToast();
  const locked = !backendOnline || readOnly;
  const modes = state?.modes;
  const freestyleActive =
    modes?.running === true || modes?.mode === "freestyle" || modes?.active_mode === "freestyle";
  const style = state?.settings?.motion?.style ?? "balanced";
  const [pending, setPending] = useState(false);

  async function startFreestyle() {
    setPending(true);
    try {
      await api.startMode("freestyle");
      show("Freestyle started.");
    } catch (e) {
      show(msg(e), "error");
    } finally {
      setPending(false);
      refresh();
    }
  }
  async function stopModes() {
    setPending(true);
    try {
      await api.stopMode();
      show("Stopped.");
    } catch (e) {
      show(msg(e), "error");
    } finally {
      setPending(false);
      refresh();
    }
  }
  async function setStyle(s: string) {
    try {
      await api.applyQuick({ style: s });
    } catch (e) {
      show(msg(e), "error");
    } finally {
      refresh();
    }
  }

  return (
    <>
      <WorkspaceHead title="Freestyle" lede="Autonomous motion — every mode is a client of the one motion engine." />

      <section className="panel autopilot">
        <p className="eyebrow">Autonomous motion</p>
        <div className="headline">
          <div>
            <h2 className="section-title">Autopilot</h2>
            <p className="hint-block narrow">
              Hands the wheel to the assistant: it changes direction, pattern, and intensity from the
              conversation — bounded by your quick-settings limits, fully traced, and interruptible by Stop
              and Pause.
            </p>
          </div>
          <label className="toggle-line" title="Coming soon">
            <span className="toggle">
              <input type="checkbox" role="switch" disabled aria-label="Autopilot (coming soon)" />
              <span className="track" aria-hidden="true" />
            </span>
          </label>
        </div>
        <p className="coming-soon">
          Coming soon — needs the autopilot planner (a Phase 11 mode). Off by default and fail-safe when it lands.
        </p>
      </section>

      <section className="panel">
        <h2 className="section-title">Freestyle</h2>
        <p className="hint-block">
          Deterministic autonomous motion across bounded arrangement segments.
        </p>
        <div className="row-actions hint-block">
          {freestyleActive ? (
            <button type="button" className="btn btn-secondary" onClick={() => void stopModes()} disabled={!backendOnline || pending}>
              Stop Freestyle
            </button>
          ) : (
            <button type="button" className="btn btn-start" onClick={() => void startFreestyle()} disabled={locked || pending}>
              Start Freestyle
            </button>
          )}
          {motion?.engine?.paused && <span className="form-status">Paused</span>}
        </div>
        <div className="field">
          <span className="label">Style <span className="hint-inline">biases pacing</span></span>
          <div className="segmented" role="group" aria-label="Motion style">
            {STYLES.map((s) => (
              <button key={s} type="button" aria-pressed={style === s} disabled={locked} onClick={() => void setStyle(s)}>
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
          Named bounded segment sets — arriving with the Pattern Library (Phase 14).
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
