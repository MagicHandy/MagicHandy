// Device glyphs: 16px strokes in currentColor (docs/ui-design-guidelines.md,
// Device kit). They name the device; its status lives in the dot beside it.
type GlyphProps = { size?: number; className?: string };
export type DeviceGlyph = (props: GlyphProps) => JSX.Element;

const frame = (size: number, className?: string) => ({
  width: size,
  height: size,
  viewBox: "0 0 16 16",
  fill: "none",
  stroke: "currentColor",
  strokeWidth: 1.4,
  strokeLinecap: "round" as const,
  strokeLinejoin: "round" as const,
  "aria-hidden": true,
  className,
});

export const HandyGlyph: DeviceGlyph = ({ size = 16, className }) => (
  <svg {...frame(size, className)}>
    <rect x="5.5" y="1.5" width="5" height="13" rx="2.5" />
    <path d="M8 5v3" />
  </svg>
);

export const CoyoteGlyph: DeviceGlyph = ({ size = 16, className }) => (
  <svg {...frame(size, className)}>
    <rect x="2" y="3.5" width="12" height="7.5" rx="2" />
    <path d="M5.5 11v3M10.5 11v3M5 7.25h1.5M9.5 7.25h1.5" />
  </svg>
);

export const VibratorGlyph: DeviceGlyph = ({ size = 16, className }) => (
  <svg {...frame(size, className)}>
    <rect x="5.5" y="1.5" width="5" height="11" rx="2.5" />
    <path d="M8 12.5v2M2.8 5.5c-.8 1.4-.8 3.6 0 5M13.2 5.5c.8 1.4.8 3.6 0 5" />
  </svg>
);

export const IntifaceGlyph: DeviceGlyph = ({ size = 16, className }) => (
  <svg {...frame(size, className)}>
    <circle cx="8" cy="8" r="2" />
    <circle cx="3" cy="3.5" r="1.4" strokeWidth={1.3} />
    <circle cx="13" cy="3.5" r="1.4" strokeWidth={1.3} />
    <circle cx="8" cy="14" r="1.4" strokeWidth={1.3} />
    <path d="M6.4 6.7 4.1 4.6M9.6 6.7l2.3-2.1M8 10v2.6" strokeWidth={1.3} />
  </svg>
);

export const HostBluetoothGlyph: DeviceGlyph = ({ size = 16, className }) => (
  <svg {...frame(size, className)}>
    <path d="M4.5 5l7 6-3.5 3V2l3.5 3-7 6" />
  </svg>
);

/** The device chip with no device added. */
export const DevicesGlyph: DeviceGlyph = ({ size = 16, className }) => (
  <svg {...frame(size, className)}>
    <path d="M3 9.5a5 5 0 0 1 10 0M5.2 9.6a2.8 2.8 0 0 1 5.6 0" />
    <circle cx="8" cy="11.6" r="1.1" fill="currentColor" stroke="none" />
  </svg>
);
