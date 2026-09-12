export interface PCMChunk {
  samples: Float32Array;
  sampleRate: number;
  channels: number;
}

export class UnsupportedPCMStream extends Error {}

// Decode only ordinary 16-bit PCM WAV (or the explicit raw PCM wire format).
// Other WAV encodings retain the browser's complete-clip decoder as a fallback.
export class PCMStreamDecoder {
  private pending: Uint8Array = new Uint8Array(0);
  private headerDone = false;
  private sampleRate = 24000;
  private channels = 1;
  private remaining: number | null = null;
  private samplesDecoded = 0;

  constructor(format: string) {
    if (format === "pcm_s16le_24000") this.headerDone = true;
    else if (format !== "wav") throw new UnsupportedPCMStream("Audio format needs a complete clip");
  }

  push(bytes: Uint8Array): PCMChunk | undefined {
    if (this.headerDone && this.remaining === 0) return undefined;
    if (!this.pending.length) this.pending = bytes;
    else {
      const joined = new Uint8Array(this.pending.length + bytes.length);
      joined.set(this.pending);
      joined.set(bytes, this.pending.length);
      this.pending = joined;
    }
    if (!this.headerDone && !this.readHeader()) {
      this.pending = this.pending.slice();
      return undefined;
    }
    const available = this.remaining === null ? this.pending.length : Math.min(this.pending.length, this.remaining);
    const length = available - available % (this.channels * 2);
    if (!length) {
      this.pending = this.pending.slice();
      return undefined;
    }
    const view = new DataView(this.pending.buffer, this.pending.byteOffset, length);
    const samples = new Float32Array(length / 2);
    this.samplesDecoded += samples.length;
    for (let index = 0; index < samples.length; index++) samples[index] = view.getInt16(index * 2, true) / 32768;
    if (this.remaining !== null) this.remaining -= length;
    this.pending = this.remaining === 0 ? new Uint8Array(0) : this.pending.slice(length);
    return { samples, sampleRate: this.sampleRate, channels: this.channels };
  }

  finish() {
    if (!this.headerDone || this.pending.length > 0 || (this.remaining !== null && this.remaining !== 0)) {
      throw new Error("PCM audio stream ended before a complete sample or WAV payload");
    }
    if (!this.samplesDecoded) throw new Error("PCM stream returned no audio samples");
  }

  private readHeader(): boolean {
    if (this.pending.length < 12) return false;
    const view = new DataView(this.pending.buffer, this.pending.byteOffset, this.pending.length);
    const tag = (at: number) => String.fromCharCode(...this.pending.subarray(at, at + 4));
    if (tag(0) !== "RIFF" || tag(8) !== "WAVE") throw new UnsupportedPCMStream("Audio is not a PCM WAV stream");
    let formatFound = false;
    let offset = 12;
    while (offset + 8 <= this.pending.length) {
      const size = view.getUint32(offset + 4, true);
      if (tag(offset) === "data") {
        if (!formatFound) throw new UnsupportedPCMStream("WAV format header is missing");
        this.remaining = size === 0xffffffff ? null : size;
        this.pending = this.pending.subarray(offset + 8);
        this.headerDone = true;
        return true;
      }
      if (size > 65536 || offset + size + 8 > 65536) throw new UnsupportedPCMStream("WAV header is too large for progressive playback");
      if (offset + 8 + size > this.pending.length) return false;
      if (tag(offset) === "fmt ") {
        if (size < 16 || view.getUint16(offset + 8, true) !== 1 || view.getUint16(offset + 22, true) !== 16) {
          throw new UnsupportedPCMStream("WAV encoding needs the browser clip decoder");
        }
        this.channels = view.getUint16(offset + 10, true);
        this.sampleRate = view.getUint32(offset + 12, true);
        if (![1, 2].includes(this.channels) || this.sampleRate < 8000 || this.sampleRate > 192000 ||
            view.getUint16(offset + 20, true) !== this.channels * 2) {
          throw new UnsupportedPCMStream("Unsupported PCM channel or sample rate");
        }
        formatFound = true;
      }
      offset += 8 + size + size % 2;
    }
    if (this.pending.length > 65536) throw new UnsupportedPCMStream("WAV header exceeds 64 KiB");
    return false;
  }
}
