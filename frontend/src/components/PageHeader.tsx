import type { ReactNode } from "react";

export function PageHeader({
  title,
  subtitle,
  intro,
  actions,
  compact,
}: {
  title: string;
  subtitle?: string;
  intro?: string;
  actions?: ReactNode;
  compact?: boolean;
}) {
  return (
    <>
      <header className={`page-head${compact ? " compact-head" : ""}`}>
        <div>
          <h1>{title}</h1>
          {subtitle && <p className="subtitle">{subtitle}</p>}
        </div>
        {actions && <div className="page-head-actions">{actions}</div>}
      </header>
      {intro && (
        <div className="page-intro" role="note">
          <svg
            className="page-intro-icon"
            viewBox="0 0 20 20"
            fill="currentColor"
            aria-hidden
          >
            <path
              fillRule="evenodd"
              d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z"
              clipRule="evenodd"
            />
          </svg>
          <p>{intro}</p>
        </div>
      )}
    </>
  );
}
