import { describe, expect, it } from "vitest";
import { PCMStreamDecoder, UnsupportedPCMStream } from "./pcm-stream";

function wav(streaming = false) {
  const bytes = new Uint8Array(52);
  const view = new DataView(bytes.buffer);
  for (const [offset, text] of [[0, "RIFF"], [8, "WAVE"], [12, "fmt "], [36, "data"]] as const) {
    bytes.set([...text].map((char) => char.charCodeAt(0)), offset);
  }
  view.setUint32(4, streaming ? 0xffffffff : 44, true);
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true);
  view.setUint16(22, 1, true);
  view.setUint32(24, 24000, true);
  view.setUint32(28, 48000, true);
  view.setUint16(32, 2, true);
  view.setUint16(34, 16, true);
  view.setUint32(40, streaming ? 0xffffffff : 8, true);
  [-32768, 0, 16384, 32767].forEach((sample, index) => view.setInt16(44 + index * 2, sample, true));
  return bytes;
}

describe("PCM streaming decoder", () => {
  it.each([false, true])("decodes a WAV across every possible header/sample split (streaming=%s)", (streaming) => {
    const bytes = wav(streaming);
    for (let split = 1; split < bytes.length; split++) {
      const decoder = new PCMStreamDecoder("wav");
      const first = decoder.push(bytes.slice(0, split));
      const second = decoder.push(bytes.slice(split));
      decoder.finish();
      expect([...(first?.samples ?? []), ...(second?.samples ?? [])]).toEqual([-1, 0, 0.5, 32767 / 32768]);
    }
  });

  it("rejects truncated samples and unsupported WAV encodings before playback", () => {
    const decoder = new PCMStreamDecoder("pcm_s16le_24000");
    decoder.push(new Uint8Array([1]));
    expect(() => decoder.finish()).toThrow("complete sample");
    const floating = wav();
    new DataView(floating.buffer).setUint16(20, 3, true);
    expect(() => new PCMStreamDecoder("wav").push(floating)).toThrow(UnsupportedPCMStream);
  });

  it("owns incomplete header and sample bytes when callers reuse an input buffer", () => {
    for (const split of [8, 44, 45, 49]) {
      const bytes = wav();
      const decoder = new PCMStreamDecoder("wav");
      const first = decoder.push(bytes.subarray(0, split));
      bytes.fill(255, 0, split);
      const last = decoder.push(bytes.subarray(split));
      decoder.finish();
      expect([...(first?.samples ?? []), ...(last?.samples ?? [])]).toEqual([-1, 0, 0.5, 32767 / 32768]);
    }
    const raw = new PCMStreamDecoder("pcm_s16le_24000");
    const bytes = new Uint8Array([0]);
    raw.push(bytes);
    bytes[0] = 255;
    expect(raw.push(new Uint8Array([64]))?.samples[0]).toBe(0.5);
    raw.finish();
  });
});
