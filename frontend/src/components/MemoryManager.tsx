import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api } from "../api/client";
import type { MemoryState } from "../api/types";
import { useStatus } from "../contexts/StatusContext";
import { useToast } from "../contexts/ToastContext";
import { UiCheckbox } from "./UiCheckbox";

const msg = (e: unknown) => (e instanceof Error ? e.message : "Request failed");

export function MemoryManager({ locked = false }: { locked?: boolean }) {
  const { t } = useTranslation();
  const { error } = useStatus();
  const { notify } = useToast();
  const [mem, setMem] = useState<MemoryState | null>(null);
  const [draft, setDraft] = useState("");
  const [confirmClear, setConfirmClear] = useState(false);

  async function reload() {
    try {
      setMem(await api.getMemory());
    } catch (e) {
      notify(msg(e), "error");
    }
  }

  useEffect(() => {
    void reload();
  }, []);

  async function run(fn: () => Promise<unknown>) {
    try {
      await fn();
      await reload();
    } catch (e) {
      notify(msg(e), "error");
    }
  }

  if (!mem) return <p className="hint">{t("common.loading")}</p>;
  const memories = Array.isArray(mem.memories) ? mem.memories : [];

  return (
    <section className="glass settings-card">
      <h3 className="section-title">{t("memory.title")}</h3>
      <UiCheckbox
        label={t("memory.enabled")}
        checked={mem.enabled}
        disabled={locked}
        onChange={(e) => void run(() => api.setMemoryEnabled(e.target.checked))}
      />

      <ul className="memory-list">
        {memories.length === 0 && <li className="hint">{t("memory.empty")}</li>}
        {memories.map((m) => (
          <li key={m.id} className="memory-item row-actions">
            <UiCheckbox
              label={m.text}
              checked={m.enabled}
              disabled={locked}
              onChange={(e) => void run(() => api.setMemoryItemEnabled(m.id, e.target.checked))}
            />
            <button
              type="button"
              className="btn btn-ghost btn-sm"
              disabled={locked}
              onClick={() => void run(() => api.removeMemory(m.id))}
            >
              {t("common.delete")}
            </button>
          </li>
        ))}
      </ul>

      <label className="field">
        <span className="label">{t("memory.newLabel")}</span>
        <textarea
          rows={2}
          maxLength={2000}
          value={draft}
          disabled={locked}
          placeholder={t("memory.newPlaceholder")}
          onChange={(e) => setDraft(e.target.value)}
        />
      </label>
      <div className="row-actions">
        <button
          type="button"
          className="btn btn-primary"
          disabled={locked || !draft.trim()}
          onClick={() =>
            void run(async () => {
              await api.addMemory(draft.trim());
              setDraft("");
            })
          }
        >
          {t("memory.add")}
        </button>
        <button
          type="button"
          className="btn btn-danger"
          disabled={locked || memories.length === 0}
          onClick={() => {
            if (!confirmClear) {
              setConfirmClear(true);
              return;
            }
            setConfirmClear(false);
            void run(() => api.clearMemory());
          }}
        >
          {confirmClear ? t("memory.confirmClear") : t("memory.clearAll")}
        </button>
      </div>
      {locked && (
        <p className="hint">{error ? t("layout.apiUnavailable") : t("memory.readOnly")}</p>
      )}
    </section>
  );
}
