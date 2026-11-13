import { BRAND_SMALL_STAR } from "./paths";

// The no-settings palette (tokens.css :root), used when the document has not
// resolved its tokens yet and in environments without a style engine.
const DEFAULT_GROUND = "#1d2024";
const DEFAULT_ACCENT = "#5b9dd9";

/** The tab icon: the star alone, centred on the palette's brand tile. */
export function brandFaviconSVG(ground: string, accent: string): string {
  return (
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64">` +
    `<rect width="64" height="64" rx="14" fill="${ground}"/>` +
    `<polygon points="${BRAND_SMALL_STAR}" fill="${accent}"/></svg>`
  );
}

// Only plain colour values are written into the icon markup.
function colour(value: string, fallback: string): string {
  const trimmed = value.trim();
  return /^#[0-9a-f]{3,8}$/i.test(trimmed) || /^rgba?\([\d\s.,%/]+\)$/i.test(trimmed) ? trimmed : fallback;
}

/**
 * Repaint the tab icon from the palette now applied to the document. Call it
 * after the document's data-theme changes.
 */
export function applyBrandFavicon(doc: Document = document): void {
  const style = getComputedStyle(doc.documentElement);
  const ground = colour(style.getPropertyValue("--brand-ground"), DEFAULT_GROUND);
  const accent = colour(style.getPropertyValue("--brand-accent"), DEFAULT_ACCENT);
  let link = doc.querySelector<HTMLLinkElement>('link[rel="icon"]');
  if (!link) {
    link = doc.createElement("link");
    link.rel = "icon";
    doc.head.appendChild(link);
  }
  link.type = "image/svg+xml";
  link.href = `data:image/svg+xml,${encodeURIComponent(brandFaviconSVG(ground, accent))}`;
}
