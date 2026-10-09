import { DismissibleNotice } from "./DismissibleNotice";
import type { HostedModel, ModelConnection } from "../api/cloud-types";
import { t } from "../i18n";

export function modelReadyMessage(elapsedMillis?: number): string {
  return elapsedMillis && elapsedMillis > 0 ? t("Ready · {seconds} s", { seconds: (elapsedMillis / 1000).toFixed(1) }) : t("Ready");
}

// Use the account catalog for ChatGPT. A manual model can keep its saved effort,
// but we do not infer that an undiscovered model accepts another model's levels.
export function selectedModelChange(model: HostedModel | undefined, id: string, connection: ModelConnection): Partial<ModelConnection> {
  const levels = model?.reasoning_efforts ?? [];
  const current = connection.reasoning_effort ?? "";
  return { model: id, reasoning_effort: levels.includes(current) ? current : levels.includes("low") ? "low" : "" };
}

export function ModelResponseSettings({ connection, models, disabled, patch, compact = false }: {
  connection: ModelConnection; models: HostedModel[]; disabled: boolean; patch: (change: Partial<ModelConnection>) => void; compact?: boolean;
}) {
  if (connection.provider !== "chatgpt" && connection.provider !== "openai") return null;
  const model = models.find(item => item.id === connection.model);
  const levels = model?.reasoning_efforts ?? [];
  const effort = connection.reasoning_effort ?? "";
  const names: Record<string, string> = {
    none: t("No extra reasoning"), minimal: t("Minimal reasoning"), low: t("Low — quicker responses"),
    medium: t("Medium — more deliberation"), high: t("High — slower responses"),
    xhigh: t("Extra high reasoning"), max: t("Maximum reasoning"),
  };
  const effortField = (levels.length > 0 || effort) && <label className="field"><span className="label">{t("Reasoning effort")}</span>
      <select value={effort} disabled={disabled} onChange={event => patch({ reasoning_effort: event.target.value })}>
        <option value="">{t("Provider default")}</option>
        {effort && !levels.includes(effort) && <option value={effort}>{names[effort] ?? effort}</option>}
        {levels.map(level => <option key={level} value={level}>{names[level] ?? level}</option>)}
      </select>
    </label>;
  if (compact && connection.provider === "chatgpt") return <div className="model-response-settings">
    <p className="hint">{t("Recommended for motion: GPT-6 Sol with Low reasoning.")}</p>
    <details><summary>{t("Response speed") + ": " + (names[effort] ?? t("Provider default"))}</summary>
      {effortField}
      <DismissibleNotice id="model-response-speed" className="model-help"><p className="hint">{t("Low reasoning favors quick updates. Faster service tiers are not verified for ChatGPT plan connections. Response time also depends on the network and account load.")}</p></DismissibleNotice>
    </details>
  </div>;
  return <div className="model-response-settings">
    {effortField}
    {connection.provider === "chatgpt" && <DismissibleNotice id="model-response-speed" className="model-help">
      <p className="hint">{t("Recommended for motion: GPT-6 Sol with Low reasoning.")}</p>
      <details><summary>{t("About response speed")}</summary>
      <p className="hint">{t("Start with GPT-6 Sol and Low reasoning for responsive, reliable motion. GPT-6 Luna is quicker for simple edits but missed details in compound requests. GPT-6.1 Sol is a more deliberate alternative.")}</p>
      <p className="hint">{t("Low reasoning favors quick updates. Faster service tiers are not verified for ChatGPT plan connections. Response time also depends on the network and account load.")}</p>
    </details></DismissibleNotice>}
  </div>;
}
