interface SpeedGaugeProps {
  value: number;
  label: string;
}

export function SpeedGauge({ value, label }: SpeedGaugeProps) {
  const clamped = Math.max(0, Math.min(100, value));
  const angle = -120 + (clamped / 100) * 240;

  return (
    <div className="motion-gauge" aria-label={`${label}: ${Math.round(clamped)}`}>
      <div className="motion-gauge-readout">
        <span className="motion-gauge-value">{Math.round(clamped)}</span>
      </div>
      <svg viewBox="0 0 120 64" className="motion-gauge-svg" aria-hidden>
        <path d="M 18 56 A 42 42 0 1 1 102 56" className="motion-gauge-track" />
        <path
          d="M 18 56 A 42 42 0 1 1 102 56"
          className="motion-gauge-arc"
          pathLength={100}
          strokeDasharray={`${clamped} 100`}
        />
        <line
          x1="60"
          y1="56"
          x2="60"
          y2="22"
          className="motion-gauge-needle"
          style={{ transform: `rotate(${angle}deg)`, transformOrigin: "60px 56px" }}
        />
        <circle cx="60" cy="56" r="4" className="motion-gauge-hub" />
      </svg>
      <span className="motion-gauge-label">{label}</span>
    </div>
  );
}
