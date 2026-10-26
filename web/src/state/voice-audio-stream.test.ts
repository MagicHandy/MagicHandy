import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { playSpeechStream } from "./voice-audio-stream";
import type { PCMChunk } from "../util/pcm-stream";

const samples = vi.hoisted(() => [] as number[]);
vi.mock("../api/client", () => ({ api: { voiceRequestAudioChunk: vi.fn() } }));
vi.mock("../util/audio", () => ({ playPCMChunks: async (chunks: AsyncIterable<PCMChunk>) => {
  for await (const chunk of chunks) samples.push(...chunk.samples);
} }));

beforeEach(() => { vi.useFakeTimers(); vi.mocked(api.voiceRequestAudioChunk).mockReset(); samples.length = 0; });
afterEach(() => vi.useRealTimers());
const chunk = (offset: number, done: boolean) => ({ offset, next_offset: offset + 4, data: btoa("\x00\x40\x00\x20"), format: "pcm_s16le_24000", state: done ? "done" : "active", done });

it("recovers a dropped fetch at the same offset without skipping or repeating audio", async () => {
  const read = vi.mocked(api.voiceRequestAudioChunk);
  read.mockResolvedValueOnce(chunk(0, false)).mockRejectedValueOnce(new TypeError("Failed to fetch")).mockResolvedValueOnce(chunk(4, true));
  const playback = playSpeechStream("tts-1", "pcm_s16le_24000", 0, new AbortController().signal);
  await vi.runAllTimersAsync();
  await playback;
  expect(read.mock.calls.map((call) => call[1])).toEqual([0, 4, 4]);
  expect(samples).toEqual([0.5, 0.25, 0.5, 0.25]);
});

it.each([401, 403, 404, 409])("does not retry a terminal access or retention response (%s)", async (status) => {
  const failure = Object.assign(new Error("unavailable"), { status });
  vi.mocked(api.voiceRequestAudioChunk).mockRejectedValue(failure);
  await expect(playSpeechStream("tts-1", "pcm_s16le_24000", 0, new AbortController().signal)).rejects.toBe(failure);
  expect(api.voiceRequestAudioChunk).toHaveBeenCalledOnce();
});

it("Stop cancels retry backoff immediately", async () => {
  vi.mocked(api.voiceRequestAudioChunk).mockRejectedValue(new TypeError("Failed to fetch"));
  const controller = new AbortController();
  const playback = playSpeechStream("tts-1", "pcm_s16le_24000", 0, controller.signal);
  const rejected = expect(playback).rejects.toMatchObject({ name: "AbortError" });
  await vi.advanceTimersByTimeAsync(1);
  controller.abort();
  await rejected;
  await vi.runAllTimersAsync();
  expect(api.voiceRequestAudioChunk).toHaveBeenCalledOnce();
});

it("cancellation wins a simultaneous network failure", async () => {
  const controller = new AbortController();
  vi.mocked(api.voiceRequestAudioChunk).mockImplementationOnce(async () => {
    controller.abort();
    throw new TypeError("Failed to fetch");
  });
  await expect(playSpeechStream("tts-1", "pcm_s16le_24000", 0, controller.signal)).rejects.toMatchObject({ name: "AbortError" });
  expect(api.voiceRequestAudioChunk).toHaveBeenCalledOnce();
});

it("bounds retries and reports a persistent outage", async () => {
  const error = Object.assign(new Error("gateway unavailable"), { status: 503 });
  vi.mocked(api.voiceRequestAudioChunk).mockRejectedValue(error);
  const rejected = expect(playSpeechStream("tts-1", "pcm_s16le_24000", 0, new AbortController().signal)).rejects.toBe(error);
  await vi.runAllTimersAsync();
  await rejected;
  expect(api.voiceRequestAudioChunk).toHaveBeenCalledTimes(4);
});
