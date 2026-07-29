import { useEffect, useRef, useState } from "react";
import { t, translateKnown } from "../i18n";
import { api } from "../api/client";
import type { Persona, PersonaDraft, PersonasPayload, PromptSet } from "../api/types";
import { CloseIcon } from "../shell/icons";
import { monogram } from "./PersonaGrid";
import { AREA_LABELS, STYLE_LABELS, VOICE_LABELS, personaOptionLabel } from "./persona-labels";

// Portraits are downscaled in the browser, on the same canvas path video covers
// already use. Server-side scaling would need a new image dependency or FFmpeg,
// and FFmpeg is deliberately optional — a portrait must not be what makes it
// mandatory (docs/persona-page.md §2.2).
const PORTRAIT_MAX_EDGE = 512;
const PORTRAIT_QUALITY = 0.85;

// resizeToJPEG draws the chosen file at a bounded edge and exports JPEG bytes.
// The result is what the server validates; nothing trusts the original file's
// declared type.
async function resizeToJPEG(file: File): Promise<Blob> {
  const url = URL.createObjectURL(file);
  try {
    const image = await new Promise<HTMLImageElement>((resolve, reject) => {
      const element = new Image();
      element.onload = () => resolve(element);
      element.onerror = () => reject(new Error("that file could not be read as an image"));
      element.src = url;
    });
    const scale = Math.min(1, PORTRAIT_MAX_EDGE / Math.max(image.naturalWidth, image.naturalHeight));
    const canvas = document.createElement("canvas");
    canvas.width = Math.max(1, Math.round(image.naturalWidth * scale));
    canvas.height = Math.max(1, Math.round(image.naturalHeight * scale));
    const context = canvas.getContext("2d");
    if (!context) throw new Error("this browser could not prepare the image");
    context.drawImage(image, 0, 0, canvas.width, canvas.height);
    const blob = await new Promise<Blob | null>((resolve) => {
      canvas.toBlob(resolve, "image/jpeg", PORTRAIT_QUALITY);
    });
    if (!blob) throw new Error("the image could not be encoded");
    return blob;
  } finally {
    URL.revokeObjectURL(url);
  }
}

interface EditorProps {
  item: Persona;
  options: PersonasPayload["options"];
  promptSets: PromptSet[];
  locked: boolean;
  onApplied: (payload: PersonasPayload) => void;
  onClose: () => void;
  onError: (message: string) => void;
}

export function PersonaEditor({ item, options, promptSets, locked, onApplied, onClose, onError }: EditorProps) {
  const [name, setName] = useState(item.name);
  const [description, setDescription] = useState(item.description);
  const [busy, setBusy] = useState(false);
  const fileInput = useRef<HTMLInputElement>(null);
  const closeButton = useRef<HTMLButtonElement>(null);

  // Reset the text fields when a different persona is opened into the same
  // drawer, otherwise the previous persona's unsaved edits leak across.
  useEffect(() => {
    setName(item.name);
    setDescription(item.description);
  }, [item.id, item.name, item.description]);

  useEffect(() => {
    closeButton.current?.focus();
  }, [item.id]);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

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

  const choosePortrait = async (file: File | undefined) => {
    if (!file) return;
    setBusy(true);
    try {
      onApplied(await api.savePersonaPortrait(item.id, await resizeToJPEG(file)));
    } catch (error) {
      onError(error instanceof Error ? error.message : String(error));
    } finally {
      setBusy(false);
      if (fileInput.current) fileInput.current.value = "";
    }
  };

  return (
    <aside className="persona-drawer" role="dialog" aria-label={t("Edit {name}", { name: item.name })}>
      <div className="persona-drawer-head">
        <h2>{item.name}</h2>
        <button
          ref={closeButton}
          type="button"
          className="btn btn-secondary"
          onClick={onClose}
          aria-label={t("Close editor")}
        >
          <CloseIcon />
        </button>
      </div>

      <section className="group">
        <h3 className="group-title">{t("Identity")}</h3>
        <div className="persona-portrait-field">
          <span className="persona-portrait-preview">
            {portrait ? (
              <img src={portrait} alt="" />
            ) : (
              <span className="persona-card-monogram" aria-hidden="true">
                <span>{monogram(item.name)}</span>
              </span>
            )}
          </span>
          <span className="persona-portrait-actions">
            <input
              ref={fileInput}
              type="file"
              accept="image/*"
              className="visually-hidden"
              onChange={(event) => void choosePortrait(event.target.files?.[0])}
            />
            <button
              type="button"
              className="btn btn-secondary"
              disabled={disabled}
              onClick={() => fileInput.current?.click()}
            >
              {portrait ? t("Replace portrait") : t("Choose portrait")}
            </button>
            {portrait && (
              <button
                type="button"
                className="btn btn-secondary"
                disabled={disabled}
                onClick={() => void run(() => api.deletePersonaPortrait(item.id))}
              >
                {t("Remove portrait")}
              </button>
            )}
            <span className="hint">
              {t("Scaled to {edge}px and stored locally beside your other app data.", { edge: PORTRAIT_MAX_EDGE })}
            </span>
          </span>
        </div>
        <label className="field">
          <span className="label">
            {t("Name")}
            <span className="hint-inline">{Array.from(name).length} / {options.max_name}</span>
          </span>
          <input
            type="text"
            value={name}
            maxLength={options.max_name}
            disabled={disabled}
            onChange={(event) => setName(event.target.value)}
          />
        </label>
        <label className="field">
          <span className="label">
            {t("Description")}
            <span className="hint-inline">{Array.from(description).length} / {options.max_description}</span>
          </span>
          <textarea
            rows={3}
            value={description}
            maxLength={options.max_description}
            disabled={disabled}
            onChange={(event) => setDescription(event.target.value)}
          />
        </label>
      </section>

      <section className="group">
        <h3 className="group-title">{t("Voice and style")}</h3>
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
        <p className="hint">
          {t("Style shapes who leads the conversation and how replies are worded. It is separate from how explicit the language may be, and it never affects the device.")}
        </p>
      </section>

      <section className="group">
        <h3 className="group-title">{t("Behavior")}</h3>
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
        <p className="hint">
          {t("A persona never changes your speed limits, your stroke range, or what the model is allowed to control. Those stay in Settings.")}
        </p>
      </section>

      <div className="persona-drawer-actions">
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
    </aside>
  );
}
