import { afterEach, describe, expect, it, vi } from "vitest";
import { api, streamChatRepair } from "./client";
import type { ChatStreamEvent } from "./types";

afterEach(() => vi.unstubAllGlobals());

describe("manual chat repair API", () => {
  it("sends only a server failure handle through the same authority-fenced SSE contract", async () => {
    const encoder = new TextEncoder();
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({ ok: true, status: 200, text: async () => JSON.stringify({ controller: { active: true, read_only: false, heartbeat_required: true, generation: 4, epoch: "repair-process" } }) })
      .mockResolvedValueOnce({ ok: true, status: 200, body: new ReadableStream({ start(controller) {
        controller.enqueue(encoder.encode('event: status\r\ndata: {"state":"repairing_response"}\r\n\r\nevent: message\ndata: {"reply":"Corrected"}\n\nevent: done\ndata: {"ok":true}\n\n'));
        controller.close();
      } }) });
    vi.stubGlobal("fetch", fetchMock);
    await api.getState();
    const controller = new AbortController();
    const events: ChatStreamEvent[] = [];
    await streamChatRepair("chat-test", 12, event => events.push(event), controller.signal, 8, "script");
    expect(fetchMock).toHaveBeenLastCalledWith("/api/chat/repair", expect.objectContaining({
      method: "POST", body: '{"session_id":"chat-test","failure_id":12,"motion_owner":"script"}', signal: controller.signal,
      headers: expect.objectContaining({ "X-MagicHandy-Control-Generation": "4", "X-MagicHandy-Control-Epoch": "repair-process", "X-MagicHandy-Stop-Sequence": "8" }),
    }));
    expect(events.map(event => event.event)).toEqual(["status", "message", "done"]);
  });

  it("returns stale failure reasons instead of retrying a new user request", async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: false, status: 409, json: async () => ({ error: "This repair is no longer available." }) });
    vi.stubGlobal("fetch", fetchMock);
    await expect(streamChatRepair("chat-test", 12, vi.fn())).rejects.toThrow("This repair is no longer available.");
    expect(fetchMock).toHaveBeenCalledOnce();
  });
});
