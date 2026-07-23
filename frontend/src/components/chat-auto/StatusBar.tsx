interface StatusBarProps {
  label: string;
  value: number;
  valueLabel?: string;
  variant?: "stamina" | "mood" | "delay";
}

export function StatusBar({ label, value, valueLabel, variant = "stamina" }: StatusBarProps) {
  const clamped = Math.max(0, Math.min(100, value));
  const display = valueLabel ?? `${Math.round(clamped)}%`;

  return (
    <div className={`chat-auto-bar chat-auto-bar--${variant}`}>
      <div className="chat-auto-bar-head">
        <span className="chat-auto-bar-label">{label}</span>
        <span className="chat-auto-bar-value">{display}</span>
      </div>
      <div
        className="chat-auto-bar-track"
        role="progressbar"
        aria-valuenow={Math.round(clamped)}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-label={`${label}: ${display}`}
      >
        <div className="chat-auto-bar-fill" style={{ width: `${clamped}%` }} />
      </div>
    </div>
  );
}
