import { render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { BrandMark } from "./BrandMark";
import { applyBrandFavicon, brandFaviconSVG } from "./favicon";

describe("brand mark", () => {
  it("is decorative and coloured only through the brand tokens", () => {
    const { container } = render(<BrandMark />);
    const svg = container.querySelector("svg.brand-mark");
    expect(svg).toHaveAttribute("aria-hidden", "true");
    expect(svg).toHaveAttribute("viewBox", "0 0 64 64");
    expect(container.querySelectorAll(".brand-mark-hand")).toHaveLength(1);
    expect(container.querySelectorAll(".brand-mark-accent")).toHaveLength(2);
    // No hard-coded colour: a palette change must be enough to recolour it.
    expect(container.innerHTML).not.toMatch(/fill="#|style=/);
  });
});

describe("brand tab icon", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    document.querySelectorAll('link[rel="icon"]').forEach((link) => link.remove());
  });

  function tokens(values: Record<string, string>) {
    vi.spyOn(window, "getComputedStyle").mockReturnValue({
      getPropertyValue: (name: string) => values[name] ?? "",
    } as unknown as CSSStyleDeclaration);
  }

  function iconMarkup() {
    const href = document.querySelector('link[rel="icon"]')?.getAttribute("href") ?? "";
    return decodeURIComponent(href.replace("data:image/svg+xml,", ""));
  }

  it("draws the centred star in the given colours", () => {
    const svg = brandFaviconSVG("#101010", "#abcdef");
    expect(svg).toContain('fill="#101010"');
    expect(svg.match(/fill="#abcdef"/g)).toHaveLength(1);
    expect(svg).not.toContain("<path");
  });

  it("repaints from the palette applied to the document", () => {
    tokens({ "--brand-ground": " #3f7fbd", "--brand-accent": " #ffffff" });
    applyBrandFavicon();
    expect(iconMarkup()).toContain('fill="#3f7fbd"');
    expect(iconMarkup()).toContain('fill="#ffffff"');

    tokens({ "--brand-ground": "#182338", "--brand-accent": "#d7a545" });
    applyBrandFavicon();
    expect(document.querySelectorAll('link[rel="icon"]')).toHaveLength(1);
    expect(iconMarkup()).toContain('fill="#d7a545"');
  });

  it("falls back to the default palette when tokens are missing or not plain colours", () => {
    tokens({ "--brand-ground": "", "--brand-accent": '"/><script>' });
    applyBrandFavicon();
    expect(iconMarkup()).toContain('fill="#1d2024"');
    expect(iconMarkup()).toContain('fill="#5b9dd9"');
    expect(iconMarkup()).not.toContain("script");
  });
});
