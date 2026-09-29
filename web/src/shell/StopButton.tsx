import { t, translateKnown } from "../i18n";
// Emergency Stop. Mounted once, outside routed content (in the nav rail footer),
// so it is present on every route and reachable by read-only/offline clients.
// Esc is the documented global shortcut. See docs/ui-design.md (Emergency Stop).
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { api, ApiError } from "../api/client";
import { useAppState, useToast } from "../state/app-state";
import { stopAllAudioPlayback } from "../util/audio";
import { StopIcon } from "./icons";

export function StopButton({ className = "" }: { className?: string }) {
  const { show } = useToast();
  const { refresh, state } = useAppState();
  const lastStopSequence = useRef<number | undefined>(undefined);
  const [fullscreen, setFullscreen] = useState(() => document.fullscreenElement);
  useLayoutEffect(() => {
    const update = () => setFullscreen(document.fullscreenElement);
    document.addEventListener("fullscreenchange", update);
    return () => document.removeEventListener("fullscreenchange", update);
  }, []);

  const stop = useCallback(async () => {
    // Browser-owned microphone capture must stop immediately, without waiting
    // for the backend round trip. /api/state carries stop_sequence to the other
    // clients after the backend accepts the same emergency-stop activation.
    stopAllAudioPlayback();
    window.dispatchEvent(new Event("magichandy:emergency-stop"));
    try {
      const result = await api.stopMotion();
      show(result?.error ? translateKnown(result.error) : t("Stopped."), result?.error ? "error" : "info");
    } catch (error) {
      const message = error instanceof ApiError
        ? translateKnown(error.message)
        : t("Stop request failed; check the connection.");
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

  const button = (
    <button
      type="button"
      className={`stop-button ${fullscreen ? "fullscreen-stop" : className}`.trim()}
      data-emergency-stop
      onClick={() => void stop()}
      aria-label={t("Emergency stop all motion")}
    >
      <StopIcon size={21} />
      <span className="stop-button-label">{t("Stop")}</span>
      <span className="kbd" aria-hidden="true">{t("Esc")}</span>
    </button>
  );
  // Fullscreen hides everything outside its root, regardless of z-index.
  // Move this same global control into it; do not add another Stop listener.
  return fullscreen ? createPortal(button, fullscreen) : button;
}
