import { createContext, useCallback, useContext, useEffect, useMemo, useRef, type ReactNode } from "react";
import { api } from "../api/client";
import { audioPlaybackToken, playBlob } from "../util/audio";
import { useToast } from "./app-state";

interface VoicePlaybackValue {
  queueSpeech: (requestId: string) => void;
}

const VoicePlaybackContext = createContext<VoicePlaybackValue | null>(null);
const REQUEST_POLL_MS = 1000;

function isAbort(error: unknown): boolean {
  return typeof error === "object" && error !== null && "name" in error && error.name === "AbortError";
}

function delay(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) {
      reject(signal.reason ?? new DOMException("Aborted", "AbortError"));
      return;
    }
    const onAbort = () => {
      window.clearTimeout(timer);
      reject(signal.reason ?? new DOMException("Aborted", "AbortError"));
    };
    const timer = window.setTimeout(() => {
      signal.removeEventListener("abort", onAbort);
      resolve();
    }, ms);
    signal.addEventListener("abort", onAbort, { once: true });
  });
}

async function waitForSpeech(requestId: string, signal: AbortSignal): Promise<boolean> {
  // The worker owns its inference timeout. NeuTTS cold starts can take more
  // than a minute, so a second, shorter browser deadline would discard valid
  // audio while the backend is still actively producing it.
  for (;;) {
    const response = await api.voiceRequest(requestId, signal);
    const request = response.request;
    switch (request?.state) {
      case "done":
        if ((request.audio_bytes ?? 0) <= 0) {
          throw new Error("the worker completed without returning audio");
        }
        return true;
      case "failed":
        throw new Error(request.error?.message || "the speech worker failed");
      case "canceled":
        return false;
      default:
        await delay(REQUEST_POLL_MS, signal);
    }
  }
}

export function VoicePlaybackProvider({ children }: { children: ReactNode }) {
  const { show } = useToast();
  const pending = useRef<string[]>([]);
  const tracked = useRef(new Set<string>());
  const draining = useRef(false);
  const activeRequest = useRef<AbortController | null>(null);

  const drain = useCallback(async () => {
    if (draining.current) return;
    draining.current = true;
    try {
      while (pending.current.length > 0) {
        const requestId = pending.current.shift();
        if (!requestId) continue;
        const controller = new AbortController();
        activeRequest.current = controller;
        try {
          if (!await waitForSpeech(requestId, controller.signal)) continue;
          const playbackToken = audioPlaybackToken();
          const audio = await api.voiceRequestAudio(requestId, controller.signal);
          await playBlob(audio, playbackToken);
        } catch (error) {
          if (!isAbort(error)) {
            const reason = error instanceof Error ? error.message : "unknown playback error";
            show(`Speech output could not play: ${reason}.`, "error");
          }
        } finally {
          tracked.current.delete(requestId);
          if (activeRequest.current === controller) activeRequest.current = null;
        }
      }
    } finally {
      draining.current = false;
    }
  }, [show]);

  const queueSpeech = useCallback((requestId: string) => {
    const id = requestId.trim();
    if (!id || tracked.current.has(id)) return;
    tracked.current.add(id);
    pending.current.push(id);
    void drain();
  }, [drain]);

  useEffect(() => {
    const cancel = () => {
      pending.current = [];
      tracked.current.clear();
      activeRequest.current?.abort();
      activeRequest.current = null;
    };
    window.addEventListener("magichandy:emergency-stop", cancel);
    return () => {
      window.removeEventListener("magichandy:emergency-stop", cancel);
      cancel();
    };
  }, []);

  const value = useMemo(() => ({ queueSpeech }), [queueSpeech]);
  return <VoicePlaybackContext.Provider value={value}>{children}</VoicePlaybackContext.Provider>;
}

export function useVoicePlayback(): VoicePlaybackValue {
  const value = useContext(VoicePlaybackContext);
  if (!value) throw new Error("useVoicePlayback must be used within VoicePlaybackProvider");
  return value;
}
