// Monochrome inline SVG icons drawn with currentColor — no emoji, no icon CDN
// (docs/ui-design-guidelines.md). 18px default, inherits color from the parent.
type P = { size?: number; className?: string };
const base = (size: number, className?: string) => ({
  width: size,
  height: size,
  viewBox: "0 0 24 24",
  fill: "none",
  stroke: "currentColor",
  strokeWidth: 1.7,
  strokeLinecap: "round" as const,
  strokeLinejoin: "round" as const,
  "aria-hidden": true,
  className,
});

export const ChatIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)}>
    <path d="M4 5h16v11H8l-4 3z" />
  </svg>
);
export const ModesIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)}>
    <rect x="5" y="7" width="14" height="12" rx="3" />
    <circle cx="9.5" cy="12" r="1" />
    <circle cx="14.5" cy="12" r="1" />
    <path d="M12 4v3" />
  </svg>
);
export const LibraryIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)}>
    <path d="M6 4h11a1 1 0 0 1 1 1v14l-3-2-3 2V4" />
    <path d="M6 4a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2" />
  </svg>
);
export const SettingsIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)}>
    <path d="M4 7h9" />
    <path d="M17 7h3" />
    <circle cx="15" cy="7" r="2" />
    <path d="M4 17h3" />
    <path d="M11 17h9" />
    <circle cx="9" cy="17" r="2" />
  </svg>
);
export const StopIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)} fill="currentColor" stroke="none">
    <rect x="6" y="6" width="12" height="12" rx="2" />
  </svg>
);
export const ClockIcon = ({ size = 16, className }: P) => (
  <svg {...base(size, className)}>
    <circle cx="12" cy="12" r="8" />
    <path d="M12 8v4l3 2" />
  </svg>
);
