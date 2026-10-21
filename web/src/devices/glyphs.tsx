// Device glyphs share a centered 16-unit drawing grid; tiles scale them to
// 20/24/28px. Status belongs to the separate dot, never the glyph artwork.
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
  className: ["device-glyph", className].filter(Boolean).join(" "),
});

export const HandyGlyph: DeviceGlyph = ({ size = 16, className }) => (
  <svg {...frame(size, className)}>
    <rect x="5" y="1.5" width="6" height="13" rx="2.5" />
    <path d="M8 5v3" />
  </svg>
);

export const CoyoteGlyph: DeviceGlyph = ({ size = 16, className }) => (
  <svg {...frame(size, className)}>
    <path d="m9.5 1.5-6 7.25H7l-.5 5.75 6-7.25H9z" />
  </svg>
);

export const VibratorGlyph: DeviceGlyph = ({ size = 16, className }) => (
  <svg {...frame(size, className)}>
    <rect x="5.5" y="2" width="5" height="10.5" rx="2.5" />
    <path d="M8 12.5V14M2.8 5.5c-.8 1.4-.8 3.6 0 5M13.2 5.5c.8 1.4.8 3.6 0 5" />
  </svg>
);

export const IntifaceGlyph: DeviceGlyph = ({ size = 16, className }) => (
  <svg {...frame(size, className)}>
    <circle cx="8" cy="7.5" r="2" />
    <circle cx="3" cy="3.2" r="1.4" strokeWidth={1.3} />
    <circle cx="13" cy="3.2" r="1.4" strokeWidth={1.3} />
    <circle cx="8" cy="12.8" r="1.4" strokeWidth={1.3} />
    <path d="m6.5 6.2-2.4-2M9.5 6.2l2.4-2M8 9.5v1.9" strokeWidth={1.3} />
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
    <path d="M2 5.6a9 9 0 0 1 12 0M4.2 8.4a5.7 5.7 0 0 1 7.6 0" />
    <circle cx="8" cy="11.6" r="1.1" fill="currentColor" stroke="none" />
  </svg>
);
