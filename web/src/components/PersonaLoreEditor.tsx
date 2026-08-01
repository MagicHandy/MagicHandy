import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api } from "../api/client";
import type { Persona, PersonaLoreDraft, PersonaLoreEntry, PersonaLorePayload } from "../api/types";
import { t } from "../i18n";
import { PlusIcon, SaveIcon, TrashIcon } from "../shell/icons";
import { codePointLength, limitCodePoints } from "../util/text";
import { LORE_MODE_LABELS, personaOptionLabel } from "./persona-labels";

interface Props {
  persona: Persona;
  loreModes: string[];
  locked: boolean;
  onPersonaChanged: (persona: Persona) => void;
  onError: (message: string) => void;
  onExportBlockedChange: (blocked: boolean) => void;
}

const errorMessage = (error: unknown) => error instanceof Error ? error.message : String(error);

function parseKeywords(value: string): string[] {
  return [...new Set(value.split(",").map((item) => item.trim().toLowerCase()).filter(Boolean))];
}

export function PersonaLoreEditor({
  persona,
  loreModes,
  locked,
  onPersonaChanged,
  onError,
  onExportBlockedChange,
}: Props) {
  const [payload, setPayload] = useState<PersonaLorePayload | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [adding, setAdding] = useState(false);
  const [dirtyCount, setDirtyCount] = useState(0);
  const dirtyRows = useRef(new Set<string>());
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

  useEffect(() => {
    dirtyRows.current.clear();
    setDirtyCount(0);
    onExportBlockedChange(false);
  }, [onExportBlockedChange, persona.id]);

  useEffect(() => {
    onExportBlockedChange(busy || dirtyCount > 0);
    return () => onExportBlockedChange(false);
  }, [busy, dirtyCount, onExportBlockedChange]);

  const reportRowDirty = useCallback((key: string, dirty: boolean) => {
    if (dirty) {
      dirtyRows.current.add(key);
    } else {
      dirtyRows.current.delete(key);
    }
    setDirtyCount(dirtyRows.current.size);
  }, []);

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
      {/* In the same two-column grid the other axis selects use, so a three-word
          dropdown does not stretch across the whole dialog. */}
      <div className="persona-editor-fields">
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
          {loreModes.map((mode) => (
            <option key={mode} value={mode}>{personaOptionLabel(LORE_MODE_LABELS, mode)}</option>
          ))}
        </select>
      </label>
      </div>
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
                maxText={Math.min(
                  payload.options.max_text,
                  payload.options.max_total - total + codePointLength(entry.text),
                )}
                maxKeywords={payload.options.max_keywords}
                maxKeyword={payload.options.max_keyword}
                disabled={disabled}
                dirtyKey={entry.id}
                onDirtyChange={reportRowDirty}
                onSave={(draft) => apply(() => api.updatePersonaLore(persona.id, entry.id, draft))}
                onDelete={() => apply(() => api.deletePersonaLore(persona.id, entry.id))}
              />
            ))}
            {adding && (
              <LoreRow
                mode={persona.lore_mode}
                maxText={Math.min(payload.options.max_text, payload.options.max_total - total)}
                maxKeywords={payload.options.max_keywords}
                maxKeyword={payload.options.max_keyword}
                disabled={disabled}
                dirtyKey="new"
                onDirtyChange={reportRowDirty}
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
              disabled={disabled
                || payload.entries.length >= payload.options.max_entries
                || total >= payload.options.max_total}
              onClick={() => setAdding(true)}
            >
              <PlusIcon size={16} />
              {t("Add lore entry")}
            </button>
          )}
          <p className="hint">
            {t("Background facts the model stays consistent with, not a manner to imitate — the description sets the manner. Lore is quoted as data before the response contract. It cannot grant motion access or change device limits.")}
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
  maxKeyword,
  disabled,
  dirtyKey,
  onDirtyChange,
  onSave,
  onDelete,
  onCancel,
}: {
  entry?: PersonaLoreEntry;
  mode: string;
  maxText: number;
  maxKeywords: number;
  maxKeyword: number;
  disabled: boolean;
  dirtyKey: string;
  onDirtyChange: (key: string, dirty: boolean) => void;
  onSave: (draft: PersonaLoreDraft) => Promise<boolean>;
  onDelete?: () => Promise<boolean>;
  onCancel?: () => void;
}) {
  const [text, setText] = useState(entry?.text ?? "");
  const [keywords, setKeywords] = useState(entry?.keywords.join(", ") ?? "");
  const parsedKeywords = parseKeywords(keywords);
  const longestKeyword = parsedKeywords.reduce(
    (longest, keyword) => Math.max(longest, codePointLength(keyword)),
    0,
  );
  const dirty = !entry || text.trim() !== entry.text || keywords.trim() !== entry.keywords.join(", ");
  const valid = text.trim() !== "" &&
    codePointLength(text) <= maxText &&
    parsedKeywords.length <= maxKeywords &&
    longestKeyword <= maxKeyword;

  useEffect(() => {
    setText(entry?.text ?? "");
    setKeywords(entry?.keywords.join(", ") ?? "");
  }, [entry?.id, entry?.keywords, entry?.text]);

  useEffect(() => {
    onDirtyChange(dirtyKey, dirty);
    return () => onDirtyChange(dirtyKey, false);
  }, [dirty, dirtyKey, onDirtyChange]);

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
          <span className="hint-inline">{codePointLength(text)} / {maxText}</span>
        </span>
        <textarea
          rows={3}
          value={text}
          disabled={disabled}
          onChange={(event) => setText(limitCodePoints(event.target.value, maxText))}
        />
      </label>
      <label className="field">
        <span className="label">
          {t("Keywords")}
          <span className="hint-inline">
            {parsedKeywords.length} / {maxKeywords} · {longestKeyword} / {maxKeyword}
          </span>
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
