import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { api } from "../api/client";
import { t, translateKnown } from "../i18n";
import { validateNoticePreferences, type NoticePreferences } from "../notice-catalog";

type NoticeState = {
  hidden: string[]; temporary: string[]; loading: boolean; busy: boolean; error: string; scope: "account" | "browser";
  dismissOnce: (id: string) => void;
  save: (id: string, hidden: boolean) => Promise<void>;
  reset: () => Promise<void>;
  refresh: () => Promise<void>;
};
const NoticeContext = createContext<NoticeState | null>(null);

export function NoticePreferencesProvider({ children, enabled, scope }: { children: ReactNode; enabled: boolean; scope: "account" | "browser" }) {
  const [stored, setStored] = useState<string[]>([]);
  const [once, setOnce] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const active = useRef<{ controller: AbortController; timer: number } | null>(null);
  const alive = useRef(true);

  const run = useCallback(async (operation: "read" | "save" | "reset", id = "", hidden = false) => {
    if (!enabled || active.current) throw new Error(t("Wait for notice preferences to finish loading."));
    const request = { controller: new AbortController(), timer: 0 };
    active.current = request;
    setBusy(true); setError("");
    const timeout = new Promise<never>((_, reject) => {
      request.timer = window.setTimeout(() => { request.controller.abort(); reject(new Error("timeout")); }, 5000);
    });
    try {
      const response = operation === "read" ? api.noticePreferences(scope, request.controller.signal)
        : operation === "reset" ? api.resetNoticePreferences(scope, request.controller.signal)
          : api.saveNoticePreference(id, hidden, scope, request.controller.signal);
      const raw = await Promise.race([response, timeout]);
      if (!alive.current || active.current !== request) return;
      const next: NoticePreferences = validateNoticePreferences(raw);
      if (next.scope !== scope) throw new Error(t("Notice preferences changed account. Refresh to continue."));
      setStored(next.hidden);
      if (operation === "reset") setOnce([]);
      else if (operation === "save" && !hidden) setOnce(current => current.filter(value => value !== id));
    } catch (reason) {
      if (!alive.current || active.current !== request) return;
      const message = request.controller.signal.aborted ? t("Notice preferences timed out. Refresh to check the saved state.")
        : reason instanceof Error ? translateKnown(reason.message) : t("Request failed");
      setError(message);
      throw new Error(message);
    } finally {
      window.clearTimeout(request.timer);
      if (active.current === request) {
        active.current = null;
        if (alive.current) { setLoading(false); setBusy(false); }
      }
    }
  }, [enabled, scope]);
  const refresh = useCallback(async () => { await run("read"); }, [run]);
  useEffect(() => {
    alive.current = true;
    if (enabled) void refresh().catch(() => undefined);
    const onVisible = () => { if (enabled && document.visibilityState === "visible" && !active.current) void refresh().catch(() => undefined); };
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      alive.current = false;
      if (active.current) { window.clearTimeout(active.current.timer); active.current.controller.abort(); active.current = null; }
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [enabled, refresh]);
  return <NoticeContext.Provider value={{ hidden: [...new Set([...stored, ...once])], temporary: once.filter(id => !stored.includes(id)), loading, busy, error, scope,
    dismissOnce: id => setOnce(current => current.includes(id) ? current : [...current, id]),
    save: (id, hidden) => {
      if (!hidden && once.includes(id) && !stored.includes(id)) { setOnce(current => current.filter(value => value !== id)); return Promise.resolve(); }
      return run("save", id, hidden);
    }, reset: () => run("reset"), refresh }}>{children}</NoticeContext.Provider>;
}

// Outside the app provider, reusable panels keep their explanation visible.
// Persisted controls are available only with the backend-owned preference state.
export const useNoticePreferences = () => useContext(NoticeContext);
