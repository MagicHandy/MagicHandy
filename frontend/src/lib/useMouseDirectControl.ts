import { useCallback, useEffect, useRef, useState } from "react";

import { api } from "../api/client";



const DEFAULT_MIN_SEND_MS = 16;

const UNLIMITED_MIN_SEND_MS = 1;

const MIN_DELTA_LIMITED = 0.004;

const MIN_DELTA_UNLIMITED = 0.0005;



export function useMouseDirectControl({

  active,

  durationMs,

  limitsEnabled = true,

  minSendIntervalMs,

  onError,

}: {

  active: boolean;

  durationMs: number;

  limitsEnabled?: boolean;

  minSendIntervalMs?: number;

  onError: (message: string) => void;

}) {

  const [targetNorm, setTargetNorm] = useState(0.5);

  const [sentPct, setSentPct] = useState<number | null>(null);

  const [sending, setSending] = useState(false);

  const lastSentRef = useRef(0);

  const lastNormRef = useRef(0.5);

  const lastAtRef = useRef(0);

  const padRef = useRef<HTMLDivElement>(null);

  const activeRef = useRef(active);

  const durationRef = useRef(durationMs);

  const limitsRef = useRef(limitsEnabled);

  const minSendRef = useRef(minSendIntervalMs ?? DEFAULT_MIN_SEND_MS);



  useEffect(() => {

    activeRef.current = active;

  }, [active]);



  useEffect(() => {

    durationRef.current = durationMs;

  }, [durationMs]);



  useEffect(() => {

    limitsRef.current = limitsEnabled;

  }, [limitsEnabled]);



  useEffect(() => {

    minSendRef.current =

      minSendIntervalMs ??

      (limitsEnabled ? DEFAULT_MIN_SEND_MS : UNLIMITED_MIN_SEND_MS);

  }, [limitsEnabled, minSendIntervalMs]);



  const normFromClientY = useCallback((clientY: number) => {

    const pad = padRef.current;

    if (!pad) return null;

    const rect = pad.getBoundingClientRect();

    if (rect.height <= 0) return null;

    const y = Math.max(0, Math.min(rect.height, clientY - rect.top));

    return 1 - y / rect.height;

  }, []);



  const flushMove = useCallback(

    async (norm: number) => {

      if (!activeRef.current) return;

      const now = performance.now();

      const minSend = minSendRef.current;

      const minDelta = limitsRef.current ? MIN_DELTA_LIMITED : MIN_DELTA_UNLIMITED;

      if (

        now - lastAtRef.current < minSend &&

        Math.abs(norm - lastNormRef.current) < minDelta

      ) {

        return;

      }

      lastAtRef.current = now;

      lastNormRef.current = norm;

      setSending(true);

      try {

        const res = await api.sendDirectControlMove(norm, durationRef.current);

        // #region agent log
        fetch("http://127.0.0.1:7754/ingest/6a8fd47b-60f9-4a35-a0cd-8c5a35f2a945", {
          method: "POST",
          headers: { "Content-Type": "application/json", "X-Debug-Session-Id": "745bf8" },
          body: JSON.stringify({
            sessionId: "745bf8",
            hypothesisId: res.skipped ? "H5" : "H1",
            location: "useMouseDirectControl.ts:flushMove",
            message: res.skipped ? "client_move_skipped" : "client_move_sent",
            data: { norm, skipped: res.skipped, position_pct: res.position_pct },
            timestamp: Date.now(),
          }),
        }).catch(() => {});
        // #endregion

        if (!res.skipped && res.position_pct != null) {

          setSentPct(res.position_pct);

        }

      } catch (e) {

        // #region agent log
        fetch("http://127.0.0.1:7754/ingest/6a8fd47b-60f9-4a35-a0cd-8c5a35f2a945", {
          method: "POST",
          headers: { "Content-Type": "application/json", "X-Debug-Session-Id": "745bf8" },
          body: JSON.stringify({
            sessionId: "745bf8",
            hypothesisId: "H2",
            location: "useMouseDirectControl.ts:flushMove",
            message: "client_move_error",
            data: { norm, error: e instanceof Error ? e.message : String(e) },
            timestamp: Date.now(),
          }),
        }).catch(() => {});
        // #endregion

        onError(e instanceof Error ? e.message : "Erro ao enviar movimento");

      } finally {

        setSending(false);

      }

    },

    [onError],

  );



  useEffect(() => {

    if (!active) return;



    let raf = 0;

    const tick = () => {

      const norm = lastSentRef.current;

      if (activeRef.current && norm >= 0) {

        void flushMove(norm);

      }

      raf = requestAnimationFrame(tick);

    };

    raf = requestAnimationFrame(tick);

    return () => cancelAnimationFrame(raf);

  }, [active, flushMove]);



  const handlePointerMove = useCallback(

    (clientY: number) => {

      const norm = normFromClientY(clientY);

      if (norm == null) return;

      setTargetNorm(norm);

      lastSentRef.current = norm;

    },

    [normFromClientY],

  );



  const handlePointerLeave = useCallback(() => {

    lastSentRef.current = -1;

  }, []);



  return {

    padRef,

    targetNorm,

    sentPct,

    sending,

    handlePointerMove,

    handlePointerLeave,

  };

}

