import { useCallback, useEffect, useState } from "react";
import { api } from "../api/client";
import type { PromptCompositionPayload } from "../api/types";
import { t, translateKnown } from "../i18n";
import { useAppState, useToast } from "../state/app-state";

const message = (error: unknown) => error instanceof Error ? translateKnown(error.message) : t("Request failed");

export function PromptCompositionInspector() {
  const { backendOnline } = useAppState();
  const { show } = useToast();
  const [payload, setPayload] = useState<PromptCompositionPayload | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);

  const load = useCallback(async (signal?: AbortSignal) => {
    setLoading(true);
    try {
      setPayload(await api.promptComposition(signal));
      setError("");
    } catch (loadError) {
      if (!signal?.aborted) setError(message(loadError));
    } finally {
      if (!signal?.aborted) setLoading(false);
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const copy = async () => {
    if (!payload) return;
    try {
      await navigator.clipboard.writeText(payload.composition.prompt);
      show(t("Exact model prompt copied."));
    } catch {
      show(t("Clipboard unavailable."), "error");
    }
  };

  return (
    <div className="group prompt-inspector">
      <h3 className="group-title">{t("Prompt composition")}</h3>
      <p className="hint-block">
        {t("This is the exact system prompt the backend would compose for the active chat now. A new message can change Relevant only lore matches.")}
      </p>
      <div className="row-actions">
        <button type="button" className="btn btn-primary" disabled={!payload} onClick={() => void copy()}>
          {t("Copy exact prompt")}
        </button>
        <button type="button" className="btn btn-secondary" disabled={!backendOnline || loading} onClick={() => void load()}>
          {t("Refresh")}
        </button>
      </div>
      {loading && <p className="form-status" role="status">{t("Composing prompt")}</p>}
      {error && <p className="form-status" role="alert">{error}</p>}
      {payload && (
        <>
          <dl className="prompt-inspector-summary">
            <div><dt>{t("Model")}</dt><dd>{payload.model || t("Not selected")}</dd></div>
            <div><dt>{t("Prompt set")}</dt><dd>{payload.prompt_set}</dd></div>
            <div><dt>{t("Persona")}</dt><dd>{payload.persona_name || t("No persona")}</dd></div>
            <div>
              <dt>{t("Lore")}</dt>
              <dd>
                {loreModeLabel(payload.lore_mode)} / {t("{count} characters", { count: payload.lore.characters ?? 0 })}
              </dd>
            </div>
            <div>
              <dt>{t("Total")}</dt>
              <dd>{t("{count} characters", { count: payload.composition.characters })}</dd>
            </div>
          </dl>
          <div className="prompt-section-list" aria-label={t("Prompt sections")}>
            {payload.composition.sections.map((section) => (
              <details key={section.id} className="prompt-section">
                <summary>
                  <span>{translateKnown(section.title)}</span>
                  <span>{t("{count} characters", { count: section.characters })}</span>
                </summary>
                <pre>{section.text}</pre>
              </details>
            ))}
          </div>
          <details className="prompt-exact">
            <summary>{t("View exact prompt")}</summary>
            <pre className="diagnostics-report">{payload.composition.prompt}</pre>
          </details>
        </>
      )}
    </div>
  );
}

function loreModeLabel(mode?: string): string {
  switch (mode) {
    case "relevant": return t("Relevant only");
    case "full": return t("Full");
    default: return t("Off");
  }
}
