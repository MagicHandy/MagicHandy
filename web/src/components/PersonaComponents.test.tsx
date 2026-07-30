import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { DefaultPersona, Persona, PersonaLorePayload, PersonasPayload } from "../api/types";
import { PersonaEditor } from "./PersonaEditor";
import { PersonaGrid, monogram } from "./PersonaGrid";
import { PersonaSwitcher } from "./PersonaSwitcher";

vi.mock("../api/client", () => ({
  api: {
    personas: vi.fn(),
    createPersona: vi.fn(),
    importPersona: vi.fn(),
    updatePersona: vi.fn(),
    duplicatePersona: vi.fn(),
    deletePersona: vi.fn(),
    exportPersona: vi.fn(),
    savePersonaPortrait: vi.fn(),
    deletePersonaPortrait: vi.fn(),
    personaLore: vi.fn(),
    createPersonaLore: vi.fn(),
    updatePersonaLore: vi.fn(),
    deletePersonaLore: vi.fn(),
    selectSessionPersona: vi.fn(),
    personaPortraitURL: (item: Persona) => (item.has_portrait ? `/api/personas/${item.id}/portrait?v=1` : ""),
  },
}));

const personas = vi.mocked(api.personas);
const updatePersona = vi.mocked(api.updatePersona);
const selectSessionPersona = vi.mocked(api.selectSessionPersona);
const deletePersonaPortrait = vi.mocked(api.deletePersonaPortrait);

function persona(overrides: Partial<Persona> = {}): Persona {
  return {
    id: "persona-0123456789ab",
    name: "Rowan",
    description: "Steady, low-voiced.",
    chat_voice: "intimate",
    reaction_style: "tender",
    prompt_set_id: "",
    default_focus_area: "full",
    lore_mode: "off",
    greeting: "",
    lore_count: 0,
    has_portrait: false,
    created_at: "2026-07-29T10:00:00Z",
    updated_at: "2026-07-29T10:00:00Z",
    ...overrides,
  };
}

function genericPersona(overrides: Partial<DefaultPersona> = {}): DefaultPersona {
  return {
    name: "MagicHandy",
    description: "",
    chat_voice: "utility",
    prompt_set_id: "magichandy_motion_v1",
    ...overrides,
  };
}

function payload(overrides: Partial<PersonasPayload> = {}): PersonasPayload {
  return {
    personas: [persona()],
    default_persona: genericPersona(),
    active_persona_id: "",
    active_session_id: "chat-1",
    prompt_sets: [{ id: "magichandy_motion_v1", name: "MagicHandy Motion", system: "", builtin: true }],
    options: {
      chat_voices: ["utility", "warm", "intimate", "explicit"],
      reaction_styles: ["neutral", "playful", "tender", "dominant", "submissive", "teasing"],
      focus_areas: ["tip", "shaft", "base", "full"],
      lore_modes: ["off", "relevant", "full"],
      max_name: 60,
      max_description: 500,
      max_greeting: 2000,
      max_portrait_edge: 1024,
      max_lore_entries: 8,
      max_lore_text: 500,
      max_lore_total: 2000,
      max_lore_keywords: 12,
      max_lore_keyword: 40,
    },
    ...overrides,
  };
}

function lorePayload(item = persona()): PersonaLorePayload {
  return {
    persona: item,
    entries: [],
    options: {
      max_entries: 8,
      max_text: 500,
      max_total: 2000,
      max_keywords: 12,
      max_keyword: 40,
    },
  };
}

describe("PersonaGrid", () => {
  it("puts the create control first so keyboard users reach it before the cards", () => {
    render(
      <PersonaGrid
        personas={[persona({ id: "persona-aaaaaaaaaaaa", name: "Ash" }), persona({ id: "persona-bbbbbbbbbbbb", name: "Mara" })]}
        defaultPersona={genericPersona()}
        activeID=""
        locked={false}
        onOpen={vi.fn()}
        onCreate={vi.fn()}
        onImport={vi.fn()}
      />,
    );

    const buttons = screen.getAllByRole("button");
    expect(buttons[0]).toHaveAccessibleName("New persona");
    expect(buttons[1]).toHaveAccessibleName("Import persona");
  });

  it("passes the selected portable file through the import half", () => {
    const onImport = vi.fn();
    const { container } = render(
      <PersonaGrid
        personas={[]}
        defaultPersona={genericPersona()}
        activeID=""
        locked={false}
        onOpen={vi.fn()}
        onCreate={vi.fn()}
        onImport={onImport}
      />,
    );
    const file = new File(["archive"], "rowan.mhpersona", {
      type: "application/vnd.magichandy.persona+zip",
    });
    const input = container.querySelector<HTMLInputElement>('input[type="file"]');

    expect(input).not.toBeNull();
    fireEvent.change(input!, { target: { files: [file] } });
    expect(onImport).toHaveBeenCalledWith(file);
  });

  it("always lists the Settings-backed MagicHandy default", () => {
    render(
      <PersonaGrid
        personas={[]}
        defaultPersona={genericPersona()}
        activeID=""
        locked={false}
        onOpen={vi.fn()}
        onCreate={vi.fn()}
        onImport={vi.fn()}
      />,
    );

    const defaultTile = screen.getByRole("link", { name: "MagicHandy (Default)" });
    expect(defaultTile).toHaveAttribute("href", "#/settings/model");
    expect(defaultTile).toHaveAttribute("aria-current", "true");
    expect(screen.getByText("1 persona")).toBeInTheDocument();
  });

  it("marks only the active persona and reports the register on every tile", () => {
    render(
      <PersonaGrid
        personas={[persona({ id: "persona-aaaaaaaaaaaa", name: "Ash", chat_voice: "warm" }), persona({ id: "persona-bbbbbbbbbbbb", name: "Mara", chat_voice: "explicit" })]}
        defaultPersona={genericPersona()}
        activeID="persona-bbbbbbbbbbbb"
        locked={false}
        onOpen={vi.fn()}
        onCreate={vi.fn()}
        onImport={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "Edit Mara" })).toHaveAttribute("aria-current", "true");
    expect(screen.getByRole("button", { name: "Edit Ash" })).not.toHaveAttribute("aria-current");
    expect(screen.getAllByText("active")).toHaveLength(1);
    expect(screen.getByText("Warm")).toBeInTheDocument();
    expect(screen.getByText("Explicit")).toBeInTheDocument();
  });

  it("falls back to a monogram rather than a broken image", () => {
    render(
      <PersonaGrid
        personas={[persona({ name: "vesper" }), persona({ id: "persona-cccccccccccc", name: "Ash", has_portrait: true })]}
        defaultPersona={genericPersona()}
        activeID=""
        locked={false}
        onOpen={vi.fn()}
        onCreate={vi.fn()}
        onImport={vi.fn()}
      />,
    );

    // One portrait, one monogram: no persona renders an <img> with no source.
    const images = screen.getAllByRole("presentation", { hidden: true }).filter((node) => node.tagName === "IMG");
    expect(images).toHaveLength(1);
    expect(screen.getByText("V")).toBeInTheDocument();
  });

  it("disables only the create control when the client is read-only", () => {
    render(
      <PersonaGrid
        personas={[persona()]}
        defaultPersona={genericPersona()}
        activeID=""
        locked
        onOpen={vi.fn()}
        onCreate={vi.fn()}
        onImport={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "New persona" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Import persona" })).toBeDisabled();
    // Opening a persona to look at it is not a write, so it stays available.
    expect(screen.getByRole("button", { name: "Edit Rowan" })).toBeEnabled();
  });

  it("takes the first Unicode code point of a name, not the first code unit", () => {
    expect(monogram("Rowan")).toBe("R");
    expect(monogram("  ash  ")).toBe("A");
    expect(monogram("")).toBe("?");
    expect(monogram("🜁 sigil")).toBe("🜁");
  });
});

describe("PersonaEditor", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    updatePersona.mockResolvedValue(payload());
    deletePersonaPortrait.mockResolvedValue(payload());
    // Most PersonaEditor tests exercise the existing axes and do not need to
    // settle the independently loaded lore group. Leaving that request pending
    // avoids unrelated post-assertion state updates; the lore-specific test
    // below resolves it and waits for the rendered result.
    vi.mocked(api.personaLore).mockImplementation(() => new Promise(() => {}));
  });
  afterEach(() => vi.unstubAllGlobals());

  const renderEditor = (item = persona(), locked = false) => {
    const state = payload();
    const onApplied = vi.fn();
    const onPersonaChanged = vi.fn();
    const onClose = vi.fn();
    const onError = vi.fn();
    const onExported = vi.fn();
    const view = render(
      <PersonaEditor
        item={item}
        options={state.options}
        promptSets={state.prompt_sets ?? []}
        locked={locked}
        exportAvailable
        onApplied={onApplied}
        onPersonaChanged={onPersonaChanged}
        onClose={onClose}
        onError={onError}
        onExported={onExported}
      />,
    );
    return { ...view, onApplied, onPersonaChanged, onClose, onError, onExported };
  };

  it("opens as a focused modal window and closes from its backdrop", () => {
    const { onClose, unmount } = renderEditor();
    const dialog = screen.getByRole("dialog");
    const overlay = dialog.parentElement;

    expect(dialog).toHaveClass("persona-editor-window");
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(overlay).toHaveClass("modal-scrim", "persona-editor-overlay");
    expect(screen.getByRole("button", { name: "Close editor" })).toHaveFocus();
    expect(document.body.style.overflow).toBe("hidden");

    fireEvent.mouseDown(dialog);
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.mouseDown(overlay!);
    expect(onClose).toHaveBeenCalledOnce();

    unmount();
    expect(document.body.style.overflow).toBe("");
  });

  it("applies selects immediately and holds text behind Save", async () => {
    renderEditor();

    fireEvent.change(screen.getByRole("combobox", { name: /Reaction style/ }), { target: { value: "dominant" } });
    await waitFor(() => expect(updatePersona).toHaveBeenCalledWith("persona-0123456789ab", { reaction_style: "dominant" }));

    // Save is inert until something actually changed, so it never suggests there
    // are unsaved edits when there are none.
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    fireEvent.change(screen.getByRole("textbox", { name: /Name/ }), { target: { value: "Rowan Vale" } });
    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();

    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(updatePersona).toHaveBeenLastCalledWith("persona-0123456789ab", {
      name: "Rowan Vale",
      description: "Steady, low-voiced.",
      greeting: "",
    }));
  });

  it("refuses to save an empty name", () => {
    renderEditor();
    fireEvent.change(screen.getByRole("textbox", { name: /Name/ }), { target: { value: "   " } });
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("applies character limits by Unicode code point", async () => {
    renderEditor();
    const requested = "😀".repeat(61);
    const accepted = "😀".repeat(60);
    const nameInput = screen.getByRole("textbox", { name: /Name/ });

    fireEvent.change(nameInput, { target: { value: requested } });
    expect(nameInput).toHaveValue(accepted);
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(updatePersona).toHaveBeenLastCalledWith(
      "persona-0123456789ab",
      { name: accepted, description: "Steady, low-voiced.", greeting: "" },
    ));
  });

  it("offers exactly the axes the server sent and no capability controls", () => {
    renderEditor();

    expect(screen.getByRole("combobox", { name: /Reply register/ })).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: /Reaction style/ })).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: /Starting zone/ })).toBeInTheDocument();
    // The guardrail, asserted in the UI as well as the API: a persona editor must
    // not offer a way to change limits or what the model may control.
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
    expect(screen.queryByRole("slider")).not.toBeInTheDocument();
    expect(screen.getByText(/never changes your speed limits/i)).toBeInTheDocument();
  });

  it("says what a deletion keeps, and does nothing when the confirm is declined", async () => {
    vi.stubGlobal("confirm", vi.fn(() => false));
    renderEditor();

    fireEvent.click(screen.getByRole("button", { name: "Delete persona" }));
    expect(window.confirm).toHaveBeenCalledWith(expect.stringContaining("keep their messages"));
    await waitFor(() => expect(api.deletePersona).not.toHaveBeenCalled());
  });

  it("offers a portrait remove control only when there is one to remove", () => {
    renderEditor(persona({ has_portrait: true }));
    expect(screen.getByRole("button", { name: "Replace portrait" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Remove portrait" })).toBeInTheDocument();
  });

  it("locks every write for a read-only client", () => {
    renderEditor(persona(), true);
    expect(screen.getByRole("combobox", { name: /Reply register/ })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Choose portrait" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Duplicate" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Delete persona" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Export persona" })).toBeEnabled();
  });

  it("exports the persisted persona with the server-provided filename", async () => {
    const blob = new Blob(["portable"]);
    vi.mocked(api.exportPersona).mockResolvedValue({ blob, filename: "rowan.mhpersona" });
    const createObjectURL = vi.fn(() => "blob:persona");
    const revokeObjectURL = vi.fn();
    vi.stubGlobal("URL", { ...URL, createObjectURL, revokeObjectURL });
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
    const { onExported } = renderEditor();

    fireEvent.click(screen.getByRole("button", { name: "Export persona" }));

    await waitFor(() => expect(api.exportPersona).toHaveBeenCalledWith("persona-0123456789ab"));
    expect(click).toHaveBeenCalledOnce();
    expect(onExported).toHaveBeenCalledWith("Rowan");
    click.mockRestore();
  });

  it("surfaces a rejected change instead of showing it as applied", async () => {
    updatePersona.mockRejectedValue(new Error("unknown reaction style"));
    const { onApplied, onError } = renderEditor();

    fireEvent.change(screen.getByRole("combobox", { name: /Reaction style/ }), { target: { value: "playful" } });
    await waitFor(() => expect(onError).toHaveBeenCalledWith("unknown reaction style"));
    expect(onApplied).not.toHaveBeenCalled();
  });

  it("loads lore separately and applies its prompt policy immediately", async () => {
    const current = persona({ lore_mode: "relevant", lore_count: 1 });
    const next = persona({ lore_mode: "full", lore_count: 1 });
    vi.mocked(api.personaLore).mockResolvedValueOnce({
      ...lorePayload(current),
      entries: [{
        id: "lore-0123456789ab",
        persona_id: current.id,
        text: "Blue velvet is familiar.",
        keywords: ["velvet"],
        enabled: true,
        created_at: "2026-07-29T10:00:00Z",
        updated_at: "2026-07-29T10:00:00Z",
      }],
    });
    updatePersona.mockResolvedValueOnce(payload({ personas: [next], persona: next }));
    const { onPersonaChanged } = renderEditor(current);

    expect(await screen.findByDisplayValue("Blue velvet is familiar.")).toBeInTheDocument();
    fireEvent.change(screen.getByRole("combobox", { name: "Prompt use" }), { target: { value: "full" } });

    await waitFor(() => expect(updatePersona).toHaveBeenCalledWith(current.id, { lore_mode: "full" }));
    await waitFor(() => expect(onPersonaChanged).toHaveBeenCalledWith(next));
  });

  it("uses server-provided lore modes and rejects an overlong keyword before Save", async () => {
    const current = persona({ lore_mode: "off" });
    vi.mocked(api.personaLore).mockResolvedValue(lorePayload(current));
    const state = payload();
    state.options.lore_modes = ["off", "full"];
    render(
      <PersonaEditor
        item={current}
        options={state.options}
        promptSets={state.prompt_sets}
        locked={false}
        exportAvailable
        onApplied={vi.fn()}
        onPersonaChanged={vi.fn()}
        onClose={vi.fn()}
        onError={vi.fn()}
        onExported={vi.fn()}
      />,
    );

    const mode = await screen.findByRole("combobox", { name: "Prompt use" });
    expect(within(mode).getAllByRole("option").map((option) => option.getAttribute("value")))
      .toEqual(["off", "full"]);

    fireEvent.click(screen.getByRole("button", { name: "Add lore entry" }));
    expect(screen.getByRole("button", { name: "Export persona" })).toBeDisabled();
    const row = document.querySelector(".persona-lore-row");
    expect(row).not.toBeNull();
    fireEvent.change(within(row as HTMLElement).getByRole("textbox", { name: /Lore text/ }), {
      target: { value: "A remembered detail." },
    });
    fireEvent.change(within(row as HTMLElement).getByRole("textbox", { name: /Keywords/ }), {
      target: { value: "😀".repeat(41) },
    });
    expect(within(row as HTMLElement).getByRole("button", { name: "Save" })).toBeDisabled();

    fireEvent.change(within(row as HTMLElement).getByRole("textbox", { name: /Keywords/ }), {
      target: { value: "😀".repeat(40) },
    });
    expect(within(row as HTMLElement).getByRole("button", { name: "Save" })).toBeEnabled();
  });

  it("keeps loaded lore and the scroll region stable when parent callbacks change", async () => {
    const current = persona({ lore_mode: "full", lore_count: 1 });
    vi.mocked(api.personaLore).mockResolvedValue({
      ...lorePayload(current),
      entries: [{
        id: "lore-0123456789ab",
        persona_id: current.id,
        text: "Blue velvet is familiar.",
        keywords: ["velvet"],
        enabled: true,
        created_at: "2026-07-29T10:00:00Z",
        updated_at: "2026-07-29T10:00:00Z",
      }],
    });
    const state = payload();
    const common = {
      item: current,
      options: state.options,
      promptSets: state.prompt_sets ?? [],
      locked: false,
      exportAvailable: true,
      onApplied: vi.fn(),
      onPersonaChanged: vi.fn(),
      onClose: vi.fn(),
      onExported: vi.fn(),
    };
    const view = render(<PersonaEditor {...common} onError={vi.fn()} />);

    expect(await screen.findByDisplayValue("Blue velvet is familiar.")).toBeInTheDocument();
    const dialog = screen.getByRole("dialog");
    const scrollBody = dialog.querySelector<HTMLElement>(".persona-editor-window-body");
    const actions = dialog.querySelector<HTMLElement>(".persona-editor-window-actions");
    expect(scrollBody).not.toContainElement(actions);
    expect(scrollBody?.nextElementSibling).toBe(actions);

    view.rerender(<PersonaEditor {...common} onError={vi.fn()} />);

    await waitFor(() => expect(api.personaLore).toHaveBeenCalledTimes(1));
    expect(screen.getByDisplayValue("Blue velvet is familiar.")).toBeInTheDocument();
    expect(screen.queryByText("Loading lore")).not.toBeInTheDocument();
  });
});

describe("PersonaSwitcher", () => {
  beforeEach(() => vi.resetAllMocks());

  it("shows the generic default when there are no custom personas", async () => {
    personas.mockResolvedValue(payload({ personas: [] }));
    render(<PersonaSwitcher sessionID="chat-1" disabled={false} />);

    const trigger = await screen.findByRole("button", { name: "MagicHandy" });
    fireEvent.click(trigger);
    expect(screen.getByRole("menuitemradio", { name: /MagicHandy Default/ })).toHaveAttribute("aria-checked", "true");
  });

  // A persona chip is decoration on top of chat. An unreadable library must not
  // put an error in the chat header the user can do nothing about.
  it("stays silent when the library cannot be read", async () => {
    personas.mockRejectedValue(new Error("persona storage is unavailable"));
    const { container } = render(<PersonaSwitcher sessionID="chat-1" disabled={false} />);
    await waitFor(() => expect(personas).toHaveBeenCalled());
    expect(container).toBeEmptyDOMElement();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("shows the bound persona and offers a way back to the global settings", async () => {
    personas.mockResolvedValue(payload({ active_persona_id: "persona-0123456789ab" }));
    selectSessionPersona.mockResolvedValue(payload({ active_persona_id: "" }));
    const onChanged = vi.fn();
    render(<PersonaSwitcher sessionID="chat-1" disabled={false} onChanged={onChanged} />);

    const chip = await screen.findByRole("button", { name: /Rowan/ });
    expect(chip.closest(".persona-switcher-wrap")).toBeInTheDocument();
    expect(chip.querySelector(".persona-chip-chevron")).toBeInTheDocument();
    fireEvent.click(chip);
    expect(chip).toHaveAttribute("aria-expanded", "true");
    const clear = screen.getByRole("menuitemradio", { name: /MagicHandy Default/ });
    expect(screen.getByRole("menuitemradio", { name: /Rowan/ })).toHaveAttribute("aria-checked", "true");

    fireEvent.click(clear);
    await waitFor(() => expect(selectSessionPersona).toHaveBeenCalledWith("chat-1", ""));
    await waitFor(() => expect(onChanged).toHaveBeenCalled());
  });

  it("reads MagicHandy when the session uses the generic default", async () => {
    personas.mockResolvedValue(payload());
    render(<PersonaSwitcher sessionID="chat-1" disabled={false} />);
    expect(await screen.findByRole("button", { name: "MagicHandy" })).toBeInTheDocument();
  });

  it("cannot switch persona from a read-only client", async () => {
    personas.mockResolvedValue(payload());
    render(<PersonaSwitcher sessionID="chat-1" disabled />);
    expect(await screen.findByRole("button", { name: "MagicHandy" })).toBeDisabled();
  });
});
