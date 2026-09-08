import { api } from "../api/client";
import { playPCMChunks } from "../util/audio";
import { PCMStreamDecoder, type PCMChunk } from "../util/pcm-stream";

const MAX_AUDIO_BYTES = 8 * 1024 * 1024;

export async function playSpeechStream(id: string, format: string, token: number, signal: AbortSignal) {
  const decoder = new PCMStreamDecoder(format);
  const controller = new AbortController();
  const abort = () => controller.abort();
  const parent = signal;
  if (parent.aborted) abort();
  else parent.addEventListener("abort", abort, { once: true });
  window.addEventListener("magichandy:audio-stop", abort);
  signal = controller.signal;
  async function* chunks(): AsyncGenerator<PCMChunk> {
    let offset = 0;
    for (;;) {
      if (signal.aborted) throw new DOMException("Aborted", "AbortError");
      const chunk = await api.voiceRequestAudioChunk(id, offset, signal);
      if (signal.aborted || chunk.state === "canceled") throw new DOMException("Aborted", "AbortError");
      if (chunk.state === "failed") throw new Error(chunk.error?.message || "Speech generation failed");
      const data = Uint8Array.from(atob(chunk.data ?? ""), (character) => character.charCodeAt(0));
      if (chunk.offset !== offset || chunk.next_offset !== offset + data.length || chunk.next_offset > MAX_AUDIO_BYTES || chunk.format !== format) {
        throw new Error("Speech stream returned an invalid audio sequence");
      }
      offset = chunk.next_offset;
      const pcm = decoder.push(data);
      if (pcm) yield pcm;
      if (chunk.done) { decoder.finish(); return; }
      if (!data.length) await waitForAudio(100, signal);
    }
  }
  try {
    await playPCMChunks(chunks(), token, signal);
  } finally {
    parent.removeEventListener("abort", abort);
    window.removeEventListener("magichandy:audio-stop", abort);
  }
}

function waitForAudio(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const abort = () => { clearTimeout(timer); reject(new DOMException("Aborted", "AbortError")); };
    const timer = setTimeout(() => { signal.removeEventListener("abort", abort); resolve(); }, ms);
    if (signal.aborted) abort();
    else signal.addEventListener("abort", abort, { once: true });
  });
}
