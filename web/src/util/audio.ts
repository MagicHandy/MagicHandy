// Shared clip playback for completed TTS requests (chat speak-replies and the
// settings test button). Resolves when playback ends, fails, or Emergency Stop
// cancels it; a broken clip never blocks the caller's queue.
const activePlayback = new Map<HTMLAudioElement, () => void>();
let playbackGeneration = 0;

export function audioPlaybackToken() {
  return playbackGeneration;
}

export function playBlob(blob: Blob, token = playbackGeneration): Promise<void> {
  if (token !== playbackGeneration) return Promise.resolve();
  return new Promise((resolve) => {
    const url = URL.createObjectURL(blob);
    const audio = new Audio(url);
    let settled = false;
    const finish = () => {
      if (settled) return;
      settled = true;
      activePlayback.delete(audio);
      URL.revokeObjectURL(url);
      resolve();
    };
    activePlayback.set(audio, finish);
    audio.onended = finish;
    audio.onerror = finish;
    void audio.play().catch(finish);
  });
}

export function stopAllAudioPlayback() {
  playbackGeneration++;
  for (const [audio, finish] of [...activePlayback]) {
    audio.pause();
    audio.removeAttribute("src");
    finish();
  }
}
