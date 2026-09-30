import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { LLMModelManagerSnapshot } from "../api/types";
import { RuntimeUpdateSettings } from "./RuntimeUpdateSettings";

const outdated = {
  models: [],
  imports: [],
  store_path: "C:\\MagicHandy\\models",
  suggested_ollama_path: "",
  runtime: {
    state: "outdated", installed: true, current: false, build_supported: true,
    supported_backends: ["auto", "cpu", "cuda"], expected_version: "b11149", version: "b9966",
    backend: "cuda", message: "Managed llama.cpp b9966 is installed; b11149 is the current pinned build.",
  },
} as LLMModelManagerSnapshot;

describe("RuntimeUpdateSettings", () => {
  beforeEach(() => {
    vi.spyOn(api, "llmModels").mockResolvedValue(outdated);
    vi.spyOn(api, "buildManagedLlamaRuntime").mockResolvedValue({
      build: { id: "build", backend: "cuda", status: "queued", message: "queued", started_at: "", updated_at: "" },
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("offers an update with the backend already in use", async () => {
    render(<RuntimeUpdateSettings automatic={false} preferenceDisabled={false} actionDisabled={false} onAutomaticChange={vi.fn()} />);
    expect(await screen.findByText("b9966 is installed; this release uses b11149.")).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "Update now" }));
    await waitFor(() => expect(api.buildManagedLlamaRuntime).toHaveBeenCalledWith("cuda"));
  });

  it("keeps automatic updates a saved user choice", async () => {
    const change = vi.fn();
    render(<RuntimeUpdateSettings automatic preferenceDisabled={false} actionDisabled={false} onAutomaticChange={change} />);
    const toggle = await screen.findByRole("checkbox", { name: "Update the llama.cpp runtime automatically" });
    expect(toggle).toBeChecked();
    fireEvent.click(toggle);
    expect(change).toHaveBeenCalledWith(false);
  });
});
