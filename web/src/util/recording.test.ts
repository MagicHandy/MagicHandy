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

  it("delegates filtered browser-rate conversion and downmixing to Web Audio", async () => {
    const close = vi.fn(async () => undefined);
    const decoded = {
      numberOfChannels: 2,
      length: 6,
      duration: 6 / 48000,
      sampleRate: 48000,
      getChannelData: () => new Float32Array(6),
    };
    class FakeAudioContext {
      close = close;
      async decodeAudioData() {
        return decoded;
      }
    }
    const source = { buffer: null as typeof decoded | null, connect: vi.fn(), start: vi.fn() };
    class FakeOfflineAudioContext {
      destination = {};
      constructor(channels: number, frames: number, sampleRate: number) {
        expect([channels, frames, sampleRate]).toEqual([1, 2, 16000]);
      }
      createBufferSource() { return source; }
      async startRendering() {
        return { getChannelData: () => new Float32Array([0.25, -0.25]) };
      }
    }
    vi.stubGlobal("AudioContext", FakeAudioContext);
    vi.stubGlobal("OfflineAudioContext", FakeOfflineAudioContext);

    const recording = { arrayBuffer: async () => new ArrayBuffer(6) } as Blob;
    const wav = await recordingToWAV(recording);
    expect(source.buffer).toBe(decoded);
    expect(source.connect).toHaveBeenCalledOnce();
    expect(source.start).toHaveBeenCalledOnce();
    expect(wav.type).toBe("audio/wav");
    expect(wav.size).toBe(48);
    expect(close).toHaveBeenCalledOnce();
  });

  it("reuses a warmed decoder without closing it", async () => {
    const close = vi.fn(async () => undefined);
    const decoder = {
      close,
      decodeAudioData: async () => ({
        duration: 1 / 16000,
        numberOfChannels: 1,
        sampleRate: 16000,
        getChannelData: () => new Float32Array([0]),
      }),
    } as unknown as AudioContext;
    class FakeOfflineAudioContext {
      destination = {};
      createBufferSource() { return { buffer: null, connect: vi.fn(), start: vi.fn() }; }
      async startRendering() { return { getChannelData: () => new Float32Array([0]) }; }
    }
    vi.stubGlobal("OfflineAudioContext", FakeOfflineAudioContext);

    await recordingToWAV({ arrayBuffer: async () => new ArrayBuffer(1) } as Blob, decoder);
    expect(close).not.toHaveBeenCalled();
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
