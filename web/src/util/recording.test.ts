import { afterEach, describe, expect, it, vi } from "vitest";
import { encodePCM16WAV, recordingToWAV } from "./recording";

afterEach(() => vi.unstubAllGlobals());

describe("encodePCM16WAV", () => {
  it("downmixes and resamples browser audio into a valid 16 kHz mono WAV", () => {
    const wav = encodePCM16WAV([
      new Float32Array([1, 0, -1, 0]),
      new Float32Array([0, 0, 0, 0]),
    ], 16000);
    const view = new DataView(wav.buffer);

    expect(new TextDecoder().decode(wav.slice(0, 4))).toBe("RIFF");
    expect(new TextDecoder().decode(wav.slice(8, 12))).toBe("WAVE");
    expect(view.getUint16(20, true)).toBe(1);
    expect(view.getUint16(22, true)).toBe(1);
    expect(view.getUint32(24, true)).toBe(16000);
    expect(view.getUint16(34, true)).toBe(16);
    expect(view.getUint32(40, true)).toBe(8);
    expect(view.getInt16(44, true)).toBe(16384);
    expect(view.getInt16(46, true)).toBe(0);
    expect(view.getInt16(48, true)).toBe(-16384);
  });

  it("rejects empty recordings", () => {
    expect(() => encodePCM16WAV([], 48000)).toThrow(/no samples/i);
  });

  it("decodes, downmixes, and delegates browser-rate conversion to OfflineAudioContext", async () => {
    const close = vi.fn(async () => undefined);
    const channels = [new Float32Array([1, 0, -1]), new Float32Array([0, 0, 1])];
    class FakeAudioContext {
      close = close;
      async decodeAudioData() {
        return {
          numberOfChannels: 2,
          length: 3,
          duration: 3 / 48000,
          sampleRate: 48000,
          getChannelData: (channel: number) => channels[channel],
        };
      }
    }
    let monoSamples = new Float32Array();
    class FakeOfflineAudioContext {
      destination = {};
      constructor(channelsCount: number, frameCount: number, sampleRate: number) {
        expect([channelsCount, frameCount, sampleRate]).toEqual([1, 1, 16000]);
      }
      createBuffer(_channels: number, length: number, sampleRate: number) {
        expect([length, sampleRate]).toEqual([3, 48000]);
        monoSamples = new Float32Array(length);
        return { getChannelData: () => monoSamples };
      }
      createBufferSource() {
        return { buffer: null, connect: vi.fn(), start: vi.fn() };
      }
      async startRendering() {
        return { getChannelData: () => new Float32Array([0.25]) };
      }
    }
    vi.stubGlobal("AudioContext", FakeAudioContext);
    vi.stubGlobal("OfflineAudioContext", FakeOfflineAudioContext);

    const recording = { arrayBuffer: async () => new ArrayBuffer(3) } as Blob;
    const wav = await recordingToWAV(recording);
    expect(Array.from(monoSamples)).toEqual([0.5, 0, 0]);
    expect(wav.type).toBe("audio/wav");
    expect(wav.size).toBe(46);
    expect(close).toHaveBeenCalledOnce();
  });

  it("closes the decoder when browser audio decoding fails", async () => {
    const close = vi.fn(async () => undefined);
    class FailingAudioContext {
      close = close;
      async decodeAudioData() { throw new Error("unsupported codec"); }
    }
    vi.stubGlobal("AudioContext", FailingAudioContext);

    const recording = { arrayBuffer: async () => new ArrayBuffer(1) } as Blob;
    await expect(recordingToWAV(recording)).rejects.toThrow(/unsupported codec/i);
    expect(close).toHaveBeenCalledOnce();
  });
});
