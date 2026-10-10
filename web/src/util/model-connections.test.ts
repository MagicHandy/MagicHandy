import { describe, expect, it } from "vitest";
import type { LLMSettings } from "../api/llm-settings-types";
import { connectionRoles, newModelConnection, removeConnection } from "./model-connections";

const router = { ...newModelConnection("openrouter", []), model: "vendor/model" };
const server = { ...newModelConnection("compatible", []), model: "local-model" };

describe("removing a model connection", () => {
  it("moves chat to the local model and keeps Autopilot following chat", () => {
    const settings = { connections: [router, server], conversation_connection_id: router.id, motion_planner: { provider: "conversation", model: "" } } as LLMSettings;
    expect(connectionRoles(settings, router.id)).toEqual(["chat", "autopilot"]);
    const change = removeConnection(settings, router.id);
    expect(change.connections).toEqual([server]);
    expect(change.conversation_connection_id).toBe("local");
    expect(change.motion_planner).toBeUndefined();
  });

  it("moves an Autopilot assignment to the local model with conversation context", () => {
    const settings = { connections: [router, server], conversation_connection_id: router.id, motion_planner: { provider: "connection", connection_id: server.id, model: "", context_policy: "technical" } } as LLMSettings;
    expect(connectionRoles(settings, server.id)).toEqual(["autopilot"]);
    const change = removeConnection(settings, server.id);
    expect(change.conversation_connection_id).toBeUndefined();
    expect(change.motion_planner).toEqual({ provider: "local", connection_id: "", model: "", context_policy: "conversation" });
  });

  it("leaves roles alone for an unassigned connection", () => {
    const settings = { connections: [router, server], conversation_connection_id: "local" } as LLMSettings;
    expect(connectionRoles(settings, server.id)).toEqual([]);
    expect(removeConnection(settings, server.id)).toEqual({ connections: [router] });
  });
});
