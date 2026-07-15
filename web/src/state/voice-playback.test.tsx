import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { playBlob } from "../util/audio";
import { VoicePlaybackProvider, useVoicePlayback } from "./voice-playback";

const show = vi.hoisted(() => vi.fn());

vi.mock("../api/client", () => ({
  api: {
    voiceRequest: vi.fn(),
    voiceRequestAudio: vi.fn(),
  },
}));

vi.mock("../util/audio", () => ({
  audioPlaybackToken: vi.fn(() => 0),
  playBlob: vi.fn(async () => undefined),
}));

vi.mock("./app-state", () => ({
  useToast: () => ({ show }),
}));

const voiceRequest = vi.mocked(api.voiceRequest);
const voiceRequestAudio = vi.mocked(api.voiceRequestAudio);
const playAudio = vi.mocked(playBlob);

function QueueButton({ id = "tts-1" }: { id?: string }) {
  const { queueSpeech } = useVoicePlayback();
  return <button type="button" onClick={() => queueSpeech(id)}>Queue</button>;
}

describe("VoicePlaybackProvider", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-15T12:00:00Z"));
    show.mockReset();
    voiceRequest.mockReset();
    voiceRequestAudio.mockReset();
    playAudio.mockReset();
    voiceRequestAudio.mockResolvedValue(new Blob(["wav"], { type: "audio/wav" }));
    playAudio.mockResolvedValue(undefined);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("keeps polling beyond the former browser timeout and plays completed audio", async () => {
    const started = Date.now();
    voiceRequest.mockImplementation(async (id) => ({
      request: {
        id,
        role: "tts",
        type: "speak",
        state: Date.now() - started >= 35_000 ? "done" : "active",
        created_at: "2026-07-15T12:00:00Z",
        audio_bytes: Date.now() - started >= 35_000 ? 48_000 : 0,
      },
    }));

    render(<VoicePlaybackProvider><QueueButton /></VoicePlaybackProvider>);
    fireEvent.click(screen.getByRole("button", { name: "Queue" }));

    await act(async () => {
      await vi.advanceTimersByTimeAsync(36_000);
    });

    expect(voiceRequestAudio).toHaveBeenCalledWith("tts-1", expect.any(AbortSignal));
    expect(playAudio).toHaveBeenCalledOnce();
    expect(show).not.toHaveBeenCalled();
  });

  it("reports a terminal synthesis failure instead of dropping it silently", async () => {
    voiceRequest.mockResolvedValue({
      request: {
        id: "tts-1",
        role: "tts",
        type: "speak",
        state: "failed",
        created_at: "2026-07-15T12:00:00Z",
        error: { code: "inference_failed", message: "decoder failed" },
      },
    });

    render(<VoicePlaybackProvider><QueueButton /></VoicePlaybackProvider>);
    fireEvent.click(screen.getByRole("button", { name: "Queue" }));

    await act(async () => {
      await Promise.resolve();
    });
    expect(show).toHaveBeenCalledWith(
      "Speech output could not play: decoder failed.",
      "error",
    );
    expect(voiceRequestAudio).not.toHaveBeenCalled();
  });

  it("abandons pending playback immediately on Emergency Stop", async () => {
    voiceRequest.mockResolvedValue({
      request: {
        id: "tts-1",
        role: "tts",
        type: "speak",
        state: "active",
        created_at: "2026-07-15T12:00:00Z",
      },
    });

    render(<VoicePlaybackProvider><QueueButton /></VoicePlaybackProvider>);
    fireEvent.click(screen.getByRole("button", { name: "Queue" }));
    await act(async () => window.dispatchEvent(new Event("magichandy:emergency-stop")));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });

    expect(voiceRequestAudio).not.toHaveBeenCalled();
    expect(show).not.toHaveBeenCalled();
  });
});
