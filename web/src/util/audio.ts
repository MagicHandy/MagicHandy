// Shared clip playback for completed TTS requests (chat speak-replies and the
// settings test button). The context is unlocked during a real user gesture so
// completed asynchronous requests are not rejected by browser autoplay policy.
import type { PCMChunk } from "./pcm-stream";
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
// registry as complete clips. At most about two seconds are scheduled ahead.
export async function playPCMChunks(chunks: AsyncIterable<PCMChunk>, token: number, signal: AbortSignal): Promise<void> {
  const check = () => {
    if (signal.aborted || token !== playbackGeneration) throw new DOMException("Aborted", "AbortError");
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
  signal.addEventListener("abort", stop, { once: true });
  try {
    for await (const chunk of chunks) {
      check();
      while (nextStart - audioContext.currentTime > 1.25 && endings.size) {
        await Promise.race(endings);
        check();
      }
      const frames = chunk.samples.length / chunk.channels;
      if (!frames) continue;
      const buffer = audioContext.createBuffer(chunk.channels, frames, chunk.sampleRate);
      for (let channel = 0; channel < chunk.channels; channel++) {
        const output = buffer.getChannelData(channel);
        for (let frame = 0; frame < frames; frame++) output[frame] = chunk.samples[frame * chunk.channels + channel];
      }
      const source = audioContext.createBufferSource();
      source.buffer = buffer;
      source.connect(audioContext.destination);
      let resolveEnd!: () => void;
      const ended = new Promise<void>((resolve) => { resolveEnd = resolve; });
      const finish = () => {
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
      nextStart = Math.max(nextStart, audioContext.currentTime + 0.035);
      source.start(nextStart);
      nextStart += frames / chunk.sampleRate;
    }
    await Promise.all(endings);
    check();
  } finally {
    signal.removeEventListener("abort", stop);
    stop();
  }
}

function abortable<T>(promise: Promise<T>, signal: AbortSignal): Promise<T> {
  return new Promise((resolve, reject) => {
    const abort = () => reject(new DOMException("Aborted", "AbortError"));
    if (signal.aborted) { abort(); return; }
    signal.addEventListener("abort", abort, { once: true });
    promise.then((value) => { signal.removeEventListener("abort", abort); resolve(value); },
      (error) => { signal.removeEventListener("abort", abort); reject(error); });
  });
}
