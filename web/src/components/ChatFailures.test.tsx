import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { ChatFailures } from "./ChatFailures";

vi.mock("../api/client", () => ({ api: { chatFailures: vi.fn(), chatFailure: vi.fn() } }));
const show = vi.hoisted(() => vi.fn());
vi.mock("../state/app-state", () => ({ useToast: () => ({ show }) }));
const failure = { id: 7, created_at: "2026-10-11T12:00:00Z", error: "action is missing", motion_mode: "creative_v2", reasoning_mode: "auto", max_tokens: 1536, app_version: "review", app_commit: "test", diagnostics: { provider: "ollama", model: "fixture" } };

beforeEach(() => {
  vi.mocked(api.chatFailures).mockReset().mockResolvedValue({ failures: [failure] });
  vi.mocked(api.chatFailure).mockReset().mockResolvedValue({ ...failure, content: { raw_response: "PRIVATE MODEL OUTPUT", system_prompt: "PRIVATE PROMPT", response_schema: "{}", truncated: false } });
});

it("loads retained metadata on demand and copies content only after explicit selection", async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  vi.stubGlobal("navigator", { ...navigator, clipboard: { writeText } });
  render(<ChatFailures sessionID="saved-chat" busy={false} />);
  expect(api.chatFailures).not.toHaveBeenCalled();
  const details = screen.getByText("Recent request failures").closest("details")!;
  details.open = true;
  fireEvent(details, new Event("toggle"));
  expect(await screen.findByText("action is missing")).toBeInTheDocument();
  expect(api.chatFailure).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Copy diagnostics", hidden: true }));
  await waitFor(() => expect(writeText).toHaveBeenCalledTimes(1));
  expect(writeText.mock.calls[0][0]).not.toContain("PRIVATE");
  fireEvent.click(screen.getByLabelText("Include prompt and model output"));
  expect(await screen.findByText("PRIVATE MODEL OUTPUT")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Copy diagnostics", hidden: true }));
  await waitFor(() => expect(writeText).toHaveBeenCalledTimes(2));
  expect(writeText.mock.calls[1][0]).toContain("PRIVATE MODEL OUTPUT");
  details.open = false;
  fireEvent(details, new Event("toggle"));
  await waitFor(() => expect(screen.queryByText("PRIVATE MODEL OUTPUT")).not.toBeInTheDocument());
});
