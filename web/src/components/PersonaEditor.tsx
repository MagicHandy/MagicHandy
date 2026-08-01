import { useEffect, useRef, useState } from "react";
import { t, translateKnown } from "../i18n";
import { api } from "../api/client";
import type { Persona, PersonaDraft, PersonasPayload, PromptSet } from "../api/types";
import { CloseIcon, DownloadIcon, PencilIcon, TrashIcon } from "../shell/icons";
import { trapModalTab } from "../util/modal";
import { codePointLength, limitCodePoints } from "../util/text";
import { monogram } from "./PersonaGrid";
import { PersonaLoreEditor } from "./PersonaLoreEditor";
import { AREA_LABELS, STYLE_LABELS, VOICE_LABELS, personaOptionLabel } from "./persona-labels";

// Portraits are downscaled in the browser. Server-side scaling would need a new
// image dependency or FFmpeg, and FFmpeg is deliberately optional — a portrait
// must not be what makes it mandatory (docs/persona-page.md §2.2).
//
// 640 matches the video-thumbnail store's edge, so the two local image stores
// agree. Quality 0.92 rather than 0.85: a portrait is a few tens of kilobytes at
// this size, so the bytes saved by a lower setting are not worth the ringing it
// adds around hair and edges.
const PORTRAIT_MAX_EDGE = 640;
const PORTRAIT_QUALITY = 0.92;

// fitWithin bounds the long edge while preserving the aspect ratio.
function fitWithin(width: number, height: number, maxEdge: number) {
  const scale = Math.min(1, maxEdge / Math.max(width, height));
  return {
    width: Math.max(1, Math.round(width * scale)),
    height: Math.max(1, Math.round(height * scale)),
  };
}

function paint(source: CanvasImageSource, width: number, height: number): HTMLCanvasElement {
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const context = canvas.getContext("2d");
  if (!context) throw new Error("this browser could not prepare the image");
  context.imageSmoothingEnabled = true;
  context.imageSmoothingQuality = "high";
  context.drawImage(source, 0, 0, width, height);
  return canvas;
}

// stepDown is the fallback resampler, and the reason the old output aliased.
//
// A single drawImage to a much smaller size is one bilinear sample: it reads a
// 2x2 neighbourhood per output pixel, so reducing 2048px to 640px never looks at
// roughly 90% of the source and turns fine detail into stair-stepping and moire.
// Halving repeatedly keeps every step within 2x, where a 2x2 read is a true box
// average of the pixels being merged, so no source detail is skipped.
function stepDown(source: CanvasImageSource, sourceWidth: number, sourceHeight: number,
  targetWidth: number, targetHeight: number): HTMLCanvasElement {
  let width = sourceWidth;
  let height = sourceHeight;
  let current = source;
  while (width > targetWidth * 2 && height > targetHeight * 2) {
    width = Math.max(targetWidth, Math.floor(width / 2));
    height = Math.max(targetHeight, Math.floor(height / 2));
    current = paint(current, width, height);
  }
  return paint(current, targetWidth, targetHeight);
}

// resizeToJPEG bounds the chosen file and exports JPEG bytes. The result is what
// the server validates; nothing trusts the original file's declared type.
//
// createImageBitmap resizes during decode with the browser's own high-quality
// resampler, which is both better and cheaper than anything reachable from a
// canvas. Its resize options are not universally supported, so a bitmap that
// comes back at the wrong size falls through to the manual path rather than
// being trusted.
async function resizeToJPEG(file: File): Promise<Blob> {
  let bitmap: ImageBitmap | null = null;
  let canvas: HTMLCanvasElement | null = null;
  try {
    if (typeof createImageBitmap === "function") {
      bitmap = await createImageBitmap(file).catch(() => null);
    }
    if (bitmap) {
      const target = fitWithin(bitmap.width, bitmap.height, PORTRAIT_MAX_EDGE);
      if (target.width !== bitmap.width || target.height !== bitmap.height) {
        const resized = await createImageBitmap(file, {
          resizeWidth: target.width,
          resizeHeight: target.height,
          resizeQuality: "high",
        }).catch(() => null);
        if (resized && resized.width === target.width && resized.height === target.height) {
          bitmap.close();
          bitmap = resized;
          canvas = paint(bitmap, target.width, target.height);
        } else {
          resized?.close();
          canvas = stepDown(bitmap, bitmap.width, bitmap.height, target.width, target.height);
        }
      } else {
        canvas = paint(bitmap, target.width, target.height);
      }
    } else {
      // No createImageBitmap at all: decode through an element instead.
      const url = URL.createObjectURL(file);
      try {
        const image = await new Promise<HTMLImageElement>((resolve, reject) => {
          const element = new Image();
          element.onload = () => resolve(element);
          element.onerror = () => reject(new Error("that file could not be read as an image"));
          element.src = url;
        });
        const target = fitWithin(image.naturalWidth, image.naturalHeight, PORTRAIT_MAX_EDGE);
        canvas = stepDown(image, image.naturalWidth, image.naturalHeight, target.width, target.height);
      } finally {
        URL.revokeObjectURL(url);
      }
    }
    const blob = await new Promise<Blob | null>((resolve) => {
      canvas?.toBlob(resolve, "image/jpeg", PORTRAIT_QUALITY);
    });
    if (!blob) throw new Error("the image could not be encoded");
    return blob;
  } finally {
    bitmap?.close();
  }
}

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
      onApplied(await api.savePersonaPortrait(item.id, await resizeToJPEG(file)));
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
              {t("Scaled to {edge}px and stored locally beside your other app data.", { edge: PORTRAIT_MAX_EDGE })}
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
