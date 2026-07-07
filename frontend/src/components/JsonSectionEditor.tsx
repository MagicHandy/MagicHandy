import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { UiCheckbox } from "./UiCheckbox";

export type JsonFieldDef = {
  key: string;
  label: string;
  kind: "string" | "number" | "boolean" | "select";
  options?: { value: string; label: string }[];
  min?: number;
  max?: number;
  step?: number;
  placeholder?: string;
};

function parseObject(raw: string): Record<string, unknown> | null {
  if (!raw.trim()) return {};
  try {
    const value = JSON.parse(raw);
    if (value && typeof value === "object" && !Array.isArray(value)) {
      return value as Record<string, unknown>;
    }
    return null;
  } catch {
    return null;
  }
}

export function JsonSectionEditor({
  label,
  description,
  valueJson,
  onChangeJson,
  fields,
}: {
  label: string;
  description?: string;
  valueJson: string;
  onChangeJson: (json: string) => void;
  fields: JsonFieldDef[];
}) {
  const { t } = useTranslation();
  const [mode, setMode] = useState<"ui" | "raw">("ui");
  const [rawDraft, setRawDraft] = useState(valueJson);
  const [rawError, setRawError] = useState<string | null>(null);

  const obj = useMemo(() => parseObject(valueJson) ?? {}, [valueJson]);

  useEffect(() => {
    if (mode === "raw") setRawDraft(valueJson);
  }, [valueJson, mode]);

  const patch = (key: string, val: unknown) => {
    const next = { ...obj, [key]: val };
    onChangeJson(JSON.stringify(next, null, 2));
  };

  const applyRaw = () => {
    const parsed = parseObject(rawDraft);
    if (parsed === null) {
      setRawError(t("config.raw.invalidDetail"));
      return;
    }
    setRawError(null);
    onChangeJson(JSON.stringify(parsed, null, 2));
  };

  return (
    <section className="json-section-editor">
      <header className="json-section-head">
        <div>
          <h4 className="json-section-title">{label}</h4>
          {description && <p className="hint json-section-desc">{description}</p>}
        </div>
        <div className="view-mode-toggle" role="tablist">
          <button
            type="button"
            role="tab"
            aria-selected={mode === "ui"}
            className={`view-mode-btn${mode === "ui" ? " active" : ""}`}
            onClick={() => setMode("ui")}
          >
            {t("persona.formMode")}
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={mode === "raw"}
            className={`view-mode-btn${mode === "raw" ? " active" : ""}`}
            onClick={() => setMode("raw")}
          >
            {t("persona.jsonMode")}
          </button>
        </div>
      </header>

      {mode === "ui" ? (
        <div className="json-section-fields form-grid two">
          {fields.map((field) => {
            const rawVal = obj[field.key];
            if (field.kind === "boolean") {
              return (
                <label key={field.key} className="field field-check">
                  <UiCheckbox
                    label={field.label}
                    checked={Boolean(rawVal)}
                    onChange={(e) => patch(field.key, e.target.checked)}
                  />
                </label>
              );
            }
            if (field.kind === "select") {
              return (
                <label key={field.key} className="field">
                  <span>{field.label}</span>
                  <select
                    value={typeof rawVal === "string" ? rawVal : ""}
                    onChange={(e) => patch(field.key, e.target.value)}
                  >
                    <option value="">—</option>
                    {(field.options ?? []).map((opt) => (
                      <option key={opt.value} value={opt.value}>
                        {opt.label}
                      </option>
                    ))}
                  </select>
                </label>
              );
            }
            if (field.kind === "number") {
              return (
                <label key={field.key} className="field">
                  <span>{field.label}</span>
                  <input
                    type="number"
                    min={field.min}
                    max={field.max}
                    step={field.step ?? 1}
                    placeholder={field.placeholder}
                    value={typeof rawVal === "number" ? rawVal : ""}
                    onChange={(e) => {
                      const n = Number(e.target.value);
                      patch(field.key, Number.isFinite(n) ? n : 0);
                    }}
                  />
                </label>
              );
            }
            return (
              <label key={field.key} className="field">
                <span>{field.label}</span>
                <input
                  type="text"
                  placeholder={field.placeholder}
                  value={typeof rawVal === "string" ? rawVal : rawVal != null ? String(rawVal) : ""}
                  onChange={(e) => patch(field.key, e.target.value)}
                />
              </label>
            );
          })}
        </div>
      ) : (
        <div className="json-raw-block">
          <textarea
            className="mono json-raw-area"
            rows={10}
            value={rawDraft}
            onChange={(e) => {
              setRawDraft(e.target.value);
              setRawError(null);
            }}
            spellCheck={false}
          />
          {rawError && <p className="hint json-raw-error">{rawError}</p>}
          <button type="button" className="btn btn-sm btn-ghost" onClick={applyRaw}>
            {t("config.raw.apply")}
          </button>
        </div>
      )}
    </section>
  );
}
