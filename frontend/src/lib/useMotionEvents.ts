import { useEffect, useState } from "react";
import type { MotionInfo } from "../api/types";
import { controllerClientID } from "./controllerClient";

/** Dedicated SSE consumer for live motion engine state (250ms server push). */
export function useMotionEvents(): MotionInfo | null {
  const [motion, setMotion] = useState<MotionInfo | null>(null);

  useEffect(() => {
    let source: EventSource | null = null;
    try {
      source = new EventSource(
        `/api/motion/events?client_id=${encodeURIComponent(controllerClientID())}`,
      );
      source.addEventListener("motion", (ev) => {
        try {
          setMotion(JSON.parse((ev as MessageEvent).data) as MotionInfo);
        } catch {
          /* ignore malformed payload */
        }
      });
      source.onerror = () => setMotion(null);
    } catch {
      source = null;
    }
    return () => source?.close();
  }, []);

  return motion;
}
