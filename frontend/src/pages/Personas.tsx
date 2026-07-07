import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api } from "../api/client";
import type { Persona } from "../api/types";
import { JsonSectionEditor } from "../components/JsonSectionEditor";
import { PersonaAvatar } from "../components/PersonaAvatar";
import { PersonaMotionBiasEditor } from "../components/PersonaMotionBiasEditor";
import {
  getPersonaBehaviorFields,
  getPersonaRulesFields,
  getPersonaToneFields,
} from "../config/personaFieldSchemas";
import { useToast } from "../contexts/ToastContext";

const emptyForm = (): Partial<Persona> => ({
  name: "",
  description: "",
  system_prompt: "",
  tone_json: "",
  mood_json: "",
  boundaries_json: "",
  motion_bias_json: "",
});

export function PersonasPanel() {
  const { t } = useTranslation();
  const { notify } = useToast();
  const [list, setList] = useState<Persona[]>([]);
  const [activeId, setActiveId] = useState("");
  const [form, setForm] = useState<Partial<Persona>>(emptyForm());
  const [editId, setEditId] = useState<string | null>(null);
  const [avatarUrl, setAvatarUrl] = useState<string | null>(null);
  const [uploadingAvatar, setUploadingAvatar] = useState(false);

  const toneFields = getPersonaToneFields(t);
  const behaviorFields = getPersonaBehaviorFields(t);
  const rulesFields = getPersonaRulesFields(t);

  const load = useCallback(async () => {
    const data = await api.listPersonas();
    setList(data.personas);
    setActiveId(data.active_persona_id);
  }, []);

  useEffect(() => {
    load().catch((e) =>
      notify(e instanceof Error ? e.message : t("common.error"), "error"),
    );
  }, [load, notify, t]);

  const select = (p: Persona) => {
    setEditId(p.id);
    setAvatarUrl(null);
    setForm({
      name: p.name,
      description: p.description ?? "",
      system_prompt: p.system_prompt,
      tone_json: p.tone_json ?? (p.tone ? JSON.stringify(p.tone, null, 2) : ""),
      mood_json: p.mood_json ?? (p.mood ? JSON.stringify(p.mood, null, 2) : ""),
      boundaries_json:
        p.boundaries_json ??
        (p.boundaries ? JSON.stringify(p.boundaries, null, 2) : ""),
      motion_bias_json:
        p.motion_bias_json ??
        (p.motion_bias ? JSON.stringify(p.motion_bias, null, 2) : ""),
    });
    api.getStatus().then((s) => {
      if (s.persona_id === p.id && s.persona_avatar_url) {
        setAvatarUrl(s.persona_avatar_url);
      }
    }).catch(() => {});
  };

  const uploadAvatar = async (file: File) => {
    if (!editId) {
      notify(t("persona.saveBeforeAvatar"), "info");
      return;
    }
    setUploadingAvatar(true);
    try {
      const res = await api.uploadPersonaAvatar(editId, file);
      setAvatarUrl(res.avatar_url ?? null);
      notify(t("persona.avatarUpdated"), "ok");
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    } finally {
      setUploadingAvatar(false);
    }
  };

  const save = async () => {
    if (!form.name?.trim() || !form.system_prompt?.trim()) {
      notify(t("persona.requiredFields"), "error");
      return;
    }
    const body = {
      name: form.name.trim(),
      description: form.description || null,
      system_prompt: form.system_prompt.trim(),
      tone_json: form.tone_json || null,
      mood_json: form.mood_json || null,
      boundaries_json: form.boundaries_json || null,
      motion_bias_json: form.motion_bias_json || null,
    };
    try {
      if (editId) await api.updatePersona(editId, body);
      else await api.createPersona(body);
      notify(t("persona.saved"), "ok");
      setEditId(null);
      setForm(emptyForm());
      await load();
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  const remove = async () => {
    if (!editId) return;
    if (!window.confirm(t("persona.confirmDelete"))) return;
    try {
      await api.deletePersona(editId);
      notify(t("persona.deleted"), "ok");
      setEditId(null);
      setForm(emptyForm());
      await load();
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  const activate = async (id: string) => {
    try {
      await api.activatePersona(id);
      notify(t("persona.activated"), "ok");
      await load();
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  return (
    <div className="split-layout config-split-layout">
      <aside className="glass list-panel">
        <div className="list-panel-head">
          <span className="section-label">{t("persona.list")}</span>
          <button
            type="button"
            className="btn btn-ghost btn-sm"
            onClick={() => {
              setEditId(null);
              setForm(emptyForm());
            }}
          >
            {t("persona.new")}
          </button>
        </div>
        <ul className="entity-list">
          {list.map((p) => (
            <li key={p.id}>
              <button
                type="button"
                className={`entity-item${editId === p.id ? " selected" : ""}`}
                onClick={() => select(p)}
              >
                <strong>{p.name}</strong>
                {p.id === activeId && (
                  <span className="pill pill-ok">{t("persona.active")}</span>
                )}
              </button>
              {p.id !== activeId && (
                <button
                  type="button"
                  className="btn btn-ghost btn-sm link-btn"
                  onClick={() => activate(p.id)}
                >
                  {t("persona.activate")}
                </button>
              )}
            </li>
          ))}
        </ul>
      </aside>

      <div className="glass form-panel persona-form-panel">
        <h3>{editId ? t("persona.edit") : t("persona.create")}</h3>
        <div className="form-grid">
          <label className="field">
            <span>{t("persona.name")}</span>
            <input
              value={form.name ?? ""}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
          </label>
          <label className="field">
            <span>{t("persona.description")}</span>
            <input
              value={form.description ?? ""}
              onChange={(e) => setForm({ ...form, description: e.target.value })}
            />
          </label>
        </div>
        <label className="field">
          <span>{t("persona.systemPrompt")}</span>
          <textarea
            value={form.system_prompt ?? ""}
            onChange={(e) => setForm({ ...form, system_prompt: e.target.value })}
            rows={5}
          />
        </label>
        {editId && (
          <div className="persona-avatar-upload">
            <PersonaAvatar url={avatarUrl} name={form.name ?? t("persona.defaultName")} size={72} />
            <label className="field">
              <span>{t("persona.avatar")}</span>
              <input
                type="file"
                accept="image/png,image/jpeg,image/webp"
                disabled={uploadingAvatar}
                onChange={(e) => {
                  const file = e.target.files?.[0];
                  if (file) void uploadAvatar(file);
                  e.target.value = "";
                }}
              />
            </label>
          </div>
        )}
        {(
          [
            ["tone_json", t("persona.sections.tone"), t("persona.sections.toneHint"), toneFields],
            ["mood_json", t("persona.sections.behavior"), t("persona.sections.behaviorHint"), behaviorFields],
            ["boundaries_json", t("persona.sections.rules"), t("persona.sections.rulesHint"), rulesFields],
          ] as const
        ).map(([key, title, desc, fields]) => (
          <JsonSectionEditor
            key={key}
            label={title}
            description={desc}
            valueJson={(form[key] as string) ?? "{}"}
            onChangeJson={(json) => setForm({ ...form, [key]: json })}
            fields={[...fields]}
          />
        ))}
        <PersonaMotionBiasEditor
          valueJson={form.motion_bias_json ?? "{}"}
          onChangeJson={(json) => setForm({ ...form, motion_bias_json: json })}
        />
        <div className="btn-row">
          <button type="button" className="btn btn-primary" onClick={save}>
            {t("common.save")}
          </button>
          {editId && (
            <button type="button" className="btn btn-danger" onClick={remove}>
              {t("common.delete")}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
