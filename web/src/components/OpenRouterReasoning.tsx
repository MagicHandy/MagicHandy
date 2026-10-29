import type { HostedModel, ModelConnection } from "../api/cloud-types";
import { t } from "../i18n";

export function routerReasoningChange(connection: ModelConnection, model?: HostedModel): Partial<ModelConnection> {
  const effort = connection.reasoning_effort ?? "";
  const enabled = connection.reasoning_enabled;
  if (model?.reasoning_supported && enabled !== undefined && (!model.reasoning_mandatory || enabled)) {
    return { reasoning_effort: "", reasoning_enabled: enabled };
  }
  return { reasoning_effort: model?.reasoning_efforts?.includes(effort) && !(effort === "none" && model.reasoning_mandatory) ? effort : "", reasoning_enabled: undefined };
}

export function OpenRouterReasoning({ connection, model, disabled, patch }: {
  connection: ModelConnection; model?: HostedModel; disabled: boolean; patch: (change: Partial<ModelConnection>) => void;
}) {
  const effort = connection.reasoning_effort ?? "";
  const value = connection.reasoning_enabled === false || effort === "none" ? "off" : connection.reasoning_enabled === true ? "on" : effort;
  if (!model?.reasoning_supported && !value) return null;
  const names: Record<string, string> = { none: t("No extra reasoning"), minimal: t("Minimal reasoning"), low: t("Low — quicker responses"),
    medium: t("Medium — more deliberation"), high: t("High — slower responses"), xhigh: t("Extra high reasoning"), max: t("Maximum reasoning") };
  const options = model?.reasoning_supported ? [
    ...(!model.reasoning_mandatory ? [{ value: "off", name: t("Off") }] : []),
    { value: "on", name: t("Enabled — provider decides effort") },
    ...(model.reasoning_efforts ?? []).filter(level => level !== "none").map(level => ({ value: level, name: names[level] ?? level })),
  ] : [];
  const unavailable = value !== "" && !options.some(option => option.value === value);
  return <div className="form-row model-response-settings">
    <span className="form-row-label"><label htmlFor={`reasoning-${connection.id}`}><strong>{t("Reasoning")}</strong></label>
      <small id={`reasoning-help-${connection.id}`}>{model?.reasoning_mandatory
        ? t("This model requires reasoning. Lower effort usually responds sooner.")
        : t("Less reasoning can respond sooner, but may reduce motion quality. Provider default keeps the model's own setting.")}</small></span>
    <select id={`reasoning-${connection.id}`} aria-describedby={`reasoning-help-${connection.id}`} disabled={disabled} value={value}
      onChange={event => { const next = event.target.value; const offEffort = next === "off" && model?.reasoning_efforts?.includes("none"); patch({ reasoning_effort: offEffort ? "none" : next === "on" || next === "off" ? "" : next, reasoning_enabled: next === "on" ? true : next === "off" && !offEffort ? false : undefined }); }}>
      <option value="">{t("Provider default")}</option>
      {unavailable && <option value={value} disabled>{t("Saved setting unavailable — choose another")}</option>}
      {options.map(option => <option value={option.value} key={option.value}>{option.name}</option>)}
    </select>
  </div>;
}
