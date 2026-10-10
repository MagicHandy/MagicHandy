import type { SVGProps } from "react";

/** What every drawing receives: engine state only, already resolved. */
export interface DrawingProps {
  /** Commanded position estimate in physical travel percent (100 = tip). */
  position: number;
  /** Stroke window in physical travel percent. */
  min: number;
  max: number;
  /** A target is active, so the stroke window and position are shown. */
  active: boolean;
  /** The status-bar form. */
  mini: boolean;
  /** Full travel of the device, for drawings that read in millimetres. */
  travelMillimetres: number;
  /** data-* attributes and classes for the drawing's root <svg>. */
  svgProps: SVGProps<SVGSVGElement>;
}

/** Linear map from a travel percent onto a drawing's coordinate range. */
export function travelY(percent: number, bottom: number, top: number): number {
  return bottom - ((bottom - top) * percent) / 100;
}
