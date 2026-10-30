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

describe("model roles and external providers", () => {
  afterEach(async () => { await act(async () => {}); vi.restoreAllMocks(); });
  const quietCatalog = () => {
    vi.spyOn(api, "cloudPlanningStatus").mockResolvedValue(status);
    vi.spyOn(api, "modelConnectionModels").mockResolvedValue({ models: [] });
    vi.spyOn(api, "modelConnectionKeyStatus").mockResolvedValue({ key_set: false });
  };

  it("keeps second opinion opt-in, discloses message sharing, and does not reroute chat", async () => {
    quietCatalog();
    render(<Panel initial={{ connections: [chatgpt], conversation_connection_id: "local", provider: "ollama", model: "local-model" } as PublicSettings["llm"]} />);
    const review = screen.getByRole("combobox", { name: "Second opinion" });
    expect(review).toHaveValue("off");
    fireEvent.change(review, { target: { value: chatgpt.id } });
    expect(review).toHaveValue(chatgpt.id);
    expect(screen.getByRole("combobox", { name: "Chat model" })).toHaveValue("local");
    expect(screen.getByText(/Shares motion details and up to eight recent user messages/)).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "Discard model changes" }));
    expect(review).toHaveValue("off");
  });

  it("turns review off when its provider is removed instead of assigning the local model as its own reviewer", async () => {
    quietCatalog();
    render(<Panel initial={{ connections: [chatgpt], conversation_connection_id: "local", motion_reviewer: { enabled: true, connection_id: chatgpt.id } } as PublicSettings["llm"]} />);
    fireEvent.click(screen.getByRole("button", { name: "Remove · ChatGPT" }));
    const confirm = screen.getByRole("alertdialog", { name: "Remove ChatGPT?" });
    expect(confirm).toHaveTextContent("Second opinion will be switched off.");
    fireEvent.click(within(confirm).getByRole("button", { name: "Remove" }));
    expect(screen.getByRole("combobox", { name: "Second opinion" })).toHaveValue("off");
    expect(screen.getByRole("combobox", { name: "Chat model" })).toHaveValue("local");
  });

  it("reviews a Simple compatible server and leaves Advanced roles unreviewed", async () => {
    quietCatalog();
    render(<Panel initial={{ connections: [chatgpt, compatible], conversation_connection_id: chatgpt.id } as PublicSettings["llm"]} />);
    expect(screen.queryByRole("combobox", { name: "Second opinion" })).not.toBeInTheDocument();
    fireEvent.change(screen.getByRole("combobox", { name: "Chat model" }), { target: { value: compatible.id } });
    expect(screen.getByRole("combobox", { name: "Second opinion" })).toHaveValue("off");
    expect(screen.queryByRole("checkbox", { name: /^Automatically repair/ })).not.toBeInTheDocument();
  });

  it("names each role's capability and lists the local model first in Models", async () => {
    quietCatalog();
    const initial = { connections: [compatible], conversation_connection_id: "local", provider: "ollama", model: "local-model", ollama_base_url: "http://127.0.0.1:11434", local_capability_level: "intermediate" } as PublicSettings["llm"];
    function LocalPanel() {
      const [settings, setSettings] = useState(initial);
      return <ModelConnectionsPanel settings={settings} saved={initial} locked={false} patch={change => setSettings(current => ({ ...current, ...change }))} localEditor={<p>Local editor</p>} />;
    }
    render(<LocalPanel />);
    const models = screen.getByRole("region", { name: "Models" });
    const rows = within(models).getAllByRole("button", { name: /^(Edit|Close) · / });
    expect(rows[0]).toHaveAccessibleName("Close · Local model");
    expect(within(models).getByText("Ollama · 127.0.0.1:11434")).toBeVisible();
    expect(within(models).getByText("Local editor")).toBeVisible();
    expect(within(models).getAllByText("Intermediate")).toHaveLength(1);
    expect(within(models).getByText("Simple")).toBeVisible();
    const roles = screen.getByRole("region", { name: "Model roles" });
    expect(within(roles).getAllByText("Intermediate")).toHaveLength(2);
    fireEvent.click(rows[0]);
    expect(within(models).queryByText("Local editor")).not.toBeInTheDocument();
    fireEvent.change(screen.getByRole("combobox", { name: "Chat model" }), { target: { value: compatible.id } });
    expect(within(roles).getAllByText("Simple")).toHaveLength(2);
    expect(screen.getByText(/^Not saved yet\./)).toBeVisible();
  });

  it("keeps local retry opt-in and includes the toggle in model draft discard", async () => {
    quietCatalog();
    render(<Panel initial={{ connections: [chatgpt], conversation_connection_id: chatgpt.id, provider: "ollama", model: "local-model" } as PublicSettings["llm"]} />);
    const retry = screen.getByRole("checkbox", { name: /^Retry declined chat requests with the local model/ });
    expect(retry).not.toBeChecked();
    fireEvent.click(retry);
    expect(retry).toBeChecked();
    expect(screen.getByText("Local retry model: Ollama · local-model")).toBeVisible();
    expect(screen.getByRole("combobox", { name: "Chat model" })).toHaveValue(chatgpt.id);
    expect(screen.getByRole("combobox", { name: "Autopilot model" })).toHaveValue("conversation");
    fireEvent.click(screen.getByRole("button", { name: "Discard model changes" }));
    expect(retry).not.toBeChecked();
  });

  it.each(["advanced", "advanced_no_reasoning"] as const)("keeps repair available and second opinion hidden for %s connections", async level => {
    quietCatalog();
    const connection = { ...chatgpt, capability_level: level };
    render(<Panel initial={{ connections: [connection], conversation_connection_id: connection.id, provider: "ollama", model: "local-model" } as PublicSettings["llm"]} />);
    expect(screen.getByRole("checkbox", { name: /^Automatically repair/ })).toBeChecked();
    expect(screen.queryByRole("combobox", { name: "Second opinion" })).not.toBeInTheDocument();
    fireEvent.change(screen.getByRole("combobox", { name: "Autopilot model" }), { target: { value: "local" } });
    expect(screen.getByRole("combobox", { name: "Second opinion" })).toHaveValue("off");
    expect(screen.getByRole("checkbox", { name: /^Automatically repair/ })).toBeChecked();
  });

  it("defaults hosted response repair on, preserves routing, and restores it when model drafts are discarded", async () => {
    quietCatalog();
    render(<Panel initial={{ connections: [chatgpt], conversation_connection_id: chatgpt.id, provider: "ollama", model: "local-model" } as PublicSettings["llm"]} />);
    const toggle = screen.getByRole("checkbox", { name: /^Automatically repair rejected chat responses/ });
    expect(toggle).toBeChecked();
    fireEvent.click(toggle);
    expect(toggle).not.toBeChecked();
    expect(screen.getByRole("combobox", { name: "Chat model" })).toHaveValue(chatgpt.id);
    expect(screen.getByRole("checkbox", { name: /^Retry declined chat requests with the local model/ })).not.toBeChecked();
    fireEvent.click(screen.getByRole("button", { name: "Discard model changes" }));
    expect(toggle).toBeChecked();
    expect(screen.queryByRole("button", { name: "Discard model changes" })).not.toBeInTheDocument();
  });

  it("offers repair for Advanced local models while preserving local routing and read-only settings", async () => {
    quietCatalog();
    const local = { connections: [], conversation_connection_id: "local" } as unknown as PublicSettings["llm"];
    const view = render(<ModelConnectionsPanel settings={local} saved={local} locked={false} patch={vi.fn()} />);
    expect(screen.queryByRole("checkbox", { name: /^Automatically repair/ })).not.toBeInTheDocument();
    const advanced = { ...local, local_capability_level: "advanced" as const };
    view.rerender(<ModelConnectionsPanel settings={advanced} saved={advanced} locked={false} patch={vi.fn()} />);
    expect(screen.getByRole("checkbox", { name: /^Automatically repair/ })).toBeChecked();
    expect(screen.getByRole("combobox", { name: "Chat model" })).toHaveValue("local");
    const external = { ...local, connections: [chatgpt], conversation_connection_id: chatgpt.id, hosted_response_repair_enabled: false };
    view.rerender(<ModelConnectionsPanel settings={external} saved={external} locked patch={vi.fn()} />);
    const toggle = screen.getByRole("checkbox", { name: /^Automatically repair rejected chat responses/ });
    expect(toggle).not.toBeChecked();
    expect(toggle).toBeDisabled();
  });

  it("never creates a provider from a role choice; adding one leaves both roles alone", async () => {
    quietCatalog();
    render(<Panel initial={{ connections: [chatgpt], conversation_connection_id: chatgpt.id } as PublicSettings["llm"]} />);
    const chat = screen.getByRole("combobox", { name: "Chat model" });
    expect(within(chat).queryByRole("option", { name: "OpenRouter" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Add provider" }));
    fireEvent.click(within(screen.getByRole("group", { name: "Add provider" })).getByRole("button", { name: /^OpenRouter/ }));
    expect(chat).toHaveValue(chatgpt.id);
    expect(screen.getByRole("combobox", { name: "Autopilot model" })).toHaveValue("conversation");
    const providers = screen.getByRole("region", { name: "External providers" });
    expect(within(providers).getByLabelText("API key")).toBeVisible();
    expect(within(providers).queryByText("ChatGPT connected")).not.toBeInTheDocument();
    expect(within(chat).getByRole("option", { name: "OpenRouter · Choose a model" })).toBeInTheDocument();
    expect(screen.getByText(/Unsaved model changes/)).toBeVisible();
    // The saved routing text stays with the saved destination until a save.
    expect(screen.getByText(/Chat messages go to ChatGPT/)).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "Discard model changes" }));
    expect(within(chat).queryByRole("option", { name: "OpenRouter · Choose a model" })).not.toBeInTheDocument();
    expect(screen.queryByText(/Unsaved model changes/)).not.toBeInTheDocument();
  });

  it("keeps provider identity visible despite a misleading connection name", () => {
    vi.spyOn(api, "cloudPlanningStatus").mockResolvedValue(status);
    const router = { ...newModelConnection("openrouter", []), name: "ChatGPT", model: "vendor/model" };
    render(<Panel initial={{ connections: [router], conversation_connection_id: router.id } as PublicSettings["llm"]} />);
    expect(within(screen.getByRole("combobox", { name: "Chat model" })).getByRole("option", { name: "OpenRouter · ChatGPT · vendor/model" })).toBeVisible();
    expect(within(screen.getByRole("region", { name: "External providers" })).getByText("OpenRouter")).toBeVisible();
  });

  it("discards model edits without losing persona drafts from another settings page", async () => {
    quietCatalog();
    const saved = { connections: [chatgpt], conversation_connection_id: chatgpt.id, persona_description: "Saved persona" } as PublicSettings["llm"];
    function DraftPanel() {
      const [draft, setDraft] = useState({ ...saved, persona_description: "Unsaved persona" });
      return <><output aria-label="Persona draft">{draft.persona_description}</output><ModelConnectionsPanel settings={draft} saved={saved} locked={false} patch={change => setDraft(current => ({ ...current, ...change }))} /></>;
    }
    render(<DraftPanel />);
    fireEvent.change(screen.getByRole("combobox", { name: "Chat model" }), { target: { value: "local" } });
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
    quietCatalog();
    render(<Panel initial={{ connections: [chatgpt, compatible], conversation_connection_id: "local", motion_planner: { provider: "connection", connection_id: chatgpt.id, model: "", context_policy: "technical" } } as PublicSettings["llm"]} />);
    expect(screen.getByRole("combobox", { name: "Chat model" })).toHaveValue("local");
    expect(screen.getByText("Autopilot receives motion details only. Chat messages, personas and memories are excluded from its requests.")).toBeVisible();
    fireEvent.change(screen.getByRole("combobox", { name: "Autopilot model" }), { target: { value: compatible.id } });
    expect(screen.getByRole("combobox", { name: "Chat model" })).toHaveValue("local");
    expect(screen.getByRole("combobox", { name: "Autopilot context sharing" })).toHaveValue("technical");
  });

  it("removes a provider in use after an inline confirmation and hands its roles to the local model", async () => {
    quietCatalog();
    render(<Panel initial={{ connections: [chatgpt, compatible], conversation_connection_id: compatible.id } as PublicSettings["llm"]} />);
    const providers = screen.getByRole("region", { name: "External providers" });
    fireEvent.click(within(providers).getByRole("button", { name: "Remove · Compatible API" }));
    const confirm = screen.getByRole("alertdialog", { name: "Remove Compatible API?" });
    expect(confirm).toHaveTextContent("Chat · Autopilot will switch to the local model.");
    fireEvent.click(within(confirm).getByRole("button", { name: "Cancel" }));
    expect(within(providers).getByRole("button", { name: "Remove · Compatible API" })).toBeVisible();
    fireEvent.click(within(providers).getByRole("button", { name: "Remove · Compatible API" }));
    fireEvent.click(within(screen.getByRole("alertdialog")).getByRole("button", { name: "Remove" }));
    expect(within(providers).queryByText("Compatible API")).not.toBeInTheDocument();
    const chat = screen.getByRole("combobox", { name: "Chat model" });
    expect(chat).toHaveValue("local");
    expect(within(chat).queryByRole("option", { name: /Other compatible provider/ })).not.toBeInTheDocument();
    expect(screen.getByText(/Unsaved model changes/)).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "Discard model changes" }));
    expect(within(providers).getByText("Compatible API")).toBeVisible();
    expect(chat).toHaveValue(compatible.id);
  });

  it("does not let a late ChatGPT catalog enter the OpenRouter editor", async () => {
    vi.spyOn(api, "cloudPlanningStatus").mockResolvedValue(status);
    vi.spyOn(api, "modelConnectionKeyStatus").mockResolvedValue({ key_set: false });
    let finish!: (result: { models: Array<{ id: string; name: string }> }) => void;
    vi.spyOn(api, "modelConnectionModels").mockImplementation(() => new Promise(resolve => { finish = resolve; }));
    const router = newModelConnection("openrouter", [chatgpt]);
    render(<Panel initial={{ connections: [chatgpt, router], conversation_connection_id: chatgpt.id } as PublicSettings["llm"]} />);
    fireEvent.click(screen.getByRole("button", { name: "Edit · ChatGPT" }));
    await waitFor(() => expect(api.modelConnectionModels).toHaveBeenCalled());
    fireEvent.click(screen.getByRole("button", { name: "Edit · OpenRouter" }));
    await act(async () => { finish({ models: [{ id: "private-account-model", name: "Private account model" }] }); });
    expect(screen.queryByRole("option", { name: "Private account model" })).not.toBeInTheDocument();
    expect(screen.getByLabelText("API key")).toBeVisible();
  });

  it("shows exact readiness and drops it once the endpoint is edited", async () => {
    vi.spyOn(api, "cloudPlanningStatus").mockResolvedValue(status);
    vi.spyOn(api, "modelConnectionTest");
    vi.spyOn(api, "modelConnectionModels").mockResolvedValue({ models: [] });
    vi.spyOn(api, "modelConnectionKeyStatus").mockResolvedValue({ key_set: false });
    render(<Panel initial={{ connections: [chatgpt, compatible], conversation_connection_id: chatgpt.id } as PublicSettings["llm"]} />);
    const providers = screen.getByRole("region", { name: "External providers" });
    await within(providers).findByText("same-model · Ready");
    fireEvent.click(within(providers).getByRole("button", { name: "Edit · Compatible API" }));
    fireEvent.change(screen.getByRole("textbox", { name: "API base URL" }), { target: { value: "http://127.0.0.1:9000/v1" } });
    expect(within(providers).queryByText("same-model · Ready")).not.toBeInTheDocument();
    expect(within(providers).getByText("same-model · Not checked")).toBeVisible();
    expect(api.modelConnectionTest).not.toHaveBeenCalled();
  });

  it("renders a saved local Autopilot assignment honestly", async () => {
    vi.spyOn(api, "cloudPlanningStatus").mockResolvedValue(status);
    render(<Panel initial={{ connections: [chatgpt], conversation_connection_id: chatgpt.id, motion_planner: { provider: "local", model: "local-model", context_policy: "technical" } } as PublicSettings["llm"]} />);
    expect(screen.getByRole("combobox", { name: "Autopilot model" })).toHaveValue("local");
    expect(screen.queryByRole("combobox", { name: "Autopilot context sharing" })).not.toBeInTheDocument();
    await act(async () => {});
  });
});
