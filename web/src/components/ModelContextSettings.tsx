import type { HostedModel, ModelConnection } from "../api/cloud-types";
import type { ModelCapabilityOption } from "../api/llm-settings-types";
import { t } from "../i18n";
import { capabilityFloor, capabilityNames, connectionCapability, connectionContextSizes, contextLabel } from "../util/model-connections";
import { CapabilityRows } from "./ModelCapabilityField";
import { FieldRow } from "./SetupSection";

const levels = Object.keys(capabilityNames) as Array<keyof typeof capabilityNames>;

// A connection's capability and request budget, the same pair the local model
// has. A server such as LM Studio can run a small model with a short loaded
// context, so its budget can drop to the Simple or Intermediate floor.
export function ModelContextSettings({ connection, models, options = [], disabled, patch }: {
  connection: ModelConnection; models: HostedModel[]; options?: ModelCapabilityOption[]; disabled: boolean; patch: (change: Partial<ModelConnection>) => void;
}) {
  const capacity = models.find(model => model.id === connection.model)?.context_length;
  const level = connectionCapability(connection);
  const choices = options.length ? options : levels.map(item => ({ level: item, minimum_context: capabilityFloor(item), minimum_output: 0 }));
  const value = Math.max(connection.context_window || 0, capabilityFloor(level, choices));
  const sizes = connectionContextSizes.filter(size => !capacity || size <= capacity);
  const id = `connection-${connection.id}`;
  const server = connection.provider === "compatible";
  return <>
    <CapabilityRows id={id} level={level} options={choices} context={value} contextSizes={sizes} locked={disabled}
      contextHint={server
        ? t("Set the server's context length to at least {context}. MagicHandy budgets each request to fit.", { context: contextLabel(value) })
        : t("Room for instructions, motion state and conversation. Choose 128K for richer motion planning when the model supports it.")}
      onChange={change => patch({ ...(change.level ? { capability_level: change.level.level } : {}), ...(change.context !== undefined ? { context_window: change.context } : {}) })} />
    <details className="form-row-details"><summary>{t("Conversation history and usage")}</summary>
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
  </>;
}
