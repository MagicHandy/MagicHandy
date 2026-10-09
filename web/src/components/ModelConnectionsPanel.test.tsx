import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { CloudPlanningStatus } from "../api/cloud-types";
import type { PublicSettings } from "../api/types";
import { newModelConnection } from "../util/model-connections";
import { ModelConnectionsPanel } from "./ModelConnectionsPanel";

const chatgpt = { ...newModelConnection("chatgpt", []), model: "hosted-model" };
const compatible = { ...newModelConnection("compatible", []), model: "same-model", no_authentication: true };
const status: CloudPlanningStatus = {
  connection: { active: "test", profiles: [{ id: "test", label: "Test account", connected: true, plan_authorized: true, welcome_pending: false }], pending: false, state: "connected", generation: 4, decisions_key_set: false },
  models: [], readiness: { provider: "", model: "", ready: false, state: "untested" }, motion_planner: { provider: "conversation", model: "" },
  connection_readiness: Object.fromEntries([chatgpt, compatible].map(connection => [connection.id, { provider: connection.provider, model: connection.model, ready: true, state: "ready", connection }])),
};

function Panel({ initial }: { initial: PublicSettings["llm"] }) {
  const [settings, setSettings] = useState(initial);
  return <ModelConnectionsPanel settings={settings} locked={false} patch={change => setSettings(current => ({ ...current, ...change }))} />;
}

describe("saved model connection editing", () => {
  afterEach(() => vi.restoreAllMocks());

  it("keeps exact readiness across editor changes and invalidates an edited endpoint", async () => {
    vi.spyOn(api, "cloudPlanningStatus").mockResolvedValue(status);
    vi.spyOn(api, "modelConnectionTest");
    vi.spyOn(api, "modelConnectionKeyStatus").mockResolvedValue({ key_set: false });
    render(<Panel initial={{ connections: [chatgpt, compatible], conversation_connection_id: chatgpt.id } as PublicSettings["llm"]} />);
    await screen.findByText("Ready");
    const editor = screen.getByRole("combobox", { name: "Edit connection" });
    fireEvent.change(editor, { target: { value: compatible.id } });
    expect(screen.getByText("Ready")).toBeVisible();
    fireEvent.change(editor, { target: { value: chatgpt.id } });
    await screen.findByText("ChatGPT connected");
    await waitFor(() => expect(screen.getByText("Ready")).toBeVisible());
    fireEvent.change(editor, { target: { value: compatible.id } });
    fireEvent.change(screen.getByRole("textbox", { name: "API base URL" }), { target: { value: "http://127.0.0.1:9000/v1" } });
    expect(screen.queryByText("Ready")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Use for chat and motion" })).toBeDisabled();
    expect(api.modelConnectionTest).not.toHaveBeenCalled();
  });

  it("renders a saved local Autopilot assignment honestly", async () => {
    vi.spyOn(api, "cloudPlanningStatus").mockResolvedValue(status);
    render(<Panel initial={{ connections: [chatgpt], conversation_connection_id: chatgpt.id, motion_planner: { provider: "local", model: "local-model", context_policy: "technical" } } as PublicSettings["llm"]} />);
    await screen.findByText("ChatGPT connected");
    expect(screen.getByRole("combobox", { name: "Autopilot model" })).toHaveValue("local");
    expect(screen.queryByRole("combobox", { name: "Autopilot context sharing" })).not.toBeInTheDocument();
  });
});
