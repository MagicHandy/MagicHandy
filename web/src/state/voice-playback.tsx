import { createContext, useCallback, useContext, useEffect, useMemo, useRef, type ReactNode } from "react";
import { api } from "../api/client";
import { audioPlaybackToken, installAudioPlaybackUnlock, playBlob, stopAllAudioPlayback } from "../util/audio";
import { useToast } from "./app-state";

interface VoicePlaybackValue {
  queueSpeech: (requestId: string) => void;
}

const VoicePlaybackContext = createContext<VoicePlaybackValue | null>(null);
const REQUEST_POLL_MS = 250;

interface SpeechQueueEntry {
  id: string;
  controller: AbortController;
  audio: Promise<SpeechAudioResult>;
}

type SpeechAudioResult =
  | { ok: true; audio: Blob }
  | { ok: false; error?: unknown };

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
    if (request?.role !== "tts" || request.type !== "speak") {
      throw new Error("the voice worker returned the wrong request");
    }
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

async function prepareSpeech(requestId: string, signal: AbortSignal): Promise<SpeechAudioResult> {
  try {
    if (!await waitForSpeech(requestId, signal)) return { ok: false };
    return { ok: true, audio: await api.voiceRequestAudio(requestId, signal) };
  } catch (error) {
    return { ok: false, error };
  }
}

export function VoicePlaybackProvider({ children }: { children: ReactNode }) {
  const { show } = useToast();
  const pending = useRef<SpeechQueueEntry[]>([]);
  const tracked = useRef(new Set<string>());
  const controllers = useRef(new Map<string, AbortController>());
  const draining = useRef(false);
  const disposed = useRef(false);

  const drain = useCallback(async () => {
    if (draining.current) return;
    draining.current = true;
    try {
      while (pending.current.length > 0) {
        const entry = pending.current.shift();
        if (!entry) continue;
        try {
          const result = await entry.audio;
          if (!result.ok) {
            if (result.error && !isAbort(result.error) && !disposed.current) {
              const reason = result.error instanceof Error ? result.error.message : "unknown playback error";
              show(`Speech output could not play: ${reason}.`, "error");
            }
            continue;
          }
          const playbackToken = audioPlaybackToken();
          await playBlob(result.audio, playbackToken);
        } catch (error) {
          if (!isAbort(error) && !disposed.current) {
            const reason = error instanceof Error ? error.message : "unknown playback error";
            show(`Speech output could not play: ${reason}.`, "error");
          }
        } finally {
          if (controllers.current.get(entry.id) === entry.controller) {
            tracked.current.delete(entry.id);
            controllers.current.delete(entry.id);
          }
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
    const controller = new AbortController();
    controllers.current.set(id, controller);
    pending.current.push({
      id,
      controller,
      audio: prepareSpeech(id, controller.signal),
    });
    void drain();
  }, [drain]);

  useEffect(() => {
    disposed.current = false;
    const removePlaybackUnlock = installAudioPlaybackUnlock();
    const cancel = () => {
      pending.current = [];
      tracked.current.clear();
      controllers.current.forEach((controller) => controller.abort());
      controllers.current.clear();
      stopAllAudioPlayback();
    };
    window.addEventListener("magichandy:emergency-stop", cancel);
    return () => {
      disposed.current = true;
      removePlaybackUnlock();
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
