import { useCallback, useEffect, useState } from "react";
import { api } from "../api/client";
import type { PromptCompositionPayload } from "../api/types";
import { t, translateKnown } from "../i18n";
import { useAppState, useToast } from "../state/app-state";

export function PromptAssignment({ selectedID, defaultID, chooseDefault, locked, unsaved }: {
  selectedID: string; defaultID: string; chooseDefault: (id: string) => void; locked: boolean; unsaved: boolean;
}) {
  const { state, refresh } = useAppState();
  const { show } = useToast();
  const [active, setActive] = useState<PromptCompositionPayload>();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const sessionID = state?.chat?.active_session_id;
  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      const next = await api.promptComposition(signal);
      if (!signal?.aborted) { setActive(next); setError(""); }
    } catch (reason) {
      if (!signal?.aborted) setError(reason instanceof Error ? translateKnown(reason.message) : t("Request failed"));
    }
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    setActive(undefined);
    void load(controller.signal);
    return () => controller.abort();
  }, [load, sessionID, state?.settings?.llm.prompt_set]);
  async function assignPersona() {
    if (!active?.persona_id || busy || locked || unsaved) return;
    setBusy(true);
    try {
      await api.updatePersona(active.persona_id, { prompt_set_id: selectedID });
      await load();
      refresh();
      show(t("Prompt set assigned."));
    } catch (reason) { show(reason instanceof Error ? translateKnown(reason.message) : t("Request failed"), "error"); }
    finally { setBusy(false); }
  }
  return <div className="prompt-assignment">
    {active && <>
      <p><strong>{t("Used by the current chat")}</strong>{" · "}{active.prompt_set_name || active.prompt_set}</p>
      <p className="hint-block">{active.prompt_set_source === "persona"
        ? t("{name}'s prompt set overrides the default.", { name: active.persona_name || t("Persona") })
        : t("This chat follows the default prompt set.")}</p>
    </>}
    {error && <p className="form-status" role="alert">{error} <button type="button" className="btn btn-secondary" onClick={() => void load()}>{t("Retry")}</button></p>}
    <div className="row-actions">
      {active?.persona_id && <button type="button" className="btn btn-secondary" disabled={locked || busy || unsaved || active.prompt_set === selectedID} onClick={() => void assignPersona()}>
        {t("Use for {name}", { name: active.persona_name || t("Persona") })}
      </button>}
      <button type="button" className="btn btn-secondary" disabled={locked || busy || unsaved || selectedID === defaultID} onClick={() => {
        chooseDefault(selectedID);
        show(t("Default selected. Save settings to apply."));
      }}>{t("Choose as default")}</button>
    </div>
    {unsaved && <p className="hint-block">{t("Save this set before assigning it.")}</p>}
  </div>;
}
