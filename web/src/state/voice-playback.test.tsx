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
  installAudioPlaybackUnlock: vi.fn(() => () => undefined),
  playBlob: vi.fn(async () => undefined),
  stopAllAudioPlayback: vi.fn(),
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

function QueuePair() {
  const { queueSpeech } = useVoicePlayback();
  return (
    <>
      <button type="button" onClick={() => queueSpeech("tts-1")}>Queue first</button>
      <button type="button" onClick={() => queueSpeech("tts-2")}>Queue second</button>
    </>
  );
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
        state: Date.now() - started >= 35_100 ? "done" : "active",
        created_at: "2026-07-15T12:00:00Z",
        audio_bytes: Date.now() - started >= 35_100 ? 48_000 : 0,
      },
    }));

    render(<VoicePlaybackProvider><QueueButton /></VoicePlaybackProvider>);
    fireEvent.click(screen.getByRole("button", { name: "Queue" }));

    await act(async () => {
      await vi.advanceTimersByTimeAsync(35_500);
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

  it("fetches later audio eagerly while preserving playback order", async () => {
    voiceRequest.mockImplementation(async (id) => ({
      request: {
        id,
        role: "tts",
        type: "speak",
        state: "done",
        created_at: "2026-07-15T12:00:00Z",
        audio_bytes: 100,
      },
    }));
    const firstAudio = new Blob(["first"], { type: "audio/wav" });
    const secondAudio = new Blob(["second"], { type: "audio/wav" });
    voiceRequestAudio.mockImplementation(async (id) => id === "tts-1" ? firstAudio : secondAudio);
    let releaseFirst!: () => void;
    playAudio.mockImplementationOnce(() => new Promise<void>((resolve) => { releaseFirst = resolve; }));

    render(<VoicePlaybackProvider><QueuePair /></VoicePlaybackProvider>);
    fireEvent.click(screen.getByRole("button", { name: "Queue first" }));
    fireEvent.click(screen.getByRole("button", { name: "Queue second" }));

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(voiceRequestAudio).toHaveBeenCalledTimes(2);
    expect(voiceRequestAudio.mock.calls.map(([id]) => id)).toEqual(["tts-1", "tts-2"]);
    expect(playAudio).toHaveBeenCalledOnce();

    await act(async () => {
      releaseFirst();
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(playAudio).toHaveBeenCalledTimes(2);
    expect(playAudio.mock.calls.map(([blob]) => blob)).toEqual([firstAudio, secondAudio]);
  });

  it("aborts every queued synthesis poll on Emergency Stop", async () => {
    const signals = new Map<string, AbortSignal>();
    voiceRequest.mockImplementation((id, signal) => new Promise((_resolve, reject) => {
      if (signal) signals.set(id, signal);
      signal?.addEventListener("abort", () => reject(new DOMException("Aborted", "AbortError")), { once: true });
    }));

    render(<VoicePlaybackProvider><QueuePair /></VoicePlaybackProvider>);
    fireEvent.click(screen.getByRole("button", { name: "Queue first" }));
    fireEvent.click(screen.getByRole("button", { name: "Queue second" }));
    expect(signals.size).toBe(2);

    await act(async () => window.dispatchEvent(new Event("magichandy:emergency-stop")));

    expect([...signals.values()].every((signal) => signal.aborted)).toBe(true);
    expect(show).not.toHaveBeenCalled();
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
