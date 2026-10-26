import { render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { VoiceQuickControls } from "./VoiceQuickControls";

const worker = vi.hoisted(() => ({ state: "running", model_state: "loading" }));
vi.mock("../state/app-state", () => ({
  useToast: () => ({ show: vi.fn() }),
  useAppState: () => ({ backendOnline: true, readOnly: false, refresh: vi.fn(), state: {
    settings: { voice: { enabled: true, tts_provider: "faster_qwen3_tts", speak_replies: true } },
    voice: { workers: { tts: worker } },
  } }),
}));

it("describes startup as loading instead of telling the user to start it again", () => {
  worker.model_state = "loading";
  const view = render(<VoiceQuickControls />);
  expect(screen.getByText("Loading...")).toBeInTheDocument();
  expect(screen.queryByText(/start it in Settings/)).not.toBeInTheDocument();
  worker.model_state = "ready";
  view.rerender(<VoiceQuickControls />);
  expect(screen.queryByText("Loading...")).not.toBeInTheDocument();
});
