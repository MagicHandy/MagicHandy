import { describe, expect, it } from "vitest";
import { SpeechBuffer } from "./speech-buffer";

const audio = (seconds: number) => ({ samples: new Float32Array(seconds * 24000), sampleRate: 24000, channels: 1 });

describe("speech jitter buffering", () => {
  it("collects a startup reserve and then passes on-time audio through", () => {
    const buffer = new SpeechBuffer();
    expect(buffer.push(audio(0.4), true)).toHaveLength(0);
    expect(buffer.push(audio(0.4), true)).toHaveLength(2);
    expect(buffer.push(audio(0.4), false)).toHaveLength(1);
  });

  it("refills a larger reserve after an underrun instead of restarting each small chunk", () => {
    const buffer = new SpeechBuffer();
    buffer.push(audio(1), true);
    for (let index = 0; index < 3; index++) expect(buffer.push(audio(0.4), true)).toHaveLength(0);
    expect(buffer.push(audio(0.4), true)).toHaveLength(4);
    for (let index = 0; index < 5; index++) expect(buffer.push(audio(0.5), true)).toHaveLength(0);
    expect(buffer.push(audio(0.5), true)).toHaveLength(6);
    expect(buffer.push(audio(3), true)).toHaveLength(1); // Reserve is capped.
  });

  it("flushes a short completed reply without padding it or losing its last samples", () => {
    const buffer = new SpeechBuffer();
    const short = audio(0.1);
    expect(buffer.push(short, true)).toHaveLength(0);
    expect(buffer.flush()).toEqual([short]);
    expect(buffer.flush()).toEqual([]);
  });
});
