import { formatNumber, t } from "../i18n";
// The single authoritative visualizer. It renders engine state only and labels
// position as a commanded estimate — never a guessed or device-confirmed value.
import type { ComponentType } from "react";
import type { MotionInfo } from "../api/types";
import { clampPercent } from "../util/format";
import { handyTravelMillimetres, visualizerKind, type VisualizerDevice, type VisualizerKind } from "./visualizer/device";
import { HandyDotDisplay } from "./visualizer/HandyDotDisplay";
import { HandyTwoFront } from "./visualizer/HandyTwoFront";
import { OriginalHandyFront } from "./visualizer/OriginalHandyFront";
import { RibbonTrace } from "./visualizer/RibbonTrace";
import { ScanBar } from "./visualizer/ScanBar";
import type { DrawingProps } from "./visualizer/types";

const DRAWINGS: Record<VisualizerKind, ComponentType<DrawingProps>> = {
  "handy-dots": HandyDotDisplay,
  "handy-original": OriginalHandyFront,
  scan: ScanBar,
  "handy-front": HandyTwoFront,
  ribbon: RibbonTrace,
};

function paceLimiterLabel(limiter: string): string {
  if (limiter === "device_velocity") return t("device velocity");
  if (limiter === "acceleration") return t("acceleration");
  if (limiter === "jerk") return t("smoothness");
  if (limiter === "reversal_spacing") return t("reversal spacing");
  if (limiter === "curve_geometry") return t("curve geometry");
  return limiter.replaceAll("_", " ");
}

export function MotionVisualizer({ motion, mini = false, device, kind: kindOverride }: {
  motion: MotionInfo | null;
  mini?: boolean;
  /** The dispatch owner and Handy model; selects the drawing. */
  device?: VisualizerDevice;
  /** Forces a drawing, including a disabled one (design review and tests). */
  kind?: VisualizerKind;
}) {
  const engine = motion?.engine;
  const running = engine?.running === true;
  const starting = engine?.starting === true;
  const paused = engine?.paused === true;
  const firstBound = clampPercent(engine?.settings?.stroke_min_percent, 0);
  const secondBound = clampPercent(engine?.settings?.stroke_max_percent, 100);
  const min = Math.min(firstBound, secondBound);
  const max = Math.max(firstBound, secondBound);
  const semanticPosition = clampPercent(
    engine?.current_sample?.position_percent ?? engine?.last_sample?.position_percent,
    50,
  );
  const wirePosition = engine?.settings?.reverse_direction ? 100 - semanticPosition : semanticPosition;
  const pos = min + (wirePosition / 100) * (max - min);
  const hasError = Boolean(engine?.last_error || motion?.error);
  let state = "idle";
  if (motion?.available === false) {
    state = "unavailable";
  } else if (hasError) {
    state = "error";
  } else if (paused) {
    state = "paused";
  } else if (engine?.completing) {
    state = "completing";
  } else if (starting) {
    state = "starting";
  } else if (running) {
    state = "running";
  }
  const stateLabel = state === "unavailable" ? t("Unavailable")
    : state === "error" ? t("Error")
      : state === "paused" ? t("Paused")
        : state === "completing" ? t("Completing")
          : state === "starting" ? t("Starting")
            : state === "running" ? t("Running")
              : t("Idle");
  const roundedPosition = Math.round(pos);
  const active = running || starting || paused || engine?.completing === true;
  const speed = active && typeof engine?.target?.speed_percent === "number"
    ? `${Math.round(clampPercent(engine.target.speed_percent, 0))}%`
    : "--";
  const dynamic = active ? engine?.target?.dynamic : undefined;
  const pace = dynamic ? engine?.pace : undefined;
  const paceEffective = pace && Number.isFinite(pace.effective_percent)
    ? Math.round(clampPercent(pace.effective_percent, 0))
    : undefined;
  const paceRequested = pace && Number.isFinite(pace.requested_percent)
    ? Math.round(clampPercent(pace.requested_percent, 0))
    : undefined;
  const paceDisplay = paceEffective !== undefined && paceRequested !== undefined
    ? pace?.limited ? `${paceEffective}% / ${paceRequested}%` : `${paceEffective}%`
    : speed;
  const paceLimiterLabels = (pace?.limiters ?? []).map(paceLimiterLabel);
  const paceTitle = paceEffective !== undefined && paceRequested !== undefined
    ? `${t("Effective {effective}%; requested {requested}%.", { effective: paceEffective, requested: paceRequested })}${pace?.limited && paceLimiterLabels.length ? ` ${t("Limited by {limiters}.", { limiters: paceLimiterLabels.join(", ") })}` : ""}`
    : "";
  const resolvedPatternName = engine?.target?.pattern_name?.trim() || engine?.target?.pattern_id?.trim();
  const resolvedMediaName = engine?.target?.source === "media"
    ? engine.target.label?.trim() || t("Video funscript")
    : "";
  const patternName = active
    ? dynamic ? t("Creative") : resolvedMediaName || resolvedPatternName || (engine?.target?.program_id ? t("Program playback") : t("Unknown pattern"))
    : t("No active pattern");
  const rawSource = engine?.target?.source?.trim();
  const source = active && rawSource ? rawSource.replaceAll("_", " ") : "--";
  const dynamicSections = dynamic?.sections ?? [];
  const dynamicAnchors = dynamic?.anchors?.map((anchor) => anchor.name).filter(Boolean) ?? [];
  const sectionOuterSpans = dynamicSections.map((section) => section.span_percent);
  const sectionInnerSpans = dynamicSections.map((section) => section.span_min_percent ?? section.span_percent);
  const outerSpan = dynamicSections.length ? Math.max(...sectionOuterSpans) : dynamic?.span_percent;
  const innerSpan = dynamicSections.length ? Math.min(...sectionInnerSpans) : dynamic?.span_min_percent;
  const dynamicSpan = dynamic && typeof outerSpan === "number"
    ? typeof innerSpan === "number" && innerSpan < outerSpan ? `${innerSpan}-${outerSpan}%` : `${outerSpan}%`
    : "";
  const dynamicSpanProfile = dynamic?.span_profile === "steady" ? t("Steady")
    : dynamic?.span_profile === "breathe" ? t("Breathe")
      : dynamic?.span_profile === "wander" ? t("Wander")
        : dynamic?.span_profile === "contrast" ? t("Contrast")
          : "";
  const dynamicSectionLabel = dynamicSections.length
    ? t("{count} sections", { count: dynamicSections.length })
    : "";
  const dynamicVariation = dynamicSections.length
    ? (() => {
      const values = dynamicSections.map((section) => section.variation_percent);
      const minimum = Math.min(...values);
      const maximum = Math.max(...values);
      return minimum === maximum ? `${minimum}%` : `${minimum}-${maximum}%`;
    })()
    : dynamic ? `${dynamic.variation_percent}%` : "";
  const dynamicMeta = dynamic
    ? `${dynamicSectionLabel || (dynamicAnchors.length ? dynamicAnchors.join(" → ") : `${t("Center")} ${dynamic.center_percent}%`)} · ${!dynamicSectionLabel && dynamicSpanProfile ? `${dynamicSpanProfile} · ` : ""}${dynamic.segment_seconds}s · ${source}`
    : "";
  const model = engine?.settings?.handy_model || device?.model;
  const kind = kindOverride ?? visualizerKind({ owner: device?.owner, model });
  const Drawing = DRAWINGS[kind];
  const handy = kind !== "scan" && kind !== "ribbon";
  const travelMillimetres = handyTravelMillimetres(model);
  const millimetres = Math.round((pos / 100) * travelMillimetres);
  const label = t("Motion {state}; pattern {pattern}; commanded position estimate {position} percent; stroke range {minimum} to {maximum} percent", {
    state: stateLabel,
    pattern: patternName,
    position: formatNumber(roundedPosition),
    minimum: formatNumber(Math.round(min)),
    maximum: formatNumber(Math.round(max)),
  });

  return (
    <div className={`visualizer${mini ? " mini" : ""}`} data-state={state} data-kind={kind} role="img" aria-label={label}>
      <Drawing
        position={pos}
        min={min}
        max={max}
        active={active}
        mini={mini}
        travelMillimetres={travelMillimetres}
        svgProps={{
          className: "viz-device",
          "data-position": roundedPosition,
          "data-range-min": Math.round(min),
          "data-range-max": Math.round(max),
        } as DrawingProps["svgProps"]}
      />
      {!mini && (
        <div className="viz-telemetry">
          <div className="viz-summary">
            <span className="viz-state"><span className="viz-state-dot" aria-hidden="true" />{stateLabel}</span>
            <span className="viz-commanded" title={handy ? t("{n}%", { n: roundedPosition }) : undefined}><strong>{handy ? t("{n} mm", { n: millimetres }) : t("{n}%", { n: roundedPosition })}</strong><small>{t("commanded")}</small></span>
          </div>
          <div className="viz-pattern">
            <span>{dynamic ? t("Motion") : t("Pattern")}</span>
            <strong title={patternName}>{patternName}</strong>
            {dynamicMeta && <small title={dynamicMeta}>{dynamicMeta}</small>}
          </div>
          {dynamic ? (
            <dl className="viz-metrics dynamic-metrics">
              <div>
                <dt>{dynamicSectionLabel ? t("Motion") : t("Center")}</dt>
                <dd>{dynamicSectionLabel ? dynamicSectionLabel : <>{formatNumber(dynamic.center_percent)}%</>}</dd>
              </div>
              <div><dt>{t("Span")}</dt><dd>{dynamicSpan}</dd></div>
              <div><dt>{t("Pace")}</dt><dd title={paceTitle} aria-label={paceTitle || undefined}>{paceDisplay}</dd></div>
              <div><dt>{t("Variation")}</dt><dd>{dynamicVariation}</dd></div>
            </dl>
          ) : (
            <dl className="viz-metrics">
              <div>
                <dt>{t("Range")}</dt>
                <dd>{Math.round(min)}-{Math.round(max)}%</dd>
              </div>
              <div>
                <dt>{t("Speed")}</dt>
                <dd>{speed}</dd>
              </div>
              <div>
                <dt>{t("Source")}</dt>
                <dd title={source}>{source}</dd>
              </div>
            </dl>
          )}
        </div>
      )}
    </div>
  );
}
