// Shared clip playback for completed TTS requests (chat speak-replies and the
// settings test button). Resolves when playback ends or fails; a broken clip
// never blocks the caller's queue.
export function playBlob(blob: Blob): Promise<void> {
  return new Promise((resolve) => {
    const url = URL.createObjectURL(blob);
    const audio = new Audio(url);
    const finish = () => {
      URL.revokeObjectURL(url);
      resolve();
    };
    audio.onended = finish;
    audio.onerror = finish;
    void audio.play().catch(finish);
  });
}
