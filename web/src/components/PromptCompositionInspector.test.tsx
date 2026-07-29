import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { PromptCompositionInspector } from "./PromptCompositionInspector";

const app = vi.hoisted(() => ({
  show: vi.fn(),
}));

vi.mock("../api/client", () => ({
  api: {
    promptComposition: vi.fn(),
  },
}));

vi.mock("../state/app-state", () => ({
  useAppState: () => ({ backendOnline: true }),
  useToast: () => ({ show: app.show }),
}));

const promptComposition = vi.mocked(api.promptComposition);

describe("PromptCompositionInspector", () => {
  beforeEach(() => {
    app.show.mockReset();
    promptComposition.mockReset();
    promptComposition.mockResolvedValue({
      session_id: "chat-test",
      provider: "llama_cpp",
      model: "gemma-3",
      prompt_set: "persona-behavior",
      persona_id: "persona-rowan",
      persona_name: "Rowan",
      lore_mode: "relevant",
      lore: {
        characters: 24,
        entry_ids: ["lore-a"],
      },
      composition: {
        prompt: "Behavior.\n\nExact contract.",
        characters: 27,
        bytes: 27,
        sections: [
          { id: "behavior", title: "Behavior profile", text: "Behavior.", characters: 9, bytes: 9 },
          { id: "response_contract", title: "Response contract", text: "Exact contract.", characters: 15, bytes: 15 },
        ],
      },
    });
  });

  it("renders backend sections and copies the backend-exact prompt", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal("navigator", { ...navigator, clipboard: { writeText } });

    render(<PromptCompositionInspector />);

    expect(await screen.findByText("gemma-3")).toBeInTheDocument();
    expect(screen.getByText("Rowan")).toBeInTheDocument();
    expect(screen.getByText("Behavior profile")).toBeInTheDocument();
    expect(screen.getByText("Response contract")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Copy exact prompt" }));
    await waitFor(() => expect(writeText).toHaveBeenCalledWith("Behavior.\n\nExact contract."));
    expect(app.show).toHaveBeenCalledWith("Exact model prompt copied.");
  });

  it("keeps a failed composition visible and allows retry", async () => {
    promptComposition
      .mockRejectedValueOnce(new Error("prompt database unavailable"))
      .mockResolvedValueOnce({
        session_id: "chat-test",
        provider: "llama_cpp",
        model: "",
        prompt_set: "default",
        persona_id: "",
        persona_name: "",
        lore: { characters: 0, entry_ids: [] },
        composition: {
          prompt: "Recovered.",
          characters: 10,
          bytes: 10,
          sections: [{ id: "behavior", title: "Behavior profile", text: "Recovered.", characters: 10, bytes: 10 }],
        },
      });

    render(<PromptCompositionInspector />);

    expect(await screen.findByRole("alert")).toHaveTextContent("prompt database unavailable");
    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
    expect(await screen.findByText("Not selected")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
