import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { ControllerSnapshot } from "../api/types";
import { validController } from "./observation-order";

const HEARTBEAT_MS = 2000;
const HEARTBEAT_TIMEOUT_MS = 3000;

export interface ControllerConnection {
  snapshot: ControllerSnapshot | null;
  fresh: boolean;
}

// Controller liveness cannot wait behind /api/state's database/model diagnostics.
// This channel owns one request and one timer, and never renews while hidden.
export function useControllerConnection(enabled: boolean): ControllerConnection {
  const [connection, setConnection] = useState<ControllerConnection>({ snapshot: null, fresh: false });
  useEffect(() => {
    if (!enabled) { setConnection({ snapshot: null, fresh: false }); return; }
    let closed = false;
    let generation = 0;
    let known: ControllerSnapshot | null = null;
    let active: AbortController | null = null;
    let timer: number | undefined;
    let timeout: number | undefined;
    let failures = 0;
    const visible = () => document.visibilityState !== "hidden";
    const cancel = () => {
      generation++;
      window.clearTimeout(timer);
      window.clearTimeout(timeout);
      const previous = active;
      active = null;
      previous?.abort();
    };
    const update = async () => {
      if (closed || !visible() || active) return;
      const admitted = generation;
      const request = new AbortController();
      active = request;
      timeout = window.setTimeout(() => request.abort(), HEARTBEAT_TIMEOUT_MS);
      try {
        let next = known?.heartbeat_required
          ? await api.controllerHeartbeat(request.signal)
          : await api.controllerState(request.signal);
        if (closed || !visible() || admitted !== generation || request.signal.aborted) return;
        if (!validController(next)) throw new Error("Invalid controller snapshot");
        // Discovery is read-only for protected sessions. Renew explicitly once
        // its backend epoch/generation have been learned by the API client.
        if (!known?.heartbeat_required && next.heartbeat_required) {
          next = await api.controllerHeartbeat(request.signal);
        }
        if (closed || !visible() || admitted !== generation || request.signal.aborted) return;
        if (!validController(next)) throw new Error("Invalid controller snapshot");
        known = next;
        failures = 0;
        setConnection({ snapshot: next, fresh: true });
      } catch {
        if (closed || admitted !== generation) return;
        failures++;
        setConnection((current) => ({ ...current, fresh: false }));
      } finally {
        if (active === request) {
          window.clearTimeout(timeout);
          active = null;
          if (!closed && visible() && admitted === generation) {
            const base = Math.min(8000, HEARTBEAT_MS * 2 ** Math.min(failures, 2));
            const delay = failures ? base * (0.8 + Math.random() * 0.4) : base;
            timer = window.setTimeout(() => void update(), delay);
          }
        }
      }
    };
    const resume = () => {
      cancel();
      known = null;
      failures = 0;
      setConnection((current) => ({ ...current, fresh: false }));
      if (visible()) void update();
    };
    document.addEventListener("visibilitychange", resume);
    window.addEventListener("pageshow", resume);
    void update();
    return () => {
      closed = true;
      cancel();
      document.removeEventListener("visibilitychange", resume);
      window.removeEventListener("pageshow", resume);
    };
  }, [enabled]);
  return connection;
}
