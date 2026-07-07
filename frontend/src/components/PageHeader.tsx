import type { ReactNode } from "react";

export function PageHeader({
  title,
  subtitle,
  actions,
  compact,
}: {
  title: string;
  subtitle?: string;
  actions?: ReactNode;
  compact?: boolean;
}) {
  return (
    <header className={`page-head${compact ? " compact-head" : ""}`}>
      <div>
        <h1>{title}</h1>
        {subtitle && <p className="subtitle">{subtitle}</p>}
      </div>
      {actions && <div className="page-head-actions">{actions}</div>}
    </header>
  );
}
