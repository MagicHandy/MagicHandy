import type { ReactNode } from "react";
import type { PublicSettings } from "../api/types";
import { t, translateKnown, type MessageKey } from "../i18n";
import { ModelSettingsPanel } from "./ModelSettingsPanel";
import { PromptSetEditor } from "./PromptSetEditor";
import { MemoryManager } from "./MemoryManager";
import { SettingsNavigation, SettingsNavigationLabel } from "./SettingsNavigation";

const sections = [
  { id: "conversation", label: "Conversation" },
  { id: "model", label: "Model" },
  { id: "prompts", label: "Prompts & memory", compact: "Prompts" },
] as const;
export type ChatSettingsSection = (typeof sections)[number]["id"];
export const resolveChatSettingsSection = (requested: string): ChatSettingsSection => sections.find(item => item.id === requested)?.id || "conversation";

const CHAT_VOICE_LABELS: Partial<Record<string, MessageKey>> = {
  utility: "Utility (neutral assistant)",
  warm: "Warm (flirtatious, never explicit)",
  intimate: "Intimate (sensual partner)",
  explicit: "Explicit (direct sexual language)",
};
const USER_ANATOMY_LABELS: Partial<Record<string, MessageKey>> = {
  penis: "Penis",
  vagina: "Vagina / vulva",
  custom: "Custom wording",
};
const optionLabel = (labels: Partial<Record<string, MessageKey>>, value: string) => {
  const label = labels[value];
  return label ? translateKnown(label) : value;
};
const MAX_CUSTOM_ANATOMY_CHARS = 120;
const MAX_PERSONA_DESCRIPTION_CHARS = 500;
const clampCharacters = (value: string, limit: number) => Array.from(value).slice(0, limit).join("");
const PROMPT_SET_LABELS: Record<string, string> = {
  magichandy_motion_v1: "English",
  magichandy_motion_v1_es: "Español",
  magichandy_motion_v1_pt_br: "Português (Brasil)",
  magichandy_motion_v1_zh_hans: "简体中文",
  magichandy_motion_v1_ja: "日本語",
};


export function ChatSettingsPanel({ section, settings: s, saved, options: opt, locked, patchLLM, patchChat, actions }: {
  section: ChatSettingsSection;
  settings: PublicSettings;
  saved: PublicSettings | null;
  options: PublicSettings["options"];
  locked: boolean;
  patchLLM: (next: Partial<PublicSettings["llm"]>) => void;
  patchChat: (next: Partial<NonNullable<PublicSettings["chat"]>>) => void;
  actions: ReactNode;
}) {
  return <>
    <h2 className="section-title">{t("Chat")}</h2>
    <div className="settings-split">
      <SettingsNavigation className="settings-sidebar" label={t("Chat sections")} current={section}>
        <ul>{sections.map(item => <li key={item.id}><a href={`#/settings/chat/${item.id}`} aria-label={translateKnown(item.label)} title={translateKnown(item.label)} aria-current={section === item.id ? "page" : undefined}><SettingsNavigationLabel label={item.label} compact={"compact" in item ? item.compact : undefined} /></a></li>)}</ul>
      </SettingsNavigation>
      <div className="settings-content">
        {section === "model" && (
          <ModelSettingsPanel
            settings={s.llm}
            saved={saved?.llm}
            providers={opt.llm_providers ?? []}
            llamaModes={opt.llama_cpp_modes ?? []}
            managedLoadPolicies={opt.llm_managed_load_policies ?? []}
            llamaContextSizes={opt.llama_cpp_context_sizes ?? []}
            reasoningModes={opt.llm_reasoning_modes ?? []}
            maxOutputOptions={opt.llm_max_output_tokens ?? []}
            locked={locked}
            patch={patchLLM}
          />
        )}

        {section === "conversation" && (
          <>
            <h3 className="section-title">{t("Conversation")}</h3>
            <div className="group">
              <h4 className="group-title">{t("Sessions")}</h4>
              <label className="field">
                <span className="label">{t("When MagicHandy starts")}</span>
                <select
                  value={s.chat?.startup_behavior ?? "previous"}
                  disabled={locked}
                  onChange={(event) => patchChat({
                    startup_behavior: event.target.value,
                    ...(event.target.value === "new" ? { keep_unsaved_on_exit: false } : {}),
                  })}
                >
                  {(opt.chat_startup_behaviors ?? ["previous", "new"]).map((behavior) => (
                    <option key={behavior} value={behavior}>{behavior === "new" ? t("Start a new chat") : t("Open the previous chat")}</option>
                  ))}
                </select>
                <span className="hint-block">{t("Previous restores the last retained chat. New creates a blank, unsaved tab on every launch.")}</span>
              </label>
              <label className="toggle-line">
                <span className="toggle">
                  <input
                    type="checkbox"
                    checked={s.chat?.keep_unsaved_on_exit ?? false}
                    disabled={locked || s.chat?.startup_behavior === "new"}
                    onChange={(event) => patchChat({ keep_unsaved_on_exit: event.target.checked })}
                  />
                  <span className="track" aria-hidden="true" />
                </span>
                <span>{t("Keep an unsaved current chat after closing MagicHandy")}<small>{s.chat?.startup_behavior === "new"
                    ? t("Starting with a new chat always removes the prior unsaved draft.")
                    : t("Off by default. Saved tabs are always kept; use Save chat from the tab menu or its right-click menu.")}</small>
                </span>
              </label>
            </div>
          </>
        )}

        {section === "prompts" && (
          <>
            <h3 className="section-title">{t("Prompts & memory")}</h3>
            <div className="group">
            <h4 className="group-title">{t("Reply style")}</h4>
            <label className="field">
              <span className="label">{t("Active prompt set")}<span className="hint-inline">{t("saved with Save settings")}</span></span>
              <select value={s.llm.prompt_set} disabled={locked} onChange={(event) => patchLLM({ prompt_set: event.target.value })}>
                {(opt.prompt_sets?.length ? opt.prompt_sets : [s.llm.prompt_set]).map((promptSet) => (
                  <option key={promptSet} value={promptSet}>{PROMPT_SET_LABELS[promptSet] ?? promptSet}</option>
                ))}
              </select>
              <span className="hint-block">{t("Built-in prompt sets choose the default language for model replies. Custom prompt sets keep their own instructions.")}</span>
            </label>
            <label className="field">
              <span className="label">{t("Chat voice")}<span className="hint-inline">{t("how sexual the model's replies may be")}</span></span>
              <select value={s.llm.chat_voice ?? "utility"} disabled={locked} onChange={(e) => patchLLM({ chat_voice: e.target.value })}>
                {(opt.llm_chat_voices?.length ? opt.llm_chat_voices : ["utility"]).map((voice) => (
                  <option key={voice} value={voice}>{optionLabel(CHAT_VOICE_LABELS, voice)}</option>
                ))}
              </select>
            </label>
            <p className="hint">{t("Utility keeps the neutral assistant register. Warm is flirtatious but never explicit. Intimate speaks as a partner with sensual language. Explicit permits direct sexual language like the legacy app. Voice changes wording only; motion limits, capability gates, and Stop are identical at every level.")}</p>
            </div>
            <div className="group">
            <h4 className="group-title">{t("Persona and anatomy")}</h4>
            <label className="field">
              <span className="label">{t("User anatomy")}<span className="hint-inline">{t("separate from partner persona")}</span></span>
              <select
                value={s.llm.user_anatomy ?? "penis"}
                disabled={locked}
                onChange={(event) => patchLLM({ user_anatomy: event.target.value as PublicSettings["llm"]["user_anatomy"] })}
              >
                {(opt.llm_user_anatomies?.length ? opt.llm_user_anatomies : ["penis", "vagina", "custom"]).map((anatomy) => (
                  <option key={anatomy} value={anatomy}>{optionLabel(USER_ANATOMY_LABELS, anatomy)}</option>
                ))}
              </select>
            </label>
            {(s.llm.user_anatomy ?? "penis") === "custom" && (
              <label className="field">
                <span className="label">{t("Custom anatomy wording")}<span className="hint-inline">{Array.from(s.llm.custom_anatomy ?? "").length} / {MAX_CUSTOM_ANATOMY_CHARS}</span>
                </span>
                <input
                  type="text"
                  value={s.llm.custom_anatomy ?? ""}
                  disabled={locked}
                  autoComplete="off"
                  onChange={(event) => patchLLM({ custom_anatomy: clampCharacters(event.target.value, MAX_CUSTOM_ANATOMY_CHARS) })}
                />
              </label>
            )}
            <label className="field">
              <span className="label">{t("Persona description")}<span className="hint-inline">{t("optional · {current} / {max}", { current: Array.from(s.llm.persona_description ?? "").length, max: MAX_PERSONA_DESCRIPTION_CHARS })}</span>
              </span>
              <textarea
                rows={3}
                value={s.llm.persona_description ?? ""}
                disabled={locked}
                onChange={(event) => patchLLM({ persona_description: clampCharacters(event.target.value, MAX_PERSONA_DESCRIPTION_CHARS) })}
              />
            </label>
            <p className="hint">{t("Anatomy context and persona apply to interactive Warm, Intimate, and Explicit replies. Direct anatomy wording is reserved for Explicit; the other levels keep references indirect. This bounded context cannot change motion permissions or limits.")}</p>
            </div>
            <PromptSetEditor locked={locked} />
            <MemoryManager locked={locked} />
          </>
        )}


        {actions}
      </div>
    </div>
  </>;
}
