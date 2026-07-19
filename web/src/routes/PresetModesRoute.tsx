// Preset Modes owns deterministic autonomous motion. Assistant-driven
// Autopilot lives with its conversation on the Chat route; both remain clients
// of the same motion engine.
import { useRef, useState } from "react";
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
  const freestyleActive = modes?.mode === "freestyle" || modes?.active_mode === "freestyle";
  const style = state?.settings?.motion?.style ?? "balanced";
  const [pending, setPending] = useState(false);
  const pendingRef = useRef(false);
  const [stylePending, setStylePending] = useState(false);
  const stylePendingRef = useRef(false);

  async function startFreestyle() {
    if (pendingRef.current || locked) return;
    pendingRef.current = true;
    setPending(true);
    try {
      await api.startMode("freestyle");
      show("Freestyle started.");
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
      <WorkspaceHead title="Preset modes" lede="Deterministic autonomous motion through the shared engine." />

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
            <button type="button" className="btn btn-start" onClick={() => void startFreestyle()} disabled={locked || pending}>
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
