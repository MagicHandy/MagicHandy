import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { ProductionChatTest } from "./ProductionChatTest";

vi.mock("../api/client", () => ({ api: { productionChatTest: vi.fn() } }));
const show = vi.hoisted(() => vi.fn());
vi.mock("../state/app-state", () => ({ useAppState: () => ({ backendOnline: true }), useToast: () => ({ show }) }));

beforeEach(() => { vi.mocked(api.productionChatTest).mockReset(); });

it("shows the production result as a preview without submitting chat or motion", async () => {
  vi.mocked(api.productionChatTest).mockResolvedValue({ valid: true, report: { id: 0, created_at: "", error: "", motion_mode: "creative_v2", reasoning_mode: "off", max_tokens: 512, app_version: "review", app_commit: "test", diagnostics: {}, content: { raw_response: "MODEL OUTPUT", system_prompt: "", response_schema: "{}", truncated: false } } });
  render(<ProductionChatTest />);
  fireEvent.click(screen.getByText("Test active chat configuration"));
  fireEvent.change(screen.getByLabelText("Test message"), { target: { value: "Use the upper half." } });
  fireEvent.click(screen.getByRole("button", { name: "Test without motion" }));
  expect(await screen.findByText("Response accepted. Motion was not applied.")).toBeInTheDocument();
  expect(screen.getByText("MODEL OUTPUT")).toBeInTheDocument();
  expect(api.productionChatTest).toHaveBeenCalledWith("Use the upper half.", expect.any(AbortSignal));
});

it("cancels the model request on Stop and ignores a late result", async () => {
  let resolve!: (value: Awaited<ReturnType<typeof api.productionChatTest>>) => void;
  vi.mocked(api.productionChatTest).mockImplementation(() => new Promise(done => { resolve = done; }));
  render(<ProductionChatTest />);
  fireEvent.click(screen.getByText("Test active chat configuration"));
  fireEvent.change(screen.getByLabelText("Test message"), { target: { value: "Hello" } });
  fireEvent.click(screen.getByRole("button", { name: "Test without motion" }));
  const signal = vi.mocked(api.productionChatTest).mock.calls[0][1]!;
  fireEvent(window, new Event("magichandy:emergency-stop"));
  expect(signal.aborted).toBe(true);
  await act(async () => resolve({ valid: true, report: {} as never }));
  await waitFor(() => expect(screen.queryByText("Generating")).not.toBeInTheDocument());
  expect(screen.queryByText("Response accepted. Motion was not applied.")).not.toBeInTheDocument();
});
