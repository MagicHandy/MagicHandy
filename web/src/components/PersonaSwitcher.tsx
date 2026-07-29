import { useCallback, useEffect, useRef, useState } from "react";
import { t } from "../i18n";
import { api } from "../api/client";
import type { Persona } from "../api/types";
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
  const [activeID, setActiveID] = useState("");
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const wrapper = useRef<HTMLDivElement>(null);

  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      const payload = await api.personas(signal);
      setPersonas(payload.personas);
      setActiveID(payload.active_persona_id);
    } catch {
      // A persona chip is decoration on top of chat. If the library cannot be
      // read, the header simply shows nothing rather than an error the user can
      // do nothing about mid-conversation.
      setPersonas([]);
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
  const select = async (personaID: string) => {
    setBusy(true);
    try {
      const payload = await api.selectSessionPersona(sessionID, personaID);
      setPersonas(payload.personas);
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

  if (personas.length === 0) return null;
  const portrait = active ? api.personaPortraitURL(active) : "";

  return (
    <div ref={wrapper} style={{ position: "relative" }}>
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
          : <span className="persona-chip-avatar-text" aria-hidden="true">—</span>}
        <span className="persona-chip-name">{active ? active.name : t("No persona")}</span>
      </button>
      {open && (
        <div className="persona-switcher" role="menu" style={{ top: "calc(100% + 6px)", right: 0 }}>
          <button
            type="button"
            role="menuitemradio"
            aria-checked={!activeID}
            aria-current={!activeID ? "true" : undefined}
            className="persona-switcher-option"
            disabled={busy}
            onClick={() => void select("")}
          >
            <span className="persona-chip-avatar-text" aria-hidden="true">—</span>
            <span>{t("No persona (use Settings)")}</span>
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
