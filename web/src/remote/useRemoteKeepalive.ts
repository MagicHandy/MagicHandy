import { useEffect, useState } from "react";
import { api } from "../api/client";

const KEEPALIVE_MS = 3000;
const KEEPALIVE_TIMEOUT_MS = 3000;

// While this page is in front, the phone keeps its desktop's session alive
// (ADR 0055). A hidden desktop tab pauses its own heartbeat, and the server
// stops a session nobody holds once its 15-second lease runs out. A hidden or
// locked phone sends nothing, so the usual Stop follows when neither is in
// front. Returns whether the last keepalive renewed a session, or null before
// the first answer.
export function useRemoteKeepalive(enabled: boolean): boolean | null {
  const [renewed, setRenewed] = useState<boolean | null>(null);
  useEffect(() => {
    setRenewed(null);
    if (!enabled) return;
    let closed = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let active: AbortController | null = null;
    const visible = () => document.visibilityState !== "hidden";
    const schedule = () => {
      clearTimeout(timer);
      if (!closed && visible()) timer = setTimeout(() => void renew(), KEEPALIVE_MS);
    };
    const renew = async () => {
      if (closed || !visible() || active) return;
      const request = new AbortController();
      active = request;
      const timeout = setTimeout(() => request.abort(), KEEPALIVE_TIMEOUT_MS);
      try {
        const response = await api.remoteKeepalive(request.signal);
        if (!closed && !request.signal.aborted) setRenewed(response.keepalive.renewed);
      } catch {
        if (!closed && !request.signal.aborted) setRenewed(false);
      } finally {
        clearTimeout(timeout);
        if (active === request) active = null;
        schedule();
      }
    };
    const visibility = () => {
      if (visible()) {
        void renew();
        return;
      }
      clearTimeout(timer);
      active?.abort();
      active = null;
    };
    void renew();
    document.addEventListener("visibilitychange", visibility);
    return () => {
      closed = true;
      clearTimeout(timer);
      active?.abort();
      document.removeEventListener("visibilitychange", visibility);
    };
  }, [enabled]);
  return renewed;
}
