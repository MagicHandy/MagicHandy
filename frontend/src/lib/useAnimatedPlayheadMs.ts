import { useEffect, useRef, useState } from "react";

/** Avança o playhead entre polls para movimento fluido no gráfico. */
export function useAnimatedPlayheadMs(
  playheadMs: number | null | undefined,
  live: boolean,
  durationMs: number,
) {
  const anchorRef = useRef({ ms: 0, at: 0 });
  const [animated, setAnimated] = useState(playheadMs ?? 0);

  useEffect(() => {
    if (!live || playheadMs == null) {
      setAnimated(playheadMs ?? 0);
      return;
    }
    anchorRef.current = { ms: playheadMs, at: performance.now() };
    setAnimated(playheadMs);
  }, [playheadMs, live]);

  useEffect(() => {
    if (!live || playheadMs == null) return;

    let raf = 0;
    const tick = () => {
      const { ms, at } = anchorRef.current;
      const elapsed = performance.now() - at;
      const next = ms + elapsed;
      const cap = durationMs > 0 ? durationMs : next;
      setAnimated(Math.min(next, cap));
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [live, playheadMs, durationMs]);

  return live ? animated : (playheadMs ?? 0);
}
