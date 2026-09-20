# Mobile Chat spacing and connection alignment

Baseline: `5399767494d8660eba780055941b9e4591b0cc0a`.

At 390 × 844, status readouts and the read-only controller action pushed the
connection trigger beyond the header's clipped edge. A phone-specific rule
also capped the message log at 240px. Absolute hidden labels below the visible
workspace contributed to an extra document scrollbar.

The header now reserves room for actions and lets only its readout group shrink
or wrap. The connection button's mobile grid is centered explicitly. Desktop
had a second alignment cause: its two-line label could enlarge the implicit
grid row inside a fixed-height button. A bounded row, symmetric padding and
explicit label line height fix that without nudging the artwork.

Chat now fills the available workspace at the stacked breakpoint. The controls
heading remains visible below it, with the existing controls reachable by
scrolling. The composer stays outside the scrolling message log. Phone margins,
bubble insets and tab spacing are reduced; neutral selected-tab outlines match
the compact settings navigation. No motion, controller or chat behavior changes.

## Browser evidence

Chromium viewport checks used an isolated simulated app with hardware disconnected.
Both controller and read-only layouts were checked; no control takeover was used.
These are desktop-browser viewport checks, not a physical iOS/Android keyboard test.

| Viewport | Message-log height | Composer bottom | Mobile footer top |
| --- | ---: | ---: | ---: |
| 320 × 640 | 317px | 470px | 533px |
| 360 × 740 | 417px | 570px | 633px |
| 390 × 844 | 521px, previously 240px | 674px | 737px |
| 430 × 932 | 609px | 762px | 825px |
| 768 × 1024 | 675px | 846px | 917px |
| 390 × 450 | 136px | 289px | 344px |

The 900px stacked boundary, the first width above it, and 1280 × 800 desktop
were also inspected. All checked pages fit the viewport without document
overflow. The connection icon's vertical center offset rounds to 0.00px at
every width; its mobile horizontal offset is also zero. At 320 × 640, the open
connection panel ends at y=524 and Stop begins at y=540, leaving Stop reachable.

All 658 frontend tests, TypeScript, localization and the production UI build
pass. Embedded-asset and architecture tests and a pure-Go build pass. The local
Ollama model completed a real text-only app chat response without motion or a
repair/fallback diagnostic. Bundle/startup measurements are in the scorecard.
