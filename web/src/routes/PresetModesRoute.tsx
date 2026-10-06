import { t, translateKnown } from "../i18n";
// Preset Modes owns deterministic autonomous motion. Assistant-driven
// Autopilot lives with its conversation on the Chat route; both remain clients
// of the same motion engine.
import { useRef, useState } from "react";
import { api } from "../api/client";
import { FreestylePreferences, freestyleShapeStatus } from "../components/FreestyleControls";
import { WorkspaceHead } from "../components/WorkspaceHead";
import { useAppState, useToast , useMotionState } from "../state/app-state";

const msg = (e: unknown) => (e instanceof Error ? translateKnown(e.message) : t("Request failed"));

export function PresetModesRoute() {
  const { state, backendOnline, readOnly, refresh } = useAppState();
  const motion = useMotionState();
  const { show } = useToast();
  const locked = !backendOnline || readOnly;
  const modes = state?.modes;
  const freestyleActive = modes?.mode === "freestyle" || modes?.active_mode === "freestyle";
  const preferences = state?.settings?.freestyle;
  const shapeStatus = freestyleActive ? freestyleShapeStatus(modes?.freestyle) : "";
  const [pending, setPending] = useState(false);
  const pendingRef = useRef(false);

  async function startFreestyle() {
    if (pendingRef.current || locked) return;
    pendingRef.current = true;
    setPending(true);
    try {
      await api.startMode("freestyle");
      show(t("Freestyle started."));
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
      show(t("Stopped."));
    } catch (e) {
      show(msg(e), "error");
    } finally {
      pendingRef.current = false;
      setPending(false);
      refresh();
    }
  }

  return (
    <>
      <WorkspaceHead title={t("Preset modes")} />

      <section className="panel">
        <h2 className="section-title">{t("Freestyle")}</h2>
        <p className="hint-block">{t("One continuous stream of strokes that keeps evolving. Shape it with the controls below; changes ease in while it plays.")}</p>
        <div className="row-actions hint-block">
          {freestyleActive ? (
            <button type="button" className="btn btn-secondary" onClick={() => void stopModes()} disabled={locked || pending}>{t("Stop Freestyle")}</button>
          ) : (
            <button type="button" className="btn btn-start" onClick={() => void startFreestyle()} disabled={locked || pending}>{t("Start Freestyle")}</button>
          )}
          {freestyleActive && motion?.engine?.paused && <span className="form-status">{t("Paused")}</span>}
          {shapeStatus && <span className="form-status freestyle-shape-status" role="status">{shapeStatus}</span>}
        </div>
        {preferences && (
          <FreestylePreferences
            value={preferences}
            disabled={locked || modes?.freestyle?.ending === true}
            onSaved={() => refresh()}
            onError={(error) => show(msg(error), "error")}
          />
        )}
        {locked && <p className="form-status">{readOnly ? t("Read-only client.") : t("Core offline.")}</p>}
      </section>
    </>
  );
}
