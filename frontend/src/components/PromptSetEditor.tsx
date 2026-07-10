import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api } from "../api/client";
import type { PromptSet } from "../api/types";
import { useStatus } from "../contexts/StatusContext";
import { useToast } from "../contexts/ToastContext";

const msg = (e: unknown) => (e instanceof Error ? e.message : "Request failed");

export function PromptSetEditor({ locked = false }: { locked?: boolean }) {
  const { t } = useTranslation();
  const { error } = useStatus();
  const { notify } = useToast();
  const [sets, setSets] = useState<PromptSet[]>([]);
  const [selId, setSelId] = useState("");
  const [name, setName] = useState("");
  const [system, setSystem] = useState("");

  async function reload(keep?: string) {
    try {
      const res = await api.getPromptSets();
      const list = Array.isArray(res.sets) ? res.sets : [];
      setSets(list);
      const pick = list.find((s) => s.id === (keep ?? selId)) ?? list[0];
      if (pick) {
        setSelId(pick.id);
        setName(pick.name);
        setSystem(pick.system);
      }
    } catch (e) {
      notify(msg(e), "error");
    }
  }

  useEffect(() => {
    void reload();
  }, []);

  const selected = sets.find((s) => s.id === selId);
  const builtin = selected?.builtin === true;
  const backendOnline = !error;

  function select(id: string) {
    const s = sets.find((x) => x.id === id);
    setSelId(id);
    if (s) {
      setName(s.name);
      setSystem(s.system);
    }
  }

  async function save() {
    if (builtin || !selected) return;
    try {
      await api.updatePromptSet(selected.id, name.trim(), system.trim());
      notify(t("promptSets.saved"), "ok");
      await reload(selected.id);
    } catch (e) {
      notify(msg(e), "error");
    }
  }

  async function duplicate() {
    try {
      const created = await api.createPromptSet(
        `${name || t("promptSets.defaultName")} copy`,
        system || t("promptSets.defaultSystem"),
      );
      notify(t("promptSets.duplicated"), "ok");
      await reload(created.set?.id);
    } catch (e) {
      notify(msg(e), "error");
    }
  }

  async function remove() {
    if (builtin || !selected) return;
    try {
      await api.deletePromptSet(selected.id);
      notify(t("promptSets.deleted"), "ok");
      await reload("");
    } catch (e) {
      notify(msg(e), "error");
    }
  }

  return (
    <section className="glass settings-card">
      <h3 className="section-title">{t("promptSets.title")}</h3>
      <label className="field">
        <span className="label">{t("promptSets.select")}</span>
        <select value={selId} disabled={!backendOnline} onChange={(e) => select(e.target.value)}>
          {sets.map((s) => (
            <option key={s.id} value={s.id}>
              {s.name}
              {s.builtin ? ` (${t("promptSets.builtin")})` : ""}
            </option>
          ))}
        </select>
      </label>
      <label className="field">
        <span className="label">
          {t("promptSets.name")}{" "}
          {builtin && <span className="pill pill-muted">{t("promptSets.builtinReadOnly")}</span>}
        </span>
        <input
          type="text"
          maxLength={80}
          value={name}
          readOnly={builtin || locked}
          onChange={(e) => setName(e.target.value)}
        />
      </label>
      <label className="field">
        <span className="label">{t("promptSets.instructions")}</span>
        <textarea
          rows={6}
          maxLength={16384}
          value={system}
          readOnly={builtin || locked}
          onChange={(e) => setSystem(e.target.value)}
        />
      </label>
      <div className="row-actions">
        <button type="button" className="btn btn-secondary" disabled={locked} onClick={() => void duplicate()}>
          {t("promptSets.duplicate")}
        </button>
        <button type="button" className="btn btn-primary" disabled={locked || builtin} onClick={() => void save()}>
          {t("common.save")}
        </button>
        <button type="button" className="btn btn-danger" disabled={locked || builtin} onClick={() => void remove()}>
          {t("common.delete")}
        </button>
      </div>
      {locked && <p className="hint">{t("memory.readOnly")}</p>}
    </section>
  );
}
