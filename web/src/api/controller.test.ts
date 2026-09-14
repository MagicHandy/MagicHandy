import { afterEach, describe, expect, it, vi } from "vitest";
import { api, request } from "./client";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("controller API contract", () => {
  it("does not roll authority back when an earlier HTTP response arrives late", async () => {
    let finishOld!: (value: unknown) => void;
    const response = (epoch: string, generation: number) => ({ ok: true, status: 200,
      text: async () => JSON.stringify({ controller: { heartbeat_required: true, generation, epoch } }) });
    const fetchMock = vi.fn().mockImplementationOnce(() => new Promise((resolve) => { finishOld = resolve; }))
      .mockResolvedValueOnce(response("new-process", 2)).mockResolvedValue({ ok: true, status: 200, text: async () => "{}" });
    vi.stubGlobal("fetch", fetchMock);
    const old = api.getState();
    await api.getState();
    finishOld(response("old-process", 90));
    await old;
    await request("POST", "/api/motion/quick", { speed_max_percent: 20 });
    expect(fetchMock.mock.calls[2][1].headers["X-MagicHandy-Control-Epoch"]).toBe("new-process");
    expect(fetchMock.mock.calls[2][1].headers["X-MagicHandy-Control-Generation"]).toBe("2");
    fetchMock.mockResolvedValueOnce({ ok: true, status: 200, text: async () => JSON.stringify({ heartbeat_required: false }) });
    await api.getState();
  });
  it("echoes the backend ownership generation and replaces it after takeover", async () => {
    let generation = 7;
    const fetchMock = vi.fn().mockImplementation(async () => ({
      ok: true, status: 200,
      text: async () => JSON.stringify({ controller: { active: true, read_only: false, heartbeat_required: true, generation } }),
    }));
    vi.stubGlobal("fetch", fetchMock);
    await api.getState();
    await request("POST", "/api/motion/quick", { speed_max_percent: 20 });
    expect(fetchMock.mock.calls[1][1].headers["X-MagicHandy-Control-Generation"]).toBe("7");
    generation = 9;
    await api.takeControl();
    await request("POST", "/api/motion/quick", { speed_max_percent: 30 });
    expect(fetchMock.mock.calls[3][1].headers["X-MagicHandy-Control-Generation"]).toBe("9");
    fetchMock.mockResolvedValueOnce({ ok: true, status: 200, text: async () => JSON.stringify({ controller: { heartbeat_required: false } }) });
    await api.getState();
    await api.stopMotion();
    expect(fetchMock.mock.calls[5][1].headers["X-MagicHandy-Control-Generation"]).toBeUndefined();
  });

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
