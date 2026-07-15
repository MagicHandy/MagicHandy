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
  destination = {} as AudioDestinationNode;
  source = new FakeSource();
  resume = vi.fn(async () => {
    this.state = "running";
  });
  decodeAudioData = vi.fn(async () => ({} as AudioBuffer));
  createBufferSource = vi.fn(() => this.source as unknown as AudioBufferSourceNode);

  constructor() {
    FakeAudioContext.instances.push(this);
  }
}

describe("shared audio playback", () => {
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
});
