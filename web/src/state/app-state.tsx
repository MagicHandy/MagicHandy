// Backend-authoritative app state. Polls /api/state, streams live motion over
// SSE, tracks backend availability and the controller read-only lock, and hosts
// the single toast channel. React holds no parallel motion/settings model.
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { api, clientId } from "../api/client";
import type { AppState, MotionInfo } from "../api/types";

interface AppStateValue {
  state: AppState | null;
  backendOnline: boolean;
  stale: boolean;
  motion: MotionInfo | null;
  readOnly: boolean;
  startupError: string;
  refresh: () => Promise<void>;
}

const AppStateContext = createContext<AppStateValue | null>(null);

const POLL_MS = 2000;
const STATE_TIMEOUT_MS = 8000;

export function AppStateProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AppState | null>(null);
  const [backendOnline, setBackendOnline] = useState(true);
  const [stale, setStale] = useState(false);
  const [liveMotion, setLiveMotion] = useState<MotionInfo | null>(null);
  const [startupError, setStartupError] = useState("");
  const inFlight = useRef<Promise<void> | null>(null);
  const activeRequest = useRef<AbortController | null>(null);

  const performRefresh = useCallback((): Promise<void> => {
    if (inFlight.current) return inFlight.current;
    const controller = new AbortController();
    activeRequest.current = controller;
    const timeout = window.setTimeout(() => controller.abort(), STATE_TIMEOUT_MS);
    const task = (async () => {
      try {
        const next = await api.getState(controller.signal);
        if (controller.signal.aborted) return;
        setState(next);
        setBackendOnline(true);
        setStale(false);
        setStartupError("");
      } catch (error) {
        if (controller.signal.aborted && activeRequest.current !== controller) return;
        setBackendOnline(false);
        setStale(true);
        setStartupError(error instanceof DOMException && error.name === "AbortError"
          ? "The core is taking longer than expected to become ready."
          : "The core did not return its startup state.");
      } finally {
        window.clearTimeout(timeout);
        if (activeRequest.current === controller) activeRequest.current = null;
      }
    })();
    const tracked = task.finally(() => {
      if (inFlight.current === tracked) inFlight.current = null;
    });
    inFlight.current = tracked;
    return tracked;
  }, []);

  const refresh = useCallback(async () => {
    if (inFlight.current) await inFlight.current;
    await performRefresh();
  }, [performRefresh]);

  useEffect(() => {
    let stopped = false;
    let timer: number | undefined;
    const poll = async () => {
      await performRefresh();
      if (!stopped) timer = window.setTimeout(() => void poll(), POLL_MS);
    };
    void poll();
    return () => {
      stopped = true;
      window.clearTimeout(timer);
      const controller = activeRequest.current;
      activeRequest.current = null;
      controller?.abort();
    };
  }, [performRefresh]);

  // Live motion over SSE for a responsive visualizer; the poll snapshot remains
  // the source of truth and reconciles this between events.
  useEffect(() => {
    let source: EventSource | null = null;
    try {
      source = new EventSource(`/api/motion/events?client_id=${encodeURIComponent(clientId)}`);
      source.addEventListener("motion", (ev) => {
        try {
          setLiveMotion(JSON.parse((ev as MessageEvent).data) as MotionInfo);
        } catch {
          /* ignore */
        }
      });
      source.onerror = () => setLiveMotion(null);
    } catch {
      source = null;
    }
    return () => source?.close();
  }, []);

  const controller = state?.controller;
  const readOnly = controller ? controller.read_only === true : false;
  const motion = liveMotion ?? state?.motion ?? null;

  return (
    <AppStateContext.Provider value={{ state, backendOnline, stale, motion, readOnly, startupError, refresh }}>
      {children}
    </AppStateContext.Provider>
  );
}

export function useAppState(): AppStateValue {
  const value = useContext(AppStateContext);
  if (!value) throw new Error("useAppState must be used within AppStateProvider");
  return value;
}

// ---- Toast (single feedback channel) ----
interface ToastValue {
  show: (message: string, tone?: "info" | "error") => void;
}
const ToastContext = createContext<ToastValue | null>(null);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toast, setToast] = useState<{ message: string; tone: string; visible: boolean }>({
    message: "",
    tone: "info",
    visible: false,
  });
  const timer = useRef<number | undefined>(undefined);

  const show = useCallback((message: string, tone: "info" | "error" = "info") => {
    window.clearTimeout(timer.current);
    setToast({ message, tone, visible: true });
    timer.current = window.setTimeout(() => setToast((t) => ({ ...t, visible: false })), 3200);
  }, []);

  return (
    <ToastContext.Provider value={{ show }}>
      {children}
      <div className="toast" role="status" aria-live="polite" data-visible={toast.visible} data-tone={toast.tone}>
        {toast.message}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastValue {
  const value = useContext(ToastContext);
  if (!value) throw new Error("useToast must be used within ToastProvider");
  return value;
}

// ---- Tiny hash router ----
export function useHashRoute(): string {
  const [hash, setHash] = useState(() => window.location.hash || "#/chat");
  useEffect(() => {
    const onChange = () => setHash(window.location.hash || "#/chat");
    window.addEventListener("hashchange", onChange);
    return () => window.removeEventListener("hashchange", onChange);
  }, []);
  return hash;
}
