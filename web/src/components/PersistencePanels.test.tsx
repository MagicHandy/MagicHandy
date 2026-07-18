import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { MemoryState, PromptSetsPayload } from "../api/types";
import { MemoryManager } from "./MemoryManager";
import { PromptSetEditor } from "./PromptSetEditor";

const app = vi.hoisted(() => ({ show: vi.fn() }));

vi.mock("../api/client", () => ({
  api: {
    getMemory: vi.fn(),
    addMemory: vi.fn(),
    setMemoryEnabled: vi.fn(),
    setMemoryItemEnabled: vi.fn(),
    removeMemory: vi.fn(),
    clearMemory: vi.fn(),
    getPromptSets: vi.fn(),
    createPromptSet: vi.fn(),
    updatePromptSet: vi.fn(),
    deletePromptSet: vi.fn(),
  },
}));

vi.mock("../state/app-state", () => ({
  useAppState: () => ({ backendOnline: true }),
  useToast: () => ({ show: app.show }),
}));

const getMemory = vi.mocked(api.getMemory);
const addMemory = vi.mocked(api.addMemory);
const getPromptSets = vi.mocked(api.getPromptSets);
const createPromptSet = vi.mocked(api.createPromptSet);

const emptyMemory: MemoryState = { enabled: true, memories: [] };
const promptSets: PromptSetsPayload = {
  sets: [{ id: "builtin", name: "Default", system: "Be helpful.", builtin: true }],
};

describe("persistence panels", () => {
  beforeEach(() => {
    app.show.mockReset();
    for (const mock of [
      api.getMemory,
      api.addMemory,
      api.setMemoryEnabled,
      api.setMemoryItemEnabled,
      api.removeMemory,
      api.clearMemory,
      api.getPromptSets,
      api.createPromptSet,
      api.updatePromptSet,
      api.deletePromptSet,
    ]) vi.mocked(mock).mockReset();
  });

  it("keeps a memory load failure distinct from a valid empty memory list", async () => {
    getMemory
      .mockRejectedValueOnce(new Error("memory database unavailable"))
      .mockResolvedValueOnce(emptyMemory);

    render(<MemoryManager />);

    expect(await screen.findByRole("alert")).toHaveTextContent("memory database unavailable");
    expect(screen.queryByText("No memories saved.")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));

    expect(await screen.findByText("No memories saved.")).toBeInTheDocument();
  });

  it("serializes rapid memory additions", async () => {
    let release!: (value: MemoryState) => void;
    getMemory.mockResolvedValue(emptyMemory);
    addMemory.mockImplementation(() => new Promise((resolve) => { release = resolve; }));
    render(<MemoryManager />);
    await screen.findByText("No memories saved.");
    fireEvent.change(screen.getByRole("textbox", { name: "New memory" }), { target: { value: "Remember this" } });
    const add = screen.getByRole("button", { name: "Add memory" });

    act(() => {
      add.click();
      add.click();
    });

    expect(addMemory).toHaveBeenCalledOnce();
    await act(async () => release(emptyMemory));
  });

  it("keeps prompt-set load failures retryable and prevents actions before a catalog exists", async () => {
    getPromptSets
      .mockRejectedValueOnce(new Error("prompt catalog unavailable"))
      .mockResolvedValueOnce(promptSets);

    render(<PromptSetEditor />);

    expect(await screen.findByRole("alert")).toHaveTextContent("prompt catalog unavailable");
    expect(screen.queryByRole("button", { name: "Duplicate as new" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));

    expect(await screen.findByRole("combobox", { name: "Edit set" })).toHaveValue("builtin");
  });

  it("serializes rapid prompt-set duplication requests", async () => {
    let release!: (value: PromptSetsPayload) => void;
    getPromptSets.mockResolvedValue(promptSets);
    createPromptSet.mockImplementation(() => new Promise((resolve) => { release = resolve; }));
    render(<PromptSetEditor />);
    const duplicate = await screen.findByRole("button", { name: "Duplicate as new" });

    act(() => {
      duplicate.click();
      duplicate.click();
    });

    expect(createPromptSet).toHaveBeenCalledOnce();
    await act(async () => release(promptSets));
    await waitFor(() => expect(duplicate).toBeEnabled());
  });
});
