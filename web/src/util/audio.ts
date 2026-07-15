// Shared clip playback for completed TTS requests (chat speak-replies and the
// settings test button). The context is unlocked during a real user gesture so
// completed asynchronous requests are not rejected by browser autoplay policy.
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
  for (const [source, finish] of [...activePlayback]) {
    try {
      source.stop();
    } catch {
      // The source may already have reached its natural end.
    }
    finish();
  }
}
