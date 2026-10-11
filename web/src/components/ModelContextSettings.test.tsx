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
    const select = screen.getByRole("combobox", { name: "Conversation history" });
    expect(select).toHaveValue("32768");
    fireEvent.change(select, { target: { value: "131072" } });
    expect(patch).toHaveBeenCalledWith({ context_window: 131072 });
    expect(connectionSignature(connection)).not.toBe(connectionSignature({ ...connection, context_window: 131072 }));
  });
  it("uses advertised capacity for new model recommendations without assuming manual models support it", () => {
    const connection = newModelConnection("chatgpt", []);
    expect(selectedModelChange({ id: "large", name: "Large", context_length: 272000 }, "large", connection).context_window).toBe(131072);
    expect(selectedModelChange(undefined, "manual", connection).context_window).toBe(32768);
    expect(selectedModelChange({ id: "local-api", name: "Local API", context_length: 262144 }, "local-api", { ...connection, provider: "compatible" }).context_window).toBe(32768);
    render(<ModelContextSettings connection={{ ...connection, model: "small" }} models={[{ id: "small", name: "Small", context_length: 64000 }]} disabled={false} patch={vi.fn()} />);
    expect(screen.getByRole("option", { name: /Long —/ })).toBeDisabled();
    expect(screen.getByRole("option", { name: /Extended —/ })).toBeDisabled();
  });
});
