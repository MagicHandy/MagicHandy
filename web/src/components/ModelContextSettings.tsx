import type { HostedModel, ModelConnection } from "../api/cloud-types";
import { t } from "../i18n";
import { FieldRow } from "./SetupSection";

export function ModelContextSettings({ connection, models, disabled, patch }: {
  connection: ModelConnection; models: HostedModel[]; disabled: boolean; patch: (change: Partial<ModelConnection>) => void;
}) {
  const capacity = models.find(model => model.id === connection.model)?.context_length;
  const value = connection.context_window || 65536;
  const id = `connection-context-${connection.id}`;
  return <div className="form-row-details model-context-settings">
    <FieldRow id={id} label={t("External context budget")} hint={t("Room for instructions, motion state and conversation. Your local model keeps its own context setting.")}>
      <select id={id} aria-describedby={`${id}-hint`} value={value} disabled={disabled} onChange={event => patch({ context_window: Number(event.target.value) })}>
        <option value={65536}>{t("64K — standard")}</option>
        <option value={131072} disabled={Boolean(capacity && capacity < 131072)}>{t("128K — recommended when supported")}</option>
        <option value={262144} disabled={Boolean(capacity && capacity < 262144)}>{t("256K — extended")}</option>
      </select>
    </FieldRow>
    <details><summary>{t("Conversation history and usage")}</summary>
      <FieldRow id={`${id}-history`} label={t("History sent")} hint={t("Use a shorter history for lighter requests without reducing room for motion planning.")}>
        <select id={`${id}-history`} aria-describedby={`${id}-history-hint`} value={connection.history_size || ""} disabled={disabled} onChange={event => patch({ history_size: event.target.value as ModelConnection["history_size"] })}>
          <option value="">{t("Match context budget")}</option>
          <option value="recent">{t("Recent — lighter requests")}</option>
          <option value="long" disabled={value < 131072}>{t("Long — more continuity")}</option>
          <option value="extended" disabled={value < 262144}>{t("Extended — longest conversations")}</option>
        </select>
      </FieldRow>
      <p className="hint">{t("The budget is a ceiling, not a fixed request size. Choose 128K for richer motion planning when your model supports it; sending more history can increase latency and usage. History is always capped by the selected budget.")}</p>
      <p className="hint">{t("Recent includes up to 64 messages / 24 KB; Long 256 messages / 96 KB; Extended 512 messages / 192 KB. The newest complete messages that fit are sent. Larger histories can increase response time and plan usage or API charges.")}</p>
      <p className="hint">{t("Choose a budget within your model's actual context capacity. For an unlisted or self-hosted model, check its configured context first.")}</p>
      <p className="hint">{t("Applies to chat and conversation-aware Autopilot using this provider. Motion-only planning and the optional second opinion keep their limited context. Local model prompts keep their existing limits.")}</p>
      <p className="hint">{t("Each chat keeps up to 1,000 messages. The chat view shows the latest 200; the model can use earlier retained messages within its history limit. Previously deleted history cannot be recovered.")}</p>
    </details>
  </div>;
}
