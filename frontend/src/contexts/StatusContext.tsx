import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { api } from "../api/client";
import type { StatusSnapshot } from "../api/types";

type StatusContextValue = {
  snap: StatusSnapshot | null;
  error: string | null;
  refresh: () => Promise<StatusSnapshot>;
};

const StatusContext = createContext<StatusContextValue | null>(null);

export function StatusProvider({
  children,
  intervalMs = 1500,
}: {
  children: ReactNode;
  intervalMs?: number;
}) {
  const [snap, setSnap] = useState<StatusSnapshot | null>(null);
  const [error, setError] = useState<string | null>(null);
  const loadingRef = useRef(false);

  const refresh = useCallback(async () => {
    const data = await api.getStatus();
    setSnap(data);
    setError(null);
    return data;
  }, []);

  useEffect(() => {
    let alive = true;
    const load = async () => {
      if (loadingRef.current) return;
      loadingRef.current = true;
      try {
        const data = await api.getStatus();
        if (!alive) return;
        setSnap((prev) => {
          if (prev && JSON.stringify(prev) === JSON.stringify(data)) {
            return prev;
          }
          return data;
        });
        setError(null);
      } catch (e) {
        if (!alive) return;
        setError(e instanceof Error ? e.message : "API offline");
        setSnap(null);
      } finally {
        loadingRef.current = false;
      }
    };
    load();
    const id = window.setInterval(load, intervalMs);
    return () => {
      alive = false;
      clearInterval(id);
    };
  }, [intervalMs]);

  return (
    <StatusContext.Provider value={{ snap, error, refresh }}>
      {children}
    </StatusContext.Provider>
  );
}

export function useStatus(_intervalMs?: number) {
  const ctx = useContext(StatusContext);
  if (ctx) return ctx;

  const [snap, setSnap] = useState<StatusSnapshot | null>(null);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    let alive = true;
    const load = async () => {
      try {
        const data = await api.getStatus();
        if (alive) {
          setSnap(data);
          setError(null);
        }
      } catch (e) {
        if (alive) {
          setError(e instanceof Error ? e.message : "API offline");
          setSnap(null);
        }
      }
    };
    load();
    const id = window.setInterval(load, _intervalMs ?? 1500);
    return () => {
      alive = false;
      clearInterval(id);
    };
  }, [_intervalMs]);

  const refresh = async () => {
    const data = await api.getStatus();
    setSnap(data);
    setError(null);
    return data;
  };

  return { snap, error, refresh };
}
