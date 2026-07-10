import { useEffect } from "react";

/** Global Esc shortcut for emergency stop — present on every route. */
export function useGlobalEscStop(onStop: () => void | Promise<void>) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      e.preventDefault();
      void onStop();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onStop]);
}
