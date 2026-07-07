/**
 * Funscript heatmap estilo EroScripts / Lucifie (funscript-utils).
 * Timeline absoluto + stats por extrema + bandas verticais nítidas.
 */

export type FunscriptAction = { at: number; pos: number };

type Rgb = [number, number, number];

const HEATMAP_COLORS: Rgb[] = [
  [0, 0, 0],
  [30, 144, 255],
  [34, 139, 34],
  [255, 215, 0],
  [220, 20, 60],
  [147, 112, 219],
  [37, 22, 122],
];

const STEP_SIZE = 120;
const GAP_THRESHOLD_MS = 5000;
const X_WINDOW = 50;
/** Suavização extra entre colunas (degradê horizontal). */
const COLUMN_BLEND = true;

export function getSpeed(a: FunscriptAction, b: FunscriptAction): number {
  if (a.at === b.at) return 0;
  let first = a;
  let second = b;
  if (second.at < first.at) {
    first = b;
    second = a;
  }
  const dt = Math.abs(second.at - first.at);
  if (dt <= 0) return 0;
  return (1000 * Math.abs(second.pos - first.pos)) / dt;
}

function lerpColor(a: Rgb, b: Rgb, t: number): Rgb {
  return [
    Math.round(a[0] + (b[0] - a[0]) * t),
    Math.round(a[1] + (b[1] - a[1]) * t),
    Math.round(a[2] + (b[2] - a[2]) * t),
  ];
}

export function getHeatmapColor(intensity: number): Rgb {
  if (intensity <= 0) return HEATMAP_COLORS[0];
  if (intensity > 5 * STEP_SIZE) return HEATMAP_COLORS[6];

  const v = intensity + STEP_SIZE / 2;
  const lo = Math.floor(v / STEP_SIZE);
  const hi = lo + 1;
  if (lo >= HEATMAP_COLORS.length - 1) return HEATMAP_COLORS[HEATMAP_COLORS.length - 1];
  const t = Math.max(0, Math.min(1, (v - lo * STEP_SIZE) / STEP_SIZE));
  return lerpColor(HEATMAP_COLORS[lo], HEATMAP_COLORS[Math.min(hi, HEATMAP_COLORS.length - 1)], t);
}

export function formatRgb(c: Rgb, alpha = 1): string {
  return `rgb(${c[0]}, ${c[1]}, ${c[2]}, ${alpha})`;
}

/** Realça saturação/brilho para a linha do waveform (estilo neon EroScripts). */
function boostNeonColor(c: Rgb): Rgb {
  const sum = c[0] + c[1] + c[2];
  if (sum < 24) return c;
  const peak = Math.max(c[0], c[1], c[2]) / 255;
  const gain = 1.18 + peak * 0.22;
  return [
    Math.min(255, Math.round(c[0] * gain + 12)),
    Math.min(255, Math.round(c[1] * gain + 10)),
    Math.min(255, Math.round(c[2] * gain + 8)),
  ];
}

type ColorSample = { x: number; color: Rgb };

function buildColorTimeline(
  actions: FunscriptAction[],
  timelineMs: number,
  chartW: number,
): ColorSample[] {
  if (actions.length < 2 || timelineMs <= 0 || chartW <= 0) return [];

  const msToX = chartW / timelineMs;
  const samples: ColorSample[] = [];
  let intensityList: number[] = [];

  for (let i = 1; i < actions.length; i += 1) {
    if (GAP_THRESHOLD_MS > 0 && actions[i].at - actions[i - 1].at > GAP_THRESHOLD_MS) {
      intensityList = [];
      continue;
    }

    const intensity = getSpeed(actions[i - 1], actions[i]);
    intensityList.push(intensity);
    if (intensityList.length > X_WINDOW) intensityList = intensityList.slice(-X_WINDOW);

    const avgIntensity =
      intensityList.reduce((acc, cur) => acc + cur, 0) / intensityList.length;

    samples.push({
      x: msToX * actions[i].at,
      color: getHeatmapColor(avgIntensity),
    });
  }

  return samples;
}

function sampleColorAtX(samples: ColorSample[], x: number): Rgb {
  if (samples.length === 0) return HEATMAP_COLORS[0];
  if (x <= samples[0].x) return samples[0].color;
  if (x >= samples[samples.length - 1].x) return samples[samples.length - 1].color;

  let lo = 0;
  for (let i = 1; i < samples.length; i += 1) {
    if (samples[i].x >= x) {
      lo = i - 1;
      break;
    }
    lo = i - 1;
  }

  const a = samples[lo];
  const b = samples[Math.min(lo + 1, samples.length - 1)];
  if (b.x <= a.x) return a.color;
  const t = Math.max(0, Math.min(1, (x - a.x) / (b.x - a.x)));
  return COLUMN_BLEND ? lerpColor(a.color, b.color, t) : a.color;
}

function drawSmoothColorBand(
  ctx: CanvasRenderingContext2D,
  samples: ColorSample[],
  x: number,
  y: number,
  w: number,
  h: number,
  alpha: number,
): void {
  if (w <= 0 || h <= 0 || samples.length === 0) return;

  const width = Math.max(1, Math.floor(w));
  const height = Math.max(1, Math.floor(h));
  const img = ctx.createImageData(width, height);
  const data = img.data;
  const a = Math.round(Math.max(0, Math.min(1, alpha)) * 255);

  for (let col = 0; col < width; col += 1) {
    const color = sampleColorAtX(samples, x + col);
    for (let row = 0; row < height; row += 1) {
      const offset = (row * width + col) * 4;
      data[offset] = color[0];
      data[offset + 1] = color[1];
      data[offset + 2] = color[2];
      data[offset + 3] = a;
    }
  }

  ctx.putImageData(img, x, y);
}

export type HeatmapStats = {
  actionCount: number;
  durationMs: number;
  maxSpeed: number;
  avgSpeed: number;
};

export function roundActions(actions: FunscriptAction[]): FunscriptAction[] {
  return actions
    .filter((a) => Number.isFinite(a.at) && Number.isFinite(a.pos))
    .map((a) => ({
      at: Math.max(0, Math.round(a.at)),
      pos: Math.max(0, Math.min(100, Math.round(a.pos))),
    }))
    .sort((a, b) => a.at - b.at);
}

export function resolveScriptDurationMs(
  actions: FunscriptAction[],
  metadataDuration?: number | null,
): number {
  const rounded = roundActions(actions);
  if (rounded.length === 0) return 0;

  const metaMs =
    metadataDuration != null && Number.isFinite(metadataDuration) && metadataDuration > 0
      ? metadataDuration < 100_000
        ? Math.round(metadataDuration * 1000)
        : Math.round(metadataDuration)
      : null;

  const motionEnd = motionPlaybackEndMs(rounded);
  const firstAt = rounded[0].at;

  if (rounded.length < 2) {
    return metaMs ?? motionEnd ?? firstAt;
  }

  const activeSpan = Math.max(1, motionEnd - firstAt);

  if (metaMs && metaMs > 0) {
    const trailing = metaMs - motionEnd;
    if (trailing > GAP_THRESHOLD_MS) {
      const motionSpan = Math.max(1, motionEnd - firstAt);
      if (trailing > 15_000 || trailing / motionSpan > 0.25) {
        return firstAt <= 0 ? activeSpan : motionEnd;
      }
    }
    return metaMs;
  }

  return firstAt <= 0 ? activeSpan : motionEnd;
}

/** Posição interpolada (0–100) no funscript no instante `tMs`. */
export function interpolateFunscriptPosition(
  actions: FunscriptAction[],
  tMs: number,
): number {
  return interpolateWaveformPosition(actions, tMs);
}

/** Posição na linha desenhada do heatmap (respeita gaps como o canvas). */
export function interpolateWaveformPosition(
  actions: FunscriptAction[],
  tMs: number,
): number {
  const rounded = roundActions(actions);
  if (rounded.length === 0) return 50;
  if (rounded.length === 1) return rounded[0].pos;

  const t = Math.max(0, tMs);
  if (t <= rounded[0].at) return rounded[0].pos;

  for (let i = 1; i < rounded.length; i += 1) {
    const prev = rounded[i - 1];
    const curr = rounded[i];
    const gap = curr.at - prev.at;

    if (gap > GAP_THRESHOLD_MS) {
      if (t < curr.at) return prev.pos;
      continue;
    }

    if (t <= curr.at) {
      const span = curr.at - prev.at;
      const ratio = span > 0 ? (t - prev.at) / span : 0;
      return prev.pos + (curr.pos - prev.pos) * ratio;
    }
  }

  return rounded[rounded.length - 1].pos;
}

const POSITION_EPS = 0.5;
const MAX_TAIL_HOLD_MS = 800;

function motionPlaybackEndMs(actions: FunscriptAction[]): number {
  if (actions.length === 0) return 0;
  if (actions.length === 1) return actions[0].at + 100;

  let lastChangeIdx = 0;
  for (let i = 1; i < actions.length; i += 1) {
    if (Math.abs(actions[i].pos - actions[i - 1].pos) >= POSITION_EPS) {
      lastChangeIdx = i;
    }
  }

  const lastAt = actions[lastChangeIdx].at;
  const step =
    lastChangeIdx > 0
      ? Math.max(1, lastAt - actions[lastChangeIdx - 1].at)
      : 100;
  const tail = Math.min(step, MAX_TAIL_HOLD_MS);

  if (lastChangeIdx + 1 < actions.length) {
    const nextAt = actions[lastChangeIdx + 1].at;
    const gap = nextAt - lastAt;
    if (gap <= GAP_THRESHOLD_MS) return nextAt;
    return lastAt + tail;
  }

  return lastAt + tail;
}

export function extractHeatmapExtrema(actions: FunscriptAction[]): FunscriptAction[] {
  if (actions.length < 3) return [...actions];
  const extrema: FunscriptAction[] = [actions[0]];
  for (let i = 1; i < actions.length - 1; i += 1) {
    const prev = actions[i - 1].pos;
    const pos = actions[i].pos;
    const next = actions[i + 1].pos;
    if ((pos >= prev && pos >= next) || (pos <= prev && pos <= next)) {
      const last = extrema[extrema.length - 1];
      if (last.at !== actions[i].at || last.pos !== actions[i].pos) {
        extrema.push(actions[i]);
      }
    }
  }
  const lastAction = actions[actions.length - 1];
  const tail = extrema[extrema.length - 1];
  if (tail.at !== lastAction.at || tail.pos !== lastAction.pos) {
    extrema.push(lastAction);
  }
  return extrema;
}

/** Stats estilo EroScripts (extrema + metadata.duration). */
export function computeEroScriptsStats(
  actions: FunscriptAction[],
  metadataDurationMs?: number | null,
): HeatmapStats {
  const rounded = roundActions(actions);
  if (rounded.length === 0) {
    return { actionCount: 0, durationMs: 0, maxSpeed: 0, avgSpeed: 0 };
  }

  const extrema = extractHeatmapExtrema(rounded);
  const statsActions = extrema.length > 1 ? extrema.slice(1) : extrema;
  const speeds: number[] = [];

  for (let i = 1; i < statsActions.length; i += 1) {
    const dt = statsActions[i].at - statsActions[i - 1].at;
    if (dt <= 0 || dt > GAP_THRESHOLD_MS) continue;
    speeds.push(getSpeed(statsActions[i - 1], statsActions[i]));
  }

  const durationMs =
    metadataDurationMs != null && metadataDurationMs > 0
      ? resolveScriptDurationMs(rounded, metadataDurationMs)
      : resolveScriptDurationMs(rounded);

  return {
    actionCount: statsActions.length,
    durationMs,
    maxSpeed: speeds.length ? Math.max(...speeds) : 0,
    avgSpeed: speeds.length ? speeds.reduce((a, b) => a + b, 0) / speeds.length : 0,
  };
}

export function computeHeatmapStats(actions: FunscriptAction[]): HeatmapStats {
  return computeEroScriptsStats(actions);
}

type HeatSegment = {
  x0: number;
  x1: number;
  color: Rgb;
  lineColor: Rgb;
  pos0: number;
  pos1: number;
};

function buildHeatSegments(
  actions: FunscriptAction[],
  timelineMs: number,
  chartX: number,
  chartW: number,
): HeatSegment[] {
  if (timelineMs <= 0) return [];
  const msToX = chartW / timelineMs;
  const segments: HeatSegment[] = [];
  let intensityList: number[] = [];
  let lastX = chartX;

  for (let i = 1; i < actions.length; i += 1) {
    const x = chartX + msToX * actions[i].at;

    if (GAP_THRESHOLD_MS > 0 && actions[i].at - actions[i - 1].at > GAP_THRESHOLD_MS) {
      intensityList = [];
      lastX = x;
      continue;
    }

    const instant = getSpeed(actions[i - 1], actions[i]);
    intensityList.push(instant);
    if (intensityList.length > X_WINDOW) intensityList = intensityList.slice(-X_WINDOW);

    const avgIntensity =
      intensityList.reduce((acc, cur) => acc + cur, 0) / intensityList.length;
    const bgColor = getHeatmapColor(avgIntensity);
    const lineColor = boostNeonColor(getHeatmapColor(instant));

    const x0 = lastX;
    const x1 = Math.max(x0 + 0.35, x);
    if (x1 > x0) {
      segments.push({
        x0,
        x1,
        color: bgColor,
        lineColor,
        pos0: actions[i - 1].pos,
        pos1: actions[i].pos,
      });
    }
    lastX = x1;
  }

  return segments;
}

function posToY(pos: number, regionY: number, regionH: number): number {
  return regionY + regionH - (pos / 100) * regionH;
}

function drawEroStats(
  ctx: CanvasRenderingContext2D,
  stats: HeatmapStats,
  x: number,
  y: number,
  w: number,
  h: number,
): void {
  const colW = w / 2;
  const rowH = h / 2;
  const labels = [
    ["Duration", "Actions"],
    ["Max Speed", "Avg Speed"],
  ];
  const durSec = stats.durationMs / 1000;
  const mm = Math.floor(durSec / 60);
  const ss = Math.floor(durSec % 60);
  const durLabel = mm > 0 ? `${mm}:${String(ss).padStart(2, "0")}` : `${ss}s`;
  const values = [
    [durLabel, String(stats.actionCount)],
    [String(Math.round(stats.maxSpeed)), String(Math.round(stats.avgSpeed))],
  ];

  ctx.textAlign = "center";
  ctx.textBaseline = "top";
  for (let row = 0; row < 2; row += 1) {
    for (let col = 0; col < 2; col += 1) {
      const cx = x + col * colW + colW / 2;
      const cy = y + row * rowH;
      ctx.font = "8px ui-monospace, monospace";
      ctx.fillStyle = "rgba(200, 210, 225, 0.75)";
      ctx.fillText(labels[row][col], cx, cy + 1);
      ctx.font = "bold 11px ui-monospace, monospace";
      ctx.fillStyle = "rgba(255, 255, 255, 0.95)";
      ctx.fillText(values[row][col], cx, cy + 11);
    }
  }
}

export type RenderHeatmapOptions = {
  background?: string;
  showStats?: boolean;
  stripRatio?: number;
  /** Duração do eixo X (metadata.duration ou último `at`). */
  timelineDurationMs?: number;
  /** Stats pré-calculados (API). */
  statsOverride?: HeatmapStats | null;
  /** Omit EroScripts hold line from t=0 → first keyframe (library blocks). */
  skipLeadingHoldLine?: boolean;
  /** Playhead (ms desde o início da timeline). */
  playheadMs?: number | null;
};

export function renderFunscriptHeatmap(
  canvas: HTMLCanvasElement,
  actions: FunscriptAction[],
  width: number,
  height: number,
  options: RenderHeatmapOptions = {},
): HeatmapStats | null {
  const rounded = roundActions(actions);
  const timelineMs =
    options.timelineDurationMs && options.timelineDurationMs > 0
      ? options.timelineDurationMs
      : resolveScriptDurationMs(rounded);

  const stats =
    options.statsOverride ??
    computeEroScriptsStats(rounded, timelineMs);

  if (rounded.length < 2 || timelineMs <= 0) return null;

  const showStats = options.showStats ?? false;
  const stripRatio = options.stripRatio ?? 0.26;
  const statsW = showStats ? Math.min(118, Math.max(96, width * 0.28)) : 0;

  const dpr = Math.min(window.devicePixelRatio || 1, 2.5);
  canvas.width = Math.floor(width * dpr);
  canvas.height = Math.floor(height * dpr);
  canvas.style.width = `${width}px`;
  canvas.style.height = `${height}px`;

  const ctx = canvas.getContext("2d");
  if (!ctx) return null;
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

  const bg = options.background ?? "#12141c";
  ctx.fillStyle = bg;
  ctx.fillRect(0, 0, width, height);

  const chartX = 0;
  const chartW = width - statsW;
  const stripH = Math.max(10, Math.floor(height * stripRatio));
  const waveY = stripH + 1;
  const waveH = Math.max(14, height - waveY);

  const segments = buildHeatSegments(rounded, timelineMs, chartX, chartW);
  const colorTimeline = buildColorTimeline(rounded, timelineMs, chartW);

  // Faixa superior — degradê suave coluna a coluna
  drawSmoothColorBand(ctx, colorTimeline, chartX, 0, chartW, stripH, 1);

  ctx.fillStyle = "rgba(0, 0, 0, 0.45)";
  ctx.fillRect(chartX, stripH, chartW, 1);

  // Fundo waveform escuro
  ctx.fillStyle = "#06080d";
  ctx.fillRect(chartX, waveY, chartW, waveH);

  // Heatmap de fundo — mesmo degradê, mais suave
  drawSmoothColorBand(ctx, colorTimeline, chartX, waveY, chartW, waveH, 0.38);

  // Linha fina neon (velocidade instantânea por trecho)
  const lineW = Math.max(0.55, Math.min(0.95, waveH / 52));
  ctx.lineJoin = "round";
  ctx.lineCap = "round";

  const xAt = (t: number) => chartX + (t / timelineMs) * chartW;

  const strokeNeonLine = (
    xA: number,
    yA: number,
    xB: number,
    yB: number,
    color: Rgb,
  ) => {
    const neon = boostNeonColor(color);
    ctx.strokeStyle = formatRgb(neon, 0.35);
    ctx.lineWidth = lineW + 1.4;
    ctx.shadowColor = formatRgb(neon, 0.85);
    ctx.shadowBlur = 2.5;
    ctx.beginPath();
    ctx.moveTo(xA, yA);
    ctx.lineTo(xB, yB);
    ctx.stroke();

    ctx.shadowBlur = 0;
    ctx.strokeStyle = formatRgb(neon, 1);
    ctx.lineWidth = lineW;
    ctx.beginPath();
    ctx.moveTo(xA, yA);
    ctx.lineTo(xB, yB);
    ctx.stroke();
  };

  if (!options.skipLeadingHoldLine && rounded.length > 0) {
    const xStart = xAt(rounded[0].at);
    const yStart = posToY(rounded[0].pos, waveY, waveH);
    const c0 = segments[0]?.lineColor ?? boostNeonColor(getHeatmapColor(0));
    strokeNeonLine(chartX, yStart, xStart, yStart, c0);
  }

  for (const seg of segments) {
    const y0 = posToY(seg.pos0, waveY, waveH);
    const y1 = posToY(seg.pos1, waveY, waveH);
    strokeNeonLine(seg.x0, y0, seg.x1, y1, seg.lineColor);
  }

  ctx.shadowBlur = 0;

  if (
    options.playheadMs != null &&
    options.playheadMs >= 0 &&
    timelineMs > 0
  ) {
    const t = Math.min(Math.max(0, options.playheadMs), timelineMs);
    const px = xAt(t);
    const posPct = interpolateWaveformPosition(rounded, t);
    const py = posToY(posPct, waveY, waveH);

    const r = Math.max(5, Math.min(8, waveH / 18));
    ctx.shadowColor = "rgba(255, 255, 255, 0.95)";
    ctx.shadowBlur = 10;
    ctx.fillStyle = "rgba(255, 255, 255, 0.35)";
    ctx.beginPath();
    ctx.arc(px, py, r + 3, 0, Math.PI * 2);
    ctx.fill();

    ctx.shadowBlur = 4;
    ctx.fillStyle = "#ffffff";
    ctx.beginPath();
    ctx.arc(px, py, r, 0, Math.PI * 2);
    ctx.fill();

    ctx.shadowBlur = 0;
    ctx.strokeStyle = "rgba(167, 139, 250, 0.55)";
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    ctx.arc(px, py, r + 1, 0, Math.PI * 2);
    ctx.stroke();
  }

  if (showStats && statsW > 0) {
    const sx = width - statsW;
    ctx.fillStyle = "rgba(0, 0, 0, 0.4)";
    ctx.fillRect(sx, 0, statsW, height);
    drawEroStats(ctx, stats, sx, Math.max(2, (height - 36) / 2), statsW, 36);
  }

  return stats;
}

export function pointsToActions(points: { t_ms: number; pos: number }[]): FunscriptAction[] {
  return points
    .filter((p) => Number.isFinite(p.t_ms) && Number.isFinite(p.pos))
    .map((p) => ({ at: p.t_ms, pos: p.pos }))
    .sort((a, b) => a.at - b.at);
}

export function mapHeatmapStatsFromApi(raw: {
  action_count?: number;
  duration_ms?: number;
  max_speed?: number;
  avg_speed?: number;
} | null | undefined): HeatmapStats | null {
  if (!raw) return null;
  return {
    actionCount: Number(raw.action_count ?? 0),
    durationMs: Number(raw.duration_ms ?? 0),
    maxSpeed: Number(raw.max_speed ?? 0),
    avgSpeed: Number(raw.avg_speed ?? 0),
  };
}
