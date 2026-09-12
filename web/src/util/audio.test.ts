import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

class FakeSource {
  buffer: AudioBuffer | null = null;
  onended: ((this: AudioScheduledSourceNode, ev: Event) => unknown) | null = null;
  connect = vi.fn();
  disconnect = vi.fn();
  start = vi.fn(() => queueMicrotask(() => this.onended?.call(this as unknown as AudioScheduledSourceNode, new Event("ended"))));
  stop = vi.fn();
}

class FakeAudioContext {
  static instances: FakeAudioContext[] = [];
  state: AudioContextState = "suspended";
  currentTime = 0;
  destination = {} as AudioDestinationNode;
  source = new FakeSource();
  sources: FakeSource[] = [];
  resume = vi.fn(async () => {
    this.state = "running";
  });
  decodeAudioData = vi.fn(async () => ({} as AudioBuffer));
  createBufferSource = vi.fn(() => {
    const source = this.sources.length ? new FakeSource() : this.source;
    this.sources.push(source);
    return source as unknown as AudioBufferSourceNode;
  });
  createBuffer = vi.fn((channels: number, frames: number) => {
    const data = Array.from({ length: channels }, () => new Float32Array(frames));
    return { getChannelData: (channel: number) => data[channel] } as AudioBuffer;
  });

  constructor() {
    FakeAudioContext.instances.push(this);
  }
}

describe("shared audio playback", () => {
  it("schedules PCM before the producer finishes and rejects later audio after cancellation", async () => {
    const { audioPlaybackToken, playPCMChunks, stopAllAudioPlayback } = await import("./audio");
    const controller = new AbortController();
    let release!: () => void;
    const gate = new Promise<void>((resolve) => { release = resolve; });
    async function* chunks() {
      const samples = new Float32Array(24000);
      samples[1] = 0.5;
      yield { samples, sampleRate: 24000, channels: 1 };
      await gate;
      yield { samples: new Float32Array([0.25]), sampleRate: 24000, channels: 1 };
    }
    const playing = playPCMChunks(chunks(), audioPlaybackToken(), controller.signal);
    const rejected = expect(playing).rejects.toMatchObject({ name: "AbortError" });
    await vi.waitFor(() => expect(FakeAudioContext.instances[0].source.start).toHaveBeenCalledOnce());
    const audioContext = FakeAudioContext.instances[0];
    expect([...audioContext.createBuffer.mock.results[0].value.getChannelData(0).slice(0, 2)]).toEqual([0, 0.5]);
    controller.abort();
    stopAllAudioPlayback();
    release();
    await rejected;
    expect(audioContext.createBufferSource).toHaveBeenCalledOnce();
  });
  const cleanup: Array<() => void> = [];

  beforeEach(() => {
    vi.resetModules();
    FakeAudioContext.instances = [];
    Object.defineProperty(globalThis, "AudioContext", { value: FakeAudioContext, configurable: true });
  });

  afterEach(() => {
    cleanup.splice(0).forEach((remove) => remove());
    vi.restoreAllMocks();
  });

  it("unlocks the persistent audio context during a user gesture", async () => {
    const { installAudioPlaybackUnlock } = await import("./audio");
    const remove = installAudioPlaybackUnlock();
    cleanup.push(remove);

    window.dispatchEvent(new Event("pointerdown"));
    await Promise.resolve();

    expect(FakeAudioContext.instances).toHaveLength(1);
    expect(FakeAudioContext.instances[0].resume).toHaveBeenCalledOnce();
  });

  it("decodes and plays completed speech through the unlocked context", async () => {
    const { installAudioPlaybackUnlock, playBlob } = await import("./audio");
    const remove = installAudioPlaybackUnlock();
    cleanup.push(remove);
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter" }));
    await Promise.resolve();

    const audioContext = FakeAudioContext.instances[0];
    const blob = { arrayBuffer: vi.fn(async () => new ArrayBuffer(8)) } as unknown as Blob;
    await playBlob(blob);

    expect(blob.arrayBuffer).toHaveBeenCalledOnce();
    expect(audioContext.decodeAudioData).toHaveBeenCalledOnce();
    expect(audioContext.source.connect).toHaveBeenCalledWith(audioContext.destination);
    expect(audioContext.source.start).toHaveBeenCalledOnce();
    expect(audioContext.source.disconnect).toHaveBeenCalledOnce();
  });

  it("stops active speech immediately", async () => {
    const { installAudioPlaybackUnlock, playBlob, stopAllAudioPlayback } = await import("./audio");
    const remove = installAudioPlaybackUnlock();
    cleanup.push(remove);
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter" }));
    await Promise.resolve();

    const audioContext = FakeAudioContext.instances[0];
    audioContext.source.start.mockImplementation(() => undefined);
    const playback = playBlob({ arrayBuffer: async () => new ArrayBuffer(8) } as unknown as Blob);
    await vi.waitFor(() => expect(audioContext.source.start).toHaveBeenCalledOnce());
    stopAllAudioPlayback();
    await playback;

    expect(audioContext.source.stop).toHaveBeenCalledOnce();
  });

  it("cancels an unfinished prebuffer even if the next producer read never settles", async () => {
    const { audioPlaybackToken, playPCMChunks, stopAllAudioPlayback } = await import("./audio");
    const closed = vi.fn(async () => ({ done: true as const, value: undefined }));
    const next = vi.fn().mockResolvedValueOnce({ done: false, value: { samples: new Float32Array(2400), sampleRate: 24000, channels: 1 } })
      .mockImplementation(() => new Promise(() => undefined));
    const chunks = { [Symbol.asyncIterator]: () => ({ next, return: closed }) };
    const playing = playPCMChunks(chunks, audioPlaybackToken(), new AbortController().signal);
    const rejected = expect(playing).rejects.toMatchObject({ name: "AbortError" });
    await vi.waitFor(() => expect(next).toHaveBeenCalledTimes(2));
    expect(FakeAudioContext.instances[0].createBufferSource).not.toHaveBeenCalled();
    stopAllAudioPlayback();
    await rejected;
    expect(closed).toHaveBeenCalledOnce();
  });

  it("cancels while the browser is still waiting to unlock its audio context", async () => {
    const { audioPlaybackToken, installAudioPlaybackUnlock, playPCMChunks, stopAllAudioPlayback } = await import("./audio");
    cleanup.push(installAudioPlaybackUnlock());
    window.dispatchEvent(new Event("pointerdown"));
    await Promise.resolve();
    const audioContext = FakeAudioContext.instances[0];
    audioContext.state = "suspended";
    audioContext.resume.mockImplementation(() => new Promise(() => undefined));
    const next = vi.fn();
    const playing = playPCMChunks({ [Symbol.asyncIterator]: () => ({ next }) }, audioPlaybackToken(), new AbortController().signal);
    const rejected = expect(playing).rejects.toMatchObject({ name: "AbortError" });
    stopAllAudioPlayback();
    await rejected;
    expect(next).not.toHaveBeenCalled();
    expect(audioContext.createBufferSource).not.toHaveBeenCalled();
  });

  it("does not insert a gap when the next chunk arrives shortly before the boundary", async () => {
    const { audioPlaybackToken, playPCMChunks } = await import("./audio");
    async function* chunks() {
      yield { samples: new Float32Array(19200), sampleRate: 24000, channels: 1 };
      FakeAudioContext.instances[0].currentTime = 0.82;
      yield { samples: new Float32Array(4800), sampleRate: 24000, channels: 1 };
    }
    await playPCMChunks(chunks(), audioPlaybackToken(), new AbortController().signal);
    const starts = FakeAudioContext.instances[0].sources.flatMap((source) => source.start.mock.calls) as unknown as number[][];
    expect(starts).toHaveLength(2);
    expect(starts[0][0]).toBeCloseTo(0.035);
    expect(starts[1][0]).toBeCloseTo(0.835);
    for (const source of FakeAudioContext.instances[0].sources) expect(source.disconnect).toHaveBeenCalledOnce();
  });

  it("plays unevenly delivered speech continuously after collecting a reserve", async () => {
    const { audioPlaybackToken, playPCMChunks } = await import("./audio");
    async function* chunks() {
      for (let index = 0; index < 24; index++) {
        // Alternating 550/50 ms arrival intervals, each carrying 300 ms audio.
        FakeAudioContext.instances[0].currentTime = Math.floor(index / 2) * 0.6 + (index % 2 ? 0.55 : 0);
        yield { samples: new Float32Array(7200), sampleRate: 24000, channels: 1 };
      }
    }
    await playPCMChunks(chunks(), audioPlaybackToken(), new AbortController().signal);
    const audioContext = FakeAudioContext.instances[0];
    const starts = audioContext.sources.flatMap((source) => source.start.mock.calls) as unknown as number[][];
    expect(starts).toHaveLength(24);
    for (let index = 0; index < starts.length; index++) expect(starts[index][0]).toBeCloseTo(0.635 + index * 0.3);
    for (const source of audioContext.sources) expect(source.disconnect).toHaveBeenCalledOnce();
  });
});
