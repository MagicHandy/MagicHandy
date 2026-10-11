import type { HostedModel, ModelConnection } from "../api/cloud-types";
import { t } from "../i18n";
import { FieldRow } from "./SetupSection";

export function ModelContextSettings({ connection, models, disabled, patch }: {
  connection: ModelConnection; models: HostedModel[]; disabled: boolean; patch: (change: Partial<ModelConnection>) => void;
}) {
  const capacity = models.find(model => model.id === connection.model)?.context_length;
  const value = connection.context_window || 32768;
  const id = `connection-context-${connection.id}`;
  return <div className="form-row-details model-context-settings">
    <FieldRow id={id} label={t("Conversation history")} hint={t("More history helps keep stories and motion plans consistent across turns.")}>
      <select id={id} aria-describedby={`${id}-hint`} value={value} disabled={disabled} onChange={event => patch({ context_window: Number(event.target.value) })}>
        <option value={32768}>{t("Recent — lighter requests")}</option>
        <option value={131072} disabled={Boolean(capacity && capacity < 131072)}>{connection.provider === "chatgpt" ? t("Long — recommended for ChatGPT") : t("Long — more continuity")}</option>
        <option value={262144} disabled={Boolean(capacity && capacity < 262144)}>{t("Extended — longest conversations")}</option>
      </select>
    </FieldRow>
    <details><summary>{t("History limits and usage")}</summary>
      <p className="hint">{t("Recent includes up to 64 messages / 24 KB; Long 256 messages / 96 KB; Extended 512 messages / 192 KB. The newest complete messages that fit are sent. Larger histories can increase response time and plan usage or API charges.")}</p>
      <p className="hint">{t("Long needs a model with at least 128K context; Extended needs 256K. This setting cannot increase the model's own capacity. For an unlisted or self-hosted model, check its configured context first.")}</p>
      <p className="hint">{t("Applies to chat and conversation-aware Autopilot using this provider. Motion-only planning and the optional second opinion keep their limited context. Local model prompts keep their existing limits.")}</p>
      <p className="hint">{t("Each chat keeps up to 1,000 messages. The chat view shows the latest 200; the model can use earlier retained messages within its history limit. Previously deleted history cannot be recovered.")}</p>
    </details>
  </div>;
}
