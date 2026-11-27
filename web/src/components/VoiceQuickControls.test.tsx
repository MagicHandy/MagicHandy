import { render, screen, within } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { VoiceQuickControls } from "./VoiceQuickControls";

const worker = vi.hoisted(() => ({ state: "running", model_state: "loading", last_error: "" }));
vi.mock("../state/app-state", () => ({
  useToast: () => ({ show: vi.fn() }),
  useAppState: () => ({ backendOnline: true, readOnly: false, refresh: vi.fn(), state: {
    settings: { voice: { enabled: true, tts_provider: "elevenlabs", speak_replies: true } },
    voice: { workers: { tts: worker } },
  } }),
}));

beforeEach(() => {
  worker.state = "running";
  worker.model_state = "loading";
  worker.last_error = "";
});

it("describes startup as starting instead of telling the user to start it again", () => {
  const view = render(<VoiceQuickControls />);
  expect(screen.getByText("Starting voice…")).toBeInTheDocument();
  expect(screen.queryByText(/start it in Settings/)).not.toBeInTheDocument();
  worker.model_state = "ready";
  view.rerender(<VoiceQuickControls />);
  expect(screen.queryByText("Starting voice…")).not.toBeInTheDocument();
  expect(screen.getByText("Voice ready")).toBeInTheDocument();
});

it("keeps a failure and its reason on screen with a way to fix it", () => {
  worker.model_state = "error";
  worker.last_error = "ElevenLabs did not accept the API key.";
  render(<VoiceQuickControls />);
  const alert = screen.getByRole("alert");
  expect(alert).toHaveTextContent("Replies aren't being spoken: ElevenLabs did not accept the API key.");
  expect(within(alert).getByRole("link", { name: "Fix in Settings › Voice" })).toHaveAttribute("href", "#/settings/voice");
  expect(screen.queryByText("Starting voice…")).not.toBeInTheDocument();
});

it("asks for the voice worker to be started when it is stopped without an error", () => {
  worker.state = "stopped";
  worker.model_state = "unloaded";
  render(<VoiceQuickControls />);
  expect(screen.getByRole("status")).toHaveTextContent("Voice output is not ready — start it in Settings → Voice");
});
