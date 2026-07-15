import { describe, expect, it } from "vitest";
import { VoiceActivitySegmenter } from "./voice-activity";

const chunk = (value: number, frames = 50) => new Float32Array(frames).fill(value);

describe("VoiceActivitySegmenter", () => {
  it("segments repeated phrases without ending the listening session", () => {
    const vad = new VoiceActivitySegmenter({ sampleRate: 1000, sensitivity: 55, silenceMillis: 300 });
    for (let index = 0; index < 5; index += 1) vad.push(chunk(0));
    for (let index = 0; index < 6; index += 1) vad.push(chunk(0.15));
    let first: Float32Array | undefined;
    for (let index = 0; index < 6; index += 1) first = vad.push(chunk(0)).segment ?? first;
    expect(first?.length).toBeGreaterThan(300);

    for (let index = 0; index < 5; index += 1) vad.push(chunk(0.12));
    let second: Float32Array | undefined;
    for (let index = 0; index < 6; index += 1) second = vad.push(chunk(0)).segment ?? second;
    expect(second?.length).toBeGreaterThan(250);
  });

  it("flushes a final phrase when listening is stopped manually", () => {
    const vad = new VoiceActivitySegmenter({ sampleRate: 1000, sensitivity: 55, silenceMillis: 600 });
    vad.push(chunk(0));
    for (let index = 0; index < 8; index += 1) vad.push(chunk(0.2));
    expect(vad.flush()?.length).toBeGreaterThan(200);
    expect(vad.flush()).toBeUndefined();
  });

  it("uses sensitivity to reject or accept quiet speech", () => {
    const low = new VoiceActivitySegmenter({ sampleRate: 1000, sensitivity: 5, silenceMillis: 300 });
    const high = new VoiceActivitySegmenter({ sampleRate: 1000, sensitivity: 95, silenceMillis: 300 });
    for (let index = 0; index < 5; index += 1) {
      low.push(chunk(0));
      high.push(chunk(0));
    }
    for (let index = 0; index < 5; index += 1) {
      low.push(chunk(0.008));
      high.push(chunk(0.008));
    }
    expect(low.flush()).toBeUndefined();
    expect(high.flush()).toBeDefined();
  });

  it("calibrates steady room noise before accepting louder speech", () => {
    const vad = new VoiceActivitySegmenter({ sampleRate: 1000, sensitivity: 55, silenceMillis: 300 });
    for (let index = 0; index < 12; index += 1) vad.push(chunk(0.02));
    expect(vad.flush()).toBeUndefined();

    for (let index = 0; index < 5; index += 1) vad.push(chunk(0.12));
    expect(vad.flush()).toBeDefined();
  });
});
