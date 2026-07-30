import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "./client";
import type { PersonasPayload } from "./types";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("persona API contract", () => {
  it("rejects a partial response instead of inventing backend vocabulary", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      text: async () => JSON.stringify({
        personas: [],
        default_persona: {
          name: "MagicHandy",
          description: "",
          chat_voice: "utility",
          prompt_set_id: "default",
        },
        active_persona_id: "",
        active_session_id: "chat-1",
        prompt_sets: [],
        options: {
          chat_voices: ["utility"],
          reaction_styles: ["neutral"],
          focus_areas: ["full"],
          lore_modes: ["off"],
          max_name: 60,
        },
      }),
    }));

    await expect(api.personas()).rejects.toThrow(
      "Persona response is incomplete; restart or update the MagicHandy core.",
    );
  });

  it("uploads a portable persona as raw archive bytes", async () => {
    const response = completePayload();
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      text: async () => JSON.stringify(response),
    });
    vi.stubGlobal("fetch", fetchMock);
    const file = new File(["archive"], "rowan.mhpersona");

    await expect(api.importPersona(file)).resolves.toEqual(response);

    expect(fetchMock).toHaveBeenCalledWith("/api/personas/import", expect.objectContaining({
      method: "POST",
      body: file,
      headers: expect.objectContaining({
        "Content-Type": "application/vnd.magichandy.persona+zip",
      }),
    }));
  });

  it("uses the archive filename supplied by the export endpoint", async () => {
    const blob = new Blob(["archive"]);
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers({
        "Content-Disposition": 'attachment; filename="rowan.mhpersona"',
      }),
      blob: async () => blob,
    }));

    await expect(api.exportPersona("persona-0123456789ab")).resolves.toEqual({
      blob,
      filename: "rowan.mhpersona",
    });
  });
});

function completePayload(): PersonasPayload {
  return {
    personas: [],
    default_persona: {
      name: "MagicHandy",
      description: "",
      chat_voice: "utility",
      prompt_set_id: "default",
    },
    active_persona_id: "",
    active_session_id: "chat-1",
    prompt_sets: [],
    options: {
      chat_voices: ["utility"],
      reaction_styles: ["neutral"],
      focus_areas: ["full"],
      lore_modes: ["off"],
      max_name: 60,
      max_description: 500,
      max_portrait_edge: 1024,
      max_lore_entries: 8,
      max_lore_text: 500,
      max_lore_total: 2000,
      max_lore_keywords: 12,
      max_lore_keyword: 40,
    },
  };
}
