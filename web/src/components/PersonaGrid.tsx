import { t } from "../i18n";
import { api } from "../api/client";
import type { Persona } from "../api/types";
import { PlusIcon } from "../shell/icons";
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

export function PersonaGrid({
  personas,
  activeID,
  locked,
  onOpen,
  onCreate,
}: {
  personas: Persona[];
  activeID: string;
  locked: boolean;
  onOpen: (item: Persona) => void;
  onCreate: () => void;
}) {
  return (
    <>
      <div className="persona-grid">
        {/* First in DOM order, not just visually first: keyboard users reach the
            create control before the cards, and a screen reader announces it as a
            button among cards rather than as another persona. */}
        <button
          type="button"
          className="persona-card persona-card-new"
          onClick={onCreate}
          disabled={locked}
        >
          <span className="icon" aria-hidden="true"><PlusIcon size={20} /></span>
          <span>{t("New persona")}</span>
        </button>
        {personas.map((item) => (
          <PersonaTile key={item.id} item={item} active={item.id === activeID} onOpen={onOpen} />
        ))}
      </div>
      {personas.length > 0 && (
        <p className="persona-count">
          {/* An explicit singular key rather than a plural rule: the app has no
              pluralization machinery, and the catalog count in the video library
              already reads this way. */}
          {personas.length === 1
            ? t("1 persona")
            : t("{count} personas · ordered by last used", { count: personas.length })}
        </p>
      )}
    </>
  );
}
