import { useRef } from "react";
import { t } from "../i18n";
import { api } from "../api/client";
import type { DefaultPersona, Persona } from "../api/types";
import { PlusIcon, UploadIcon } from "../shell/icons";
import { VOICE_CHIP_LABELS, personaOptionLabel } from "./persona-labels";

// monogram takes the first Unicode code point rather than the first UTF-16 code
// unit, so an astral symbol is not rendered as a broken surrogate.
export function monogram(name: string): string {
  const first = Array.from(name.trim())[0] ?? "?";
  return first.toUpperCase();
}

export function PersonaTile({
  item,
  active,
  onOpen,
}: {
  item: Persona;
  active: boolean;
  onOpen: (item: Persona) => void;
}) {
  const portrait = api.personaPortraitURL(item);
  return (
    <button
      type="button"
      className="persona-card"
      aria-current={active ? "true" : undefined}
      aria-label={t("Edit {name}", { name: item.name })}
      onClick={() => onOpen(item)}
    >
      {portrait ? (
        <img className="persona-card-portrait" src={portrait} alt="" loading="lazy" decoding="async" />
      ) : (
        // Not a broken-image frame: no portrait is a normal state, and a plate
        // with an initial reads as "no picture yet" rather than "this failed".
        <span className="persona-card-monogram" aria-hidden="true">
          <span>{monogram(item.name)}</span>
        </span>
      )}
      <span className="persona-card-badges">
        {active && <span className="badge persona-active-badge">{t("active")}</span>}
        <span className="badge">{personaOptionLabel(VOICE_CHIP_LABELS, item.chat_voice)}</span>
      </span>
      <span className="persona-card-copy">
        {item.lore_count > 0 && (
          <span className="badge persona-lore-badge">
            {item.lore_count === 1
              ? t("1 lore entry")
              : t("{count} lore entries", { count: item.lore_count })}
          </span>
        )}
        <strong>{item.name}</strong>
        {item.description && <span className="persona-card-description">{item.description}</span>}
      </span>
    </button>
  );
}

export function DefaultPersonaTile({
  item,
  active,
}: {
  item: DefaultPersona;
  active: boolean;
}) {
  return (
    <a
      className="persona-card persona-card-default"
      href="#/settings/model"
      aria-current={active ? "true" : undefined}
      aria-label={t("{name} (Default)", { name: item.name })}
    >
      <span className="persona-card-monogram" aria-hidden="true">
        <span>{monogram(item.name)}</span>
      </span>
      <span className="persona-card-badges">
        {active && <span className="badge persona-active-badge">{t("active")}</span>}
        <span className="badge">{t("Default")}</span>
        <span className="badge">{personaOptionLabel(VOICE_CHIP_LABELS, item.chat_voice)}</span>
      </span>
      <span className="persona-card-copy">
        <strong>{item.name}</strong>
        {item.description && <span className="persona-card-description">{item.description}</span>}
      </span>
    </a>
  );
}

export function PersonaGrid({
  personas,
  defaultPersona,
  activeID,
  locked,
  onOpen,
  onCreate,
  onImport,
}: {
  personas: Persona[];
  defaultPersona: DefaultPersona;
  activeID: string;
  locked: boolean;
  onOpen: (item: Persona) => void;
  onCreate: () => void;
  onImport: (file: File) => void;
}) {
  const importInput = useRef<HTMLInputElement>(null);
  const activePersonaExists = personas.some((item) => item.id === activeID);
  const count = personas.length + 1;

  return (
    <>
      <div className="persona-grid">
        {/* First in DOM order, not just visually first: keyboard users reach New
            then Import before the cards. The shared footprint keeps the empty
            library from changing shape after its first import. */}
        <div className="persona-card persona-card-actions">
          <button
            type="button"
            className="persona-card-action"
            onClick={onCreate}
            disabled={locked}
          >
            <span className="icon" aria-hidden="true"><PlusIcon size={20} /></span>
            <span>{t("New persona")}</span>
          </button>
          <button
            type="button"
            className="persona-card-action"
            onClick={() => importInput.current?.click()}
            disabled={locked}
          >
            <span className="icon" aria-hidden="true"><UploadIcon size={20} /></span>
            <span>{t("Import persona")}</span>
          </button>
          <input
            ref={importInput}
            type="file"
            className="visually-hidden"
            accept=".mhpersona,application/vnd.magichandy.persona+zip,application/zip"
            onChange={(event) => {
              const file = event.target.files?.[0];
              if (file) onImport(file);
              event.target.value = "";
            }}
          />
        </div>
        <DefaultPersonaTile item={defaultPersona} active={!activeID || !activePersonaExists} />
        {personas.map((item) => (
          <PersonaTile key={item.id} item={item} active={item.id === activeID} onOpen={onOpen} />
        ))}
      </div>
      <p className="persona-count">
        {/* An explicit singular key rather than a plural rule: the app has no
            pluralization machinery, and the catalog count in the video library
            already reads this way. */}
        {count === 1
          ? t("1 persona")
          : t("{count} personas", { count })}
      </p>
    </>
  );
}
