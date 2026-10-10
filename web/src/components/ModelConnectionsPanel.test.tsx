import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
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
  const selected = initial.connections?.find(item => item.id === initial.conversation_connection_id);
  const route = selected ? { kind: "hosted" as const, provider: selected.provider, model: selected.model, context_policy: "conversation" as const, state: "configured" as const } : { kind: "local" as const, provider: "ollama", model: "local-model", context_policy: "conversation" as const, state: "configured" as const };
  const plannerConnection = initial.connections?.find(item => item.id === initial.motion_planner?.connection_id);
  const routing = { chat: route, autopilot: plannerConnection ? { kind: "hosted" as const, provider: plannerConnection.provider, model: plannerConnection.model, context_policy: initial.motion_planner?.context_policy ?? "conversation", state: "configured" as const } : route };
  return <ModelConnectionsPanel routing={routing} settings={settings} saved={initial} locked={false} patch={change => setSettings(current => ({ ...current, ...change }))} />;
}

describe("saved model connection editing", () => {
  afterEach(async () => { await act(async () => {}); vi.restoreAllMocks(); });
  it("keeps local retry opt-in and includes the toggle in model draft discard", async () => {
    vi.spyOn(api, "cloudPlanningStatus").mockResolvedValue(status);
    vi.spyOn(api, "modelConnectionModels").mockResolvedValue({ models: [] });
    render(<Panel initial={{ connections: [chatgpt], conversation_connection_id: chatgpt.id, provider: "ollama", model: "local-model" } as PublicSettings["llm"]} />);
    const retry = screen.getByRole("checkbox", { name: "Retry declined chat requests with the local model" });
    expect(retry).not.toBeChecked();
    fireEvent.click(retry);
    expect(retry).toBeChecked();
    expect(screen.getByText("Local retry model: Ollama · local-model")).toBeVisible();
    expect(screen.getByRole("combobox", { name: "Chat model" })).toHaveValue(chatgpt.id);
    expect(screen.getByRole("combobox", { name: "Autopilot model" })).toHaveValue("conversation");
    fireEvent.click(screen.getByRole("button", { name: "Discard model changes" }));
    expect(retry).not.toBeChecked();
  });
  it("offers OpenRouter without a saved connection and separates its editor from ChatGPT", async () => {
    vi.spyOn(api, "cloudPlanningStatus").mockResolvedValue(status);
    vi.spyOn(api, "modelConnectionModels").mockResolvedValue({ models: [] });
    vi.spyOn(api, "modelConnectionKeyStatus").mockResolvedValue({ key_set: false });
    render(<Panel initial={{ connections: [chatgpt], conversation_connection_id: chatgpt.id } as PublicSettings["llm"]} />);
    const choice = screen.getByRole("combobox", { name: "Chat model" });
    expect(within(choice).getByRole("option", { name: "OpenRouter" })).toBeEnabled();
    fireEvent.change(choice, { target: { value: "new:openrouter" } });
    const editor = screen.getByRole("region", { name: "Chat model settings" });
    expect(within(editor).getByLabelText("API key")).toBeVisible();
    expect(within(editor).queryByText("ChatGPT connected")).not.toBeInTheDocument();
    expect(within(editor).queryByText("Manage account")).not.toBeInTheDocument();
    expect(screen.getByText(/Unsaved model changes/)).toBeVisible();
    const saved = screen.getByRole("region", { name: "Saved model routing" });
    expect(within(saved).getByText(/Chat messages go to ChatGPT/)).toBeVisible();
    expect(within(saved).queryByText(/OpenRouter/)).not.toBeInTheDocument();
    fireEvent.change(choice, { target: { value: chatgpt.id } });
    expect(screen.getByText(/Unsaved model changes/)).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "Discard model changes" }));
    expect(within(choice).getByRole("option", { name: "OpenRouter" })).toBeEnabled();
    expect(screen.queryByText(/Unsaved model changes/)).not.toBeInTheDocument();
  });

  it("keeps provider identity visible despite a misleading connection name", () => {
    vi.spyOn(api, "cloudPlanningStatus").mockResolvedValue(status);
    const router = { ...newModelConnection("openrouter", []), name: "ChatGPT", model: "vendor/model" };
    render(<Panel initial={{ connections: [router], conversation_connection_id: router.id } as PublicSettings["llm"]} />);
    expect(within(screen.getByRole("combobox", { name: "Chat model" })).getByRole("option", { name: "OpenRouter · ChatGPT · vendor/model" })).toBeVisible();
  });

  it("discards model edits without losing persona drafts from another settings page", async () => {
    vi.spyOn(api, "cloudPlanningStatus").mockResolvedValue(status);
    vi.spyOn(api, "modelConnectionModels").mockResolvedValue({ models: [] });
    vi.spyOn(api, "modelConnectionKeyStatus").mockResolvedValue({ key_set: false });
    const saved = { connections: [chatgpt], conversation_connection_id: chatgpt.id, persona_description: "Saved persona" } as PublicSettings["llm"];
    function DraftPanel() {
      const [draft, setDraft] = useState({ ...saved, persona_description: "Unsaved persona" });
      return <><output aria-label="Persona draft">{draft.persona_description}</output><ModelConnectionsPanel settings={draft} saved={saved} locked={false} patch={change => setDraft(current => ({ ...current, ...change }))} /></>;
    }
    render(<DraftPanel />);
    fireEvent.change(screen.getByRole("combobox", { name: "Chat model" }), { target: { value: "new:openrouter" } });
    fireEvent.click(screen.getByRole("button", { name: "Discard model changes" }));
    expect(screen.getByLabelText("Persona draft")).toHaveTextContent("Unsaved persona");
    expect(screen.getByRole("combobox", { name: "Chat model" })).toHaveValue(chatgpt.id);
    expect(screen.queryByText(/Unsaved model changes/)).not.toBeInTheDocument();
  });

  it("shows a missing saved destination as unavailable without implying local routing", () => {
    const settings = { connections: [], conversation_connection_id: "missing" } as unknown as PublicSettings["llm"];
    const route = { kind: "unavailable" as const, provider: "unavailable", model: "", state: "unavailable" as const, context_policy: "conversation" as const };
    render(<ModelConnectionsPanel settings={settings} saved={settings} routing={{ chat: route, autopilot: route }} locked={false} patch={vi.fn()} />);
    expect(screen.getByRole("combobox", { name: "Chat model" })).toHaveValue("missing");
    expect(within(screen.getByRole("combobox", { name: "Chat model" })).getByRole("option", { name: "Unavailable connection" })).toBeDisabled();
    expect(screen.getByText("Chat cannot use this connection. Choose another model and save settings.")).toBeVisible();
    expect(screen.queryByText("Chat messages go to your configured local-model endpoint.")).not.toBeInTheDocument();
    expect(screen.queryByText("llama.cpp")).not.toBeInTheDocument();
  });

  it("changes Autopilot independently and states technical-only sharing", async () => {
    vi.spyOn(api, "cloudPlanningStatus").mockResolvedValue(status);
    vi.spyOn(api, "modelConnectionModels").mockResolvedValue({ models: [] });
    vi.spyOn(api, "modelConnectionKeyStatus").mockResolvedValue({ key_set: false });
    render(<Panel initial={{ connections: [chatgpt], conversation_connection_id: "local", motion_planner: { provider: "connection", connection_id: chatgpt.id, model: "", context_policy: "technical" } } as PublicSettings["llm"]} />);
    expect(screen.getByRole("combobox", { name: "Chat model" })).toHaveValue("local");
    expect(screen.getByText("Autopilot receives motion details only. Chat messages, personas and memories are excluded from its requests.")).toBeVisible();
    fireEvent.change(screen.getByRole("combobox", { name: "Autopilot model" }), { target: { value: "new:openrouter" } });
    expect(screen.getByRole("combobox", { name: "Chat model" })).toHaveValue("local");
    expect(within(screen.getByRole("region", { name: "Autopilot model settings" })).getByLabelText("API key")).toBeVisible();
    expect(screen.getByRole("combobox", { name: "Autopilot context sharing" })).toHaveValue("technical");
  });


  it("adds a named provider without assigning either role", async () => {
    vi.spyOn(api, "cloudPlanningStatus").mockResolvedValue(status);
    vi.spyOn(api, "modelConnectionModels").mockResolvedValue({ models: [] });
    vi.spyOn(api, "modelConnectionKeyStatus").mockResolvedValue({ key_set: false });
    render(<Panel initial={{ connections: [chatgpt], conversation_connection_id: chatgpt.id } as PublicSettings["llm"]} />);
    await screen.findByText("Ready");
    fireEvent.click(screen.getByText("Named connections"));
    fireEvent.click(screen.getByRole("button", { name: "Add OpenRouter" }));
    expect(screen.getByRole("combobox", { name: "Chat model" })).toHaveValue(chatgpt.id);
    expect(screen.getByRole("combobox", { name: "Autopilot model" })).toHaveValue("conversation");
    expect(screen.getByRole("combobox", { name: "Configure named connection" })).toHaveValue("openrouter-1");
    expect(screen.getByLabelText("API key")).toBeVisible();
  });

  it("does not let a late ChatGPT catalog enter the OpenRouter editor", async () => {
    vi.spyOn(api, "cloudPlanningStatus").mockResolvedValue(status);
    vi.spyOn(api, "modelConnectionKeyStatus").mockResolvedValue({ key_set: false });
    let finish!: (result: { models: Array<{ id: string; name: string }> }) => void;
    vi.spyOn(api, "modelConnectionModels").mockImplementation(() => new Promise(resolve => { finish = resolve; }));
    render(<Panel initial={{ connections: [chatgpt], conversation_connection_id: chatgpt.id } as PublicSettings["llm"]} />);
    await waitFor(() => expect(api.modelConnectionModels).toHaveBeenCalled());
    fireEvent.change(screen.getByRole("combobox", { name: "Chat model" }), { target: { value: "new:openrouter" } });
    await act(async () => { finish({ models: [{ id: "private-account-model", name: "Private account model" }] }); });
    expect(screen.queryByRole("option", { name: "Private account model" })).not.toBeInTheDocument();
    expect(screen.getByLabelText("API key")).toBeVisible();
  });

  it("keeps exact readiness across editor changes and invalidates an edited endpoint", async () => {
    vi.spyOn(api, "cloudPlanningStatus").mockResolvedValue(status);
    vi.spyOn(api, "modelConnectionTest");
    vi.spyOn(api, "modelConnectionKeyStatus").mockResolvedValue({ key_set: false });
    render(<Panel initial={{ connections: [chatgpt, compatible], conversation_connection_id: chatgpt.id } as PublicSettings["llm"]} />);
    await screen.findByText("Ready");
    const editor = screen.getByRole("combobox", { name: "Chat model" });
    fireEvent.change(editor, { target: { value: compatible.id } });
    await screen.findByText("Ready");
    fireEvent.change(editor, { target: { value: chatgpt.id } });
    await screen.findByText("ChatGPT connected");
    await waitFor(() => expect(screen.getByText("Ready")).toBeVisible());
    fireEvent.change(editor, { target: { value: compatible.id } });
    fireEvent.change(screen.getByRole("textbox", { name: "API base URL" }), { target: { value: "http://127.0.0.1:9000/v1" } });
    expect(screen.queryByText("Ready")).not.toBeInTheDocument();
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
