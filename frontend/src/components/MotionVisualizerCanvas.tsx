import { useTranslation } from "react-i18next";
import { useMotionVisualizer } from "../hooks/useMotionVisualizer";

type Props = {
  enabled?: boolean;
  className?: string;
};

/** Canvas sparkline synced to Go HSP schedule via SSE (60fps rAF). */
export function MotionVisualizerCanvas({ enabled = true, className }: Props) {
  const { t } = useTranslation();
  const { canvasRef, frame } = useMotionVisualizer(enabled);

  return (
    <canvas
      ref={canvasRef}
      className={className ?? "motion-visualizer-canvas"}
      role="img"
      aria-label={t("layout.visualizer.position")}
      data-live={frame.playbackActive ? "true" : "false"}
    />
  );
}
