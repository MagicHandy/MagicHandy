import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { newModelConnection, connectionSignature } from "../util/model-connections";
import { ModelContextSettings } from "./ModelContextSettings";
import { selectedModelChange } from "./ModelResponseSettings";

describe("provider conversation history", () => {
  it("keeps existing connections recent and saves larger context only to that connection", () => {
    const connection = { ...newModelConnection("chatgpt", []), model: "model" };
    const patch = vi.fn();
    render(<ModelContextSettings connection={connection} models={[]} disabled={false} patch={patch} />);
    const select = screen.getByRole("combobox", { name: "Context" });
    expect(select).toHaveValue("65536");
    expect(screen.getByRole("combobox", { name: "Capability" })).toHaveValue("advanced");
    fireEvent.change(select, { target: { value: "131072" } });
    expect(patch).toHaveBeenCalledWith({ context_window: 131072 });
    expect(connectionSignature(connection)).not.toBe(connectionSignature({ ...connection, context_window: 131072 }));
  });
  it("uses advertised capacity for new model recommendations without assuming manual models support it", () => {
    const connection = newModelConnection("chatgpt", []);
    expect(selectedModelChange({ id: "large", name: "Large", context_length: 272000 }, "large", connection).context_window).toBe(131072);
    expect(selectedModelChange(undefined, "manual", connection).context_window).toBe(65536);
    expect(selectedModelChange({ id: "local-api", name: "Local API", context_length: 262144 }, "local-api", { ...connection, provider: "compatible" }).context_window).toBe(65536);
    render(<ModelContextSettings connection={{ ...connection, model: "small" }} models={[{ id: "small", name: "Small", context_length: 64000 }]} disabled={false} patch={vi.fn()} />);
    // Budgets beyond the model's advertised capacity are not offered.
    expect(Array.from(screen.getByRole("combobox", { name: "Context" }).querySelectorAll("option")).map(option => option.textContent)).toEqual(["64K"]);
  });
  it("keeps a large external request budget when choosing shorter history", () => {
    const connection = { ...newModelConnection("chatgpt", []), model: "model", context_window: 131072 };
    const patch = vi.fn();
    render(<ModelContextSettings connection={connection} models={[]} disabled={false} patch={patch} />);
    fireEvent.click(screen.getByText("Conversation history and usage"));
    fireEvent.change(screen.getByRole("combobox", { name: "History sent" }), { target: { value: "recent" } });
    expect(patch).toHaveBeenCalledWith({ history_size: "recent" });
    expect(screen.getByRole("combobox", { name: "Context" })).toHaveValue("131072");
    expect(screen.getByRole("option", { name: /Extended —/ })).toBeDisabled();
    expect(connectionSignature(connection)).not.toBe(connectionSignature({ ...connection, history_size: "recent" }));
  });
  it("locks both external settings for a read-only client", () => {
    render(<ModelContextSettings connection={newModelConnection("chatgpt", [])} models={[]} disabled patch={vi.fn()} />);
    fireEvent.click(screen.getByText("Conversation history and usage"));
    for (const select of screen.getAllByRole("combobox")) expect(select).toBeDisabled();
  });
  it("lets a compatible server choose Simple with a 16K budget, and raises the budget with the level", () => {
    const server = { ...newModelConnection("compatible", []), model: "qwen3-14b", no_authentication: true };
    expect(server.capability_level).toBe("simple");
    const patch = vi.fn();
    const view = render(<ModelContextSettings connection={server} models={[]} disabled={false} patch={patch} />);
    expect(screen.getByRole("combobox", { name: "Capability" })).toHaveValue("simple");
    expect(screen.getByRole("combobox", { name: "Context" })).toHaveValue("16384");
    expect(screen.getByText("Set the server's context length to at least 16K. MagicHandy budgets each request to fit.")).toBeInTheDocument();
    fireEvent.change(screen.getByRole("combobox", { name: "Capability" }), { target: { value: "intermediate" } });
    expect(patch).toHaveBeenCalledWith({ capability_level: "intermediate", context_window: 32768 });
    view.rerender(<ModelContextSettings connection={{ ...server, capability_level: "advanced", context_window: 131072 }} models={[]} disabled={false} patch={patch} />);
    fireEvent.change(screen.getByRole("combobox", { name: "Capability" }), { target: { value: "simple" } });
    // A larger saved budget is kept when the level goes down.
    expect(patch).toHaveBeenLastCalledWith({ capability_level: "simple", context_window: 131072 });
    expect(selectedModelChange(undefined, "other-model", { ...server, context_window: 32768 }, 16384).context_window).toBe(32768);
  });
});
