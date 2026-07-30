import { useCallback, useEffect, useState } from "react";
import { t } from "../i18n";
import { api } from "../api/client";
import type { Persona, PersonasPayload } from "../api/types";
import { PersonaEditor } from "../components/PersonaEditor";
import { PersonaGrid } from "../components/PersonaGrid";
import { WorkspaceHead } from "../components/WorkspaceHead";
import { useAppState, useToast } from "../state/app-state";

export function PersonasRoute() {
  const { backendOnline, readOnly, state } = useAppState();
  const { show } = useToast();
  const [payload, setPayload] = useState<PersonasPayload | null>(null);
  const [error, setError] = useState("");
  const [editingID, setEditingID] = useState("");
  const [adding, setAdding] = useState(false);
  const autopilotActive = state?.modes?.mode === "autopilot"
    || state?.modes?.active_mode === "autopilot";
  const locked = !backendOnline || readOnly || autopilotActive;

  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      setPayload(await api.personas(signal));
      setError("");
    } catch (loadError) {
      if (signal?.aborted) return;
      setError(loadError instanceof Error ? loadError.message : String(loadError));
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const reportError = useCallback((message: string) => {
    setError(message);
    show(message, "error");
  }, [show]);

  const applyPayload = useCallback((next: PersonasPayload) => {
    setPayload(next);
    setError("");
  }, []);

  const applyPersona = useCallback((changed: Persona) => {
    setPayload((current) => current
      ? {
          ...current,
          personas: current.personas.map((item) => item.id === changed.id ? changed : item),
          persona: current.persona?.id === changed.id ? changed : current.persona,
        }
      : current);
    setError("");
  }, []);

  const closeEditor = useCallback(() => setEditingID(""), []);

  const create = async () => {
    setAdding(true);
    try {
      const created = await api.createPersona({ name: t("New persona") });
      setPayload(created);
      // Open the new row straight into the editor: a tile called "New persona"
      // with no picture is not a finished thing, and making the user find and
      // click it again is a step with no purpose.
      if (created.persona) setEditingID(created.persona.id);
    } catch (createError) {
      reportError(createError instanceof Error ? createError.message : String(createError));
    } finally {
      setAdding(false);
    }
  };

  const importPersona = async (file: File) => {
    setAdding(true);
    try {
      const imported = await api.importPersona(file);
      setPayload(imported);
      setError("");
      if (imported.persona) {
        setEditingID(imported.persona.id);
        show(t("{name} imported.", { name: imported.persona.name }), "success");
      }
    } catch (importError) {
      reportError(importError instanceof Error ? importError.message : String(importError));
    } finally {
      setAdding(false);
    }
  };

  const editing: Persona | undefined = payload?.personas.find((item) => item.id === editingID);

  return (
    <>
      <WorkspaceHead
        title={t("Personas")}
        lede={t("Portrait, name, register, and behavior — saved together. Choosing one changes this chat, not your settings.")}
        wide
      />
      <div className="persona-page" data-requires-backend>
        {error && <p className="hint" role="alert">{error}</p>}
        {!payload ? (
          <p className="hint" aria-live="polite">{t("Loading personas…")}</p>
        ) : (
          <PersonaGrid
            personas={payload.personas}
            defaultPersona={payload.default_persona}
            activeID={payload.active_persona_id}
            locked={locked || adding}
            onOpen={(item) => setEditingID(item.id)}
            onCreate={() => void create()}
            onImport={(file) => void importPersona(file)}
          />
        )}
        {payload && editing && (
          <PersonaEditor
            item={editing}
            options={payload.options}
            promptSets={payload.prompt_sets ?? []}
            locked={locked}
            exportAvailable={backendOnline}
            onApplied={applyPayload}
            onPersonaChanged={applyPersona}
            onClose={closeEditor}
            onError={reportError}
            onExported={(name) => show(t("{name} exported.", { name }), "success")}
          />
        )}
      </div>
    </>
  );
}
