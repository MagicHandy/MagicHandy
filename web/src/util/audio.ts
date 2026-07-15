// Shared clip playback for completed TTS requests (chat speak-replies and the
// settings test button). Resolves when playback ends or Emergency Stop cancels
// it. Media failures reject so the UI can tell the user that no sound played.
const activePlayback = new Map<HTMLAudioElement, () => void>();
let playbackGeneration = 0;

export function audioPlaybackToken() {
  return playbackGeneration;
}

export function playBlob(blob: Blob, token = playbackGeneration): Promise<void> {
  if (token !== playbackGeneration) return Promise.resolve();
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(blob);
    const audio = new Audio(url);
    let settled = false;
    const finish = (error?: Error) => {
      if (settled) return;
      settled = true;
      activePlayback.delete(audio);
      URL.revokeObjectURL(url);
      if (error) reject(error);
      else resolve();
    };
    activePlayback.set(audio, () => finish());
    audio.onended = () => finish();
    audio.onerror = () => finish(new Error("the browser could not decode the generated audio"));
    void audio.play().catch((error: unknown) => {
      const reason = error instanceof Error ? error.message : "the browser blocked audio playback";
      finish(new Error(reason));
    });
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
