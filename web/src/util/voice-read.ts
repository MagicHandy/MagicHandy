// Audio/status reads are idempotent. Retry a brief network interruption at the
// same byte offset; never retry synthesis, expired permissions, or cancellation.
export async function readVoice<T>(read: () => Promise<T>, signal: AbortSignal): Promise<T> {
  for (let attempt = 0; ; attempt++) {
    if (signal.aborted) throw new DOMException("Aborted", "AbortError");
    try { return await read(); } catch (error) {
      if (signal.aborted) throw new DOMException("Aborted", "AbortError");
      const status = typeof error === "object" && error !== null && "status" in error ? error.status : undefined;
      const transient = error instanceof TypeError || status === 502 || status === 503 || status === 504;
      if (!transient || attempt >= 3) throw error;
      await waitForVoice(200 * 2 ** attempt, signal);
    }
  }
}

export function waitForVoice(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const abort = () => { clearTimeout(timer); reject(new DOMException("Aborted", "AbortError")); };
    const timer = setTimeout(() => { signal.removeEventListener("abort", abort); resolve(); }, ms);
    if (signal.aborted) abort();
    else signal.addEventListener("abort", abort, { once: true });
  });
}
