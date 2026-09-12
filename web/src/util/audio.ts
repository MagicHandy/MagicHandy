// Shared clip playback for completed TTS requests (chat speak-replies and the
// settings test button). The context is unlocked during a real user gesture so
// completed asynchronous requests are not rejected by browser autoplay policy.
import type { PCMChunk } from "./pcm-stream";
import { SpeechBuffer } from "./speech-buffer";
const activePlayback = new Map<AudioBufferSourceNode, () => void>();
let playbackContext: AudioContext | null = null;
let playbackGeneration = 0;

function context(): AudioContext {
  playbackContext ??= new AudioContext({ latencyHint: "interactive" });
  return playbackContext;
}

async function resumeContext(): Promise<AudioContext> {
  const audioContext = context();
  if (audioContext.state !== "running") await audioContext.resume();
  if (audioContext.state !== "running") {
    throw new Error("the browser requires a click or key press before audio playback");
  }
  return audioContext;
}

export function installAudioPlaybackUnlock(): () => void {
  const unlock = () => {
    void resumeContext().catch(() => undefined);
  };
  window.addEventListener("pointerdown", unlock, true);
  window.addEventListener("keydown", unlock, true);
  return () => {
    window.removeEventListener("pointerdown", unlock, true);
    window.removeEventListener("keydown", unlock, true);
  };
}

export function audioPlaybackToken() {
  return playbackGeneration;
}

export async function playBlob(blob: Blob, token = playbackGeneration): Promise<void> {
  if (token !== playbackGeneration) return Promise.resolve();
  const audioContext = await resumeContext();
  const payload = await blob.arrayBuffer();
  const decoded = await audioContext.decodeAudioData(payload);
  if (token !== playbackGeneration) return;

  return new Promise((resolve, reject) => {
    const source = audioContext.createBufferSource();
    source.buffer = decoded;
    source.connect(audioContext.destination);
    let settled = false;
    const finish = (error?: Error) => {
      if (settled) return;
      settled = true;
      activePlayback.delete(source);
      source.disconnect();
      if (error) reject(error);
      else resolve();
    };
    activePlayback.set(source, () => finish());
    source.onended = () => finish();
    try {
      source.start();
    } catch (error) {
      const reason = error instanceof Error ? error.message : "the browser could not start audio playback";
      finish(new Error(reason));
    }
  });
}

export function stopAllAudioPlayback() {
  playbackGeneration++;
  window.dispatchEvent(new Event("magichandy:audio-stop"));
  for (const [source, finish] of [...activePlayback]) {
    try {
      source.stop();
    } catch {
      // The source may already have reached its natural end.
    }
    finish();
  }
}

// Progressive speech shares the same context, Stop generation and active-node
// registry as complete clips, with a bounded reserve for uneven delivery.
export async function playPCMChunks(chunks: AsyncIterable<PCMChunk>, token: number, signal: AbortSignal): Promise<void> {
  const stopped = new AbortController();
  const abort = () => stopped.abort();
  signal.addEventListener("abort", abort, { once: true });
  window.addEventListener("magichandy:audio-stop", abort);
  if (signal.aborted || token !== playbackGeneration) abort();
  try {
    await playPCMStream(chunks, stopped.signal);
  } finally {
    signal.removeEventListener("abort", abort);
    window.removeEventListener("magichandy:audio-stop", abort);
  }
}

async function playPCMStream(chunks: AsyncIterable<PCMChunk>, signal: AbortSignal): Promise<void> {
  const check = () => {
    if (signal.aborted) throw new DOMException("Aborted", "AbortError");
  };
  check();
  const audioContext = await abortable(resumeContext(), signal);
  check();
  const local = new Map<AudioBufferSourceNode, () => void>();
  const endings = new Set<Promise<void>>();
  let nextStart = audioContext.currentTime;
  const stop = () => {
    for (const [source, finish] of [...local]) {
      try { source.stop(); } catch { /* Already stopped. */ }
      finish();
    }
  };
  const iterator = chunks[Symbol.asyncIterator]();
  const buffered = new SpeechBuffer();
  async function schedule(chunk: PCMChunk) {
    check();
    while (nextStart - audioContext.currentTime > 1.25 && endings.size) {
      await abortable(Promise.race(endings), signal);
      check();
    }
    const frames = chunk.samples.length / chunk.channels;
    if (!frames) return;
    const buffer = audioContext.createBuffer(chunk.channels, frames, chunk.sampleRate);
    for (let channel = 0; channel < chunk.channels; channel++) {
      const output = buffer.getChannelData(channel);
      if (chunk.channels === 1) output.set(chunk.samples);
      else for (let frame = 0; frame < frames; frame++) output[frame] = chunk.samples[frame * chunk.channels + channel];
    }
    const source = audioContext.createBufferSource();
    source.buffer = buffer;
    source.connect(audioContext.destination);
    let resolveEnd!: () => void;
    const ended = new Promise<void>((resolve) => { resolveEnd = resolve; });
    let settled = false;
    const finish = () => {
      if (settled) return;
      settled = true;
      local.delete(source);
      activePlayback.delete(source);
      endings.delete(ended);
      source.disconnect();
      resolveEnd();
    };
    endings.add(ended);
    local.set(source, finish);
    activePlayback.set(source, finish);
    source.onended = finish;
    // Reserve startup headroom only when starting/restarting. Reapplying it
    // to an on-time chunk near the boundary inserts an artificial gap.
    if (nextStart <= audioContext.currentTime) nextStart = audioContext.currentTime + 0.035;
    source.start(nextStart);
    nextStart += frames / chunk.sampleRate;
  }
  try {
    for (;;) {
      const result = await abortable(iterator.next(), signal);
      check();
      if (result.done) break;
      for (const chunk of buffered.push(result.value, nextStart <= audioContext.currentTime)) await schedule(chunk);
    }
    for (const chunk of buffered.flush()) await schedule(chunk);
    await abortable(Promise.all(endings), signal);
    check();
  } finally {
    stop();
    // Do not make Stop wait for a producer that is stuck in its next read.
    void iterator.return?.().catch(() => undefined);
  }
}

function abortable<T>(promise: Promise<T>, signal: AbortSignal): Promise<T> {
  return new Promise((resolve, reject) => {
    const abort = () => reject(new DOMException("Aborted", "AbortError"));
    promise.then((value) => { signal.removeEventListener("abort", abort); resolve(value); },
      (error) => { signal.removeEventListener("abort", abort); reject(error); });
    if (signal.aborted) abort();
    else signal.addEventListener("abort", abort, { once: true });
  });
}
