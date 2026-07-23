import type { CSSProperties } from "react";

interface IntensityFlameProps {
  value: number;
  label: string;
}

function layerCount(value: number): number {
  if (value <= 0) return 0;
  return Math.min(10, Math.floor(value / 10) + 1);
}

function layerGradient(layerIndex: number, totalLayers: number, intensity: number): string {
  const outerRatio = totalLayers <= 1 ? 0 : layerIndex / (totalLayers - 1);

  if (layerIndex === 0) {
    return "linear-gradient(to top, #f59e0b 0%, #fef08a 72%, #fffbeb 100%)";
  }

  if (intensity > 70 && outerRatio >= 0.45) {
    const blueMix = Math.min(1, (intensity - 70) / 30);
    const outerBlend = (outerRatio - 0.45) / 0.55;
    const t = blueMix * outerBlend;
    if (t >= 0.75) {
      return "linear-gradient(to top, #1d4ed8 0%, #22d3ee 45%, #67e8f9 100%)";
    }
    if (t >= 0.4) {
      return "linear-gradient(to top, #7c3aed 0%, #2563eb 55%, #38bdf8 100%)";
    }
    return "linear-gradient(to top, #b91c1c 0%, #dc2626 40%, #38bdf8 100%)";
  }

  if (layerIndex <= 2) {
    return "linear-gradient(to top, #ea580c 0%, #fb923c 42%, #fde047 100%)";
  }
  if (layerIndex <= 5) {
    return "linear-gradient(to top, #c2410c 0%, #f97316 50%, #fdba74 100%)";
  }
  return "linear-gradient(to top, #7f1d1d 0%, #dc2626 48%, #f87171 100%)";
}

function layerSize(layerIndex: number, intensity: number) {
  const scale = 0.45 + (intensity / 100) * 0.55;
  const width = (11 + layerIndex * 5.2) * scale;
  const height = (13 + layerIndex * 7.5) * scale;
  const sway = layerIndex % 2 === 0 ? -1 : 1;
  return { width, height, sway };
}

export function IntensityFlame({ value, label }: IntensityFlameProps) {
  const clamped = Math.max(0, Math.min(100, value));
  const layers = layerCount(clamped);
  const displayLayers = layers === 0 ? 1 : layers;
  const isIdle = clamped <= 0;
  const intensityTier = clamped > 70 ? "hot" : clamped > 35 ? "warm" : "low";

  return (
    <div
      className="motion-flame"
      aria-label={`${label}: ${Math.round(clamped)}`}
      role="img"
    >
      <div
        className={`motion-flame-visual${isIdle ? " motion-flame-visual--idle" : ""}`}
        data-intensity-tier={isIdle ? "idle" : intensityTier}
        style={{ "--flame-layers": displayLayers } as CSSProperties}
      >
        {Array.from({ length: displayLayers }, (_, layerIndex) => {
          const { width, height, sway } = layerSize(layerIndex, clamped);
          const style = {
            "--flame-w": `${width}px`,
            "--flame-h": `${height}px`,
            "--flame-gradient": layerGradient(layerIndex, displayLayers, clamped),
            "--flame-sway": sway,
            "--layer-index": layerIndex,
            animationDelay: `${layerIndex * 0.09}s`,
          } as CSSProperties;

          return (
            <span
              key={layerIndex}
              className="motion-flame-layer"
              style={style}
              aria-hidden
            />
          );
        })}
        <span className="motion-flame-base" aria-hidden />
      </div>
      <span className="motion-flame-value">{Math.round(clamped)}</span>
      <span className="motion-flame-label">{label}</span>
    </div>
  );
}
