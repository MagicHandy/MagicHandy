import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { api } from "../api/client";
import type { MotionVisual } from "../api/types";
import { mergeMotionVisual } from "../lib/mergeMotionVisual";
import { useFluidMotionVisual } from "../lib/useFluidMotionVisual";
import { useMotionEvents } from "../lib/useMotionEvents";
import { useStatus } from "./StatusContext";

function isPlaybackActive(
  visual: MotionVisual | null,
  snap: ReturnType<typeof useStatus>["snap"],
): boolean {
  return Boolean(
    visual?.playback_active ||
      snap?.manual_queue_playing ||
      snap?.playback_active ||
      snap?.direct_control_active,
  );
}

interface PositionVisualContextValue {
  visual: MotionVisual | null;
  playbackActive: boolean;
  offset: number;
  setOffset: (ms: number) => void;
  saving: boolean;
  setSaving: (saving: boolean) => void;
  loadVisual: () => Promise<void>;
  pos: number;
  pathD: string;
  measuredRtt: number | null;
  useCanvasStream: boolean;
}

const PositionVisualContext = createContext<PositionVisualContextValue | null>(null);

export function PositionVisualProvider({ children }: { children: ReactNode }) {
  const { snap } = useStatus();
  const liveMotion = useMotionEvents();
  const [cachedVisual, setCachedVisual] = useState<MotionVisual | null>(null);
  const [offset, setOffset] = useState(-160);
  const [saving, setSaving] = useState(false);
  const loadingRef = useRef(false);
  const playbackWasActiveRef = useRef(false);

  const loadVisual = useCallback(async () => {
    if (loadingRef.current) return;
    loadingRef.current = true;
    try {
      const v = await api.getMotionVisual();
      setCachedVisual(v);
      setOffset(v.offset_ms);
    } catch {
      /* */
    } finally {
      loadingRef.current = false;
    }
  }, []);

  useEffect(() => {
    void loadVisual();
  }, [loadVisual]);

  const visual = useMemo(
    () => mergeMotionVisual({ cached: cachedVisual, motion: liveMotion, snap }),
    [cachedVisual, liveMotion, snap],
  );

  const playbackActive = isPlaybackActive(visual, snap);

  useEffect(() => {
    const wasActive = playbackWasActiveRef.current;
    playbackWasActiveRef.current = playbackActive;
    if (playbackActive && !wasActive) {
      void loadVisual();
    }
  }, [playbackActive, loadVisual]);

  const polledPct =
    visual?.live_position_pct ??
    visual?.position_pct ??
    snap?.motion_position_pct ??
    50;
  const { pos, pathD } = useFluidMotionVisual(visual, polledPct);
  const useCanvasStream = Boolean(visual?.schedule_active || playbackActive);

  const value = useMemo<PositionVisualContextValue>(
    () => ({
      visual,
      playbackActive,
      offset,
      setOffset,
      saving,
      setSaving,
      loadVisual,
      pos,
      pathD,
      measuredRtt: visual?.measured_rtt_ms ?? snap?.measured_rtt_ms ?? null,
      useCanvasStream,
    }),
    [visual, playbackActive, offset, saving, loadVisual, pos, pathD, snap?.measured_rtt_ms, useCanvasStream],
  );

  return (
    <PositionVisualContext.Provider value={value}>{children}</PositionVisualContext.Provider>
  );
}

export function usePositionVisual(): PositionVisualContextValue {
  const ctx = useContext(PositionVisualContext);
  if (!ctx) {
    throw new Error("usePositionVisual must be used within PositionVisualProvider");
  }
  return ctx;
}
