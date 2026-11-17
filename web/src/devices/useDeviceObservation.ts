import { useCallback, useEffect, useRef, useState } from "react";
import type { BackendObservation } from "../api/types";

interface Stamped { observation?: BackendObservation }

export function useDeviceObservation<T extends Stamped>(full: T | undefined) {
  const [value, setValue] = useState(full);
  const epoch = useRef(full?.observation?.epoch);
  const accept = useCallback((next: T) => {
    if (!next) return;
    setValue(current => {
      const stamp = next.observation;
      if (stamp && epoch.current && stamp.epoch !== epoch.current) return current;
      const previous = current?.observation;
      if (previous && (!stamp || (stamp.epoch === previous.epoch && stamp.revision < previous.revision))) return current;
      return next;
    });
  }, []);
  useEffect(() => {
    if (!full) return;
    epoch.current = full.observation?.epoch;
    accept(full);
  }, [full, accept]);
  return [value, accept] as const;
}

// Completion schedules the next read. Cleanup also fences transports which
// resolve even after their HTTP request has been aborted.
export function useDevicePoll<T>(active: boolean, repeat: boolean, read: (signal: AbortSignal) => Promise<T>,
  accept: (next: T) => void, session: string) {
  useEffect(() => {
    if (!active) return;
    const controller = new AbortController();
    let timer: number | undefined;
    const poll = async () => {
      try {
        const next = await read(controller.signal);
        if (!controller.signal.aborted) accept(next);
      } catch {
        // The full-state poll also reports connection loss.
      } finally {
        if (repeat && !controller.signal.aborted) timer = window.setTimeout(poll, 500);
      }
    };
    void poll();
    return () => { controller.abort(); window.clearTimeout(timer); };
  }, [active, repeat, read, accept, session]);
}
