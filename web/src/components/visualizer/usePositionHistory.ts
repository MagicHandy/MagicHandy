import { useEffect, useRef, useState } from "react";

export interface PositionPoint {
  /** performance.now() when the position was shown. */
  at: number;
  position: number;
}

/**
 * The positions this visualizer has shown during the last `windowMs`, oldest
 * first. It records what the engine reported, so it never invents motion; it
 * clears while no target is active.
 */
export function usePositionHistory(position: number, active: boolean, windowMs: number, enabled = true): PositionPoint[] {
  const points = useRef<PositionPoint[]>([]);
  const [, setVersion] = useState(0);
  useEffect(() => {
    if (!enabled) return;
    if (!active) {
      if (points.current.length) {
        points.current = [];
        setVersion((version) => version + 1);
      }
      return;
    }
    const now = performance.now();
    const kept = points.current.filter((point) => now - point.at <= windowMs);
    const last = kept[kept.length - 1];
    if (!last || last.position !== position) kept.push({ at: now, position });
    points.current = kept;
    setVersion((version) => version + 1);
  }, [position, active, windowMs, enabled]);
  return points.current;
}
