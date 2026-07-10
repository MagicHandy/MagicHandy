import type { CSSProperties } from "react";

interface IntensityFlameProps {
  value: number;
  label: string;
}

export function IntensityFlame({ value, label }: IntensityFlameProps) {
  const clamped = Math.max(0, Math.min(100, value));
  const height = 8 + (clamped / 100) * 44;

  return (
    <div className="motion-flame" aria-label={label}>
      <div className="motion-flame-visual" style={{ "--flame-h": `${height}px` } as CSSProperties}>
        <span className="motion-flame-core" />
        <span className="motion-flame-outer" />
      </div>
      <span className="motion-flame-value">{Math.round(clamped)}</span>
      <span className="motion-flame-label">{label}</span>
    </div>
  );
}
