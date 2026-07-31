import { t, translateKnown } from "../i18n";
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, type ReactNode } from "react";
import { api } from "../api/client";
import { audioPlaybackToken, installAudioPlaybackUnlock, playBlob, stopAllAudioPlayback } from "../util/audio";
import { useToast } from "./app-state";

interface VoicePlaybackValue {
  queueSpeech: (requestId: string) => void;
}

const VoicePlaybackContext = createContext<VoicePlaybackValue | null>(null);
const REQUEST_POLL_MS = 250;
const normalizePlaybackReason = (reason: string) => reason.replace(/[.!?。！？]+$/u, "");

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
  // The worker owns its inference timeout. Local model startup can take
  // minutes, so a second browser deadline would discard a valid request while
  // the backend is still loading or generating audio.
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
        // Acknowledged on any terminal outcome, not only on success. Autopilot
        // starts its next speech interval from this signal, so skipping it after
        // failed synthesis or blocked autoplay left the backend waiting out its
        // full two-minute fallback — turning a Talkative 15-60s cadence into one
        // line every two minutes with nothing on screen to explain it. Autoplay
        // blocking is the common case: Chrome rejects play() without a prior user
        // gesture. Whether the audio was heard or not, the turn is over.
        let acknowledge = false;
        try {
          const result = await entry.audio;
          if (!result.ok) {
            // prepareSpeech reports a worker-side cancellation as "not ok" with no
            // error at all, and an abort as an AbortError. Neither is a completed
            // turn — the backend cancels and reschedules those itself — so only a
            // genuine failure is acknowledged.
            acknowledge = result.error !== undefined && !isAbort(result.error);
            if (result.error && !isAbort(result.error) && !disposed.current) {
              const reason = normalizePlaybackReason(result.error instanceof Error ? translateKnown(result.error.message) : t("unknown playback error"));
              show(t("Speech output could not play: {reason}.", { reason }), "error");
            }
            continue;
          }
          const playbackToken = audioPlaybackToken();
          await playBlob(result.audio, playbackToken);
          acknowledge = true;
        } catch (error) {
          acknowledge = !isAbort(error);
          if (!isAbort(error) && !disposed.current) {
            const reason = normalizePlaybackReason(error instanceof Error ? translateKnown(error.message) : t("unknown playback error"));
            show(t("Speech output could not play: {reason}.", { reason }), "error");
          }
        } finally {
          if (acknowledge) {
            try {
              await api.voiceRequestPlayed(entry.id);
            } catch {
              // The bounded backend fallback covers a genuinely lost call, which
              // is what it was always meant to be for.
            }
          }
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
