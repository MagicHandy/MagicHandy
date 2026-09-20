import { afterEach, expect, it, vi } from "vitest";
import { api } from "./client";

afterEach(() => vi.unstubAllGlobals());

it("sends an explicit permanent flag and preserves the timed API contract", async () => {
  const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 200, text: async () => '{"grant":null}' });
  vi.stubGlobal("fetch", fetchMock);
  await api.grantControl("operator", "permanent");
  await api.grantControl("operator", 60);
  expect(fetchMock).toHaveBeenNthCalledWith(1, "/api/accounts/operator/control-grant", expect.objectContaining({ method: "PUT", body: '{"permanent":true}' }));
  expect(fetchMock).toHaveBeenNthCalledWith(2, "/api/accounts/operator/control-grant", expect.objectContaining({ method: "PUT", body: '{"duration_minutes":60}' }));
});
