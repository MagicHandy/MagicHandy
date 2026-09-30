import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { ModelCheckReport } from "../api/model-check-types";
import type { ManagedLLMModel } from "../api/types";
import { ModelCheckDialog } from "./ModelCheckDialog";

const idle: ModelCheckReport = {
  state: "idle", managed: false, load_millis: 0, load_report: { offloaded_layers: 0, total_layers: 0 },
  items: [], turns: [],
  summary: { turns: 0, first_try: 0, repaired: 0, failed: 0, truncated: 0, refused: 0, reasoning_chars: 0, thinking_markup: 0, average_millis: 0, average_first_token_millis: 0, average_words: 0, tokens_per_second: 0 },
};

const finished: ModelCheckReport = {
  ...idle,
  state: "complete",
  model: "gemma-e4b",
  managed: true,
  load_millis: 2400,
  load_report: { offloaded_layers: 43, total_layers: 43 },
  runtime: { version: "b11149", expected: "b11149", current: true },
  template: { model_id: "gemma-e4b", architecture: "gemma4", fix_offer: "gemma4-close-thinking" },
  items: [
    { id: "load", status: "pass" }, { id: "gpu", status: "pass" }, { id: "template", status: "warn" },
    { id: "runtime", status: "pass" }, { id: "reasoning", status: "fail" }, { id: "contract", status: "pass" },
    { id: "truncation", status: "pass" }, { id: "speed", status: "warn" }, { id: "verbosity", status: "pass" },
    { id: "refusals", status: "skip" },
  ],
  turns: [{
    message: "Start slowly.", reply: "Slowly, then.", motion: "start 20%", millis: 4100, first_token_millis: 3900,
    valid_json: true, first_try: true, repaired: false, failed: false, truncated: false, refused: false,
    reasoning_chars: 812, thinking_markup: false, words: 2, decode_tokens: 40, decode_millis: 500,
  }],
  summary: { ...idle.summary, turns: 5, first_try: 5, reasoning_chars: 812, average_millis: 4100, average_words: 18, tokens_per_second: 80 },
};

describe("ModelCheckDialog", () => {
  beforeEach(() => {
    vi.spyOn(api, "modelCheck").mockResolvedValue(idle);
    vi.spyOn(api, "startModelCheck").mockResolvedValue(finished);
    vi.spyOn(api, "setModelTemplateFix").mockResolvedValue({ id: "gemma-e4b" } as ManagedLLMModel);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("runs only when asked and words the backend's verdicts", async () => {
    render(<ModelCheckDialog locked={false} onClose={vi.fn()} />);
    const start = await screen.findByRole("button", { name: "Start test" });
    expect(api.startModelCheck).not.toHaveBeenCalled();
    fireEvent.click(start);

    expect(await screen.findByText("Reasons silently before answering")).toBeVisible();
    expect(screen.getByText(/About 812 characters of hidden reasoning/)).toBeVisible();
    expect(screen.getByText("Runs fully on the GPU")).toBeVisible();
    expect(screen.getByText("Replies are slow")).toBeVisible();
    expect(screen.getByText("Checks needing attention: 3.")).toBeVisible();
    // Skipped checks are not listed.
    expect(screen.queryByText("Stays in character")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Run again" })).toBeEnabled();
  });

  it("turns the offered template fix on for the tested model", async () => {
    const changed = vi.fn();
    vi.mocked(api.modelCheck).mockResolvedValue(finished);
    render(<ModelCheckDialog locked={false} onClose={vi.fn()} onModelChanged={changed} />);
    fireEvent.click(await screen.findByRole("button", { name: "Turn on the fix" }));

    await waitFor(() => expect(api.setModelTemplateFix).toHaveBeenCalledWith("gemma-e4b", true));
    expect(changed).toHaveBeenCalled();
    expect(await screen.findByText("The fix is on. Run the test again to confirm it.")).toBeVisible();
  });

  it("closes with Escape", async () => {
    const close = vi.fn();
    render(<ModelCheckDialog locked={false} onClose={close} />);
    await screen.findByRole("button", { name: "Start test" });
    fireEvent.keyDown(document, { key: "Escape" });
    expect(close).toHaveBeenCalled();
  });
});
