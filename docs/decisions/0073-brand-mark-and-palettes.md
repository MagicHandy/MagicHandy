# ADR 0073: Brand mark and a palette catalog built around it

## Status

Accepted 2026-11-13 for publication at the owner's direction, preserving the
approved artwork and aesthetic. Replaces the 21 mockup-derived opt-in palettes described
in [the UI design guidelines](../ui-design-guidelines.md#opt-in-palettes).

## Context

The app had no mark. The navigation rail showed the letter "M", the startup
screen showed "MH", and the browser tab had no icon.

The palette catalog had grown to 22 entries taken from early mockups. Many were
near-duplicates, three were violet, and several used a green or red accent that
sits close to the safety colors. None of them was designed with a mark in mind.

## Decision

1. **One mark, three roles.** The mark is a hand that has just flicked a star
   into the air: a hand, a five-pointed star on the line of the index finger,
   and one streak behind the star whose head is notched to take the star's
   point. Its geometry lives in `web/src/brand/paths.ts` and carries no color.
2. **The palette colors the mark.** Three tokens paint it: `--brand-ground`
   (the tile), `--brand-hand` and `--brand-accent` (star and streak). They
   default to `--surface-2`, `--text` and `--accent`, so every palette recolors
   the mark with no extra work. A palette overrides them only when its mark
   should differ from its interface, as Ember and Harbor do.
3. **Where the mark appears.** The navigation rail, the phone remote's header
   and both startup screens render `BrandMark`. The browser tab shows the
   star alone, centred, with no hand or streak, because the hand is not
   legible below about 32 px. `applyBrandFavicon` repaints that star
   whenever the document's `data-theme` changes; `index.html` carries the
   default-palette glyph for the moments before the app has loaded.
4. **Eight palettes.** Steel Azure stays the default and Warm Titanium keeps
   its values. Six are new: Ember, Midnight Gold, Forest Brass, Harbor, Carbon
   and High Contrast. All are dark. None uses a violet, green or red accent.
5. **Retired palettes fall back.** A saved value that is no longer in the
   catalog loads as Steel Azure, through the existing obsolete-theme fallback.
   High Contrast keeps its ID with new values, so people who chose it keep a
   high-contrast palette.
6. **The mark is not a status light.** It never signals device, session or
   connection state. Green, amber and red keep their safety meanings.

## Consequences

- People who had picked one of the 19 removed palettes see Steel Azure after
  updating and can choose again in Settings.
- The catalog is small enough for each palette to carry contrast evidence.
  Every palette meets 4.5:1 for text and muted text on the page and on cards,
  4.5:1 for accent ink on the accent, and 3:1 for the accent, the focus ring
  and both mark colors against their grounds.
- The main script grows by the mark's geometry and the stylesheet shrinks by
  the removed palettes; the measurement is in
  [the goal scorecard](../goal-scorecard.md).
- The phone remote does not apply the saved palette today, so its mark stays
  in the default colors. That is existing behavior and is left unchanged.

## Not covered

- The Windows executable icon and the installer artwork are covered by
  [ADR 0074](0074-windows-icon-and-installer-artwork.md). The README header
  still has no mark.
- There is no light palette. The safety colors and several components assume
  a dark ground, so a light palette needs its own review.
- The startup animation is not built.
