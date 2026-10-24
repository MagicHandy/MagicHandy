import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { ControllerSnapshot } from "../api/types";
import { validController } from "./observation-order";

const HEARTBEAT_MS = 2000;
const HEARTBEAT_TIMEOUT_MS = 3000;
// The server's controller lease. A hidden tab cannot still hold control this
// long after its last successful observation, so it stops watching then.
const LEASE_MS = 15_000;

export interface ControllerConnection {
  snapshot: ControllerSnapshot | null;
  fresh: boolean;
}

// Controller liveness cannot wait behind /api/state's database/model diagnostics.
// This channel owns one request and one timer, and never renews while hidden.
// A hidden tab that still holds control keeps observing in the background: it
// learns whether its phone remote holds the session, and the command tickets
// that phone's renewals issue (ADR 0056).
export function useControllerConnection(enabled: boolean): ControllerConnection {
  const [connection, setConnection] = useState<ControllerConnection>({ snapshot: null, fresh: false });
  useEffect(() => {
    if (!enabled) { setConnection({ snapshot: null, fresh: false }); return; }
    let closed = false;
    let generation = 0;
    let known: ControllerSnapshot | null = null;
    let observedAt = Number.NEGATIVE_INFINITY;
    let active: AbortController | null = null;
    let timer: number | undefined;
    let timeout: number | undefined;
    let failures = 0;
    const visible = () => document.visibilityState !== "hidden";
    const watching = () => visible() || Boolean(known?.active && performance.now() - observedAt <= LEASE_MS);
    const cancel = () => {
      generation++;
      window.clearTimeout(timer);
      window.clearTimeout(timeout);
      const previous = active;
      active = null;
      previous?.abort();
    };
    const update = async () => {
      if (closed || active || !watching()) return;
      const background = !visible();
      const admitted = generation;
      const request = new AbortController();
      active = request;
      timeout = window.setTimeout(() => request.abort(), HEARTBEAT_TIMEOUT_MS);
      try {
        let next = known?.heartbeat_required && !background
          ? await api.controllerHeartbeat(request.signal)
          : await api.controllerState(request.signal, background);
        if (closed || admitted !== generation || request.signal.aborted) return;
        if (!validController(next)) throw new Error("Invalid controller snapshot");
        // Discovery is read-only for protected sessions. Renew explicitly once
        // its backend epoch/generation have been learned by the API client.
        if (!background && !known?.heartbeat_required && next.heartbeat_required) {
          next = await api.controllerHeartbeat(request.signal);
        }
        if (closed || admitted !== generation || request.signal.aborted) return;
        if (!validController(next)) throw new Error("Invalid controller snapshot");
        known = next;
        observedAt = performance.now();
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
          if (!closed && admitted === generation && watching()) {
            // A controller's 15-second lease cannot survive exponential
            // backoff after two 3-second WAN timeouts. Keep its one bounded
            // request on the normal cadence; only discovery/observers back off.
            const renewing = visible() && known?.heartbeat_required && known.active && !known.read_only;
            const base = renewing ? HEARTBEAT_MS : Math.min(8000, HEARTBEAT_MS * 2 ** Math.min(failures, 2));
            const delay = failures && !renewing ? base * (0.8 + Math.random() * 0.4) : base;
            timer = window.setTimeout(() => void update(), delay);
          }
        }
      }
    };
    const changed = (event: Event) => {
      // Watching survives a visibility change only if it was current, so a
      // hidden owner keeps its fresh view and returns without a read-only gap.
      const current = event.type !== "pageshow" && Boolean(known?.active) &&
        performance.now() - observedAt <= HEARTBEAT_MS + HEARTBEAT_TIMEOUT_MS;
      cancel();
      failures = 0;
      if (visible()) {
        // Rediscover, then renew, as on first load.
        known = null;
        if (!current) setConnection((value) => ({ ...value, fresh: false }));
      } else if (!current) {
        known = null;
        setConnection((value) => ({ ...value, fresh: false }));
      }
      void update();
    };
    document.addEventListener("visibilitychange", changed);
    window.addEventListener("pageshow", changed);
    void update();
    return () => {
      closed = true;
      cancel();
      document.removeEventListener("visibilitychange", changed);
      window.removeEventListener("pageshow", changed);
    };
  }, [enabled]);
  return connection;
}
