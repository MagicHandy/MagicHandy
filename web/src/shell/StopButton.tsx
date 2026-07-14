// Emergency Stop. Mounted once, outside routed content (in the nav rail footer),
// so it is present on every route and reachable by read-only/offline clients.
// Esc is the documented global shortcut. See docs/ui-design.md (Emergency Stop).
import { useCallback, useEffect, useRef } from "react";
import { api, ApiError } from "../api/client";
import { useAppState, useToast } from "../state/app-state";
import { stopAllAudioPlayback } from "../util/audio";
import { StopIcon } from "./icons";

export function StopButton({ className = "" }: { className?: string }) {
  const { show } = useToast();
  const { refresh, state } = useAppState();
  const lastStopSequence = useRef<number | undefined>(undefined);

  const stop = useCallback(async () => {
    // Browser-owned microphone capture must stop immediately, without waiting
    // for the backend round trip. /api/state carries stop_sequence to the other
    // clients after the backend accepts the same emergency-stop activation.
    stopAllAudioPlayback();
    window.dispatchEvent(new Event("magichandy:emergency-stop"));
    try {
      const result = await api.stopMotion();
      show(result?.error ?? "Stopped.", result?.error ? "error" : "info");
    } catch (error) {
      const message = error instanceof ApiError
        ? error.message
        : "Stop request failed — check the connection.";
      show(message, "error");
    } finally {
      refresh();
    }
  }, [show, refresh]);

  useEffect(() => {
    const sequence = state?.stop_sequence;
    if (sequence === undefined) return;
    if (lastStopSequence.current !== undefined && sequence !== lastStopSequence.current) {
      stopAllAudioPlayback();
      window.dispatchEvent(new Event("magichandy:emergency-stop"));
    }
    lastStopSequence.current = sequence;
  }, [state?.stop_sequence]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        void stop();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [stop]);

  return (
    <button
      type="button"
      className={`stop-button ${className}`.trim()}
      onClick={() => void stop()}
      aria-label="Emergency stop all motion"
    >
      <StopIcon />
      <span>Stop everything</span>
      <span className="kbd" aria-hidden="true">Esc</span>
    </button>
  );
}
