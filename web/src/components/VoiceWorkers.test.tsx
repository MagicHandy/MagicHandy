import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { VoiceWorkerStatus } from "../api/types";
import { VoiceWorkers } from "./VoiceWorkers";

vi.mock("../api/client", () => ({
  api: {},
}));

vi.mock("../state/app-state", () => ({
  useToast: () => ({ show: vi.fn() }),
}));

vi.mock("../state/voice-playback", () => ({
  useVoicePlayback: () => ({ queueSpeech: vi.fn() }),
}));

describe("VoiceWorkers", () => {
  it("shows an owned model-server failure while the adapter remains running", () => {
    const tts: VoiceWorkerStatus = {
      role: "tts",
      state: "running",
      configured: true,
      provider: "openai-compatible-tts",
      model_state: "",
      worker_queue_depth: 0,
      queue_depth: 0,
      last_error: "managed TTS server exited unexpectedly",
    };

    const { container } = render(
      <VoiceWorkers
        locked={false}
        role="tts"
        workers={{ tts }}
        requests={[]}
        modules={{}}
        refresh={vi.fn(async () => undefined)}
      />,
    );

    expect(screen.getByText("Not ready")).toBeInTheDocument();
    expect(screen.getByText("managed TTS server exited unexpectedly")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Load model" })).toBeEnabled();
    expect(screen.queryByRole("button", { name: "Send test" })).not.toBeInTheDocument();
    expect(container.querySelector('.status-dot[data-state="error"]')).not.toBeNull();
  });
});
