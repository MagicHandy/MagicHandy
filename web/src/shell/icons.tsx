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
export const MicrophoneIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)}>
    <rect x="9" y="3" width="6" height="11" rx="3" />
    <path d="M6.5 11a5.5 5.5 0 0 0 11 0" />
    <path d="M12 16.5V21" />
    <path d="M9 21h6" />
  </svg>
);
export const PlayIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)} fill="currentColor" stroke="none">
    <path d="M7 4.5 19 12 7 19.5z" />
  </svg>
);
export const VideoIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)}>
    <rect x="3" y="5" width="14" height="14" rx="2" />
    <path d="m17 10 4-2v8l-4-2" />
  </svg>
);
export const PauseIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)} fill="currentColor" stroke="none">
    <rect x="6" y="5" width="4" height="14" rx="1" />
    <rect x="14" y="5" width="4" height="14" rx="1" />
  </svg>
);
export const DownloadIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)}>
    <path d="M12 3v12" />
    <path d="m7.5 10.5 4.5 4.5 4.5-4.5" />
    <path d="M5 20h14" />
  </svg>
);
export const UploadIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)}>
    <path d="M12 21V9" />
    <path d="m7.5 13.5 4.5-4.5 4.5 4.5" />
    <path d="M5 4h14" />
  </svg>
);
export const TrashIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)}>
    <path d="M5 7h14" />
    <path d="M9 7V4h6v3" />
    <path d="m7 7 1 13h8l1-13" />
  </svg>
);
export const ThumbUpIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)}>
    <path d="M7 10v10H4V10z" />
    <path d="M7 18h9.5a2 2 0 0 0 2-1.6l1-5A2 2 0 0 0 17.5 9H14l.5-3.5A2.2 2.2 0 0 0 12.3 3L7 10" />
  </svg>
);
export const ThumbDownIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)}>
    <path d="M7 14V4H4v10z" />
    <path d="M7 6h9.5a2 2 0 0 1 2 1.6l1 5a2 2 0 0 1-2 2.4H14l.5 3.5a2.2 2.2 0 0 1-2.2 2.5L7 14" />
  </svg>
);
export const UndoIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)}>
    <path d="m9 7-5 5 5 5" />
    <path d="M5 12h8a6 6 0 0 1 6 6" />
  </svg>
);
export const ClearIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)}>
    <path d="m4 15 8-8 5 5-8 8H4z" />
    <path d="m10 9 5 5" />
  </svg>
);
export const RefreshIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)}>
    <path d="M20 6v5h-5" />
    <path d="M4 18v-5h5" />
    <path d="M18.5 10a7 7 0 0 0-12-3L4 11" />
    <path d="M5.5 14a7 7 0 0 0 12 3l2.5-4" />
  </svg>
);
export const WirelessIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)}>
    <path d="M5 9.5a10 10 0 0 1 14 0" />
    <path d="M8 13a5.7 5.7 0 0 1 8 0" />
    <circle cx="12" cy="17" r="1" fill="currentColor" stroke="none" />
  </svg>
);
export const CloseIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)}>
    <path d="m6 6 12 12" />
    <path d="m18 6-12 12" />
  </svg>
);
export const ChevronUpIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)}>
    <path d="m6 15 6-6 6 6" />
  </svg>
);
export const ArrowLeftIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)}>
    <path d="m15 6-6 6 6 6" />
  </svg>
);
export const ArrowRightIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)}>
    <path d="m9 6 6 6-6 6" />
  </svg>
);
export const ZoomInIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)}>
    <circle cx="10.5" cy="10.5" r="6.5" />
    <path d="m15.5 15.5 4.5 4.5M10.5 7.5v6M7.5 10.5h6" />
  </svg>
);
export const ZoomOutIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)}>
    <circle cx="10.5" cy="10.5" r="6.5" />
    <path d="m15.5 15.5 4.5 4.5M7.5 10.5h6" />
  </svg>
);
export const FitSelectionIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)}>
    <path d="M5 4v16M19 4v16M9 12h6M9 12l2-2M9 12l2 2M15 12l-2-2M15 12l-2 2" />
  </svg>
);
export const FitAllIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)}>
    <path d="M9 4H4v5M15 4h5v5M9 20H4v-5M15 20h5v-5" />
  </svg>
);
export const PlusIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)}>
    <path d="M12 5v14M5 12h14" />
  </svg>
);
export const MoreHorizontalIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)}>
    <circle cx="5" cy="12" r="1" fill="currentColor" stroke="none" />
    <circle cx="12" cy="12" r="1" fill="currentColor" stroke="none" />
    <circle cx="19" cy="12" r="1" fill="currentColor" stroke="none" />
  </svg>
);
export const SaveIcon = ({ size = 18, className }: P) => (
  <svg {...base(size, className)}>
    <path d="M5 4h12l2 2v14H5z" />
    <path d="M8 4v6h8V4M8 20v-6h8v6" />
  </svg>
);
