import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "./client";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("controller API contract", () => {
  it("requests an explicit takeover with the client header", async () => {
    const response = {
      controller: {
        client_id: "ui-test",
        active: true,
        read_only: false,
      },
      changed: true,
      stop_confirmed: true,
      stop_sequence: 4,
    };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      text: async () => JSON.stringify(response),
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.takeControl()).resolves.toEqual(response);

    expect(fetchMock).toHaveBeenCalledWith("/api/controller/takeover", expect.objectContaining({
      method: "POST",
      body: "{}",
      headers: expect.objectContaining({
        "Content-Type": "application/json",
        "X-MagicHandy-Client-ID": expect.stringMatching(/^ui-/),
      }),
    }));
  });
});
