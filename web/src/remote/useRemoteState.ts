import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { RemoteState } from "../api/remote-types";

export interface RemoteView {
  state: RemoteState | null;
  /** When this tab received the state, for placing a playing video's position. */
  receivedAt: number;
  /** The live stream dropped; the view may be behind until it reconnects. */
  stale: boolean;
  error: string;
  refresh: () => void;
}

// What the desktop shows, for the phone. The stream is open only while this
// page is in front; a read fills the gap after a drop.
export function useRemoteState(enabled: boolean): RemoteView {
  const [view, setView] = useState<Omit<RemoteView, "refresh">>({ state: null, receivedAt: 0, stale: false, error: "" });
  const refreshCurrent = useRef<() => void>(() => undefined);
  const refresh = useCallback(() => refreshCurrent.current(), []);

  useEffect(() => {
    setView({ state: null, receivedAt: 0, stale: true, error: "" });
    if (!enabled) return;
    let source: EventSource | null = null;
    let closed = false;
    let failures = 0;
    let retryTimer: ReturnType<typeof setTimeout> | undefined;
    let observation = 0;
    let read: AbortController | undefined;
    const accept = (next: RemoteState) => {
      observation += 1;
      setView({ state: next, receivedAt: Date.now(), stale: false, error: "" });
    };
    const refresh = () => {
      read?.abort();
      const request = new AbortController();
      read = request;
      const observed = observation;
      const current = () => !closed && !request.signal.aborted && observed === observation;
      void api.remoteState(request.signal).then(
        (response) => { if (current()) accept(response.remote); },
        (reason: unknown) => {
          if (current()) setView((previous) => ({ ...previous, stale: true, error: reason instanceof Error ? reason.message : "" }));
        },
      );
    };
    refreshCurrent.current = refresh;
    const disconnect = () => {
      clearTimeout(retryTimer);
      source?.close();
      source = null;
    };
    const retry = () => {
      disconnect();
      setView((previous) => ({ ...previous, stale: true }));
      refresh();
      failures += 1;
      retryTimer = setTimeout(connect, Math.min(10_000, 1_000 * 2 ** Math.min(failures - 1, 4)));
    };
    const connect = () => {
      disconnect();
      if (closed || document.visibilityState === "hidden") return;
      let current: EventSource;
      try {
        current = new EventSource(api.remoteEventsURL());
      } catch {
        retry();
        return;
      }
      source = current;
      current.addEventListener("state", (event) => {
        if (source !== current) return;
        failures = 0;
        try {
          accept(JSON.parse((event as MessageEvent).data) as RemoteState);
        } catch {
          /* ignore */
        }
      });
      current.onerror = () => {
        if (source !== current) return;
        retry();
      };
    };
    const visibility = () => {
      if (document.visibilityState === "hidden") {
        disconnect();
        read?.abort();
        setView((previous) => ({ ...previous, stale: true }));
      }
      else connect();
    };
    connect();
    document.addEventListener("visibilitychange", visibility);
    return () => {
      closed = true;
      read?.abort();
      refreshCurrent.current = () => undefined;
      disconnect();
      document.removeEventListener("visibilitychange", visibility);
    };
  }, [enabled]);

  return { ...view, refresh };
}

/** Where a playing video is now, placed from the last report. */
export function remotePosition(state: RemoteState | null, receivedAt: number, now: number): number {
  const video = state?.video;
  if (!video) return 0;
  const elapsed = video.playing && video.ready ? Math.max(0, now - receivedAt) * (video.rate || 1) : 0;
  const position = video.position_ms + elapsed;
  return video.duration_ms > 0 ? Math.min(position, video.duration_ms) : position;
}
