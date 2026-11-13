# Alpha.87 branding review — 2026-11-13

## Source and scope

Reviewed Claude's conversation and unpublished `claude/brand-themes` work in
`Documents/MagicHandy-brand-themes`, based on alpha.86 (`a0b362fe`). The owner
explicitly requested that the images and aesthetic be preserved. An isolated
snapshot of the 27 changed/new/deleted entries was copied to
`codex/release-alpha87`; the original worktree and scratch storyboard remain
untouched. The release's product code and canonical bundle match that snapshot
byte for byte. Only review/release documentation and version-policy entries
were added during this review.

The change ships the approved vector hand/star/streak, centered-star favicon,
palette tokens, eight-entry frontend/backend catalog and existing obsolete-theme
fallback. [ADR 0073](decisions/0073-brand-mark-and-palettes.md) records the design.
No raster artwork, dependency, motion/transport/model behavior or extra UI tree
was added. The mark remains decorative and cannot signal device state.

## Code assessment

The implementation is accepted as written. Backend snapshots own the saved
theme; frontend normalization and backend validation agree on all eight IDs.
Unsupported saved themes use the existing load-time fallback, while invalid
new settings are rejected. Selecting a radio does not apply an unsaved theme.
The favicon reads computed palette colors, allows plain color syntax before
inserting it into SVG, encodes the data URL, and reuses one icon element.
The static pre-load icon matches the centered-star design. Mark paths and
styling were preserved exactly, including the existing clipping and sizing.

The catalog change is intentional: nineteen earlier entries are removed, High
Contrast keeps its ID with new colors, and Steel Azure/Warm Titanium retain
their values. Upgrade notes explain the fallback. The remote's default-only
palette, executable/installer artwork and lack of startup animation are
existing scope limits; this release does not extend those designs.

## Validation

- Frontend typecheck, localization, all **1,128 tests in 137 files**, and a
  canonical production rebuild passed. The rebuild reproduces Claude's exact
  shipping assets. Configuration and architecture suites and the
  `CGO_ENABLED=0` build passed; formatting and whitespace checks are clean.
- An independent token audit passed **72 contrast checks across eight themes**:
  text/muted text on page/cards, accent ink, accent/focus visibility and both
  mark colors on their tile. Every override preserves all safety-color tokens.
- A fresh isolated review profile loaded a retired theme as Steel Azure while
  preserving its selected local model. All eight themes were saved through the
  UI, with computed mark colors and centered-star SVG checked. Midnight Gold
  survived reload; an unsaved Warm Titanium choice left the applied theme
  unchanged until Save. High Contrast and default screenshots were captured.
- No browser warnings/errors appeared. A real Gemma 12B generation probe and
  an ordinary text-only chat succeeded. Chat returned a complete non-empty
  response in 318 ms, without repair/fallback or motion; this is readiness
  evidence, not a latency benchmark.
- The review app at `http://127.0.0.1:50364/#/settings` uses motion simulation,
  empty device credentials, working local Gemma 12B Intermediate/32K and LLM
  motion Off. It is left stopped with Steel Azure selected and Theme expanded.
  Claude's existing review process and user data were not replaced.

Ignored evidence under `.scratch/alpha87-release/` includes the original file
hash manifest, full frontend/configuration logs, contrast audit, per-theme
browser values, screenshots, LLM probe and budget measurements. No private
profile or credential was copied. No real hardware was connected or commanded.

Full PR/main/tag CI and Windows packaging remain mandatory before publication.
The unsigned allowlist adds alpha.87 only; alpha.88 and excluded alpha.65 remain
rejected. The exact final artifacts must pass vulnerability/Defender and
installer lifecycle checks, followed by public-download checksum and source
manifest verification. Budget measurements are in
[the scorecard](goal-scorecard.md).
