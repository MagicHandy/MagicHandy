import type { ModelRoute, ModelRoutingSnapshot } from "../api/cloud-types";
import { t, translateKnown } from "../i18n";
import { modelProviderNames } from "../util/model-connections";
import { DismissibleNotice } from "./DismissibleNotice";

function providerLabel(route: ModelRoute): string {
  return route.provider === "ollama" ? "Ollama" : route.provider === "llama_cpp" ? "llama.cpp" : translateKnown(modelProviderNames[route.provider as keyof typeof modelProviderNames] ?? route.provider);
}

function routeLabel(route: ModelRoute): string {
  if (route.state === "unavailable") return t("Unavailable connection");
  if (route.kind === "decisions") return t("Decisions library selection");
  return [providerLabel(route), route.model || t("Choose a model"), route.endpoint_host].filter(Boolean).join(" · ");
}

// Render the backend resolver's projection, never infer a destination from a draft.
export function ModelRoutingSummary({ routing }: { routing?: ModelRoutingSnapshot }) {
  return <section className="model-routing-snapshot" aria-label={t("Saved model routing")}>
    <h3 className="group-title">{t("Saved model routing")}</h3>
    {!routing ? <p className="hint">{t("Loading saved model routing…")}</p> : <>
      <dl>
        <div><dt>{t("Chat")}</dt><dd>{routeLabel(routing.chat)}</dd></div>
        <div><dt>{t("Autopilot")}</dt><dd>{routeLabel(routing.autopilot)}</dd></div>
        {routing.local_retry && <div><dt>{t("On refusal")}</dt><dd>{routeLabel(routing.local_retry)}</dd></div>}
      </dl>
      {routing.chat.state === "unavailable" ? <p className="hint">{t("Chat cannot use this connection. Choose another model and save settings.")}</p> : <DismissibleNotice id="model-routing-context" className="model-help">
      <p className="hint">{routing.chat.kind === "local" ? t("Chat messages go to your configured local-model endpoint.") : t("Chat messages go to {provider}, including enabled persona and memory context.", { provider: providerLabel(routing.chat) })}</p>
      {routing.autopilot.kind !== "local" && routing.autopilot.state !== "unavailable" && <p className="hint">{routing.autopilot.context_policy === "technical"
        ? t("Autopilot receives motion details only. Chat messages, personas and memories are excluded from its requests.")
        : t("Autopilot also receives included conversation context.")}</p>}
      </DismissibleNotice>}
    </>}
  </section>;
}
