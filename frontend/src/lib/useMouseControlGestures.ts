import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";

const TOGGLE_DEBOUNCE_MS = 500;

export function useMouseControlGestures({
  active,
  durationMs,
  minDuration,
  maxDuration,
  durationStep,
  recordingBusy,
  onToggleRecording,
  setDurationMs,
}: {
  active: boolean;
  durationMs: number;
  minDuration: number;
  maxDuration: number;
  durationStep: number;
  recordingBusy: boolean;
  onToggleRecording: () => void;
  setDurationMs: Dispatch<SetStateAction<number>>;
}) {
  const [fastResponse, setFastResponse] = useState(false);
  const baselineRef = useRef(durationMs);
  const fastHoldRef = useRef(false);
  const lastToggleAtRef = useRef(0);
  const toggleRef = useRef(onToggleRecording);
  const recordingBusyRef = useRef(recordingBusy);

  useEffect(() => {
    toggleRef.current = onToggleRecording;
  }, [onToggleRecording]);

  useEffect(() => {
    recordingBusyRef.current = recordingBusy;
  }, [recordingBusy]);

  useEffect(() => {
    if (!fastHoldRef.current) baselineRef.current = durationMs;
  }, [durationMs]);

  const releaseFastResponse = useCallback(
    (pointerId: number, target: HTMLElement) => {
      if (!fastHoldRef.current) return;
      fastHoldRef.current = false;
      setFastResponse(false);
      setDurationMs(baselineRef.current);
      try {
        target.releasePointerCapture(pointerId);
      } catch {
        /* */
      }
    },
    [setDurationMs],
  );

  const onPadPointerDown = useCallback(
    (e: React.PointerEvent<HTMLDivElement>) => {
      if (!active) return;

      if (e.button === 1 && e.ctrlKey) {
        e.preventDefault();
        if (recordingBusyRef.current) return;
        const now = Date.now();
        if (now - lastToggleAtRef.current < TOGGLE_DEBOUNCE_MS) return;
        lastToggleAtRef.current = now;
        toggleRef.current();
        return;
      }

      if (e.button === 0 && e.ctrlKey) {
        e.preventDefault();
        setDurationMs((value) => Math.max(minDuration, value - durationStep));
        return;
      }

      if (e.button !== 0) return;
      baselineRef.current = durationMs;
      fastHoldRef.current = true;
      setFastResponse(true);
      setDurationMs(minDuration);
      e.currentTarget.setPointerCapture(e.pointerId);
    },
    [active, durationMs, minDuration, durationStep, setDurationMs],
  );

  const onPadPointerUp = useCallback(
    (e: React.PointerEvent<HTMLDivElement>) => {
      if (e.button !== 0) return;
      releaseFastResponse(e.pointerId, e.currentTarget);
    },
    [releaseFastResponse],
  );

  const onPadPointerCancel = useCallback(
    (e: React.PointerEvent<HTMLDivElement>) => {
      releaseFastResponse(e.pointerId, e.currentTarget);
    },
    [releaseFastResponse],
  );

  const onPadContextMenu = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (!active) return;
      e.preventDefault();
      if (!e.ctrlKey) return;
      setDurationMs((value) => Math.min(maxDuration, value + durationStep));
    },
    [active, durationStep, maxDuration, setDurationMs],
  );

  const onPadAuxClick = useCallback((e: React.MouseEvent<HTMLDivElement>) => {
    if (!active || e.button !== 1 || !e.ctrlKey) return;
    e.preventDefault();
  }, [active]);

  return {
    fastResponse,
    onPadPointerDown,
    onPadPointerUp,
    onPadPointerCancel,
    onPadContextMenu,
    onPadAuxClick,
  };
}
