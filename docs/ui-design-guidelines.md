# UI Design Guidelines

## Purpose

The concrete, implementable reference for the sidebar-navigation shell. It is
the companion to two docs and does not repeat them:

- [ui-navigation-redesign.md](ui-navigation-redesign.md) — the information
  architecture (nav rail, workspaces, Autopilot, migration).
- [ui-design.md](ui-design.md) — the enduring safety, accessibility, and parity
  rules.

This file is the token-and-component layer the shell refactor builds against:
exact values from `web/app.css`, the changes the redesign makes to them, and the
do/don't rules that keep the result from reading as generic AI output. A visual
sketch of the target shell is [ui-shell-sketch.svg](ui-shell-sketch.svg).

## Design Tokens

Use existing tokens instead of raw hex values in implementation, and do not
invent parallel values. When this document calls out a current component-local
value, either keep it local to that component or promote it to a token before
reusing it elsewhere.

### Color roles

The app is dark graphite with one interactive hue. Two stale CSS comments still
say "violet"/"teal" — those are wrong; the palette is graphite + steel azure,
and no purple or blue-green decorative tone is allowed. Named tokens below come
from `web/app.css :root`.

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
| `--accent` | `#5b9dd9` | **The one interactive hue** (steel azure): headings' eyebrow, nav state, app actions, toggles, focus |
| `--accent-strong` | `#497fb5` | Interactive hover/border/checked |
| `--accent-ink` | `#0b131c` | Text on an accent fill |
| `--steel-deep` | `#3a5a7c` | Accent gradient partner (brand, avatar) |
| `--ok` / `--ok-strong` / `--ok-ink` | `#4fc06d` / `#3da55c` / `#08130c` | **Running/go only**: Start, active visualizer, ok status |
| `--warn` | `#d8b66a` | Warning / paused / pending |
| `--danger` / `--danger-strong` | `#e5484d` / `#c93b40` | Stop / danger |
| `--focus` | `#82b4e2` | Focus ring |
| `--user-bubble` / `--user-bubble-line` | `#28303b` / `#3a4450` | User chat bubble |

Rule: a new surface picks an existing role. `--accent` = "you can act on this",
`--ok` = "it is running", `--warn` = "caution", `--danger` = "stop". Everything
else is neutral graphite.

### Radius scale (capped — the anti-bubble rule)

| Token / value | Use |
| --- | --- |
| `--radius` `12px` | Panels and cards |
| `--radius-sm` `8px` | Buttons, inputs, chips, chat bubbles, badges, nav rows |
| `4px` | Chat bubble tail corner only |
| `6px` | Keycap/shortcut hint |
| `999px` | **State dots and the toggle thumb only** |

`999px` is currently also used on status pills, the visualizer frame, and the
chat-provider chip. The redesign **retires the fully-round pill for status**
(see Status readouts). Nothing button-sized or status-sized is a pill.

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
- **Profile lockup** (top): 34–36px avatar as a `--radius-sm` squircle with the
  `--accent`→`--steel-deep` gradient (the existing brand/avatar gradient),
  `MagicHandy` in `--text` `600`, session identity (`local · llama.cpp · cloud`)
  in `--muted` `0.72rem`. It is a button → navigates to `#/settings`.
- **Nav row**: 40px tall, `0 12px`, 18px icon + `0.9rem` label, `--radius-sm`.
  - Default: `--muted` label, transparent bg.
  - Hover: `--surface-2` bg.
  - **Active: a 3px `--accent` left border + `--surface-2` fill + `--text`
    label + `--accent` icon, and `border-radius: 0`** (single-side borders never
    get rounded corners). Not a saturated pill. `aria-current="page"`.
- **Pinned Stop** (bottom, `margin-top:auto`): the existing `.stop-button`
  treatment (full-width here) — `--danger`/`--danger-strong` red gradient, white
  `700`, `--radius-sm`, `--shadow`, with the `Esc` keycap. Present on every page.

### Status readouts (changed — the headline fix)

Today's status is a `.status-pill`: `border-radius: 999px`, `--surface-2` fill,
`--line` border, and a glowing dot (`box-shadow: 0 0 6px …`). That fully-round,
filled, glowing chip is exactly the "oversized round bubble" to remove.

Replace with a compact readout in the status bar:

- an 8–9px state **dot** + text label, inline; **no pill fill, no border, no dot
  glow**; group readouts with spacing or a 0.5px `--line` divider tick.
- dot color carries state: `--accent` active, `--ok` ok/running, `--warn`
  pending/paused, `--danger` error, `--muted` idle. Severity is always dot **and**
  text, never color alone.
- the run timer is `--muted` label + tabular-nums value with a clock icon.

The one authoritative visualizer keeps its compact form in the status bar (track
+ position marker; `--ok` marker when active) and its detailed form on the Chat
page — one component, engine-driven, position labeled as a commanded estimate.

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

### Cards, groups, fields

- Panel/card: `--surface`, `--line`, `--radius`, `--shadow`, padding
  `clamp(16px, 2.4vw, 24px)`.
- Nested group (quick settings, llm settings): `--surface-2`, `--line`,
  `--radius-sm`. Do not nest rounded cards more than one level — group with
  spacing and a `--line` divider, not another shadowed box.
- Inputs / select / textarea: `--bg-inset`, `--line`, `--radius-sm`, padding
  `9px 11px`; focus swaps the border to `--accent` (plus the global focus ring).
- Range: native with `accent-color: --accent`. Toggle: 40×22 track, 18px thumb;
  off `--line` + `--muted` thumb, on `--accent-strong` track + white thumb.
- Badge (e.g. the "testing" tag): 1px `--line-strong`, `--surface-2`, `--muted`,
  `0.68rem`, `999px` is *not* used — small `--radius-sm`/pill-ish hairline chip,
  quiet.

### Chat

- Message row: 30px avatar gutter + body; assistant left, user right (mirror the
  grid). Avatar 30px, assistant uses the accent gradient, user uses
  `--user-bubble`.
- Bubble: `--radius-sm` with the opposite bottom corner squared to `4px` (the
  tail); assistant `--surface-2`/`--line`, user `--user-bubble`/`--user-bubble-line`;
  `--shadow-soft`. Speaker label + timestamp above in `--muted` `0.72rem`.
- Streaming cursor: a blinking `--accent` caret; warning state tints the border
  `--warn`. All animation respects `prefers-reduced-motion`.

### Feedback layer

- Toast: fixed bottom-center, `--surface-3`, `--line-strong`, `--shadow`,
  `--radius-sm`; slides up on show. One at a time.
- Backend banner: full-width alert, translucent `--danger` fill + border, white
  text; lives in the status bar and stays above every workspace.

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
│   Pattern    │        Chat · Preset Modes · Library · Settings│
│    Library   │                                                │
│   Settings   │                                                │
│ ──────────── │                                                │
│ [ STOP  Esc ]│                                                │
└──────────────┴───────────────────────────────────────────────┘
   ~224px           workspace column
```

- Desktop: rail + status bar + workspace; the workspace column holds the routed
  page (two-column on Chat: control column + conversation).
- `≤1040px`: rail may collapse to a 64px icon rail (labels → tooltips); Stop
  stays a pinned red icon block.
- `≤880px` (existing breakpoint): status readouts wrap; keep them compact.
- `≤720px`: rail becomes a bottom tab bar; **Stop detaches to its own fixed
  bottom bar above the tabs** so the keyboard never hides it.
- `≤520px` (existing): field rows collapse to one column; bubbles go full width.

## Motion And Accessibility

- Transitions are quiet (`0.15s ease` on interactive state; `0.24s` on the
  visualizer position). No bouncing, pulsing glows, or moving gradients.
- `prefers-reduced-motion: reduce` disables the visualizer/toast/toggle
  transitions and the chat entry animation (already wired in `app.css`).
- Focus is a `2px --focus` outline at `2px` offset on every interactive element;
  routed views and any popover trap focus and restore it on close.
- Status is text + icon + color, never color alone. One polite live region for
  status; the visualizer is not a chatty live region.
- Text and controls meet WCAG AA contrast on the dark surfaces.

## Do / Don't

Do:

- keep status as compact dot+text; cap radius at `--radius-sm` for anything
  control- or status-sized; reserve `999px` for dots and the toggle thumb.
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
- ADR 0004 (frontend strategy): fresh, minimal, embedded, no build step — these
  guidelines add no framework and no asset fetch.
