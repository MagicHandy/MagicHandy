interface SpeedGaugeProps {
  value: number;
  label: string;
}

export function SpeedGauge({ value, label }: SpeedGaugeProps) {
  const clamped = Math.max(0, Math.min(100, value));
  const angle = -120 + (clamped / 100) * 240;

  return (
    <div className="motion-gauge" aria-label={label}>
      <svg viewBox="0 0 120 72" className="motion-gauge-svg">
        <path d="M 18 60 A 42 42 0 1 1 102 60" className="motion-gauge-track" />
        <path
          d="M 18 60 A 42 42 0 1 1 102 60"
          className="motion-gauge-arc"
          pathLength={100}
          strokeDasharray={`${clamped} 100`}
        />
        <line
          x1="60"
          y1="60"
          x2="60"
          y2="24"
          className="motion-gauge-needle"
          style={{ transform: `rotate(${angle}deg)`, transformOrigin: "60px 60px" }}
        />
        <circle cx="60" cy="60" r="4" className="motion-gauge-hub" />
        <text x="60" y="68" textAnchor="middle" className="motion-gauge-value">
          {Math.round(clamped)}
        </text>
      </svg>
      <span className="motion-gauge-label">{label}</span>
    </div>
  );
}
