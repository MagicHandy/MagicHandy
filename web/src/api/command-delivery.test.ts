import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

let client: typeof import("./client");
const reply = (value: unknown, status = 200) => ({ ok: status >= 200 && status < 300, status, text: async () => JSON.stringify(value) });
const ownership = (generation = 1, sequence = 20) => ({ controller: {
  heartbeat_required: true, active: true, read_only: false, epoch: "server-process", generation,
  command_ticket: "server-issued-ticket", command_sequence: sequence,
} });

beforeEach(async () => { vi.resetModules(); client = await import("./client"); });
afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); });

describe("command delivery", () => {
  it("does not replace current delivery metadata with a stale generation from a later-issued read", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(reply(ownership(4, 12)))
      .mockResolvedValueOnce(reply(ownership(3, 0))).mockResolvedValue(reply({}));
    vi.stubGlobal("fetch", fetchMock);
    await client.api.getState();
    await client.api.getState();
    await client.request("POST", "/api/modes/stop", { stop_motion: false });
    const headers = fetchMock.mock.calls[2][1].headers;
    expect(headers["X-MagicHandy-Control-Generation"]).toBe("4");
    expect(headers["X-MagicHandy-Command-Sequence"]).toBe("13");
    expect(headers["X-MagicHandy-Command-Ticket"]).toBe("server-issued-ticket");
  });
  it("resumes its sequence from the backend and resets it for a fresh ownership generation", async () => {
    const fetchMock = vi.fn().mockResolvedValue(reply(ownership()));
    vi.stubGlobal("fetch", fetchMock);
    await client.api.getState();
    fetchMock.mockResolvedValue(reply({}));
    await client.request("POST", "/api/motion/quick", { speed_max_percent: 30 });
    const first = fetchMock.mock.calls[1][1].headers;
    expect(first["X-MagicHandy-Command-Sequence"]).toBe("21");
    expect(first["X-MagicHandy-Command-Ticket"]).toBe("server-issued-ticket");
    fetchMock.mockResolvedValueOnce(reply(ownership(2, 0)));
    await client.api.getState();
    await client.request("POST", "/api/motion/quick", { speed_max_percent: 40 });
    const second = fetchMock.mock.calls[3][1].headers;
    expect(second["X-MagicHandy-Command-Sequence"]).toBe("1");
    expect(second["X-MagicHandy-Command-ID"]).not.toBe(first["X-MagicHandy-Command-ID"]);
  });

  it("queries the original receipt after response loss without repeating the mutation", async () => {
    let originalID = "";
    let effects = 0;
    const fetchMock = vi.fn().mockImplementation(async (path: string, init: RequestInit & { headers: Record<string, string> }) => {
      if (path === "/api/state") return reply(ownership());
      if (init.method === "POST") {
        effects++; originalID = init.headers["X-MagicHandy-Command-ID"];
        return { ok: true, status: 200, text: async () => { throw new TypeError("connection lost after commit"); } };
      }
      expect(path).toBe(`/api/controller/commands/${originalID}`);
      return reply({ id: originalID, state: "complete", replayable: true, http_status: 200, response: { motion: { speed_max_percent: 41 } } });
    });
    vi.stubGlobal("fetch", fetchMock);
    await client.api.getState();
    await expect(client.request("POST", "/api/motion/quick", { speed_max_percent: 41 })).resolves.toEqual({ motion: { speed_max_percent: 41 } });
    expect(effects).toBe(1);
    expect(fetchMock).toHaveBeenCalledTimes(3);
  });

  it("reports an unknown outcome without reissuing a missing or pending command", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(reply(ownership()))
      .mockRejectedValueOnce(new TypeError("network offline"))
      .mockResolvedValueOnce(reply({ state: "pending", replayable: false }));
    vi.stubGlobal("fetch", fetchMock);
    await client.api.getState();
    await expect(client.request("POST", "/api/motion/start", {})).rejects.toThrow("outcome is unconfirmed");
    expect(fetchMock.mock.calls.filter((call) => call[1].method === "POST")).toHaveLength(1);
  });

  it("does not put a receipt lookup in the Stop failure path", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(reply(ownership())).mockRejectedValueOnce(new TypeError("offline"));
    vi.stubGlobal("fetch", fetchMock);
    await client.api.getState();
    await expect(client.api.stopMotion()).rejects.toThrow("offline");
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});
