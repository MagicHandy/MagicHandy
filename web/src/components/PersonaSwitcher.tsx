import { useCallback, useEffect, useState } from "react";
import { t } from "../i18n";
import { api } from "../api/client";
import type { DefaultPersona, Persona } from "../api/types";
import { ChevronUpIcon } from "../shell/icons";
import { monogram } from "./PersonaGrid";
import { useMenu } from "./useMenu";

// The chat header's persona control. Going to a separate page to change who you
// are talking to is a trip too many, so the switcher lives where the
// conversation is (docs/persona-page.md §5.5).
//
// Headless preview reports a 0x0 viewport, which collapses a popover positioned
// from viewport maths. The panel is therefore positioned relative to its own
// trigger with plain CSS rather than measured coordinates.
export function PersonaSwitcher({
  sessionID,
  disabled,
  onChanged,
}: {
  sessionID: string;
  disabled: boolean;
  onChanged?: () => void;
}) {
  const [personas, setPersonas] = useState<Persona[]>([]);
  const [defaultPersona, setDefaultPersona] = useState<DefaultPersona | null>(null);
  const [activeID, setActiveID] = useState("");
  const [busy, setBusy] = useState(false);
  const menu = useMenu({ disabled });

  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      const payload = await api.personas(signal);
      setPersonas(payload.personas);
      setDefaultPersona(payload.default_persona);
      setActiveID(payload.active_persona_id);
    } catch {
      // A persona chip is decoration on top of chat. If the library cannot be
      // read, the header simply shows nothing rather than an error the user can
      // do nothing about mid-conversation.
      setPersonas([]);
      setDefaultPersona(null);
      setActiveID("");
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load, sessionID]);

  const active = personas.find((item) => item.id === activeID);
  const current = active ?? defaultPersona;
  // Choosing closes the menu and returns focus to the chip at once. The chip
  // stays enabled (busy) during the request so focus has somewhere to land.
  const select = async (personaID: string) => {
    menu.close();
    if (busy) return;
    setBusy(true);
    try {
      const payload = await api.selectSessionPersona(sessionID, personaID);
      setPersonas(payload.personas);
      setDefaultPersona(payload.default_persona);
      setActiveID(payload.active_persona_id);
      onChanged?.();
    } catch {
      // Reload rather than guess: the server is the authority on what is bound.
      await load();
    } finally {
      setBusy(false);
    }
  };

  if (!defaultPersona || !current) return null;
  const portrait = active ? api.personaPortraitURL(active) : "";

  return (
    <div className="persona-switcher-wrap">
      <button
        type="button"
        className="persona-chip"
        aria-haspopup="menu"
        aria-expanded={menu.open}
        aria-controls={menu.open ? menu.id : undefined}
        aria-busy={busy || undefined}
        disabled={disabled}
        onClick={(event) => {
          if (menu.open) menu.close();
          else if (!busy) menu.show(event.currentTarget, undefined);
        }}
        onKeyDown={(event) => { if (!busy) menu.onTriggerKeyDown(event, undefined); }}
      >
        {active
          ? (portrait
            ? <img className="persona-chip-avatar" src={portrait} alt="" />
            : <span className="persona-chip-avatar-text" aria-hidden="true">{monogram(active.name)}</span>)
          : <span className="persona-chip-avatar-text" aria-hidden="true">{monogram(defaultPersona.name)}</span>}
        <span className="persona-chip-name">{current.name}</span>
        <ChevronUpIcon size={14} className="persona-chip-chevron" />
      </button>
      {menu.open && (
        <div {...menu.menuProps} className="persona-switcher" aria-label={t("Personas")}>
          <button
            type="button"
            role="menuitemradio"
            aria-checked={!active}
            className="persona-switcher-option"
            onClick={() => void select("")}
          >
            <span className="persona-chip-avatar-text" aria-hidden="true">{monogram(defaultPersona.name)}</span>
            <span className="persona-switcher-option-copy">
              <strong>{defaultPersona.name}</strong>
              <small>{t("Default")}</small>
            </span>
          </button>
          {personas.map((item) => {
            const itemPortrait = api.personaPortraitURL(item);
            return (
              <button
                key={item.id}
                type="button"
                role="menuitemradio"
                aria-checked={item.id === activeID}
                className="persona-switcher-option"
                onClick={() => void select(item.id)}
              >
                {itemPortrait
                  ? <img className="persona-chip-avatar" src={itemPortrait} alt="" />
                  : <span className="persona-chip-avatar-text" aria-hidden="true">{monogram(item.name)}</span>}
                <span>{item.name}</span>
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
