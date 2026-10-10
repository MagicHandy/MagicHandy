# UI Design Guidelines

## Purpose

The concrete, implementable reference for the sidebar-navigation shell. It is
the companion to two docs and does not repeat them:

- [ui-navigation-redesign.md](ui-navigation-redesign.md) — the information
  architecture (nav rail, workspaces, Chat Autopilot, migration).
- [ui-design.md](ui-design.md) — the enduring safety, accessibility, and parity
  rules.

This file is the **live token-and-component reference** for the shipped React
shell (status 2026-10-09: current). The visual system is **Graphite**: one
quiet dark graphite surface ladder, one steel-azure interactive hue, flat
controls, and one card level. The Chat control sidebar uses **control cards**
(small caps title, label-left/control-right rows) adopted from the Daylight
study. Tokens live in `web/src/styles/tokens.css`; shell and component rules in
`shell.css` / `components.css`, with feature files beside them. The pre-React
`web/app.css` is retired under `web/legacy/` (reference only, not embedded).

Older design reviews and sketches are history, not specification. Where one
disagrees with this file, this file and the build win. In particular, the
2026-10-09 settings review's header bands, square checkboxes and rectangular
tab strip were superseded the same day; its finding that underline tab
indicators are rejected still applies (see Tabs).

Use existing tokens instead of raw hex values in implementation, and do not
invent parallel values. A component-local value either stays local to that
component or is promoted to a token before it is reused elsewhere.

## Design Tokens

### Color roles

Steel Azure is the default palette. The values below live in
`tokens.css :root` and remain the no-settings fallback.

| Token | Value | Role |
| --- | --- | --- |
| `--bg` | `#0e0f11` | Page canvas, status bar |
| `--rail` | derived (`--bg` 62% + `--surface`) | Navigation rail and phone footer |
| `--bg-inset` | `#0b0c0e` | Recessed wells: inputs, chat log, charts, segmented tracks, code |
| `--surface` | `#16181b` | Cards and panels |
| `--surface-2` | `#1d2024` | Selected tab/segment, hover, raised rows |
| `--surface-3` | `#24282e` | Floating layers: toast, menus, popovers |
| `--line` | `#262a30` | Hairline border/divider |
| `--line-strong` | `#353b43` | Button and card-header borders, switch track off |
| `--control-line` | derived (`--muted` 62% + `--surface`) | Field boundaries (inputs, selects, textareas): at least 3:1 against the card and the inset fill |
| `--text` | `#e9e7e2` | Primary text |
| `--text-soft` | derived | Long-form reading text (assistant replies) |
| `--muted` | `#9aa1a9` | Secondary text, hints, small caps titles |
| `--faint` | derived | Decoration only (idle dots, minor ticks); never text a person must read |
| `--accent` | `#5b9dd9` | The one interactive hue: primary action, switch on, slider fill, focus, active nav icon |
| `--accent-strong` | `#497fb5` | Accent hover/pressed partner |
| `--accent-ink` | `#0b131c` | Text on an accent fill |
| `--accent-tint` | accent 15% | Quiet selected fill: active nav row, selected choice row |
| `--steel-deep` | `#3a5a7c` | Avatar fill |
| `--go` / `--go-ink` | `#287d42` / `#f2fbf4` | **Start motion only** (filled Start, Audition and Play buttons); 4.85:1 |
| `--ok` / `--ok-strong` / `--ok-ink` | `#4fc06d` / `#3da55c` / `#08130c` | Running/ok status marks |
| `--warn` | `#d8b66a` | Caution / paused / pending / unsaved |
| `--danger` / `--danger-strong` | `#d6453f` / `#b93a35` | Failure marks, destructive outline |
| `--stop-fill` / `--stop-fill-hover` | `#c43c36` / `#b03731` | Emergency Stop fill; white label at 5.19:1 |
| `--focus` | `#82b4e2` | Focus ring |
| `--user-bubble` / `--user-bubble-line` | `#1b2634` / `#2c3e52` | Your chat bubble (steel tint, distinct from the neutral reply bubble) |

Rule: a new surface picks an existing role. `--accent` = "you can act on this",
`--go`/`--ok` = "start / it is running", `--warn` = "caution", `--danger` =
"stop or failed". Green never decorates and red is never an ordinary action.

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

Palette selectors live in `web/src/styles/themes.css` and override the neutral
and interactive tokens only; `--rail`, `--text-soft` and `--faint` derive from
them so every palette stays coherent. Palettes must not ship component layouts,
type changes, decorative glows, or a second source of UI state. These
invariants apply to every palette:

- `--ok`, `--go`, `--warn`, `--danger`, and their strong/ink partners inherit
  the canonical safety colors from `tokens.css`; color is reinforced by text and
  control shape.
- Emergency Stop is a flat `--stop-fill` with white text and **no gradient,
  box shadow or glow**. Enabled Stop and Start labels keep at least 4.5:1.
- Small text and accent-ink pairs meet WCAG AA (4.5:1); focus and graphical
  accent pairs meet 3:1.
- Adding a palette requires the core catalog ID, one `themes.css` token
  selector, frontend label/swatches, contrast evidence, and catalog tests.
  Deleting any one of those pieces must fail review rather than leave a
  selectable partial theme.

### Shape

| Token / value | Use |
| --- | --- |
| `--radius` `10px` | Cards, panels, overlays, dialogs, chat conversation |
| `--radius-sm` `6px` | Buttons, inputs, selects, tabs, nav rows, Stop |
| `--radius-xs` `4px` | Badges, chips, segment thumbs, keycaps, bubble tail corner |
| `999px` / `50%` | Only circular micro-elements: dots, switch track/thumb, avatars, scrollbar thumbs |

Nothing button-sized or status-sized is a pill.

### Spacing, elevation, type

- Spacing: `--gap` `16px` between regions and between cards; component gaps
  `4 / 6 / 8 / 10 / 12 / 14px`. Workspace padding `clamp(16px, 2.4vw, 28px)`.
  Routed pages share one content width, `--content-max` `1280px`, so titles
  align across routes; Chat and Labs fill the workspace.
- Elevation: depth comes from the surface ladder. Only floating layers (menus,
  popovers, dialogs, toast) use `--shadow`; `--shadow-soft` is `none`. No glow
  anywhere.
- Type: sans-serif only. `--font-ui` Segoe UI Variable Text with system
  fallbacks; `--font-display` Segoe UI Variable Display for page titles; no web
  fonts. Numbers that change (timer, readouts, slider values, versions, paths)
  use `--font-mono` (Cascadia Mono) with tabular numerals. The scale is seven
  steps: `--fs-micro` 11, `--fs-hint` 12, `--fs-label` 13, `--fs-body` 14,
  `--fs-heading` 15, `--fs-title` 22, `--fs-display` 28 (px). Page titles use
  `--fs-title`; card headers `--fs-heading` 600; field labels `--fs-body` 500;
  hints `--fs-hint` muted. Weights: `400` body, `500` labels, `600` headings,
  selected tabs and buttons, `700` reserved for Stop. A 16px phone composer is
  the one exception (it prevents mobile focus zoom).
- Small caps titles (`--fs-micro`, 600, `+0.06em`, uppercase, `--muted`) are
  used only for control-card titles and grouping captions above several
  sections (for example `MODEL` in Settings > Chat > Model).

## Component Specs

### Navigation rail

- Width `--rail-width` `212px`, full height, `--rail` with a `--line` right
  border.
- **Product lockup** (top): 26px `--radius-sm` monogram, `MagicHandy` in
  `--text` `600`, and `local / {dispatch owner}` in `--muted` `--fs-hint`. It
  is static identity; use spacing, not a divider, to separate it from
  navigation.
- **Nav row**: 36px tall, `0 10px`, 18px icon + `--fs-body` label,
  `--radius-sm`. Default is a `--muted` label; hover uses `--surface-2`. Active
  uses the quiet `--accent-tint` fill, `--text` `600` label and `--accent` icon,
  with no edge bar, and carries `aria-current="page"`.
- **Pinned Stop** (bottom, `margin-top:auto`): the `.stop-button` treatment —
  flat `--danger`, white `700`, `--radius-sm`, 48px tall, with the `Esc` keycap
  (hidden on coarse pointers). Present on every page.

### Status readouts

The status bar (`--status-h` `52px`, `--bg`) uses compact readouts; the retired
legacy `.status-pill` with a fully-round fill, border, and glowing dot must not
return:

- an 8px state **dot** + text label, inline; **no pill fill, no border, no dot
  glow**; group readouts with spacing or a hairline divider tick.
- dot color carries state: `--accent` active, `--ok` ok/running, `--warn`
  pending/paused, `--danger` error, `--muted` idle. Severity is always dot
  **and** text, never color alone; the controller label stays visible at
  desktop widths. At `≤860px` controller ownership keeps its slot in compact
  form: the Take control glyph and **You**, borderless, with the full
  `controller: you` label kept for assistive technology. A read-only tab shows
  the bordered Take control button in the same slot, so the slot always says
  who controls.
- the run timer is a clock icon + `--font-mono` tabular value.
- On the right, one row of 36px chips with the same border, radius and hover:
  Take control (when read-only), the compact motion glyph, the bell, the
  control profile and the connection chip.

The one authoritative visualizer keeps a compact vertical Handy 2-inspired body
and sleeve in the status bar and its detailed form in the **Motion status**
card at the top of Chat's control sidebar. One component, engine-driven, with
position labeled as a commanded estimate and the active motion name resolved by
the backend rather than inferred from client controls. Dynamic motion shows its
anchor route (or center), span, effective pace, variation, decision horizon,
and source without presenting it as a saved pattern. A saturated pace shows the
effective/requested pair and exposes the backend limiter reason instead of
repeating the slider target as achieved output. It renders the backend's
clock-sampled current path point, projected through the active stroke window
and direction; the buffered queue tail is diagnostics, not position.

### Buttons

All buttons are flat fills, `--control-h` `34px` (`--control-h-sm` `28px` for
`.btn-sm`), `0 14px`, `--radius-sm`, `--fs-label` `600`. No gradients, shadows
or glows.

- Primary (app action): `--accent` fill, `--accent-ink` text. One per view.
- Start (motion go): `--go` fill, `--go-ink` text, darker on hover. Green means
  go: every action that starts motion (Start, Audition, Play) uses it; actions
  that only plot or preview do not.
- Secondary: transparent with a `--line-strong` border, `--text`; hover fills
  `--surface-2`.
- Quiet (`.btn-quiet`, Labs `.lab-text-button`, icon buttons): no border,
  `--text-soft`; hover fills `--surface-2`. Never a second link colour. Icon
  buttons are `--control-h` square so they line up with buttons and fields.
- Danger-outline: transparent with a `--danger` outline and light red text for
  destructive but reversible actions (delete set, reset settings). The solid red
  is reserved for Stop.
- Disabled: `opacity .45`, visibly inert, with a reason nearby (never a silent
  no-op). Segmented buttons share the same disabled treatment.
- Confirmations render in the app (`ConfirmHost`, `confirmThen`), never through
  `window.confirm`, which blocks the page and with it Emergency Stop and the
  Escape shortcut. Cancel has initial focus; destructive confirmations use the
  danger-outline recipe; Escape cancels while still reaching the global Stop.

### Top-bar control profile

- Authenticated sessions show a compact profile image or monogram immediately
  before notifications. The trigger is labeled **Control profile** and shows
  **Self** for a normal new session.
- The attached non-modal panel lists only backend-returned Self and active
  linked accounts. It explains that selection labels this login session and
  does not sign in as another account or transfer device control. The UI never
  infers links or keeps a parallel allow-list.
- Account imagery is content within the neutral graphite surface. It does not
  introduce a decorative accent, glow, animated presence ring, or oversized
  circular pill. A generated monogram on `--steel-deep` is the missing-image
  fallback.
- This disclosure participates in the one-open-shell-menu rule with
  notifications and connection. Close focus returns to its trigger; Escape
  remains reserved for Emergency Stop.

### Top-bar notification center

- Shell-owned and route-independent. A compact bell sits immediately before the
  connection disclosure. It may show a numeric unread badge or a small activity
  marker, but no decorative glow, pulse, or semantic red badge.
- The panel is one `--surface-3`-family overlay (`--radius`, `--shadow`), no
  wider than 360px, with three scan-friendly regions: backend-derived
  **Activity**, current **Attention**, and bounded current-session **Recent**
  history. Activity rows show honest progress and link to their owning Settings
  section.
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
- Only one shell popover is open at a time. Opening control profile,
  notifications, or connection closes either of the others; selecting a linked
  item closes the panel. The trigger and close button preserve keyboard focus.

### Top-bar connection manager

- Shell-owned and route-independent. The compact 216 x 36 trigger sits at the
  far right of the top bar and opens the panel immediately below it. Desktop
  shows provider/device text and a state icon; narrow screens keep the labeled
  button accessible while rendering only its state icon.
- The expanded non-modal panel is at most 360px wide, one overlay surface with
  dividers rather than nested cards. It contains the current provider's live
  actions, a Settings link, and compact immediate controls for the documented
  Handy model calibration, speed, stroke, and reverse direction. The model is
  a three-part merged native radio group (`Original`, `2 Standard`, `2 Pro`),
  with the selected travel and normal maximum shown directly beneath it;
  direction is right-aligned below the two range rows. Cloud REST includes one
  compact write-only connection-key row and identifies whether the bundled or
  developer API v3 application ID is active.
- Cloud REST keeps three actions distinct: the quiet refresh icon performs a
  non-motion Check, the full-width Connect reacquires control only after a
  successful HSP probe, and Disconnect closes backend motion admission before
  stopping active media, chat, modes, and device motion. Check never silently
  reacquires a released device. A failed physical Stop is reported, but the
  local release gate remains closed so another app can take control.
- Artwork is a flat 96px vector status strip on `--bg-inset`: the Handy-inspired
  capsule and domed body with LED and square marker, and three signal arcs. Arcs
  are `--accent` while connecting (a staggered opacity wave) and `--ok` when
  connected; disconnected hides them and shows the red square marker. Only a
  failed connection attempt adds the compact shaking `--danger` X; it is a
  semantic status mark, not a red action competing with Stop. There is no
  bitmap. `prefers-reduced-motion` disables the wave and shake. Keep
  [connection-artwork.md](connection-artwork.md) aligned with any SVG
  coordinate, state, or panel-sizing change.
- Open moves focus to Close; Close restores the trigger. Escape remains the
  global Stop shortcut and is never consumed by this disclosure.

### Cards, sections, fields

- **One card level.** A card is `--surface`, 1px `--line`, `--radius`, no
  shadow. Cards never nest: a group inside a card is a section separated by a
  `--line` rule with a `--fs-body` `600` title.
- **Settings and setup.** The page is a 196px vertical section list (selected
  row filled `--surface-2`, `600`) beside the content column; below 1000px the
  list becomes a scrolling strip of the same filled tabs. Each settings group is
  a card whose title is a header row (`12px 16px`, `--fs-heading` `600`,
  `--line` bottom rule). Model routing, model roles and media tools follow the
  same single level. Save settings sticks to the foot of the column on `--bg`
  with a top rule, flush with the workspace edge on every breakpoint.
- **Field rows.** A field with one short control (select, text, number, range)
  is a row when the field itself is at least 560px wide: label and inline hint
  on the left, the control in a 300px column on the right, help text under the
  label. Narrower fields (two-up grids, phones) stack. Paths, keys, textareas
  and other wide content always stack. Each field is its own size container, so
  this does not depend on the window.
- **Switch rows.** Every on/off setting is a switch row: label (and optional
  hint) left, switch right, same `--fs-body` label style everywhere. Checkboxes
  remain only where several independent items are picked from a list.
- **Inputs / select / textarea:** `--control-h`, `--bg-inset`, 1px
  `--control-line`, `--radius-sm`, `6px 10px`; focus swaps the border to
  `--accent` plus the global focus ring. Inputs without a `type` attribute are
  text inputs and get the same recipe. Placeholders use `--muted`.
- **Touch:** on coarse pointers `--control-h` becomes 44px and
  `--control-h-sm` 36px everywhere, sliders get a 32px hit area, and switch rows
  are at least 44px tall; the Esc keycap is hidden.
- **Hints:** `.hint` is muted `--fs-hint` everywhere; the Settings section list
  names the open section, so the section's own heading is kept for assistive
  technology only.
- **Disclosures:** one chevron before the label (`--fs-label`, 600) for every
  inline `<details>`; card-header disclosures put the chevron at the end.
- **Choice lists** (setup method, network scope, theme): one bordered list; the
  selected row uses `--accent-tint`; hover uses `--surface-2`.
- **Notices** are the one inset surface inside a card: `--bg-inset`, 1px
  `--line`, `--radius-sm`. Information stays distinct from selection.
- Immutable requirements and installation state use semantic notes/status
  readouts, never read-only inputs that look disabled. A caution prerequisite
  may use one `--warn` left rule; ordinary module state stays graphite with its
  compact status dot. Technical details may be disclosed, but errors,
  partial/unmet requirements, data destinations and save timing stay available
  at the decision point. Diagnostics prompt inspection is collapsed and loads
  only when opened, canceling on close.
- **Switch:** 36 x 20 track, 16px thumb; off `--line-strong` + `--muted` thumb,
  on `--accent` + white thumb. Labs uses the same proportions.
- **Sliders:** one flat recipe for every slider — native ranges (`RangeInput`),
  the named set-point scale and the dual-thumb range: a 4px `--line-strong`
  track filled with `--accent` up to the value and a 16px `--text` thumb with no
  border, shadow or glow, inside a 20px hit area (32px on coarse pointers);
  focus rings the thumb. Inputs stay native. Chromium cannot paint a native
  range's fill, so components publish the value's position as `--range-fill`
  (0–1) and the fill ends under the thumb's centre, which travels half a thumb
  in from either end. The recipe lives in `setpoint-controls.css` with no
  specificity (`:where`), so a component class can still size a slider; the
  video volume slider is the same recipe stood upright, filling from the
  bottom. Never fall back to `accent-color` paint, which differs per browser.
  A dual-thumb range keeps two
  native keyboard/AT sliders, exposes each bound's effective ARIA constraint,
  and uses one track-sized pointer target so close thumbs remain reachable.
  An ordered setting with three to eight named or numbered values uses one
  native discrete range with its endpoint labels visible, the current value in
  the head, and the active label exposed through `aria-valuetext`. Its visual
  stop axis is inset by half the handle width—the same centerline the native
  handle traverses—and stop `i` is placed at `i / (count - 1)`. Preserve the
  native arrow/Home/End interaction required by the
  [WAI-ARIA slider pattern](https://www.w3.org/WAI/ARIA/apg/patterns/slider/).
- **Categorical choices** use a segmented radio group: an inset `--bg-inset`
  track; the selected segment is raised (`--surface-2`, inset `--line-strong`
  ring, `600`). A slider would imply a false numeric order. When a categorical
  choice has more options than fit one row of the space it lives in, use a
  native select instead of wrapping segments into a grid.
- **Badges and chips:** `--radius-xs`, 1px border, `--surface-2`, quiet; never
  a pill.
- **Import timeline:** default to fit-all and put direct trim handles on the
  kept range, following the interaction model used by
  [QuickTime](https://support.apple.com/guide/quicktime-player/trim-a-movie-or-clip-qtpf2115f6fd/mac)
  and [Clipchamp](https://support.microsoft.com/en-us/clipchamp/how-to-trim-videos-images-or-audio-assets).
  Use a compact icon toolbar for Earlier, Later, Zoom in, Zoom out, Fit
  selection, and Fit all; each icon has a tooltip and accessible name. Preserve
  `+`, `-`, `0`, and arrow-key equivalents on the focused timeline. Vertical
  wheel input over the plot zooms around the cursor; horizontal or Shift-wheel
  input pans. Release outward vertical wheel input to the page at the fit-all
  and minimum-span limits. Provide a proportional horizontal scrollbar with a
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
  animation, shading, or color alone. Do not cap loop selection at the
  6.6-second routine floor: longer coherent loops are valid. Disable import with
  the exact essential-knot limit when the selected shape cannot fit the stored
  loop representation. Compact pattern previews combine backend samples with
  saved knots so long-cycle reversals remain visible without client
  interpolation.
- **Expanded plots** (the Author canvas, the Training curve, and the Labs
  position and velocity charts in Motion now, Motion preview and guided
  tests) draw edge to edge and carry their scales as `--fs-hint` HTML text
  beside and under the drawing (`PatternPlotAxes`). Never put scale text
  inside a scaled SVG or enlarge it to compensate: it shrinks with the panel
  and clips at the edges. Position plots read **Base 0 / 50 / Tip 100** on
  the bottom edge, middle and top edge; the velocity estimate reads its
  signed peak on the top and bottom edges and 0 in the middle. The time scale
  gives the start, middle and end of the domain the plot actually draws, in
  seconds. Stretched drawings keep strokes at their CSS width
  (`vector-effect: non-scaling-stroke`). Author names its accent line
  **Backend preview** (with a line swatch) only once the backend has sampled
  one; until then the muted line is the unsampled draft.

### Tabs

Navigation tabs (Settings sub-sections, Labs, Pattern library, the narrow
Settings strip) are compact text tabs: 32px, `0 12px`, `--radius-sm`. The
selected tab is **filled** (`--surface-2`, `--text`, `600`) and carries
`aria-current`; hover uses a lighter fill. **Do not use underline indicators,
moving lines, or left-edge stripes** on navigation tabs. A strip may sit above a
single `--line` rule. Overflowing strips scroll horizontally with hidden native
scrollbars and keep the selected tab and keyboard focus visible (focus outlines
are inset so the scroller cannot clip them).

Chat session tabs are document tabs, not navigation, and keep their original
treatment at every width: the active session is filled `--surface-2` with one
bottom `--accent` edge.

### Chat

- The Chat route fills the workspace beside the persistent nav rail. Its
  conversation card and the control sidebar (`clamp(280px, 24vw, 340px)`) share
  the available height; do not reapply the ordinary route max-width here. The
  sidebar scrolls independently with a stable gutter.
- **Control sidebar.** One `aside` landmark labeled **Motion controls**
  containing three control cards, top to bottom:
  1. **Motion status** — the detailed visualizer.
  2. **Autopilot** — the Autopilot state row and Start/Stop action, then rows
     for Motion change rate (eight numbered stops), **Spoken check-ins** (a
     select; Custom reveals the seconds range directly beneath it), Session
     buildup, and the Advanced disclosure.
  3. **Motion** — **LLM motion** as a select (Creative v2, Creative, Layered,
     Pattern library, Off) with the selected mode's one-sentence description
     under its label as the accessible description; **Style** (Gentle /
     Balanced / Intense) as a segmented choice; then the voice quick controls.
  Cards are headed groups (`h2.control-card-title`), not extra landmarks. Rows
  are label-left/control-right (`.control-row`); sliders and segmented choices
  stack under their label (`.control-row-stack`). Rows are separated by `--line`
  rules. `Creative` is the user-facing name for the persisted `dynamic` mode.
  The backend owns and persists these values; read-only or offline clients see
  them disabled. Settings > Chat > Model retains the secondary configuration
  surface. These are conversational controls, not global navigation: do not
  put them in the permanent rail.
- Session tabs are a 46px strip with the persona chip at its leading edge,
  ordered by creation time so active changes do not move targets. Focus belongs
  to the complete tab shape, not only its label segment when a menu action
  shares the tab. Use an amber dot plus accessible text for unsaved state, and
  arrow/Home/End keyboard focus. Tabs start at a 236px preferred width, share
  the strip evenly as sessions are added, and stop shrinking at 112px (104px on
  narrow screens). Only after that floor may the list overflow horizontally.
  Keep the native scrollbar hidden; touch, trackpad, mouse-wheel, keyboard
  focus, and active-session changes must all reveal the intended tab. New Chat
  is a borderless-at-rest icon command in a protected trailing slot immediately
  after the rightmost tab; hover and focus may add a neutral surface. When tabs
  overflow, only the tab list scrolls and New Chat remains reachable. Do not
  render an overflow trigger when that tab has no available action. Save/Delete
  live in the visible overflow menu, with right-click as a shortcut rather than
  the only path.
- New Chat always confirms the active session. Switching away from the one
  unsaved working tab requires Save or discard; Autopilot stops before the
  backend changes session. The dialog traps focus, closes with Escape when no
  mutation is pending, and never covers the shell-level Emergency Stop.
- Messages: replies sit in a bordered neutral bubble (`--surface-2`,
  `--line-strong`, `--text`, `--radius` with a `--radius-xs` tail at the lower
  left) beside the assistant avatar (focusable when it carries run provenance,
  exposing the same diagnostic tooltip on hover and keyboard focus); your turns
  are right-aligned steel-tinted bubbles (`--user-bubble`, `--user-bubble-line`,
  tail at the lower right) without an avatar. Labs replies and the phone remote
  use the same reply bubble. Speaker and time sit above in
  `--muted` `--fs-hint`. The composer is one bordered field (`--surface`,
  `--line-strong`, `--radius`) holding the textarea, voice input and Send.
- Streaming cursor: a blinking `--accent` caret; warning state tints the border
  `--warn`. All animation respects `prefers-reduced-motion`.

### Phone remote

The remote at its own origin is laid out like Chat, with touch sizing
(`--control-h` 40px, 44px on coarse pointers):

- **Header** on `--bg`: the product lockup (monogram, **MagicHandy**, context
  **Remote**), then the connection readout (8px dot + text, as in the status
  bar) with a borderless refresh icon, and the account chip and Sign out at
  the end. On phones the readout moves to its own row under the header.
- **Desktop chat** is the Chat conversation card: a small caps title row, the
  same `.chat-message` replies with avatar and bubbles, and the one-field
  composer. The phone composer uses 16px text so focusing it does not zoom.
- **Control column** (`clamp(320px, 34vw, 440px)`): a **Now playing** control
  card (title, scrubber, transport, then Volume, Speed and Motion source rows)
  and a **Video library** control card (search, then one bordered list with the
  open video tinted). Play is the filled `--go` start button; Pause is
  secondary.
- On phones a Video / Chat segmented choice shows one of the two at a time.
- The full-width Stop footer sits on `--rail`; the account view uses the
  Settings card treatment.

### Feedback layer

- Toast: fixed bottom-center, `--surface-3`, `--line-strong`, `--shadow`,
  `--radius`; slides up on show. One at a time. Whether a toast's category also
  enters the bell history follows the saved notification categories (see the
  notification center above). On phones toasts and floating panels sit above
  the reserved Stop/navigation footer (`--footer-h`).
- Notification center and menus: one overlay surface with dividers rather than
  nested cards; constrained height and vertical scrolling keep the shell usable
  on short viewports.
- **Menus** (the persona switcher, chat-tab options) share one primitive,
  `useMenu`, following the WAI-ARIA menu button pattern. The trigger carries
  `aria-haspopup="menu"`, `aria-expanded` and, while open, `aria-controls`.
  Opening focuses the checked item, or else the first enabled one (the last
  when opened with ArrowUp); ArrowUp/ArrowDown wrap, Home/End jump, a printable
  key jumps to the next item starting with it, and disabled items are skipped.
  Roving `tabindex` keeps the menu one tab stop. Choosing an item, Tab, Escape
  or a press outside closes it; every close except an outside press returns
  focus to the opener, and an action that removes its opener (saving the active
  chat, deleting a tab) hands focus to a tab that survives it. Escape is also
  Emergency Stop's shortcut: a menu closes on it but never stops its
  propagation.
- **Playback panel** (video watch page): wider than 720px it is a flat rail
  docked in the player, beside the picture and the script plot, or over the
  top of the chat column when the chat is open beside the video, so the
  picture keeps its size. It never covers the picture, its transport, the
  script plot or the motion-source switch, and stays open while they are used
  (its trigger, Close and Escape close it; focus returns to the trigger). On
  phones it floats as a sheet above the footer and closes on a press outside.
  The transport adapts to the video frame's own width (container queries), so
  it stays whole beside the rail.
- Backend banner: full-width alert with a tinted `--danger` fill and border;
  lives at the top of the workspace, immediately below the status bar and above
  routed content.
- Empty states: centered icon, `--fs-heading` title, muted sentence and at most
  one action (for example Videos, and the desktop Remote page's **Open remote
  interface** button with its address).

### Icons

One monochrome inline-SVG set drawn with `currentColor`, 18px in nav / status.
No emoji as UI icons. Decorative marks are `aria-hidden`; an icon is never the
only label for a control.

## Layout And Breakpoints

```text
┌──────────────┬───────────────────────────────────────────────┐
│ M MagicHandy │  status bar (52px) — dot+text readouts · timer │
│              ├───────────────────────────────────────────────┤
│ ▸ Chat       │                                                │
│   Personas   │        routed workspace (one mounted)          │
│   Preset     │        content width ≤ 1280px                  │
│    modes     │        (Chat and Labs fill the workspace)      │
│   Pattern    │                                                │
│    library   │                                                │
│   Videos     │                                                │
│   Remote     │                                                │
│   Settings   │                                                │
│              │                                                │
│ [ STOP  Esc ]│                                                │
└──────────────┴───────────────────────────────────────────────┘
    212px            workspace column
```

- Desktop: rail + status bar + workspace; Chat is conversation + control
  sidebar.
- `≤1000px`: the Settings section list becomes a scrolling strip of filled tabs.
- `≤980px`: the status timer hides to preserve the compact status row.
- `≤860px`: the rail becomes a reserved bottom footer with tabs and a full-width
  Stop row above them. The footer participates in layout rather than overlaying
  the workspace, so content and the keyboard do not hide Stop. Chat stacks the
  conversation above the control cards.
- Field rows stack whenever the field is narrower than 560px.

## Motion And Accessibility

- Transitions are quiet (`0.15s ease` on interactive state; `0.13s linear` on
  visualizer position between 125ms backend samples). No bouncing, pulsing
  glows, or moving gradients.
- `prefers-reduced-motion: reduce` disables the visualizer/toast/toggle
  transitions, the chat entry animation and the connection wave/shake.
- Focus is a `2px --focus` outline at `2px` offset on every interactive element
  (inset where a scroller would clip it); routed views focus their heading on
  entry. The connection disclosure is non-modal: opening moves focus to Close
  and closing restores the trigger; it does not trap focus.
- Status is text + icon + color, never color alone. One polite live region for
  status; the visualizer is not a chatty live region.
- Text and controls meet WCAG AA contrast on the dark surfaces.

## Do / Don't

Do:

- keep one card level, label-left/control-right rows, and one switch style;
  separate sections with rules and spacing.
- keep status as compact dot+text; cap radius at `--radius-sm` for anything
  control-sized; use `999px` only for circular micro-elements.
- signal depth with the surface ladder; keep `--shadow` for floating layers.
- keep one interactive hue (`--accent`); green strictly for go, amber for
  caution, red for Stop and failure; left-aligned, information-dense, real
  hierarchy; sans-serif throughout.
- judge a change by how much scatter it removes: fewer treatments, fewer
  one-off sizes, fewer link-styled actions next to buttons.

Don't:

- nest cards; add header bands inside cards inside cards; use gradients,
  glows or shadows on resting controls.
- use underline indicators on navigation tabs; mix link-coloured text actions
  with bordered buttons in one toolbar.
- wrap status or controls in fully-round pills; paint a nav active state as a
  saturated fill; use emoji as icons; center a consumer-style hero with giant
  padding. This is an instrument.
- introduce purple or blue-green decorative tones or a serif face.

## Relationship To Other Docs

Labs uses a conversation-first layout under filled workspace tabs: the main
test modes are one segmented choice with the rarer modes in a select, and once
that row would wrap (about 700px with the English labels; translations run
longer) one **Test mode** select lists every mode instead, so the choice never
splits across two wrapped mechanisms. Live
motion and Autopilot are switches beside small secondary Stop, New chat and
Configure buttons, and the global Stop remains reachable. Detailed instructions
belong in the Help tab (a vertical topic list like Settings); nearby Help links
open the relevant topic. Response details and motion plots stay available
without displacing the chat.

- [ui-navigation-redesign.md](ui-navigation-redesign.md): the shell IA these
  tokens dress.
- [ui-design.md](ui-design.md): the safety/accessibility/parity rules these
  components must not regress.
- ADR 0004 (frontend strategy) and ADR 0009 (React frontend migration): the UI
  is now React, but still statically built, embedded, and offline at runtime;
  these guidelines add no external asset fetch.

### Freestyle preferences (2026-10-01)

The Preset Modes preferences reuse SegmentedChoice and SetpointSlider. Feel,
accent and shape are categorical; pace, length, focus, roaming and variety are
five named stops. The grid fits narrow panes and tick labels wrap within their
allocated width. Keep actual values and active shape status backend-owned,
serialize pending edits, and preserve the shared global Stop outside the route.
See [ADR 0038](decisions/0038-freestyle-stroke-stream.md).
