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
import type { ControllerSnapshot, StatusSnapshot } from "../api/types";
import { controllerClientID } from "../lib/controllerClient";
import { statusSnapshotEqual } from "../lib/statusSnapshotEqual";

type StatusContextValue = {
  snap: StatusSnapshot | null;
  controller: ControllerSnapshot | null;
  readOnly: boolean;
  error: string | null;
  refresh: () => Promise<StatusSnapshot>;
};

const StatusContext = createContext<StatusContextValue | null>(null);

function applySnapshot(
  prev: StatusSnapshot | null,
  data: StatusSnapshot,
): StatusSnapshot {
  if (prev && statusSnapshotEqual(prev, data)) {
    return prev;
  }
  return data;
}

export function StatusProvider({
  children,
  intervalMs = 1500,
}: {
  children: ReactNode;
  intervalMs?: number;
}) {
  const [snap, setSnap] = useState<StatusSnapshot | null>(null);
  const [controller, setController] = useState<ControllerSnapshot | null>(null);
  const [error, setError] = useState<string | null>(null);
  const loadingRef = useRef(false);

  const refresh = useCallback(async () => {
    const data = await api.getStatus();
    setSnap((prev) => applySnapshot(prev, data));
    setError(null);
    try {
      const ctrl = await api.getController();
      setController(ctrl);
    } catch {
      setController(null);
    }
    return data;
  }, []);

  useEffect(() => {
    let alive = true;
    const load = async () => {
      if (loadingRef.current) return;
      loadingRef.current = true;
      try {
        const [data, ctrl] = await Promise.all([
          api.getStatus(),
          api.getController().catch(() => null),
        ]);
        if (!alive) return;
        setSnap((prev) => applySnapshot(prev, data));
        setController(ctrl);
        setError(null);
        // #region agent log
        fetch('http://127.0.0.1:7754/ingest/6a8fd47b-60f9-4a35-a0cd-8c5a35f2a945',{method:'POST',headers:{'Content-Type':'application/json','X-Debug-Session-Id':'324442'},body:JSON.stringify({sessionId:'324442',hypothesisId:'H1',location:'StatusContext.tsx:load',message:'status_poll',data:{client_id:controllerClientID(),read_only:ctrl?.read_only,active:ctrl?.active,active_client_id:ctrl?.active_client_id,reason:ctrl?.reason,device_transport:data.device_transport,device_connected:data.device_connected,handy_connected:data.handy_connected,handy_key_configured:data.handy_key_configured,handy_error:data.handy_error},timestamp:Date.now()})}).catch(()=>{});
        // #endregion
      } catch (e) {
        if (!alive) return;
        setError(e instanceof Error ? e.message : "API offline");
        setSnap(null);
        setController(null);
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

  const readOnly = controller?.read_only === true;

  return (
    <StatusContext.Provider value={{ snap, controller, readOnly, error, refresh }}>
      {children}
    </StatusContext.Provider>
  );
}

export function useStatus(_intervalMs?: number) {
  const ctx = useContext(StatusContext);
  if (ctx) return ctx;

  const [snap, setSnap] = useState<StatusSnapshot | null>(null);
  const [controller, setController] = useState<ControllerSnapshot | null>(null);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    let alive = true;
    const load = async () => {
      try {
        const [data, ctrl] = await Promise.all([
          api.getStatus(),
          api.getController().catch(() => null),
        ]);
        if (alive) {
          setSnap((prev) => applySnapshot(prev, data));
          setController(ctrl);
          setError(null);
        }
      } catch (e) {
        if (alive) {
          setError(e instanceof Error ? e.message : "API offline");
          setSnap(null);
          setController(null);
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
    setSnap((prev) => applySnapshot(prev, data));
    setError(null);
    try {
      const ctrl = await api.getController();
      setController(ctrl);
    } catch {
      setController(null);
    }
    return data;
  };

  return {
    snap,
    controller,
    readOnly: controller?.read_only === true,
    error,
    refresh,
  };
}
