// Emergency Stop. Mounted once, outside routed content (in the nav rail footer),
// so it is present on every route and reachable by read-only/offline clients.
// Esc is the documented global shortcut. See docs/ui-design.md (Emergency Stop).
import { useCallback, useEffect } from "react";
import { api } from "../api/client";
import { useAppState, useToast } from "../state/app-state";
import { StopIcon } from "./icons";

export function StopButton({ className = "" }: { className?: string }) {
  const { show } = useToast();
  const { refresh } = useAppState();

  const stop = useCallback(async () => {
    try {
      await api.stopMotion();
      show("Stopped.");
    } catch {
      show("Stop request failed — check the connection.", "error");
    } finally {
      refresh();
    }
  }, [show, refresh]);

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
