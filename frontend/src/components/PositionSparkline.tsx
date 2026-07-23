import { useId } from "react";

import { useTranslation } from "react-i18next";

import { MotionVisualizerCanvas } from "./MotionVisualizerCanvas";

import { usePositionVisual } from "../contexts/PositionVisualContext";



/** Sparkline — canvas SSE path when live, SVG fallback otherwise. */

export function PositionSparkline() {

  const { t } = useTranslation();

  const { playbackActive, pathD, useCanvasStream } = usePositionVisual();

  const gradId = useId().replace(/:/g, "");



  if (useCanvasStream) {

    return (

      <div

        className={`motion-choice-chart-wrap${playbackActive ? " motion-choice-chart-wrap--live" : ""}`}

      >

        <MotionVisualizerCanvas

          enabled={playbackActive}

          className="motion-choice-chart motion-visualizer-canvas"

        />

      </div>

    );

  }



  return (

    <div

      className={`motion-choice-chart-wrap${playbackActive ? " motion-choice-chart-wrap--live" : ""}`}

    >

      <svg

        className="motion-choice-chart viz-spark"

        viewBox="0 0 100 100"

        preserveAspectRatio="none"

        role="img"

        aria-label={t("layout.visualizer.position")}

      >

        {pathD && (

          <path d={pathD} fill="none" stroke={`url(#${gradId})`} strokeWidth="1.5" />

        )}

        <defs>

          <linearGradient id={gradId} x1="0" y1="0" x2="1" y2="0">

            <stop offset="0%" stopColor="#6366f1" />

            <stop offset="100%" stopColor="#a78bfa" />

          </linearGradient>

        </defs>

      </svg>

    </div>

  );

}


