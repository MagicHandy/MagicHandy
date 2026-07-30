import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "./client";

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
});
