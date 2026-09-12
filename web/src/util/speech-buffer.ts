import type { PCMChunk } from "./pcm-stream";

// A small startup reserve absorbs packet/decode jitter. If inference falls
// behind playback, refill a larger reserve instead of restarting on every word.
// This trades some latency for continuity; it cannot make a slow model faster.
export class SpeechBuffer {
  private chunks: PCMChunk[] = [];
  private seconds = 0;
  private target = 0.75;
  private buffering = true;
  private started = false;

  push(chunk: PCMChunk, starved: boolean): PCMChunk[] {
    if (!chunk.samples.length) return [];
    if (starved && this.started && !this.buffering) {
      this.buffering = true;
      this.target = Math.min(3, this.target * 2);
    }
    this.chunks.push(chunk);
    this.seconds += chunk.samples.length / (chunk.sampleRate * chunk.channels);
    return !this.buffering || this.seconds >= this.target ? this.flush() : [];
  }

  flush(): PCMChunk[] {
    const chunks = this.chunks;
    this.chunks = [];
    this.seconds = 0;
    this.buffering = false;
    this.started ||= chunks.length > 0;
    return chunks;
  }
}
