import { useEffect, useMemo, useRef, useState } from "react";
import { api } from "../api/client";
import type { Persona, PersonaLoreDraft, PersonaLoreEntry, PersonaLorePayload } from "../api/types";
import { t } from "../i18n";
import { PlusIcon, SaveIcon, TrashIcon } from "../shell/icons";

interface Props {
  persona: Persona;
  locked: boolean;
  onPersonaChanged: (persona: Persona) => void;
  onError: (message: string) => void;
}

const errorMessage = (error: unknown) => error instanceof Error ? error.message : String(error);

function parseKeywords(value: string): string[] {
  return [...new Set(value.split(",").map((item) => item.trim().toLowerCase()).filter(Boolean))];
}

export function PersonaLoreEditor({ persona, locked, onPersonaChanged, onError }: Props) {
  const [payload, setPayload] = useState<PersonaLorePayload | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [adding, setAdding] = useState(false);
  const onErrorRef = useRef(onError);

  useEffect(() => {
    onErrorRef.current = onError;
  }, [onError]);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setPayload(null);
    void api.personaLore(persona.id, controller.signal)
      .then((next) => setPayload(next))
      .catch((error) => {
        if (!controller.signal.aborted) onErrorRef.current(errorMessage(error));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [persona.id]);

  const apply = async (action: () => Promise<PersonaLorePayload>) => {
    setBusy(true);
    try {
      const next = await action();
      setPayload(next);
      onPersonaChanged(next.persona);
      return true;
    } catch (error) {
      onError(errorMessage(error));
      return false;
    } finally {
      setBusy(false);
    }
  };

  const total = useMemo(
    () => payload?.entries.reduce((sum, entry) => sum + Array.from(entry.text).length, 0) ?? 0,
    [payload],
  );
  const disabled = locked || busy;

  return (
    <section className="group persona-lore-group">
      <div className="persona-lore-heading">
        <h3 className="group-title">{t("Lore")}</h3>
        {payload && (
          <span className="hint-inline">
            {t("{used} / {total} characters", { used: total, total: payload.options.max_total })}
          </span>
        )}
      </div>
      <label className="field">
        <span className="label">{t("Prompt use")}</span>
        <select
          value={persona.lore_mode}
          disabled={disabled}
          onChange={(event) => {
            void apply(async () => {
              const personas = await api.updatePersona(persona.id, { lore_mode: event.target.value });
              const nextPersona = personas.persona ??
                personas.personas.find((item) => item.id === persona.id) ??
                persona;
              const current = payload ?? await api.personaLore(persona.id);
              return { ...current, persona: nextPersona };
            });
          }}
        >
          <option value="off">{t("Off")}</option>
          <option value="relevant">{t("Relevant only")}</option>
          <option value="full">{t("Full")}</option>
        </select>
      </label>
      <p className="hint">
        {persona.lore_mode === "off"
          ? t("Lore stays saved but does not enter the model prompt.")
          : persona.lore_mode === "relevant"
            ? t("Only enabled entries whose keywords appear in recent chat are included.")
            : t("All enabled entries enter every turn. Model-specific motion adherence is not benchmarked yet; review the exact prompt in Diagnostics.")}
      </p>

      {loading && <p className="form-status" role="status">{t("Loading lore")}</p>}
      {!loading && payload && (
        <>
          <div className="persona-lore-list">
            {payload.entries.map((entry) => (
              <LoreRow
                key={entry.id}
                entry={entry}
                mode={persona.lore_mode}
                maxText={payload.options.max_text}
                maxKeywords={payload.options.max_keywords}
                disabled={disabled}
                onSave={(draft) => apply(() => api.updatePersonaLore(persona.id, entry.id, draft))}
                onDelete={() => apply(() => api.deletePersonaLore(persona.id, entry.id))}
              />
            ))}
            {adding && (
              <LoreRow
                mode={persona.lore_mode}
                maxText={payload.options.max_text}
                maxKeywords={payload.options.max_keywords}
                disabled={disabled}
                onSave={async (draft) => {
                  const saved = await apply(() => api.createPersonaLore(persona.id, draft));
                  if (saved) setAdding(false);
                  return saved;
                }}
                onCancel={() => setAdding(false)}
              />
            )}
          </div>
          {!adding && (
            <button
              type="button"
              className="btn btn-secondary"
              disabled={disabled || payload.entries.length >= payload.options.max_entries}
              onClick={() => setAdding(true)}
            >
              <PlusIcon size={16} />
              {t("Add lore entry")}
            </button>
          )}
          <p className="hint">
            {t("Lore is quoted as data before the response contract. It cannot grant motion access or change device limits.")}
          </p>
        </>
      )}
    </section>
  );
}

function LoreRow({
  entry,
  mode,
  maxText,
  maxKeywords,
  disabled,
  onSave,
  onDelete,
  onCancel,
}: {
  entry?: PersonaLoreEntry;
  mode: string;
  maxText: number;
  maxKeywords: number;
  disabled: boolean;
  onSave: (draft: PersonaLoreDraft) => Promise<boolean>;
  onDelete?: () => Promise<boolean>;
  onCancel?: () => void;
}) {
  const [text, setText] = useState(entry?.text ?? "");
  const [keywords, setKeywords] = useState(entry?.keywords.join(", ") ?? "");
  const parsedKeywords = parseKeywords(keywords);
  const dirty = !entry || text.trim() !== entry.text || keywords.trim() !== entry.keywords.join(", ");
  const valid = text.trim() !== "" &&
    Array.from(text).length <= maxText &&
    parsedKeywords.length <= maxKeywords;

  useEffect(() => {
    setText(entry?.text ?? "");
    setKeywords(entry?.keywords.join(", ") ?? "");
  }, [entry?.id, entry?.keywords, entry?.text]);

  return (
    <div className="persona-lore-row">
      {entry && (
        <label className="switch-row persona-lore-enabled">
          <span className="switch">
            <input
              type="checkbox"
              checked={entry.enabled}
              disabled={disabled}
              onChange={(event) => void onSave({ enabled: event.target.checked })}
            />
            <span className="track" aria-hidden="true" />
          </span>
          <span>{t("Enabled")}</span>
        </label>
      )}
      <label className="field">
        <span className="label">
          {t("Lore text")}
          <span className="hint-inline">{Array.from(text).length} / {maxText}</span>
        </span>
        <textarea
          rows={3}
          value={text}
          maxLength={maxText}
          disabled={disabled}
          onChange={(event) => setText(event.target.value)}
        />
      </label>
      <label className="field">
        <span className="label">
          {t("Keywords")}
          <span className="hint-inline">{parsedKeywords.length} / {maxKeywords}</span>
        </span>
        <input
          type="text"
          value={keywords}
          disabled={disabled}
          placeholder={t("Comma-separated words or phrases")}
          onChange={(event) => setKeywords(event.target.value)}
        />
      </label>
      {mode === "relevant" && parsedKeywords.length === 0 && (
        <p className="hint">{t("Add at least one keyword for this entry to match in Relevant only mode.")}</p>
      )}
      <div className="row-actions persona-lore-actions">
        <button
          type="button"
          className="btn btn-secondary"
          disabled={disabled || !dirty || !valid}
          onClick={() => void onSave({ text: text.trim(), keywords: parsedKeywords })}
        >
          <SaveIcon size={15} />
          {t("Save")}
        </button>
        {entry && onDelete && (
          <button
            type="button"
            className="btn btn-secondary icon-btn"
            disabled={disabled}
            aria-label={t("Delete lore entry")}
            title={t("Delete lore entry")}
            onClick={() => void onDelete()}
          >
            <TrashIcon size={16} />
          </button>
        )}
        {!entry && onCancel && (
          <button type="button" className="btn btn-secondary" disabled={disabled} onClick={onCancel}>
            {t("Cancel")}
          </button>
        )}
      </div>
    </div>
  );
}
