# UI Design Guidelines

## Purpose

The concrete, implementable reference for the sidebar-navigation shell. It is
the companion to two docs and does not repeat them:

- [ui-navigation-redesign.md](ui-navigation-redesign.md) — the information
  architecture (nav rail, workspaces, Chat Autopilot, migration).
- [ui-design.md](ui-design.md) — the enduring safety, accessibility, and parity
  rules.

This file is the **live token-and-component reference** for the shipped
React shell (status 2026-07-14: current). The tokens now live in
`web/src/styles/tokens.css` (shell and component rules in
`web/src/styles/shell.css` / `components.css`); the pre-React `web/app.css` is
retired under `web/legacy/` (reference only, not embedded). The current React
styles are authoritative where legacy values differ. The
do/don't rules keep the result from reading as generic AI output. A visual
sketch of the shell as designed is
[ui-shell-sketch.svg](ui-shell-sketch.svg) (historical artifact; its Autopilot
card in Preset Modes is superseded by the Chat placement, and the build is
authoritative wherever they differ).

## Design Tokens

Use existing tokens instead of raw hex values in implementation, and do not
invent parallel values. When this document calls out a current component-local
value, either keep it local to that component or promote it to a token before
reusing it elsewhere.

### Color roles

Steel Azure is the default palette: dark graphite with one steel-azure
interactive hue and no purple or blue-green decoration. The values below live
in `web/src/styles/tokens.css :root` and remain the no-settings fallback.

| Token | Value | Role |
| --- | --- | --- |
| `--bg` | `#0e0f11` | Page canvas |
| `--bg-inset` | `#101215` | Insets: chat log, inputs (recessed below panel) |
| `--surface` | `#17191d` | Panel/card |
| `--surface-2` | `#1e2126` | Raised/nested group, chips |
| `--surface-3` | `#262a30` | Overlay: toast, popover |
| `--line` | `#2d3138` | Hairline border/divider |
| `--line-strong` | `#3d434c` | Emphasized border |
| `--text` | `#eae9e4` | Primary text |
| `--muted` | `#98a0a9` | Secondary text, labels, hints |
| `--accent` | `#5b9dd9` | Default interactive hue (steel azure): headings' eyebrow, nav state, app actions, toggles, focus |
| `--accent-strong` | `#497fb5` | Interactive hover/border/checked |
| `--accent-ink` | `#0b131c` | Text on an accent fill |
| `--steel-deep` | `#3a5a7c` | Accent gradient partner (brand, avatar) |
| `--ok` / `--ok-strong` / `--ok-ink` | `#4fc06d` / `#3da55c` / `#08130c` | **Running/go only**: Start, active visualizer, ok status |
| `--warn` | `#d8b66a` | Warning / paused / pending |
| `--danger` / `--danger-strong` | `#e5484d` / `#c93b40` | Stop / danger |
| `--focus` | `#82b4e2` | Focus ring |
| `--user-bubble` / `--user-bubble-line` | `#28303b` / `#3a4450` | User chat bubble |

Rule: a new surface picks an existing role. `--accent` = "you can act on this",
`--ok` = "it is running", `--warn` = "caution", `--danger` = "stop".

### Opt-in palettes

Settings > General exposes one dense radio list with color swatches. The saved
`ui.theme` value is validated by the core, persisted in the SQLite settings
document, published in settings option hints, and applied to the document only
from the backend state snapshot. Steel Azure remains first and is the default
for fresh, missing, invalid, or retired values.

The 21 opt-in palettes correspond to the retained design mockups:

- Readability and neutral: High Contrast, Carbon Stealth, Platinum, Zenith
  Monochrome, Sage.
- Cool: Polar Frost, Iceberg Slate, Nordic Dusk, Cyber Midnight, Midnight
  Graphite, Ocean Trench, Obsidian Teal, Carbon Emerald.
- Violet: Moonlight Lavender, Deep Violet, Obsidian Violet.
- Warm and terminal: Warm Titanium, Solaris Amber, Amber Phosphor, Green
  Phosphor, Paperwhite CRT.

Palette selectors live in `web/src/styles/themes.css` and override existing
neutral and interactive tokens only. They must not ship component-specific
layouts, type changes, decorative glows, or a second source of UI state. These
invariants apply to every palette:

- `--ok`, `--warn`, `--danger`, and their strong/ink partners inherit the
  canonical safety colors from `tokens.css`; color is reinforced by text and
  control shape.
- Emergency Stop remains a red gradient with white text and **no box shadow or
  glow**.
- Small text and accent-ink pairs meet WCAG AA (4.5:1); focus and graphical
  accent pairs meet 3:1.
- Adding a palette requires the core catalog ID, one `themes.css` token
  selector, frontend label/swatches, contrast evidence, and catalog tests.
  Deleting any one of those pieces must fail review rather than leave a
  selectable partial theme.

### Radius scale (capped — the anti-bubble rule)

| Token / value | Use |
| --- | --- |
| `--radius` `12px` | Panels and cards |
| `--radius-sm` `8px` | Buttons, inputs, chips, chat bubbles, badges, nav rows |
| `4px` | Chat bubble tail corner only |
| `6px` | Keycap/shortcut hint |
| `999px` | Circular micro-elements: state dots, toggle track/thumb, chat avatars, range track/thumbs, scrollbar thumb |

The retired legacy CSS also used `999px` on status pills, the visualizer frame,
and the chat-provider chip. The React shell has **retired the fully-round pill
for status** (see Status readouts). Nothing button-sized or status-sized is a
pill.

### Spacing, elevation, type

- Spacing: `--gap` `16px` between regions; component-internal gaps `6 / 8 / 10 /
  12 / 14px`. Panel padding `clamp(16px, 2.4vw, 24px)`.
- Elevation: `--shadow` `0 10px 30px /.38` (panel, toast), `--shadow-soft`
  `0 3px 10px /.28` (buttons, bubbles). Depth reads from the surface ladder
  first, shadow second — never a shadow or glow on every element.
- Type: system stack (`Segoe UI Variable Text`, `system-ui`); no web fonts.
  Base `1rem` / line-height `1.45`. Headings `1.35rem` / sub-headings `1.1rem`;
  eyebrow `0.72rem` uppercase `+0.14em` in `--accent`; labels/values
  `0.85–0.9rem`; hints `0.72–0.78rem`. Numbers (timer, readouts) use
  `font-variant-numeric: tabular-nums`. Weights: `400` body, `600` emphasis
  (buttons, labels, values), `700` reserved for Stop and the avatar. Do not
  scatter more weights.

## Component Specs

### Navigation rail (new)

- Width `clamp(212px, 18vw, 236px)`, full height, `--surface` with a `--line`
  right border. Not the page background — it is a distinct surface.
- **Product lockup** (top): 36px `--radius-sm` monogram on the quiet
  `--accent-tint` surface, `MagicHandy` in `--text` `600`, and
  `local / {dispatch owner}` in `--muted` `0.72rem`. It is static identity;
  Settings has one explicit destination in the page links. Use spacing, not a
  divider or underline, to separate identity from navigation.
- **Nav row**: 40px tall, `0 12px`, 18px icon + `0.9rem` label, `--radius-sm`.
  Default is a `--muted` label on a transparent background; hover uses
  `--surface-2`. Active uses the soft `--accent-tint` fill, `--text` label,
  and `--accent` icon on the rounded row, with no edge bar. It is not a
  saturated pill and carries `aria-current="page"`.
- **Pinned Stop** (bottom, `margin-top:auto`): the existing `.stop-button`
  treatment (full-width here) — `--danger`/`--danger-strong` red gradient,
  white `700`, `--radius-sm`, no shadow/glow, with the `Esc` keycap. Present on
  every page.

### Status readouts

The current React status bar uses compact readouts; the retired legacy
`.status-pill` with a fully-round fill, border, and glowing dot must not return:

- an 8–9px state **dot** + text label, inline; **no pill fill, no border, no dot
  glow**; group readouts with spacing or a 0.5px `--line` divider tick.
- dot color carries state: `--accent` active, `--ok` ok/running, `--warn`
  pending/paused, `--danger` error, `--muted` idle. Severity is always dot **and**
  text, never color alone.
- the run timer is `--muted` label + tabular-nums value with a clock icon.

The one authoritative visualizer keeps a compact vertical Handy 2-inspired body
and sleeve in the status bar and its detailed form in an attached motion-status
band at the bottom of Chat's control rail. One component, engine-driven, with
position labeled as a commanded estimate and the active motion name resolved
by the backend rather than inferred from client controls. Dynamic motion shows
its anchor route (or center), span, speed, variation, decision horizon, and
source without presenting it as a saved pattern. It renders the
backend's clock-sampled current path point, projected through the active stroke
window and direction; the buffered queue tail is diagnostics, not position.

### Buttons

- Primary (app action): `--accent`→`--accent-strong` gradient, `--accent-ink`
  text, `--radius-sm`, padding `10px 18px`, `--shadow-soft`. One primary per view.
- Start (motion go): `--ok`→`--ok-strong` gradient, `--ok-ink`, slightly larger
  padding. Green means go.
- Secondary: `--surface-2`, `--text`, `--line-strong` border; hover borders and
  tints `--accent`.
- Danger-outline: `--surface-2` with a `--danger` outline for destructive but
  reversible actions (delete set, clear memory) — the solid red is reserved for
  Stop.
- Disabled: `opacity .45`, `--surface-2`, `--muted`, no shadow — visibly inert,
  with a reason nearby (never a silent no-op).

### Top-bar notification center

- Shell-owned and route-independent. A compact bell sits immediately before the
  connection disclosure. It may show a numeric unread badge or a small activity
  marker, but no decorative glow, pulse, or semantic red badge.
- The panel is one popover surface, no wider than 360px, with three scan-friendly
  regions: backend-derived **Activity**, current **Attention**, and bounded
  current-session **Recent** history. Activity rows show honest progress and
  link to their owning Settings section.
- Toasts remain transient feedback; whether their category also enters bell
  history is a saved General setting. History categories are routine app
  feedback, core/device status, library/media tasks, voice worker alerts, and
  software updates. The quiet default records system, voice, and update events,
  not routine confirmations or successful background scans. Mark-read and clear
  are a compact management group separated from Close; clear uses the standard
  trash icon. All icon commands retain labels/tooltips. Notification history is
  session UI feedback, not durable application state. Backend scan/job/health
  snapshots remain authoritative.
- Category controls never hide live Activity, current Attention, backend-offline
  locking, or Emergency Stop. Consumed backend event keys survive reload in
  session storage even when their category is hidden, so a stale completion
  cannot reappear after a refresh.
- Only one shell popover is open at a time. Opening notifications closes the
  connection manager and vice versa; selecting a linked item closes the panel.
  The trigger and close button preserve keyboard focus.

### Top-bar connection manager

- Shell-owned and route-independent. The compact trigger sits at the far right
  of the top bar and opens the panel immediately below it. Desktop shows
  provider/device text and a state icon; narrow screens keep the labeled button
  accessible while rendering only its state icon.
- The expanded non-modal panel is at most 360px wide, one overlay surface with
  dividers rather than nested cards. It contains the current provider's live
  actions, a Settings link, and compact immediate controls for the documented
  Handy model calibration, speed, stroke, and reverse direction. The model is
  a three-part merged native radio group (`Original`, `2 Standard`, `2 Pro`),
  with the selected travel and normal maximum shown directly beneath it;
  direction is right-aligned below the two range rows.
  Cloud REST includes one compact write-only connection-key row and identifies
  whether the bundled or developer API v3 application ID is active.
- Cloud REST keeps three actions distinct: the quiet refresh icon performs a
  non-motion Check, the full-width Connect reacquires control only after a
  successful HSP probe, and Disconnect closes backend motion admission before
  stopping active media, chat, modes, and device motion. Check never silently
  reacquires a released device. A failed physical Stop is reported, but the
  local release gate remains closed so another app can take control.
- Artwork is a reference-guided transparent isolation of the reviewed conductor
  hand, rendered directly without a runtime mask or clip. Scale the hand, three
  signal arcs, and the poster's tall capsule, domed body, LED, and square marker
  into one frame; no element may touch the frame edge. The arcs use intense blue
  `#168bff` only for connecting/connected states and cascade toward the device.
  Disconnected hides every blue arc and shows the device's red square marker.
  Only a failed connection attempt adds the compact shaking `--danger` X; it is
  a semantic status mark, not a red action competing with Stop.
  `prefers-reduced-motion` disables the connecting animation.
- Keep [connection-artwork.md](connection-artwork.md) aligned with any asset,
  SVG-coordinate, state, or panel-sizing change.
- Open moves focus to Close; Close restores the trigger. Escape remains the
  global Stop shortcut and is never consumed by this disclosure.

### Cards, groups, fields

- Panel/card: `--surface`, `--line`, `--radius`, `--shadow`, padding
  `clamp(16px, 2.4vw, 24px)`.
- Nested group (quick settings, llm settings): `--surface-2`, `--line`,
  `--radius-sm`. Do not nest rounded cards more than one level — group with
  spacing and a `--line` divider, not another shadowed box.
- Inputs / select / textarea: `--bg-inset`, `--line`, `--radius-sm`, padding
  `9px 11px`; focus swaps the border to `--accent` (plus the global focus ring).
- Immutable requirements and installation state use semantic notes/status
  readouts, never read-only inputs that look disabled. A caution prerequisite
  may use one `--warn` left rule; ordinary module state stays graphite with its
  compact status dot.
- Range: native with `accent-color: --accent`. A dual-thumb range keeps two
  native keyboard/AT sliders, exposes each bound's effective ARIA constraint,
  and uses one track-sized pointer target so close thumbs remain reachable.
  An ordered setting with three to five named values uses one native discrete
  range with every set-point label visible and the active label exposed through
  `aria-valuetext`; do not hide the scale in a select. Categorical values use a
  compact segmented radio group instead, because a slider would imply a false
  numeric order. Its selected state uses the quiet accent-tint fill and text
  contrast only—do not add an underline that makes it read like a tab. In the
  compact Chat sidebar, discrete ranges use a 14px handle
  and 4px track, following the restrained proportions in the
  [Carbon slider specification](https://carbondesignsystem.com/components/slider/style/).
  Their visual stop axis is inset by half the handle width—the same centerline
  the native handle actually traverses—and stop `i` is placed at
  `i / (count - 1)`. Never center labels in `count` equal grid cells: that puts
  the first and last marks inside the real endpoints. Preserve the native
  arrow/Home/End interaction and human-readable `aria-valuetext` required by
  the [WAI-ARIA slider pattern](https://www.w3.org/WAI/ARIA/apg/patterns/slider/).
  Toggle: 40×22 track, 18px thumb; off `--line` + `--muted` thumb, on
  `--accent-strong` track + white thumb.
- Import timeline: default to fit-all and put direct trim handles on the kept
  range, following the interaction model used by
  [QuickTime](https://support.apple.com/guide/quicktime-player/trim-a-movie-or-clip-qtpf2115f6fd/mac)
  and [Clipchamp](https://support.microsoft.com/en-us/clipchamp/how-to-trim-videos-images-or-audio-assets).
  Use a compact icon toolbar for Earlier, Later, Zoom in, Zoom out, Fit
  selection, and Fit all; each icon has a tooltip and accessible name. Preserve
  `+`, `-`, `0`, and arrow-key equivalents on the focused timeline. Vertical
  wheel input over the plot zooms around the cursor; horizontal or Shift-wheel
  input pans. Release outward vertical wheel input to the page at the fit-all and
  minimum-span limits. Provide a proportional horizontal scrollbar with a
  minimum 44px thumb, direct drag/track-jump behavior, and standard scrollbar
  keyboard semantics. Keep each trim handle as a fixed-size, labeled
  keyboard-operable slider whose dependent ARIA limits update with the other
  bound, as required by the [WAI-ARIA multi-thumb slider pattern](https://www.w3.org/WAI/ARIA/apg/patterns/slider-multithumb/).
  Waveform, selection shading, pointer mapping, and handles use one measured
  coordinate system; zoom/pan changes only the source viewport, never trim
  bounds or submitted content. Kept and excluded regions need distinct fills in
  addition to the handles and exact text values. Trim bounds snap to source
  actions, and start, end, total, visible range, zoom level, selected action
  count, and selection length remain available as text with tabular numerals.
  The SVG is a raw source-action view, not a playback preview; do not rely on
  animation, shading, or color alone. Do not cap loop selection at the 6.6-second
  routine floor: longer coherent loops are valid. Disable import with the exact
  essential-knot limit when the selected shape cannot fit the stored loop
  representation. Compact pattern previews combine backend samples with saved
  knots so long-cycle reversals remain visible without client interpolation.
- Badge (e.g. the "testing" tag): 1px `--line-strong`, `--surface-2`, `--muted`,
  `0.68rem`, `999px` is *not* used — small `--radius-sm`/pill-ish hairline chip,
  quiet.

### Chat

- The Chat route fills the remaining workspace beside the persistent nav rail.
  Its conversation and compact control sidebar share the available height; do
  not reapply the ordinary route max-width to this workspace. The sidebar
  reserves equal stable vertical-scrollbar gutters on both edges so opening
  Advanced cannot shift or compress every control, and the stack remains
  visually centered in the rail.
- The Chat Controls sidebar begins with a compact segmented `LLM motion` radio
  choice for Creative / Pattern Library / Off. `Creative` is the user-facing
  name for the persisted `dynamic` mode. Motion change rate uses eight numbered
  stops without a redundant direction hint; Spoken check-ins and motion Style
  use labeled discrete sliders. The backend maps the numbered
  scale to its bounded timing windows and migrates legacy cadence presets;
  speech-
  motion authority remains categorical and uses a segmented choice.
  The LLM-motion row remains one fixed 32px line when Advanced makes the
  sidebar scrollbar appear; localized visual text may ellipsize, while the
  native radio keeps its complete accessible label. These controls are
  conversational behavior, not global
  navigation: do not put it in the permanent rail. The backend owns and
  persists the value; read-only or offline clients see it disabled. Settings >
  Model retains the secondary configuration surface.
- Expanded Autopilot Advanced controls use explicit vertical rhythm: a small
  gap after the speech-motion selector and between its toggle rows, followed by
  one compact muted session-tracking note aligned with the toggle labels. Avoid
  default paragraph margins in this rail. Keep a visible minimum gap below
  Motion style before the aligned bottom status section even when expanded
  content makes the sidebar scroll. The status section extends its darker
  inset background through the sidebar's full interior width while its own
  stable gutters keep the contents aligned with the centered, independently
  scrolling control column; do not leave it as a partial-width nested slab.
- Session tabs are a 43px restrained strip with the compact `Chat` route title
  attached at its leading edge, ordered by creation time so active changes do
  not move targets. The active tab uses one symmetric top-corner surface and a
  single bottom accent; do not combine a one-sided hard edge, a three-sided
  outline, and an underline. Focus belongs to the complete tab shape, not only
  its label segment when a menu action shares the tab. Use an amber dot plus
  accessible text for unsaved state, and arrow/Home/End keyboard focus. Tabs
  start at a 236px preferred width, share the strip evenly as sessions are
  added, and stop shrinking at 112px (104px on narrow screens). Only after that
  floor may the list overflow horizontally. Keep the native scrollbar hidden;
  touch, trackpad, mouse-wheel, keyboard focus, and active-session changes must
  all reveal the intended tab.
  New Chat is a borderless-at-rest icon command in a protected trailing slot
  immediately after the rightmost tab; hover and focus may add a neutral surface.
  When tabs overflow, only the tab list scrolls and New Chat remains reachable.
  Do not render an overflow trigger when that tab has no available action.
  Save/Delete live in the visible overflow menu, with right-click as a shortcut
  rather than the only path.
- New Chat always confirms the active session. Switching away from the one
  unsaved working tab requires Save or discard; Autopilot stops before the
  backend changes session. The dialog traps focus, closes with Escape when no
  mutation is pending, and never covers the shell-level Emergency Stop.
- Message row: 30px avatar gutter + body; assistant left, user right (mirror the
  grid). Avatar 30px, assistant uses solid deep steel with an accent border,
  user uses `--user-bubble`. Assistant avatars with run provenance are focusable
  and expose the same diagnostic tooltip on hover and keyboard focus.
- Bubble: `--radius-sm` with the opposite bottom corner squared to `4px` (the
  tail); assistant `--surface-2`/`--line`, user `--user-bubble`/`--user-bubble-line`;
  `--shadow-soft`. Speaker label + timestamp above in `--muted` `0.72rem`.
- Streaming cursor: a blinking `--accent` caret; warning state tints the border
  `--warn`. All animation respects `prefers-reduced-motion`.

### Feedback layer

- Toast: fixed bottom-center, `--surface-3`, `--line-strong`, `--shadow`,
  `--radius-sm`; slides up on show. One at a time. Each toast also enters the
  top-bar notification center's bounded current-session history.
- Notification center: anchored below the top bar, one overlay surface with
  dividers rather than nested cards; constrained height and vertical scrolling
  keep the shell usable on short viewports.
- Backend banner: full-width alert, translucent `--danger` fill + border, white
  text; lives at the top of the workspace, immediately below the status bar and
  above routed content.

### Icons

One monochrome inline-SVG set drawn with `currentColor`, 18px in nav / status.
No emoji as UI icons — replace the `⏱` timer glyph and the gradient brand blob
with set members. Decorative marks are `aria-hidden`; an icon is never the only
label for a control.

## Layout And Breakpoints

```text
┌──────────────┬───────────────────────────────────────────────┐
│ profile ▸    │  status bar (48px) — dot+text readouts · timer │
│              │                                · mini visualizer│
│ ▸ Chat       ├───────────────────────────────────────────────┤
│   Preset     │                                                │
│    Modes     │        routed workspace (one mounted)          │
│   Pattern    │        Chat · Modes · Library                  │
│    Library   │        Videos · Settings                       │
│   Videos     │                                                │
│   Settings   │                                                │
│ ──────────── │                                                │
│ [ STOP  Esc ]│                                                │
└──────────────┴───────────────────────────────────────────────┘
   ~224px           workspace column
```

- Desktop: rail + status bar + workspace; the workspace column holds the routed
  page (two-column on Chat: control column + conversation).
- `≤1100px`: the controller status label hides while its state remains visible.
- `≤980px`: the status timer hides to preserve the compact status row.
- `≤860px`: the rail becomes a reserved bottom footer with tabs and a full-width
  Stop row above them. The footer participates in layout rather than overlaying
  the workspace, so content and the keyboard do not hide Stop.
- `≤520px` (existing): field rows collapse to one column; bubbles go full width.

## Motion And Accessibility

- Transitions are quiet (`0.15s ease` on interactive state; `0.13s linear` on
  visualizer position between 125ms backend samples). No bouncing, pulsing
  glows, or moving gradients.
- `prefers-reduced-motion: reduce` disables the visualizer/toast/toggle
  transitions and the chat entry animation (wired in
  `web/src/styles/components.css`).
- Focus is a `2px --focus` outline at `2px` offset on every interactive element;
  routed views focus their heading on entry. The connection disclosure is
  non-modal: opening moves focus to Close and closing restores the trigger; it
  does not trap focus.
- Status is text + icon + color, never color alone. One polite live region for
  status; the visualizer is not a chatty live region.
- Text and controls meet WCAG AA contrast on the dark surfaces.

## Do / Don't

Do:

- keep status as compact dot+text; cap radius at `--radius-sm` for anything
  control- or status-sized; use `999px` only for circular micro-elements, never
  a text or control container.
- signal depth with the surface ladder; use the two defined shadows sparingly.
- keep one interactive hue (`--accent`); green strictly for go, amber for warn,
  red for stop; left-aligned, information-dense, real hierarchy.

Don't:

- wrap status or controls in oversized fully-round filled bubbles; add glows to
  dots, chips, or the brand mark; nest rounded shadowed cards.
- introduce purple or blue-green decorative tones; paint a nav active state as a
  saturated fill; use emoji as icons; center a consumer-style hero with giant
  padding. This is an instrument.

## Relationship To Other Docs

- [ui-navigation-redesign.md](ui-navigation-redesign.md): the shell IA these
  tokens dress.
- [ui-design.md](ui-design.md): the safety/accessibility/parity rules these
  components must not regress.
- ADR 0004 (frontend strategy) and ADR 0009 (React frontend migration): the UI
  is now React, but still statically built, embedded, and offline at runtime;
  these guidelines add no external asset fetch.
