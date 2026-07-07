type Props = {
  url?: string | null;
  name?: string;
  size?: number;
  className?: string;
};

export function PersonaAvatar({ url, name, size = 36, className = "" }: Props) {
  const label = name ?? "Persona";
  if (url) {
    return (
      <img
        src={url}
        alt={label}
        className={`persona-avatar ${className}`.trim()}
        width={size}
        height={size}
        loading="lazy"
      />
    );
  }
  const initial = label.trim().charAt(0).toUpperCase() || "?";
  return (
    <span
      className={`persona-avatar persona-avatar-fallback ${className}`.trim()}
      style={{ width: size, height: size, fontSize: size * 0.42 }}
      aria-hidden
    >
      {initial}
    </span>
  );
}
