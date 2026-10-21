import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { PromptAssignment } from "./PromptAssignment";
import type { PromptCompositionPayload } from "../api/types";

vi.mock("../api/client", () => ({ api: { promptComposition: vi.fn(), updatePersona: vi.fn() } }));
const app = vi.hoisted(() => ({ refresh: vi.fn(), show: vi.fn(), state: { chat: { active_session_id: "chat" }, settings: { llm: { prompt_set: "default" } } } }));
vi.mock("../state/app-state", () => ({ useAppState: () => app, useToast: () => ({ show: app.show }) }));
const active = { prompt_set: "previous", prompt_set_name: "Previous behavior", prompt_set_source: "persona", persona_name: "Rowan", persona_id: "rowan" } as PromptCompositionPayload;
beforeEach(() => {
  vi.mocked(api.promptComposition).mockReset().mockResolvedValue(active);
  vi.mocked(api.updatePersona).mockReset().mockResolvedValue({} as never);
});

it("distinguishes a persona override from choosing the default", async () => {
  const chooseDefault = vi.fn();
  render(<PromptAssignment selectedID="new-set" defaultID="default" chooseDefault={chooseDefault} locked={false} unsaved={false} />);
  expect(await screen.findByText("Rowan's prompt set overrides the default.")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Choose as default" }));
  expect(chooseDefault).toHaveBeenCalledWith("new-set");
  expect(api.updatePersona).not.toHaveBeenCalled();
  vi.mocked(api.promptComposition).mockResolvedValue({ ...active, prompt_set: "new-set", prompt_set_name: "New behavior" });
  fireEvent.click(screen.getByRole("button", { name: "Use for Rowan" }));
  await waitFor(() => expect(api.updatePersona).toHaveBeenCalledWith("rowan", { prompt_set_id: "new-set" }));
  expect(await screen.findByText(/New behavior/)).toBeInTheDocument();
});

it("requires saving edited content before assigning the set", async () => {
  render(<PromptAssignment selectedID="new-set" defaultID="default" chooseDefault={vi.fn()} locked={false} unsaved={true} />);
  expect(await screen.findByRole("button", { name: "Use for Rowan" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "Choose as default" })).toBeDisabled();
});
