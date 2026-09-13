// Backend-authoritative app state. Polls /api/state, streams live motion over
// SSE, tracks backend availability and the controller read-only lock, and hosts
// the single feedback channel. React holds no parallel motion/settings model.
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { api, clientId, COMMAND_RECOVERED_EVENT } from "../api/client";
import type { AppState, MotionInfo, NotificationCategory } from "../api/types";
import { notificationCategories } from "../notification-preferences";
import { useControllerConnection } from "./controller-connection";
import { controllerIsNewer, validObservation } from "./observation-order";

interface AppStateValue {
  state: AppState | null;
  backendOnline: boolean;
  stale: boolean;
  readOnly: boolean;
  startupError: string;
  refresh: () => Promise<void>;
}

const AppStateContext = createContext<AppStateValue | null>(null);
const MotionStateContext = createContext<MotionInfo | null>(null);

const POLL_MS = 2000;
const STATE_TIMEOUT_MS = 8000;

export function AppStateProvider({ children, enabled = true }: { children: ReactNode; enabled?: boolean }) {
  const [state, setState] = useState<AppState | null>(null);
  const [backendOnline, setBackendOnline] = useState(true);
  const [stale, setStale] = useState(false);
  const [liveMotion, setLiveMotion] = useState<MotionInfo | null>(null);
  const [startupError, setStartupError] = useState("");
  const [streamGeneration, setStreamGeneration] = useState(0);
  const inFlight = useRef<Promise<void> | null>(null);
  const activeRequest = useRef<AbortController | null>(null);
  const pollingEnabled = useRef(false);
  const lifecycle = useRef(0);
  const motionRevision = useRef(0);
  const stateObservation = useRef<AppState | null>(null);
  const liveObservation = useRef<MotionInfo | null>(null);
  const freshnessBarrier = useRef(0);
  const connection = useControllerConnection(enabled);

  const performRefresh = useCallback((): Promise<void> => {
    if (!enabled || !pollingEnabled.current || document.visibilityState === "hidden") return Promise.resolve();
    if (inFlight.current) return inFlight.current;
    const controller = new AbortController();
    activeRequest.current = controller;
    const revisionAtStart = motionRevision.current;
    const admittedFreshness = freshnessBarrier.current;
    const timeout = window.setTimeout(() => controller.abort(), STATE_TIMEOUT_MS);
    const task = (async () => {
      try {
        const next = await api.getState(controller.signal);
        if (controller.signal.aborted || activeRequest.current !== controller) return;
        const previous = stateObservation.current;
        if (validObservation(next.observation) && validObservation(previous?.observation) &&
          next.observation.epoch === previous.observation.epoch && next.observation.revision <= previous.observation.revision) return;
        const live = liveObservation.current;
        const epochChanged = next.observation?.epoch !== previous?.observation?.epoch;
        stateObservation.current = next;
        setState(next);
        const ordered = validObservation(next.motion?.observation) && validObservation(live?.observation);
        if (epochChanged || (ordered
          ? next.motion!.observation!.revision >= live!.observation!.revision
          : motionRevision.current === revisionAtStart)) {
          liveObservation.current = null;
          setLiveMotion(null);
        }
        setBackendOnline(true);
        if (admittedFreshness === freshnessBarrier.current) {
          setStale(false);
          setStartupError("");
        }
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
  }, [enabled]);

  const resync = useCallback(() => {
    if (!enabled || !pollingEnabled.current) return;
    const previous = activeRequest.current;
    activeRequest.current = null;
    inFlight.current = null;
    previous?.abort();
    freshnessBarrier.current++;
    stateObservation.current = null;
    liveObservation.current = null;
    setState(null);
    setLiveMotion(null);
    setStale(true);
    setStreamGeneration((value) => value + 1);
    void performRefresh();
  }, [enabled, performRefresh]);

  useEffect(() => {
    const currentEpoch = stateObservation.current?.observation?.epoch;
    const observedEpoch = connection.snapshot?.epoch;
    if (currentEpoch && observedEpoch && currentEpoch !== observedEpoch) resync();
  }, [connection.snapshot, resync]);

  const refresh = useCallback(async () => {
    const admittedLifecycle = lifecycle.current;
    if (inFlight.current) await inFlight.current;
    if (lifecycle.current !== admittedLifecycle) return;
    await performRefresh();
  }, [performRefresh]);

  useEffect(() => {
    if (!enabled) return;
    const recover = () => { void refresh(); };
    window.addEventListener(COMMAND_RECOVERED_EVENT, recover);
    return () => window.removeEventListener(COMMAND_RECOVERED_EVENT, recover);
  }, [enabled, refresh]);

  useEffect(() => {
    lifecycle.current++;
    pollingEnabled.current = enabled;
    if (!enabled) {
      stateObservation.current = null;
      liveObservation.current = null;
      setState(null);
      setLiveMotion(null);
      setStale(false);
      setStartupError("");
      return;
    }
    let stopped = false;
    let timer: number | undefined;
    const poll = async () => {
      await performRefresh();
      if (!stopped) timer = window.setTimeout(() => void poll(), POLL_MS);
    };
    void poll();
    return () => {
      stopped = true;
      pollingEnabled.current = false;
      lifecycle.current++;
      window.clearTimeout(timer);
      const controller = activeRequest.current;
      activeRequest.current = null;
      inFlight.current = null;
      controller?.abort();
    };
  }, [enabled, performRefresh]);

  useEffect(() => {
    if (!enabled) return;
    const resume = () => {
      if (document.visibilityState !== "hidden") {
        resync();
      } else {
        const previous = activeRequest.current;
        activeRequest.current = null;
        inFlight.current = null;
        previous?.abort();
        freshnessBarrier.current++;
        setStale(true);
      }
    };
    document.addEventListener("visibilitychange", resume);
    window.addEventListener("pageshow", resume);
    return () => {
      document.removeEventListener("visibilitychange", resume);
      window.removeEventListener("pageshow", resume);
    };
  }, [enabled, resync]);

  // Live motion over SSE for a responsive visualizer; the poll snapshot remains
  // the source of truth and reconciles this between events.
  useEffect(() => {
    if (!enabled) return;
    let source: EventSource | null = null;
    let closed = false;
    let retryTimer: number | undefined;
    let failures = 0;
    if (document.visibilityState === "hidden") return;
    const hide = () => {
      if (document.visibilityState === "hidden") {
        closed = true;
        window.clearTimeout(retryTimer);
        source?.close();
      }
    };
    const retry = () => {
      if (closed || document.visibilityState === "hidden") return;
      failures++;
      const delay = Math.min(10000, 1000 * 2 ** Math.min(failures - 1, 4) * (0.8 + Math.random() * 0.4));
      retryTimer = window.setTimeout(connect, delay);
    };
    const connect = () => {
      if (closed || document.visibilityState === "hidden") return;
      let currentSource: EventSource;
      try {
        currentSource = new EventSource(`/api/motion/events?client_id=${encodeURIComponent(clientId)}`);
      } catch { retry(); return; }
      source = currentSource;
      const active = () => !closed && source === currentSource;
      currentSource.addEventListener("motion", (ev) => {
        if (!active()) return;
        try {
          const next = JSON.parse((ev as MessageEvent).data) as MotionInfo;
          if (typeof next?.available !== "boolean") return;
          const current = stateObservation.current;
          if (validObservation(next.observation)) {
            if (!validObservation(current?.observation)) return;
            if (next.observation.epoch !== current.observation.epoch) {
              closed = true;
              source?.close();
              resync();
              return;
            }
            const previous = liveObservation.current ?? current.motion;
            if (validObservation(previous?.observation) && next.observation.revision <= previous.observation.revision) return;
          } else if (validObservation(current?.observation) || current?.controller?.heartbeat_required) return;
          failures = 0;
          motionRevision.current++;
          liveObservation.current = next;
          setLiveMotion(next);
        } catch {
          /* ignore */
        }
      });
      currentSource.onerror = () => {
        if (active()) {
          currentSource.close();
          source = null;
          // Keep the newest known observation until a fresher snapshot arrives;
          // falling back immediately could restore an older running/stopped view.
          freshnessBarrier.current++;
          setStale(true);
          void refresh();
          retry();
        }
      };
    };
    connect();
    document.addEventListener("visibilitychange", hide);
    return () => {
      closed = true;
      window.clearTimeout(retryTimer);
      source?.close();
      document.removeEventListener("visibilitychange", hide);
    };
  }, [enabled, state?.observation?.epoch, streamGeneration, refresh, resync]);

  const controller = connection.snapshot && controllerIsNewer(connection.snapshot, state?.controller)
    ? connection.snapshot : state?.controller;
  const epochMismatch = !!(connection.snapshot?.epoch && state?.observation?.epoch && connection.snapshot.epoch !== state.observation.epoch);
  const readOnly = !state || stale || !backendOnline || epochMismatch || controller?.read_only === true ||
    (controller?.heartbeat_required === true && (!connection.fresh || !validObservation(state.observation)));
  const motion = liveMotion ?? state?.motion ?? null;
  const observedState = useMemo(() => state && controller ? { ...state, controller } : state, [state, controller]);
  const appValue = useMemo(() => ({ state: observedState, backendOnline, stale, readOnly, startupError, refresh }),
    [observedState, backendOnline, stale, readOnly, startupError, refresh]);

  return (
    <AppStateContext.Provider value={appValue}>
      <MotionStateContext.Provider value={motion}>{children}</MotionStateContext.Provider>
    </AppStateContext.Provider>
  );
}

export function useAppState(): AppStateValue {
  const value = useContext(AppStateContext);
  if (!value) throw new Error("useAppState must be used within AppStateProvider");
  return value;
}

// Subscribe only where a live backend motion observation is rendered. Keeping
// this separate prevents 125 ms events from invalidating settings/chat consumers.
export function useMotionState(): MotionInfo | null {
  return useContext(MotionStateContext);
}

// ---- Feedback: transient toast plus bounded notification history ----
export type NotificationTone = "info" | "success" | "warning" | "error";

export interface AppNotification {
  id: string;
  title: string;
  detail?: string;
  category: NotificationCategory;
  tone: NotificationTone;
  createdAt: string;
  read: boolean;
  href?: string;
  sourceKey?: string;
}

export interface NotificationDraft {
  title: string;
  detail?: string;
  category?: AppNotification["category"];
  tone?: NotificationTone;
  href?: string;
  sourceKey?: string;
}

interface ToastValue {
  show: (message: string, tone?: NotificationTone) => void;
}

interface NotificationsValue {
  items: AppNotification[];
  unreadCount: number;
  push: (notification: NotificationDraft) => void;
  markRead: (id: string) => void;
  markAllRead: () => void;
  clear: () => void;
}

const ToastContext = createContext<ToastValue | null>(null);
const NotificationsContext = createContext<NotificationsValue | null>(null);
const MAX_NOTIFICATIONS = 40;
const MAX_NOTIFICATION_SOURCE_KEYS = 160;
const NOTIFICATION_SESSION_KEY = "magichandy-notifications-v1";

interface NotificationSession {
  items: AppNotification[];
  sourceKeys: string[];
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const appState = useContext(AppStateContext);
  const [toast, setToast] = useState<{ message: string; tone: string; visible: boolean }>({
    message: "",
    tone: "info",
    visible: false,
  });
  const [initialSession] = useState(readNotificationSession);
  const [items, setItems] = useState<AppNotification[]>(initialSession.items);
  const consumedSourceKeys = useRef(new Set(initialSession.sourceKeys));
  const timer = useRef<number | undefined>(undefined);
  const sequence = useRef(0);
  const notificationPreferencesReady = Boolean(appState?.state?.settings);
  const enabledCategoryKey = notificationCategories(appState?.state?.settings?.ui?.notification_categories).join("|");
  const enabledCategories = useMemo(() => new Set(enabledCategoryKey.split("|").filter(Boolean)), [enabledCategoryKey]);

  const push = useCallback((draft: NotificationDraft) => {
    const rememberedSource = Boolean(draft.sourceKey && rememberNotificationSource(consumedSourceKeys.current, draft.sourceKey));
    if (draft.sourceKey && !rememberedSource) return;
    if (!enabledCategories.has(draft.category ?? "app")) {
      // Persist newly consumed backend event keys even when their category is
      // hidden, otherwise the same stale completion can reappear on refresh.
      if (rememberedSource) setItems((current) => current.slice());
      return;
    }
    setItems((current) => {
      const next: AppNotification = {
        id: `notification-${Date.now()}-${++sequence.current}`,
        title: draft.title,
        detail: draft.detail,
        category: draft.category ?? "app",
        tone: draft.tone ?? "info",
        createdAt: new Date().toISOString(),
        read: false,
        href: draft.href,
        sourceKey: draft.sourceKey,
      };
      return [next, ...current].slice(0, MAX_NOTIFICATIONS);
    });
  }, [enabledCategories]);

  useEffect(() => {
    if (!notificationPreferencesReady) return;
    setItems((current) => current.filter((item) => enabledCategories.has(item.category)));
  }, [enabledCategories, notificationPreferencesReady]);

  useEffect(() => {
    writeNotificationSession({
      items,
      sourceKeys: Array.from(consumedSourceKeys.current),
    });
  }, [items]);

  const show = useCallback((message: string, tone: NotificationTone = "info") => {
    window.clearTimeout(timer.current);
    setToast({ message, tone, visible: true });
    push({ title: message, tone });
    timer.current = window.setTimeout(() => setToast((t) => ({ ...t, visible: false })), 3200);
  }, [push]);

  useEffect(() => () => window.clearTimeout(timer.current), []);

  const markRead = useCallback((id: string) => {
    setItems((current) => current.map((item) => item.id === id ? { ...item, read: true } : item));
  }, []);

  const markAllRead = useCallback(() => {
    setItems((current) => current.map((item) => item.read ? item : { ...item, read: true }));
  }, []);

  const clear = useCallback(() => setItems([]), []);
  const unreadCount = items.reduce((count, item) => count + (item.read ? 0 : 1), 0);

  return (
    <NotificationsContext.Provider value={{ items, unreadCount, push, markRead, markAllRead, clear }}>
      <ToastContext.Provider value={{ show }}>
        {children}
        <div className="toast" role="status" aria-live="polite" data-visible={toast.visible} data-tone={toast.tone}>
          {toast.message}
        </div>
      </ToastContext.Provider>
    </NotificationsContext.Provider>
  );
}

function readNotificationSession(): NotificationSession {
  try {
    const raw = window.sessionStorage.getItem(NOTIFICATION_SESSION_KEY);
    if (!raw) return { items: [], sourceKeys: [] };
    const stored = JSON.parse(raw) as { items?: unknown; sourceKeys?: unknown };
    const items = Array.isArray(stored.items)
      ? stored.items.map(readStoredNotification).filter((item): item is AppNotification => item !== null).slice(0, MAX_NOTIFICATIONS)
      : [];
    const sourceKeys = new Set<string>();
    if (Array.isArray(stored.sourceKeys)) {
      for (const key of stored.sourceKeys) {
        if (typeof key === "string" && key) sourceKeys.add(key);
      }
    }
    for (const item of [...items].reverse()) {
      if (item.sourceKey) sourceKeys.add(item.sourceKey);
    }
    return {
      items,
      sourceKeys: Array.from(sourceKeys).slice(-MAX_NOTIFICATION_SOURCE_KEYS),
    };
  } catch {
    return { items: [], sourceKeys: [] };
  }
}

function readStoredNotification(value: unknown): AppNotification | null {
  if (!value || typeof value !== "object") return null;
  const stored = value as Record<string, unknown>;
  if (
    typeof stored.id !== "string" ||
    typeof stored.title !== "string" ||
    typeof stored.createdAt !== "string" ||
    typeof stored.read !== "boolean" ||
    !isNotificationCategory(stored.category) ||
    !isNotificationTone(stored.tone)
  ) {
    return null;
  }
  return {
    id: stored.id,
    title: stored.title,
    category: stored.category,
    tone: stored.tone,
    createdAt: stored.createdAt,
    read: stored.read,
    detail: typeof stored.detail === "string" ? stored.detail : undefined,
    href: typeof stored.href === "string" ? stored.href : undefined,
    sourceKey: typeof stored.sourceKey === "string" ? stored.sourceKey : undefined,
  };
}

function isNotificationCategory(value: unknown): value is AppNotification["category"] {
  return value === "app" || value === "library" || value === "system" || value === "voice" || value === "updates";
}

function isNotificationTone(value: unknown): value is NotificationTone {
  return value === "info" || value === "success" || value === "warning" || value === "error";
}

function rememberNotificationSource(sourceKeys: Set<string>, sourceKey: string): boolean {
  if (sourceKeys.has(sourceKey)) return false;
  sourceKeys.add(sourceKey);
  while (sourceKeys.size > MAX_NOTIFICATION_SOURCE_KEYS) {
    const oldest = sourceKeys.values().next();
    if (oldest.done) break;
    sourceKeys.delete(oldest.value);
  }
  return true;
}

function writeNotificationSession(session: NotificationSession): void {
  try {
    window.sessionStorage.setItem(NOTIFICATION_SESSION_KEY, JSON.stringify(session));
  } catch {
    // Notifications still work in memory when browser storage is unavailable.
  }
}

export function useToast(): ToastValue {
  const value = useContext(ToastContext);
  if (!value) throw new Error("useToast must be used within ToastProvider");
  return value;
}

export function useNotifications(): NotificationsValue {
  const value = useContext(NotificationsContext);
  if (!value) throw new Error("useNotifications must be used within ToastProvider");
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
