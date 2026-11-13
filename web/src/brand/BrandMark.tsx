import { BRAND_HAND, BRAND_STAR, BRAND_STREAK } from "./paths";

/**
 * The MagicHandy mark: a hand that has just flicked a star into the air.
 *
 * It is decoration beside the product name, so it is hidden from assistive
 * technology. It carries no colour of its own: the hand takes --brand-hand and
 * the star and streak take --brand-accent, so the mark changes with the saved
 * palette. It is not a status indicator and must never signal device state.
 */
export function BrandMark() {
  return (
    <svg className="brand-mark" viewBox="0 0 64 64" aria-hidden="true" focusable="false">
      <path className="brand-mark-hand" d={BRAND_HAND} fillRule="evenodd" />
      <path className="brand-mark-accent" d={BRAND_STREAK} />
      <polygon className="brand-mark-accent" points={BRAND_STAR} />
    </svg>
  );
}
