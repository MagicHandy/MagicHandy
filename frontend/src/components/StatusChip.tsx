type ChipVariant = "default" | "accent" | "success" | "warn" | "danger" | "muted";

export function StatusChip({
  label,
  variant = "default",
  title,
  pulse,
}: {
  label: string;
  variant?: ChipVariant;
  title?: string;
  pulse?: boolean;
}) {
  return (
    <span
      className={`ui-chip ui-chip--${variant}${pulse ? " ui-chip--pulse" : ""}`}
      title={title}
    >
      {label}
    </span>
  );
}
