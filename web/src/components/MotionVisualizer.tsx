import { formatNumber, t } from "../i18n";
// The single authoritative visualizer. It renders engine state only and labels
// position as a commanded estimate — never a guessed or device-confirmed value.
import type { CSSProperties } from "react";
import type { MotionInfo } from "../api/types";
import { clampPercent } from "../util/format";

const DEVICE_CENTER_X = 48;
const BODY_WIDTH = 48;
const BODY_X = DEVICE_CENTER_X - BODY_WIDTH / 2;
const TRACK_WIDTH = 9;
const TRACK_X = DEVICE_CENTER_X - TRACK_WIDTH / 2;
const TRACK_INNER_WIDTH = 7.8;
const TRACK_INNER_X = TRACK_X + (TRACK_WIDTH - TRACK_INNER_WIDTH) / 2;
const COLLAR_WIDTH = 16;
const COLLAR_X = DEVICE_CENTER_X - COLLAR_WIDTH / 2;
const SLEEVE_WIDTH = 14;
const SLEEVE_X = DEVICE_CENTER_X - SLEEVE_WIDTH / 2;
const SCREEN_WIDTH = 20;
const SCREEN_X = DEVICE_CENTER_X - SCREEN_WIDTH / 2;

function paceLimiterLabel(limiter: string): string {
  if (limiter === "device_velocity") return t("device velocity");
  if (limiter === "acceleration") return t("acceleration");
  if (limiter === "jerk") return t("smoothness");
  if (limiter === "reversal_spacing") return t("reversal spacing");
  if (limiter === "curve_geometry") return t("curve geometry");
  return limiter.replaceAll("_", " ");
}

export function MotionVisualizer({ motion, mini = false }: { motion: MotionInfo | null; mini?: boolean }) {
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
  // The stroke channel and carriage ride on the device center axis. 100% is the
  // top of the channel.
  const travelTop = 30;
  const travelBottom = 104;
  const toChannelY = (percent: number) => travelBottom - ((travelBottom - travelTop) * percent) / 100;
  const rangeTop = toChannelY(max);
  const rangeBottom = toChannelY(min);
  const carriageStyle = { "--viz-carriage-y": `${toChannelY(pos)}px` } as CSSProperties;
  const label = t("Motion {state}; pattern {pattern}; commanded position estimate {position} percent; stroke range {minimum} to {maximum} percent", {
    state: stateLabel,
    pattern: patternName,
    position: formatNumber(roundedPosition),
    minimum: formatNumber(Math.round(min)),
    maximum: formatNumber(Math.round(max)),
  });

  return (
    <div className={`visualizer${mini ? " mini" : ""}`} data-state={state} role="img" aria-label={label}>
      <svg
        className="viz-device"
        data-position={roundedPosition}
        data-range-min={Math.round(min)}
        data-range-max={Math.round(max)}
        viewBox="0 0 96 132"
        preserveAspectRatio="xMidYMid meet"
        aria-hidden="true"
      >
        {/* Body: vertical capsule centered on the motion axis. */}
        <rect className="viz-body" x={BODY_X} y="10" width={BODY_WIDTH} height="112" rx="24" />
        <path className="viz-grip" d={`M${BODY_X + 10} 35h${BODY_WIDTH - 20}M${BODY_X + 10} 47h${BODY_WIDTH - 20}M${BODY_X + 10} 59h${BODY_WIDTH - 20}`} />
        <rect className="viz-screen" x={SCREEN_X} y="66" width={SCREEN_WIDTH} height="26" rx="4" />
        <circle className="viz-device-led" cx={DEVICE_CENTER_X} cy="36" r="2.6" />
        {/* Stroke channel on the center axis, with the active range inside it. */}
        <rect className="viz-track" x={TRACK_X} y="24" width={TRACK_WIDTH} height="84" rx="4.5" />
        <rect
          className="viz-stroke-range"
          x={TRACK_INNER_X}
          y={rangeTop}
          width={TRACK_INNER_WIDTH}
          height={Math.max(3, rangeBottom - rangeTop)}
          rx="3.6"
        />
        {/* Sleeve carriage: collar + ribbed sleeve, moving vertically on the axis. */}
        <g className="viz-carriage" style={carriageStyle}>
          <rect className="viz-carriage-sleeve" x={SLEEVE_X} y="-9" width={SLEEVE_WIDTH} height="18" rx="6.5" />
          <path
            className="viz-sleeve-rib"
            d={`M${SLEEVE_X + 3} -3.5h${SLEEVE_WIDTH - 6}M${SLEEVE_X + 3} 0h${SLEEVE_WIDTH - 6}M${SLEEVE_X + 3} 3.5h${SLEEVE_WIDTH - 6}`}
          />
          <rect className="viz-carriage-collar" x={COLLAR_X} y="-7.5" width={COLLAR_WIDTH} height="15" rx="5" />
        </g>
      </svg>
      {!mini && (
        <div className="viz-telemetry">
          <div className="viz-summary">
            <span className="viz-state"><span className="viz-state-dot" aria-hidden="true" />{stateLabel}</span>
            <span className="viz-commanded"><strong>{roundedPosition}%</strong><small>{t("commanded")}</small></span>
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
