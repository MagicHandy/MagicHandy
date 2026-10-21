import type { LLMSettings } from "../api/llm-settings-types";
import { t } from "../i18n";
import { modelConnectionLabel } from "../util/model-connections";
import { FieldRow } from "./SetupSection";

export function MotionReviewerSettings({ settings, locked, local, patch }: {
  settings: LLMSettings; locked: boolean; local: boolean; patch: (change: Partial<LLMSettings>) => void;
}) {
  const reviewer = settings.motion_reviewer ?? { enabled: false, connection_id: "" };
  const connections = (settings.connections ?? []).filter(connection => connection.model.trim());
  const selected = connections.find(connection => connection.id === reviewer.connection_id);
  if (!local && !reviewer.enabled) return null;
  return <section className="group cloud-motion-settings" aria-label={t("Motion second opinion")}>
    <h3 className="group-title">{t("Motion second opinion")}</h3>
    <div className="form-rows">
      <label className="toggle-line form-row"><span className="toggle"><input type="checkbox" checked={reviewer.enabled} disabled={locked || (!reviewer.enabled && connections.length === 0)} onChange={event => patch({ motion_reviewer: { enabled: event.target.checked, connection_id: selected?.id || connections[0]?.id || "" } })} /><span className="track" aria-hidden="true" /></span>
        <span>{t("Review local motion choices")}<small>{t("Local chat and Autopilot act immediately. A second model can correct a clear mismatch afterward.")}</small></span>
      </label>
      {connections.length === 0 && <p className="hint">{t("Add a provider and choose its model below to enable review.")}</p>}
      {reviewer.enabled && <>
        <FieldRow id="motion-reviewer-model" label={t("Reviewer model")} hint={t("Choose a fast model with Low reasoning where available. Reviews expire after 10 seconds.")}>
          <select id="motion-reviewer-model" aria-describedby="motion-reviewer-model-hint" value={reviewer.connection_id} disabled={locked} onChange={event => patch({ motion_reviewer: { ...reviewer, connection_id: event.target.value } })}>
            {!selected && <option value={reviewer.connection_id || ""} disabled>{t("Choose a model")}</option>}
            {connections.map(connection => <option key={connection.id} value={connection.id}>{modelConnectionLabel(connection)}</option>)}
          </select>
        </FieldRow>
        <p className="hint">{t("Shares motion details and up to eight recent user messages with the reviewer. Personas, memories and assistant replies are excluded.")}</p>
        {!local && <p className="hint" role="status">{t("Waiting for a local Chat or Autopilot model. Hosted choices are not reviewed.")}</p>}
        <details className="form-row-details"><summary>{t("How corrections work")}</summary><p className="hint">{t("Only confident, material corrections are applied within your limits. A new request or motion change makes an older review obsolete. Reviews never start or resume motion; an unavailable reviewer leaves the local choice running.")}</p></details>
      </>}
    </div>
  </section>;
}
