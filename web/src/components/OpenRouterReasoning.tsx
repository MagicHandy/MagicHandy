import type { HostedModel, ModelConnection } from "../api/cloud-types";
import { t } from "../i18n";

function reasoningSelection(value: string, model?: HostedModel): Partial<ModelConnection> {
  if (value === "off") return model?.reasoning_efforts?.includes("none")
    ? { reasoning_effort: "none", reasoning_enabled: undefined }
    : { reasoning_effort: "", reasoning_enabled: false };
  return { reasoning_effort: value === "on" ? "" : value, reasoning_enabled: value === "on" ? true : undefined };
}

export function routerReasoningChange(connection: ModelConnection, model?: HostedModel): Partial<ModelConnection> {
  const effort = connection.reasoning_effort ?? "";
  const enabled = connection.reasoning_enabled;
  if (model?.reasoning_supported) {
    if ((effort === "none" || enabled === false) && !model.reasoning_mandatory) return reasoningSelection("off", model);
    if (enabled === true) return reasoningSelection("on", model);
    if (effort !== "none" && model.reasoning_efforts?.includes(effort)) return reasoningSelection(effort, model);
  }
  return reasoningSelection("");
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
  const unknown = !model && value !== "";
  const unavailable = Boolean(model) && value !== "" && !options.some(option => option.value === value);
  const selectedName = value === "off" ? t("Off") : value === "on" ? t("Enabled — provider decides effort") : names[value] ?? value;
  const defaultEffort = model?.default_reasoning_effort;
  const shortNames: Record<string, string> = { none: t("Off"), minimal: t("Minimal"), low: t("Low"), medium: t("Medium"), high: t("High"), xhigh: t("Extra high"), max: t("Maximum") };
  const defaultName = defaultEffort && shortNames[defaultEffort] ? `${t("Provider default")} (${shortNames[defaultEffort]})` : t("Provider default");
  return <div className="form-row model-response-settings">
    <span className="form-row-label"><label htmlFor={`reasoning-${connection.id}`}><strong>{t("Reasoning")}</strong></label>
      <small id={`reasoning-help-${connection.id}`}>{model?.reasoning_mandatory
        ? t("This model requires reasoning. Lower effort usually responds sooner.")
        : t("Less reasoning can respond sooner, but may reduce motion quality. Provider default keeps the model's own setting.")}</small></span>
    <select id={`reasoning-${connection.id}`} aria-describedby={`reasoning-help-${connection.id}`} disabled={disabled} value={value}
      onChange={event => patch(reasoningSelection(event.target.value, model))}>
      <option value="">{defaultName}</option>
      {unknown && <option value={value} disabled>{selectedName}</option>}
      {unavailable && <option value={value} disabled>{t("Saved setting unavailable — choose another")}</option>}
      {options.map(option => <option value={option.value} key={option.value}>{option.name}</option>)}
    </select>
  </div>;
}
