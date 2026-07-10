interface StatusBarProps {
  label: string;
  value: number;
  valueLabel?: string;
  variant?: "stamina" | "mood";
}

export function StatusBar({ label, value, valueLabel, variant = "stamina" }: StatusBarProps) {
  const clamped = Math.max(0, Math.min(100, value));

  return (
    <div className={`chat-auto-bar chat-auto-bar--${variant}`} aria-label={label}>
      <div className="chat-auto-bar-head">
        <span className="chat-auto-bar-label">{label}</span>
        <span className="chat-auto-bar-value">
          {valueLabel ?? `${Math.round(clamped)}%`}
        </span>
      </div>
      <div className="chat-auto-bar-track" aria-hidden>
        <div className="chat-auto-bar-fill" style={{ width: `${clamped}%` }} />
      </div>
    </div>
  );
}
