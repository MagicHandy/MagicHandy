import { useCallback, useEffect, useRef, useState } from "react";
import { t } from "../i18n";
import { api } from "../api/client";
import type { DefaultPersona, Persona } from "../api/types";
import { ChevronUpIcon } from "../shell/icons";
import { monogram } from "./PersonaGrid";

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
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const wrapper = useRef<HTMLDivElement>(null);

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

  useEffect(() => {
    if (!open) return;
    const closeOutside = (event: MouseEvent) => {
      if (!wrapper.current?.contains(event.target as Node)) setOpen(false);
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };
    window.addEventListener("mousedown", closeOutside);
    window.addEventListener("keydown", closeOnEscape);
    return () => {
      window.removeEventListener("mousedown", closeOutside);
      window.removeEventListener("keydown", closeOnEscape);
    };
  }, [open]);

  const active = personas.find((item) => item.id === activeID);
  const current = active ?? defaultPersona;
  const select = async (personaID: string) => {
    setBusy(true);
    try {
      const payload = await api.selectSessionPersona(sessionID, personaID);
      setPersonas(payload.personas);
      setDefaultPersona(payload.default_persona);
      setActiveID(payload.active_persona_id);
      setOpen(false);
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
    <div ref={wrapper} className="persona-switcher-wrap">
      <button
        type="button"
        className="persona-chip"
        aria-haspopup="menu"
        aria-expanded={open}
        disabled={disabled || busy}
        onClick={() => setOpen((current) => !current)}
      >
        {active
          ? (portrait
            ? <img className="persona-chip-avatar" src={portrait} alt="" />
            : <span className="persona-chip-avatar-text" aria-hidden="true">{monogram(active.name)}</span>)
          : <span className="persona-chip-avatar-text" aria-hidden="true">{monogram(defaultPersona.name)}</span>}
        <span className="persona-chip-name">{current.name}</span>
        <ChevronUpIcon size={14} className="persona-chip-chevron" />
      </button>
      {open && (
        <div className="persona-switcher" role="menu">
          <button
            type="button"
            role="menuitemradio"
            aria-checked={!active}
            aria-current={!active ? "true" : undefined}
            className="persona-switcher-option"
            disabled={busy}
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
                aria-current={item.id === activeID ? "true" : undefined}
                className="persona-switcher-option"
                disabled={busy}
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
