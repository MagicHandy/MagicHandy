import { useEffect, useRef, useState } from "react";
import { t, translateKnown } from "../i18n";
import { api } from "../api/client";
import type { Persona, PersonaDraft, PersonasPayload, PromptSet } from "../api/types";
import { CloseIcon, DownloadIcon, PencilIcon, TrashIcon } from "../shell/icons";
import { trapModalTab } from "../util/modal";
import { codePointLength, limitCodePoints } from "../util/text";
import { PROFILE_IMAGE_MAX_EDGE, resizeImageToJPEG } from "../util/profile-image";
import { monogram } from "./PersonaGrid";
import { PersonaLoreEditor } from "./PersonaLoreEditor";
import { AREA_LABELS, STYLE_LABELS, VOICE_LABELS, personaOptionLabel } from "./persona-labels";

interface EditorProps {
  item: Persona;
  options: PersonasPayload["options"];
  promptSets: PromptSet[];
  locked: boolean;
  exportAvailable: boolean;
  onApplied: (payload: PersonasPayload) => void;
  onPersonaChanged: (persona: Persona) => void;
  onClose: () => void;
  onError: (message: string) => void;
  onExported: (name: string) => void;
}

export function PersonaEditor({
  item,
  options,
  promptSets,
  locked,
  exportAvailable,
  onApplied,
  onPersonaChanged,
  onClose,
  onError,
  onExported,
}: EditorProps) {
  const [name, setName] = useState(item.name);
  const [description, setDescription] = useState(item.description);
  const [busy, setBusy] = useState(false);
  const [loreExportBlocked, setLoreExportBlocked] = useState(false);
  const fileInput = useRef<HTMLInputElement>(null);
  const closeButton = useRef<HTMLButtonElement>(null);
  const dialogRef = useRef<HTMLElement>(null);

  // Reset the text fields when a different persona is opened into the same
  // window, otherwise the previous persona's unsaved edits leak across.
  useEffect(() => {
    setName(item.name);
    setDescription(item.description);
    setLoreExportBlocked(false);
  }, [item.id, item.name, item.description]);

  useEffect(() => {
    const previousOverflow = document.body.style.overflow;
    const returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    document.body.style.overflow = "hidden";
    closeButton.current?.focus();
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onClose();
      } else if (dialogRef.current) {
        trapModalTab(event, dialogRef.current);
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", onKeyDown);
      returnFocus?.focus();
    };
  }, [item.id, onClose]);

  const run = async (action: () => Promise<PersonasPayload>) => {
    setBusy(true);
    try {
      onApplied(await action());
    } catch (error) {
      onError(error instanceof Error ? error.message : String(error));
    } finally {
      setBusy(false);
    }
  };

  // Selects and the starting zone apply immediately; the text fields wait for
  // Save. A description is not something to persist per keystroke, and a toggle
  // that needs a button is the idiom mismatch docs/ui-design.md warns about.
  const patch = (draft: PersonaDraft) => run(() => api.updatePersona(item.id, draft));

  const dirty = name.trim() !== item.name || description.trim() !== item.description;
  const disabled = locked || busy;
  const portrait = api.personaPortraitURL(item);

  const exportPersona = async () => {
    setBusy(true);
    try {
      const { blob, filename } = await api.exportPersona(item.id);
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = filename;
      link.hidden = true;
      document.body.appendChild(link);
      link.click();
      link.remove();
      window.setTimeout(() => URL.revokeObjectURL(url), 0);
      onExported(item.name);
    } catch (error) {
      onError(error instanceof Error ? error.message : String(error));
    } finally {
      setBusy(false);
    }
  };

  const choosePortrait = async (file: File | undefined) => {
    if (!file) return;
    setBusy(true);
    try {
      onApplied(await api.savePersonaPortrait(item.id, await resizeImageToJPEG(file)));
    } catch (error) {
      onError(error instanceof Error ? error.message : String(error));
    } finally {
      setBusy(false);
      if (fileInput.current) fileInput.current.value = "";
    }
  };

  return (
    <div
      className="modal-scrim persona-editor-overlay"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <section
        ref={dialogRef}
        className="persona-editor-window"
        role="dialog"
        aria-modal="true"
        aria-labelledby="persona-editor-title"
        tabIndex={-1}
      >
        <div className="persona-editor-window-head">
          <h2 id="persona-editor-title">{t("Edit {name}", { name: item.name })}</h2>
          <button
            ref={closeButton}
            type="button"
            className="icon-button persona-editor-window-close"
            onClick={onClose}
            aria-label={t("Close editor")}
            title={t("Close editor")}
          >
            <CloseIcon />
          </button>
        </div>

        <div className="persona-editor-window-body">
          <section className="group">
            <h3 className="group-title">{t("Identity")}</h3>
            <div className="persona-portrait-field">
              {/* The portrait carries its own controls rather than a pair of wide
                  text buttons beside it: the picture is the target, so the edit
                  affordance belongs on it. They stay visible instead of appearing
                  on hover, because a hover-only control does not exist on touch
                  and gives a keyboard user nothing to find. */}
              <span className="persona-portrait-preview">
                {portrait ? (
                  <img src={portrait} alt="" />
                ) : (
                  <span className="persona-card-monogram" aria-hidden="true">
                    <span>{monogram(item.name)}</span>
                  </span>
                )}
                <span className="persona-portrait-overlay">
                  <input
                    ref={fileInput}
                    type="file"
                    accept="image/*"
                    className="visually-hidden"
                    onChange={(event) => void choosePortrait(event.target.files?.[0])}
                  />
                  <button
                    type="button"
                    className="icon-button persona-portrait-action"
                    disabled={disabled}
                    onClick={() => fileInput.current?.click()}
                    aria-label={portrait ? t("Replace portrait") : t("Choose portrait")}
                    title={portrait ? t("Replace portrait") : t("Choose portrait")}
                  >
                    <PencilIcon size={15} />
                  </button>
                  {portrait && (
                    <button
                      type="button"
                      className="icon-button persona-portrait-action"
                      disabled={disabled}
                      onClick={() => void run(() => api.deletePersonaPortrait(item.id))}
                      aria-label={t("Remove portrait")}
                      title={t("Remove portrait")}
                    >
                      <TrashIcon size={15} />
                    </button>
                  )}
                </span>
              </span>
              <span className="persona-identity-copy">
                <label className="field">
                  <span className="label">
                    {t("Name")}
                    <span className="hint-inline">{codePointLength(name)} / {options.max_name}</span>
                  </span>
                  <input
                    type="text"
                    value={name}
                    disabled={disabled}
                    onChange={(event) => setName(limitCodePoints(event.target.value, options.max_name))}
                  />
                </label>
                <label className="field">
                  <span className="label">
                    {t("Description")}
                    <span className="hint-inline">{codePointLength(description)} / {options.max_description}</span>
                  </span>
                  <textarea
                    rows={3}
                    value={description}
                    disabled={disabled}
                    onChange={(event) => setDescription(limitCodePoints(event.target.value, options.max_description))}
                  />
                  <span className="hint">
                    {t("The model reads this on every reply. It is the character being played: manner, attitude, and how they speak. Lore is separate and holds background facts.")}
                  </span>
                </label>
              </span>
            </div>
            {/* Ends the group with a hint, matching Voice and style and Behavior. */}
            <p className="hint">
              {t("Scaled to {edge}px and stored locally beside your other app data.", { edge: PROFILE_IMAGE_MAX_EDGE })}
            </p>
          </section>

          <section className="group">
            <h3 className="group-title">{t("Voice and style")}</h3>
            <div className="persona-editor-fields">
              <label className="field">
                <span className="label">{t("Reply register")}</span>
                <select
                  value={item.chat_voice}
                  disabled={disabled}
                  onChange={(event) => void patch({ chat_voice: event.target.value })}
                >
                  {options.chat_voices.map((voice) => (
                    <option key={voice} value={voice}>{personaOptionLabel(VOICE_LABELS, voice)}</option>
                  ))}
                </select>
              </label>
              <label className="field">
                <span className="label">{t("Reaction style")}</span>
                <select
                  value={item.reaction_style}
                  disabled={disabled}
                  onChange={(event) => void patch({ reaction_style: event.target.value })}
                >
                  {options.reaction_styles.map((style) => (
                    <option key={style} value={style}>{personaOptionLabel(STYLE_LABELS, style)}</option>
                  ))}
                </select>
              </label>
            </div>
            <p className="hint">
              {t("Style shapes who leads the conversation and how replies are worded. It is separate from how explicit the language may be, and it never affects the device.")}
            </p>
          </section>

          <section className="group">
            <h3 className="group-title">{t("Behavior")}</h3>
            <div className="persona-editor-fields">
              <label className="field">
                <span className="label">{t("Prompt set")}</span>
                <select
                  value={item.prompt_set_id}
                  disabled={disabled}
                  onChange={(event) => void patch({ prompt_set_id: event.target.value })}
                >
                  <option value="">{t("Use the one selected in Settings")}</option>
                  {promptSets.map((set) => (
                    <option key={set.id} value={set.id}>{translateKnown(set.name)}</option>
                  ))}
                </select>
              </label>
              <label className="field">
                <span className="label">{t("Starting zone")}</span>
                <select
                  value={item.default_focus_area}
                  disabled={disabled}
                  onChange={(event) => void patch({ default_focus_area: event.target.value })}
                >
                  {options.focus_areas.map((area) => (
                    <option key={area} value={area}>{personaOptionLabel(AREA_LABELS, area)}</option>
                  ))}
                </select>
              </label>
            </div>
            <p className="hint">
              {t("A persona never changes your speed limits, your stroke range, or what the model is allowed to control. Those stay in Settings.")}
            </p>
          </section>

          <PersonaLoreEditor
            persona={item}
            loreModes={options.lore_modes}
            locked={locked}
            onPersonaChanged={onPersonaChanged}
            onError={onError}
            onExportBlockedChange={setLoreExportBlocked}
          />
        </div>

        <div className="persona-editor-window-actions">
          <button
            type="button"
            className="btn btn-primary"
            disabled={disabled || !dirty || name.trim() === ""}
            onClick={() => void patch({ name: name.trim(), description: description.trim() })}
          >
            {t("Save")}
          </button>
          <button
            type="button"
            className="btn btn-secondary"
            disabled={disabled}
            onClick={() => void run(() => api.duplicatePersona(item.id))}
          >
            {t("Duplicate")}
          </button>
          <button
            type="button"
            className="btn btn-secondary"
            disabled={busy || !exportAvailable || dirty || loreExportBlocked}
            title={dirty || loreExportBlocked
              ? t("Save changes before exporting.")
              : t("Export {name}", { name: item.name })}
            onClick={() => void exportPersona()}
          >
            <DownloadIcon size={16} />
            {t("Export persona")}
          </button>
          <button
            type="button"
            className="btn btn-danger-outline"
            disabled={disabled}
            onClick={() => {
              // Says what survives: a transcript is not the persona's to take away.
              if (!window.confirm(t("Delete {name}? Chats you have already had with them keep their messages.", { name: item.name }))) {
                return;
              }
              void run(async () => {
                const payload = await api.deletePersona(item.id);
                onClose();
                return payload;
              });
            }}
          >
            {t("Delete persona")}
          </button>
        </div>
      </section>
    </div>
  );
}
