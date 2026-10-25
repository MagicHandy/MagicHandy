import type { LLMSettings } from "../api/llm-settings-types";
import { t } from "../i18n";
import { modelConnectionLabel } from "../util/model-connections";
import { FieldRow } from "./SetupSection";

// The second opinion is a model role like Chat and Autopilot: a connection that
// reviews motion chosen by Simple and Intermediate models, local or served.
// `eligible` says whether a draft role is below Advanced.
export function SecondOpinionRow({ settings, locked, eligible, patch }: {
  settings: LLMSettings; locked: boolean; eligible: boolean; patch: (change: Partial<LLMSettings>) => void;
}) {
  const reviewer = settings.motion_reviewer ?? { enabled: false, connection_id: "" };
  const connections = (settings.connections ?? []).filter(connection => connection.model.trim());
  const selected = connections.find(connection => connection.id === reviewer.connection_id);
  if (!eligible && !reviewer.enabled) return null;
  const hint = !reviewer.enabled
    ? connections.length ? t("Checks motion chosen by Simple and Intermediate models and corrects a clear mismatch.") : t("Checks motion chosen by Simple and Intermediate models. Add a provider under Models to choose a reviewer.")
    : !eligible ? t("Chat and Autopilot use Advanced models, which are not reviewed.")
      : t("Shares motion details and up to eight recent user messages with the reviewer. Personas, memories and assistant replies are excluded. Reviews expire after 10 seconds.");
  return <>
    <FieldRow id="model-role-review" label={t("Second opinion")} hint={hint}>
      <select id="model-role-review" aria-describedby="model-role-review-hint" value={reviewer.enabled ? reviewer.connection_id || "" : "off"} disabled={locked || (!reviewer.enabled && connections.length === 0)}
        onChange={event => patch({ motion_reviewer: event.target.value === "off" ? { enabled: false, connection_id: reviewer.connection_id ?? "" } : { enabled: true, connection_id: event.target.value } })}>
        <option value="off">{t("Off")}</option>
        {reviewer.enabled && !selected && <option value={reviewer.connection_id || ""} disabled>{t("Choose a model")}</option>}
        {connections.map(connection => <option key={connection.id} value={connection.id}>{modelConnectionLabel(connection)}</option>)}
      </select>
    </FieldRow>
    {reviewer.enabled && <details className="form-row-details"><summary>{t("How corrections work")}</summary><p className="hint">{t("Only confident, material corrections are applied within your limits. A new request or motion change makes an older review obsolete. Reviews never start or resume motion; an unavailable reviewer leaves the original choice running.")}</p></details>}
  </>;
}
