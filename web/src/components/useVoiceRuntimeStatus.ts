// The Voice settings page owns one status poll so ASR, TTS and the request
// queue render one coherent backend snapshot (ADR 0003).
import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { VoiceModuleStatus, VoiceRequestSnapshot, VoiceWorkerStatus } from "../api/types";
import { t, translateKnown } from "../i18n";

export const VOICE_STATUS_POLL_MS = 3000;

interface Snapshot {
  workers: Record<string, VoiceWorkerStatus>;
  modules: Record<string, VoiceModuleStatus>;
  requests: VoiceRequestSnapshot[];
  requestedAt: number;
  receivedAt: number;
}

export interface VoiceRuntimeStatus {
  workers: Record<string, VoiceWorkerStatus>;
  modules: Record<string, VoiceModuleStatus>;
  requests: VoiceRequestSnapshot[];
  /** At least one snapshot has arrived. */
  loaded: boolean;
  /** The newest request failed; any snapshot shown is the last one received. */
  error: string;
  /** When the shown snapshot was requested: anything begun before it may be in it. */
  requestedAt?: number;
  receivedAt?: number;
  /** A Retry the user asked for is running. */
  retrying: boolean;
  /** Fetch a snapshot that starts after every action already sent. */
  refresh: () => Promise<void>;
  retry: () => Promise<void>;
}

// fetch() rejects with a TypeError when the server cannot be reached; its text
// is browser-specific, so it is replaced rather than shown.
export function voiceStatusError(reason: unknown): string {
  if (reason instanceof TypeError) return t("MagicHandy could not be reached.");
  if (reason instanceof Error && reason.message) return translateKnown(reason.message);
  return t("Request failed");
}

const EMPTY_WORKERS: Record<string, VoiceWorkerStatus> = {};
const EMPTY_MODULES: Record<string, VoiceModuleStatus> = {};
const EMPTY_REQUESTS: VoiceRequestSnapshot[] = [];

export function useVoiceRuntimeStatus(): VoiceRuntimeStatus {
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [error, setError] = useState("");
  const [retrying, setRetrying] = useState(false);
  const mounted = useRef(false);
  const issued = useRef(0);
  const applied = useRef(0);
  const inFlight = useRef<Promise<void> | null>(null);
  const followUp = useRef<Promise<void> | null>(null);

  // Requests are serialized, so responses normally arrive in order. The
  // sequence check still keeps a response from an older request (for example
  // one that outlived a remount) from replacing a newer result.
  const fetchSnapshot = useCallback(async () => {
    const sequence = ++issued.current;
    const requestedAt = Date.now();
    try {
      const response = await api.voiceStatus();
      if (!mounted.current || sequence < applied.current) return;
      applied.current = sequence;
      setSnapshot({
        workers: response?.voice?.workers ?? EMPTY_WORKERS,
        modules: response?.voice?.modules ?? EMPTY_MODULES,
        requests: response?.requests ?? EMPTY_REQUESTS,
        requestedAt,
        receivedAt: Date.now(),
      });
      setError("");
    } catch (reason) {
      if (!mounted.current || sequence < applied.current) return;
      applied.current = sequence;
      setError(voiceStatusError(reason));
    }
  }, []);

  const start = useCallback(() => {
    const request = fetchSnapshot().finally(() => {
      if (inFlight.current === request) inFlight.current = null;
    });
    inFlight.current = request;
    return request;
  }, [fetchSnapshot]);

  // Polling joins a request that is already running.
  const join = useCallback(() => inFlight.current ?? start(), [start]);

  // After an action, a request that was already running may have been read
  // before the action finished. Queue exactly one fresh request behind it.
  const refresh = useCallback((): Promise<void> => {
    const running = inFlight.current;
    if (!running) return start();
    if (!followUp.current) {
      const next = running.then(() => {
        followUp.current = null;
        return start();
      });
      followUp.current = next;
    }
    return followUp.current;
  }, [start]);

  const retry = useCallback(async () => {
    setRetrying(true);
    try {
      await refresh();
    } finally {
      if (mounted.current) setRetrying(false);
    }
  }, [refresh]);

  useEffect(() => {
    mounted.current = true;
    let canceled = false;
    let timer: number | undefined;
    // A hidden document stops polling; returning refreshes immediately.
    const tick = async () => {
      if (!document.hidden) await join();
      if (!canceled) timer = window.setTimeout(() => void tick(), VOICE_STATUS_POLL_MS);
    };
    const onVisibilityChange = () => {
      if (!document.hidden) void refresh();
    };
    void tick();
    document.addEventListener("visibilitychange", onVisibilityChange);
    return () => {
      canceled = true;
      mounted.current = false;
      window.clearTimeout(timer);
      document.removeEventListener("visibilitychange", onVisibilityChange);
    };
  }, [join, refresh]);

  return {
    workers: snapshot?.workers ?? EMPTY_WORKERS,
    modules: snapshot?.modules ?? EMPTY_MODULES,
    requests: snapshot?.requests ?? EMPTY_REQUESTS,
    loaded: snapshot !== null,
    error,
    requestedAt: snapshot?.requestedAt,
    receivedAt: snapshot?.receivedAt,
    retrying,
    refresh,
    retry,
  };
}
