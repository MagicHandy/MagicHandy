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

const newer = (next: RemoteState, current: RemoteState | null) =>
  !current || next.revision >= current.revision || next.connected !== current.connected || next.other_account !== current.other_account;

// What the desktop shows, for the phone. The stream is open only while this
// page is in front; a read fills the gap after a drop.
export function useRemoteState(enabled: boolean): RemoteView {
  const [view, setView] = useState<Omit<RemoteView, "refresh">>({ state: null, receivedAt: 0, stale: false, error: "" });
  const current = useRef<RemoteState | null>(null);
  const accept = useCallback((next: RemoteState, fromStream: boolean) => {
    if (!fromStream && !newer(next, current.current)) return;
    current.current = next;
    setView({ state: next, receivedAt: Date.now(), stale: false, error: "" });
  }, []);

  const refresh = useCallback(() => {
    void api.remoteState().then(
      (response) => accept(response.remote, false),
      (reason: unknown) => setView((previous) => ({ ...previous, stale: true, error: reason instanceof Error ? reason.message : "" })),
    );
  }, [accept]);

  useEffect(() => {
    if (!enabled) return;
    let source: EventSource | null = null;
    let closed = false;
    let failures = 0;
    let retryTimer: ReturnType<typeof setTimeout> | undefined;
    const disconnect = () => {
      clearTimeout(retryTimer);
      source?.close();
      source = null;
    };
    const connect = () => {
      disconnect();
      if (closed || document.visibilityState === "hidden") return;
      let current: EventSource;
      try {
        current = new EventSource(api.remoteEventsURL());
      } catch {
        return;
      }
      source = current;
      current.addEventListener("state", (event) => {
        if (source !== current) return;
        failures = 0;
        try {
          accept(JSON.parse((event as MessageEvent).data) as RemoteState, true);
        } catch {
          /* ignore */
        }
      });
      current.onerror = () => {
        if (source !== current) return;
        disconnect();
        setView((previous) => ({ ...previous, stale: true }));
        refresh();
        failures += 1;
        retryTimer = setTimeout(connect, Math.min(10_000, 1_000 * 2 ** Math.min(failures - 1, 4)));
      };
    };
    const visibility = () => {
      if (document.visibilityState === "hidden") disconnect();
      else connect();
    };
    connect();
    document.addEventListener("visibilitychange", visibility);
    return () => {
      closed = true;
      disconnect();
      document.removeEventListener("visibilitychange", visibility);
    };
  }, [accept, enabled, refresh]);

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
