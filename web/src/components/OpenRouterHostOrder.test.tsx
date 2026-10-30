import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { CloudPlanningStatus } from "../api/cloud-types";
import { ModelConnectionEditor } from "./ModelConnectionEditor";
import { newModelConnection } from "../util/model-connections";

const status: CloudPlanningStatus = {
  connection: { profiles: [], active: "", pending: false, state: "disconnected", generation: 1, decisions_key_set: false },
  models: [], readiness: { provider: "", model: "", ready: false, state: "untested" }, motion_planner: { provider: "conversation", model: "" },
};

describe("OpenRouter host order", () => {
  afterEach(() => vi.restoreAllMocks());

  it("starts new connections on the fastest hosts and saves a changed order", async () => {
    vi.spyOn(api, "cloudPlanningStatus").mockResolvedValue(status);
    vi.spyOn(api, "modelConnectionModels").mockResolvedValue({ models: [] });
    const connection = newModelConnection("openrouter", []);
    expect(connection.provider_sort).toBe("throughput");
    expect(newModelConnection("chatgpt", []).provider_sort).toBeUndefined();
    expect(newModelConnection("compatible", []).provider_sort).toBeUndefined();
    const patch = vi.fn();
    render(<ModelConnectionEditor connection={connection} locked={false} patch={patch} full />);
    const order = await screen.findByLabelText("Host order");
    expect(order).toHaveValue("throughput");
    fireEvent.change(order, { target: { value: "" } });
    expect(patch).toHaveBeenCalledWith({ provider_sort: "" });
  });

  it("shows a connection saved before the setting as OpenRouter balanced", async () => {
    vi.spyOn(api, "cloudPlanningStatus").mockResolvedValue(status);
    vi.spyOn(api, "modelConnectionModels").mockResolvedValue({ models: [] });
    const { provider_sort: _, ...saved } = newModelConnection("openrouter", []);
    render(<ModelConnectionEditor connection={saved} locked={false} patch={vi.fn()} full />);
    expect(await screen.findByLabelText("Host order")).toHaveValue("");
  });
});
