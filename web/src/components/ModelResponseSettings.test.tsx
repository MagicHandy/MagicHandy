import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { connectionSignature, newModelConnection } from "../util/model-connections";
import { ModelResponseSettings, selectedModelChange } from "./ModelResponseSettings";

describe("hosted model speed choices", () => {
  const connection = { ...newModelConnection("chatgpt", []), model: "account-model" };
  const model = { id: "account-model", name: "Account model", reasoning_efforts: ["low", "medium"] };

  it("offers only account-advertised effort levels and invalidates old readiness", () => {
    const patch = vi.fn();
    render(<ModelResponseSettings connection={connection} models={[model]} disabled={false} patch={patch} />);
    const select = screen.getByRole("combobox", { name: "Reasoning effort" });
    expect(Array.from(select.querySelectorAll("option")).map(option => option.value)).toEqual(["", "low", "medium"]);
    fireEvent.change(select, { target: { value: "low" } });
    expect(patch).toHaveBeenCalledWith({ reasoning_effort: "low" });
    expect(connectionSignature(connection)).not.toBe(connectionSignature({ ...connection, reasoning_effort: "low" }));
  });

  it("picks low only when supported and preserves compatible explicit choices", () => {
    expect(selectedModelChange(model, model.id, connection)).toEqual({ model: model.id, reasoning_effort: "low" });
    expect(selectedModelChange(model, model.id, { ...connection, reasoning_effort: "medium" })).toEqual({ model: model.id, reasoning_effort: "medium" });
    expect(selectedModelChange(undefined, "manual", { ...connection, reasoning_effort: "medium" })).toEqual({ model: "manual", reasoning_effort: "" });
  });

  it("does not invent a paid tier or a control for compatible providers", () => {
    render(<ModelResponseSettings connection={{ ...connection, provider: "openrouter" }} models={[model]} disabled={false} patch={vi.fn()} />);
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
    expect(screen.queryByRole("checkbox", { name: /fast/i })).not.toBeInTheDocument();
  });
});
