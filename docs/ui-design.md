# UI Design

## Purpose

This defines the MagicHandy UI. It is designed to avoid the specific UI flaws
observed in StrokeGPT-ReVibed, not to reproduce its layout. It complements
ADR 0004 (frontend rebuilt fresh, minimal-first, backend-state-driven) and is
implemented starting in Phase 8.

The app controls hardware in real time. The UI's first duties are to show what
the device is actually doing and to stop it instantly. Everything else is
secondary, and the layout reflects that priority.

## Principles

Each principle maps to a concrete flaw; see "Flaws Explicitly Avoided".

1. The backend is the single source of truth. The UI renders motion-engine
   state and never keeps a parallel motion state model or guesses position.
2. There is exactly one authoritative device visualizer, not competing meters
   plus a separate cylinder.
3. Emergency stop is always visible and always reachable. It is never inside a
   collapsible region or hidden by the on-screen keyboard.
4. Live device controls apply immediately. No save button stands between the
   user and the device for speed, range, or direction.
5. Control idioms are consistent and predictable. A control's immediacy is
   obvious from its type; the user never wonders whether a change took effect.
6. There is one feedback channel, and a modal/route can never occlude it.
7. Navigation is flat. No modal is stacked on another modal.
8. Layout is flexible. Regions size to content with min-heights; overlays use a
   dedicated top layer, not fixed-height `overflow:hidden` containers escaped by
   absolute/fixed positioning hacks.
9. The UI is accessible by construction: real controls (not ARIA-retrofitted
   artwork), status as text + icon + color, defined focus order, reduced-motion.
10. Exactly one client controls the device. Others open read-only by design,
    not by after-the-fact warning.

## Layout

> **As built (2026-07-08, React shell).** The app is the **permanent left
> navigation sidebar that switches pages** from
> [ui-navigation-redesign.md](ui-navigation-redesign.md): a nav rail
> (Chat / Preset Modes / Pattern Library / Settings) with **Stop pinned to
> the rail footer on every page**, a status-only top bar (dot+text readouts,
> stopwatch, no controls), and workspaces as routed pages under a hash
> router. Chat keeps the live control column (controls, quick settings,
> manual test); Settings is a routed page with sibling sections
> (`#/settings/device|model|voice|prompts|diagnostics` — compaction planned
> in [settings-compaction.md](settings-compaction.md)). Token and component
> specifics live in [ui-design-guidelines.md](ui-design-guidelines.md).
>
> The subsections below marked **Historical** describe the pre-React shell
> (status bar + single control sidebar + settings window). They are kept
> because their rationale shaped the current shell — the control column,
> immediate-apply grouping, and testing-badged manual motion moved into the
> Chat page largely unchanged — but they no longer describe the build.

### Historical: persistent control bar (pre-React shell)

A slim, always-on bar owns **status only** — it is not a control strip:

- connection/transport/controller status, live
- the motion stopwatch (accumulated run time; freezes on pause, resets on stop)
- the one device visualizer in compact form
- the clickable profile button that opens the settings window

This bar is fixed to the viewport, never collapsible, and not hidden when the
keyboard opens. Controls — including Stop — live in the sidebar panel; the
bar carries the state the user glances at.

### Historical: primary content (pre-React shell)

- The conversation is the main scrollable region and should use available
  desktop width. Do not reintroduce the narrow-chat regression from the old app;
  constrain line length inside message bubbles rather than constraining the
  entire chat column unnecessarily.
- One navigation affordance — the profile button in the bar — opens Settings
  as a single window over the control view; deeper areas are sibling sections
  inside that one window, never stacked modals.

### Historical: sidebar control rail (now the Chat control column)

Controls live in **one always-visible sidebar panel** next to the chat — the
shape the old app proved out — not in the top bar (top-bar controls read as
awkward) and not between the chat log and composer. It is organized by use,
top to bottom:

1. **Controls** — Freestyle start/stop, Pause/Resume (phase-preserving,
   disabled when idle), the live run readout, and the chat-keepalive toggle
   (restarts after transport recovery only — never after a user stop or
   pause).
2. **Quick settings** — speed/stroke/reverse plus the motion style
   (gentle/balanced/intense) that biases Freestyle's deterministic scoring;
   immediate-apply.
3. **Manual motion** — Start/Stop test, pattern, speed, **explicitly badged
   "testing"** with a hint that it drives the device directly to test the
   connection; normal motion comes from chat (and modes in Phase 11).
4. **Stop everything** — full-width, red, at the bottom, with the Esc hint.

The panel is never collapsible. On narrow viewports it stacks **above** the
chat, and Stop detaches into a fixed bottom bar so it is always on screen.

### Mobile (as built)

The nav rail becomes a bottom tab bar with Stop sharing a reserved footer
that never overlays content (fixed in the Phase 13.4 shell pass); the
keyboard never hides device state or Stop; Settings pages fit the viewport
with Save reachable.

### Implemented structure (as built, React shell)

`#/chat` is the primary workspace: the chat log fills the main column and
the control column holds Controls (Freestyle, Pause/Resume + run readout,
keepalive), Quick settings (immediate-apply), the testing-badged Manual
motion group, and the motion visualizer. `#/modes` hosts Preset Modes
(Freestyle now, Autopilot when its planner lands), `#/library` is the
labeled Pattern Library placeholder until Phase 14, and
`#/settings/device|model|voice|prompts|diagnostics` are sibling sections of
the routed Settings page — deep-linkable, no window, no stacked overlays.
Stop lives in the nav-rail footer on every route (plus Escape), outside the
route tree so no navigation state can unmount it; backend loss shows the
persistent banner and locks backend-required controls while Stop stays
enabled. Nothing renders as a dashboard; new surfaces join an existing
workspace or become a nav destination, never an extra always-visible panel.

## The Device Visualizer

One component, one source of truth.

- Renders live motion-engine state: sampled position, active stroke-range
  envelope, relative speed, active pattern/label, and transport health.
- Has explicit visual states for moving, idle, paused, starved/recovering, and
  disconnected. A still frame is never ambiguous between "idle" and "stalled".
- Reads engine state pushed from the backend (SSE/WebSocket). If state is stale
  it says so; it does not extrapolate a guessed position.
- Distinguishes commanded/estimated position from device-confirmed position, and
  never presents a planned point slope as a measured device speed; when only an
  estimate is available, it is labeled as an estimate.
- Is never itself a click target: controls are separate, labeled elements
  (quick settings live in the sidebar), not artwork turned into a mystery
  button.

## Quick Controls

Speed limit, stroke range, reverse direction.

- Apply immediately to active motion (ADR 0002, invariant 9). No save step.
- Always visible in the sidebar control rail (stacked below chat on mobile) —
  no popover to hunt for.
- Reflect engine state live; if the engine clamps or resolves a value, the
  control shows the resolved value inline rather than via a status line
  elsewhere on screen.

## Settings

> **As built: Settings is a routed page**, reached from the profile lockup,
> with sibling sections under `#/settings/*` — the window-over-chat decision
> below (2026-07-03) is historical. The value the window protected — never
> leaving chat for a quick change — is kept by the live quick settings on
> the Chat page. The immediate-apply, single-layer, and
> confirmed-destructive rules below are unchanged and still enforced.
>
> **Next planned change:** provider-scoped disclosure and the Voice tab's
> split into Speech input / Speech output sections —
> [settings-compaction.md](settings-compaction.md) (Slice 13.5). Fields
> render only when the selected provider/mode makes them meaningful; status
> readouts are never hidden.

Preserve the familiar grouping (Persona, Model, Voice, Device, Motion,
Diagnostics) but fix the structural flaws:

- Settings is **one window layered over the control view** (product decision
  2026-07-03, matching the old app's settings-in-a-window shape): chat stays
  visible behind it, the persistent bar and Stop stay above it, and sub-areas
  are sibling sections inside that one window. The anti-flaw substance is
  unchanged — exactly one layer, never a modal stacked on a modal, and it can
  never occlude the feedback channel or Stop.
- Live device settings apply immediately. The few genuinely expensive or
  destructive actions (changing transport, clearing memory) use an explicit,
  clearly labeled action with confirmation.
- A commit/destructive affordance can never be removed by a single style rule;
  its presence is covered by a UI test.
- Idioms are consistent: a toggle applies now; a button commits an explicit
  action. There is no mix where some checkboxes auto-save and others need a
  button.
- Advanced and experimental settings live under a clearly marked, collapsed
  "Advanced" area, not interleaved with everyday controls.
- New features ship with a minimal settings surface. Tuning knobs start as
  diagnostics instrumentation and are promoted to settings only when real use
  proves users need them. (StrokeGPT's voice tab grew ~12 recognition knobs —
  beam size, VAD thresholds, noise floors — before defaults were validated,
  making reliability feel like the user's tuning problem.)

## Visual Language

The app is dark, layered, and hue-coded. StrokeGPT-ReVibed's visual strengths
(a semantic multi-hue system, colored section identity, chat bubbles with real
anatomy, surface depth) are kept; its weaknesses (an external font fetch, ad
hoc per-widget colors) are not.

- **Surface layering, not hairlines alone**: page < panel < raised element <
  overlay, each a distinct value with shadows. Insets (chat log, inputs) drop
  below the panel value so regions read at a glance.
- **Hue roles are semantic and fixed**: steel azure is the single interactive
  hue — headings, navigation state, app actions (Send, Save), toggles, and
  focus. Green strictly means running/go — the Start button, the active
  visualizer, ok status. Amber = warning, red = stop/danger. Everything else
  stays neutral graphite; the user's chat bubble is a muted slate. Purple and
  pale blue-green decorative tones are explicitly banned (they read as
  distracting); a new feature picks an existing role, it does not invent a
  hue.
- **Chat messages have anatomy**: speaker label with timestamp, avatar chip,
  tail-cornered bubble with shadow, right/left role alignment, a streaming
  cursor while the model is generating, and a subtle entry animation. All
  animation respects `prefers-reduced-motion`.
- **Native controls render dark**: the `color-scheme` meta is `dark` so
  scrollbars, selects, and checkboxes match (a light scrollbar on a dark app
  is a bug), with thin themed scrollbars on top.
- **No network-fetched fonts or assets**: system font stack, everything
  embedded and offline.

## Feedback And Status

- One notification system, layered above content, that a modal or route cannot
  cover. (In the old app the global status bar sat behind the settings modal,
  which forced per-section status spans to be added as a workaround.)
- Every status carries severity as text + icon + color, never color alone.
- Transient confirmations appear inline next to the control that changed; system
  events (connection lost, device offline, worker crash) use the persistent
  notification area.
- Errors state what failed and what to do, surfacing safe backend/transport
  detail (path, status) per the diagnostics contract, not a generic "error".

## Emergency Stop

- "Stop everything": full-width, red, at the bottom of the never-collapsible
  sidebar panel (the old app's placement, minus its collapsibility flaw);
  large touch target; high contrast.
- Always on screen: it renders above the settings backdrop (never dimmed or
  occluded by the settings window), and on small viewports it detaches into a
  fixed bottom bar so scrolling cannot hide it.
- Has a documented, visible global keyboard shortcut (Esc; with the settings
  window open, the first Esc closes the window and the second stops).
- Backed by ADR 0002's stop contract: it cancels motion, stops planners,
  clears any paused state, and marks stopped even if the transport call
  fails. The UI shows "stopped" immediately, then reconciles with engine
  state.
- Never disabled by a transient UI state. If the backend is unreachable, Stop
  still attempts and the UI reports whether it succeeded. Read-only clients
  can always trigger it.
- Pause is not Stop: Pause/Resume is a control action (read-only clients
  cannot trigger it) that freezes phase for continuation; Stop is the safety
  path and always resets everything, including the run clock.

## Connection And Single-Controller

- Connection/transport state is always visible in the persistent bar.
- Exactly one client may command the device. Additional clients open read-only:
  they can watch state and trigger Stop, but cannot send motion, rather than
  racing and showing a warning banner after the fact.
- Controls that require a backend or a connection are disabled with a visible
  reason (inline text/tooltip), never a silent no-op.

## Accessibility

- Interactive elements are real semantic controls. Decorative artwork stays
  `aria-hidden`; the visualizer's controls are separate, labeled elements, not a
  role bolted onto live-updating text.
- Full keyboard operability, visible focus, logical order. Routed views and any
  overlay trap focus and restore it on close.
- Status is never color-only.
- `prefers-reduced-motion` is respected for visualizer animation and transitions.
- Live regions are minimal and intentional: one polite region for status; the
  visualizer is not a chatty live region.
- Text and essential UI meet WCAG AA contrast.

## Responsive And Layout Mechanics

- Regions use flexible sizing with min-heights, not fixed heights with
  `overflow:hidden`. Content that must overlay (popovers, menus) lives in a
  dedicated top layer and is positioned from measured geometry, never by relying
  on `100vw`/`100vh` (which can resolve to 0 in some embedded/headless contexts).
- Touch targets meet a minimum size on mobile.
- No critical real-time information is hidden by the on-screen keyboard.
- The same components render across breakpoints; breakpoints change arrangement,
  not which safety-critical controls exist.

## Implementation Constraints

- Per ADR 0004: rebuilt fresh, minimal-first, ES modules, no global mutable
  god-registry, no required runtime build step, assets embedded in the binary.
- No client-side duplication of motion state; the engine state stream is the
  model.
- `web/` obeys the size and no-god-module norms in
  `docs/goals-and-guardrails.md`.
- UI tests guard: Stop is present and reachable in every layout; commit and
  destructive affordances exist; quick controls apply immediately; the
  visualizer reflects engine state including the stale and disconnected cases;
  desktop chat uses the available content width.

## Flaws Explicitly Avoided

Observed in StrokeGPT-ReVibed, then the design response here:

- Two visualizers (bottom speed/depth meters plus a sidebar cylinder) with an
  ambiguous click target -> one authoritative visualizer with an explicit,
  labeled quick-settings control.
- Quick-settings popover clipped by a fixed-height `overflow:hidden` strip,
  requiring fixed-position JS and breaking when `100vw` resolved to 0 -> flexible
  layout plus a dedicated overlay layer positioned from measured geometry.
- Visualizer/UI showing guessed state -> the UI renders pushed engine state, with
  an explicit stale/disconnected state.
- A single stale CSS rule once hid every settings Save button -> immediate-apply
  by default, minimal explicit-commit affordances, and a UI test asserting they
  exist.
- Emergency Stop in a *collapsible* sidebar, and the motion strip hidden when
  the mobile keyboard opened -> Stop lives in a never-collapsible sidebar
  panel, renders above the settings backdrop, and pins to a fixed bottom bar
  on small viewports; device state stays in the persistent bar.
- Mixed checkbox / toggle / save-on-change idioms -> one consistent, predictable
  immediacy model.
- Status scattered across many spans and occluded by the settings modal -> one
  layered notification system, never occluded, severity not color-only.
- Modals launched from inside the settings modal -> flat, routed navigation.
- `aria-hidden` artwork turned into a button, and `role="button"` on
  live-updating text -> real semantic controls; decorative stays decorative.
- Multi-tab hardware hazard handled by a warning banner -> an enforced single
  active controller; extra clients are read-only.

## Functional Parity Baseline

StrokeGPT-ReVibed's UI was aesthetically flawed but **functionally good**, and
much of that function was hard-won from real bug reports. Avoiding its visual
and structural flaws must not silently drop its behavior. This section is the
checklist of functional behaviors the old app proved out; a MagicHandy phase
that touches an area owns the corresponding rows. Compared 2026-07-01 against
`main` (post-Phase 9).

### Regressed — implemented in MagicHandy but functionally weaker

These exist in some form today but lost behavior the old app had already
learned the hard way. They are scheduled (Phase 9B unless noted):

1. **Backend-loss handling.** The old app showed a persistent connection-lost
   banner and locked `data-requires-backend` controls the moment any fetch
   failed, restoring them on the next success. MagicHandy shrinks this to a
   small "Core unavailable" pill and a failed-save toast; controls stay
   enabled and look functional while the backend is gone — the exact "tab
   keeps pretending to save" failure the old app fixed.
2. **Chat scrollback.** The old app learned near-bottom stickiness: background
   messages never yank the user away from older content, with a visible
   "jump to latest" affordance. MagicHandy force-scrolls to the bottom on
   every streamed delta.
3. **Connection verification.** The old app had a visible connection panel
   with key status; MagicHandy has a cloud connection-check endpoint but no UI
   affordance to run it, and no transport/connection state in the persistent
   bar (this document requires it). A user cannot confirm their key works
   without starting motion.
4. **Estimate honesty in the visualizer.** The old app documented that the
   position line is a commanded estimate, not a device readout. MagicHandy
   renders engine sample position with no estimate/confirmed labeling (this
   document requires the distinction).
5. **Pause/Resume.** The old app had pause/resume as a first-class control
   next to Stop, including chat-driven pause. MagicHandy has only Stop; the
   engine has no pause state (Phase 11 with the mode work, since resume must
   preserve phase).
6. **Copyable diagnostics.** The old app had one-click copyable system status
   for bug reports. MagicHandy shows a diagnostics grid and raw trace export
   but no copyable summary bundle (Phase 9B — needed for hardware-validation
   bug reports).
7. **Reset to defaults.** The old app had an explicit settings reset;
   MagicHandy only auto-recovers from a corrupt file (Phase 10 with the
   settings UI).
8. **Stop shortcut visibility.** Escape triggers Stop but nothing in the UI
   says so; this document requires the shortcut to be documented and visible.
9. **Chat continuity.** The old app kept a server-side message log with
   per-client cursors; MagicHandy chat history was a 12-turn client array
   lost on reload and invisible to a second tab. **Closed by the Phase 13
   delivery-ordering foundation** (ADR 0003): the SQLite `messages` log is
   the canonical history, the panel seeds from it on load, other tabs pick
   up new rows via the state poll, and each client advances only its own
   cursor (reads are never destructive).

### Post-Phase-10 shell pass

Row 5 (pause/resume) is closed early: the engine gained phase-preserving
Pause/Resume (`/api/motion/pause`, `/api/motion/resume`) with a run clock
(`running_ms`) that freezes on pause and resets on stop, surfaced as the
Pause/Resume control in the sidebar panel and the stopwatch in the status
bar. Chat-driven pause maps to the deterministic stop fast-path until Phase
11 wires planners. Row 9 (server-side chat continuity) closed with the Phase
13 delivery-ordering foundation, completing the parity baseline.

### Phase 10 parity implementation

Row 7 (reset to defaults) is closed: Settings > Diagnostics has an explicit
double-confirm "Reset all settings" action backed by `POST /api/settings/reset`;
memories and prompt sets are deliberately untouched by it.

### Phase 9B parity implementation

The Phase 9B UI pass closes rows 1-4, 6, and 8 at the current architecture
level: backend loss now shows a persistent banner and locks backend-required
controls; chat scrolling keeps the user's scrollback position unless they are
already near the bottom; the connection panel exposes a non-motion connection
check; the visualizer and diagnostics label position as a commanded estimate;
diagnostics has a one-click copyable summary; and the persistent Stop control
shows the Esc shortcut. Pause/resume remains Phase 11 because it needs
phase-preserving engine state. Reset to defaults remains Phase 10 with the
settings UI. Server-side chat continuity remains Phase 12 with ADR 0003.

### Not yet built — planned, not regressions

Modes and their affordances (mode buttons, "I'm close", mood/timer, sequence
log — Phase 11), memory and prompt library UI (Phase 10), voice input/output
controls (Phases 12-13), pattern library, training studio, program player and
feedback buttons (Phase 14), migration/setup wizard (Phase 15/17), Ollama/GGUF
model catalog UI (`docs/model-management.md`), multi-tab controller
enforcement (Phase 9B — stronger than the old app's warning banner), the
bounded speed-test button and live device-position layer (post-parity
backlog).

### Improved — keep these wins

One authoritative visualizer driven by engine state; Stop in a persistent bar
(the old app hid it in a collapsible sidebar); immediate-apply quick controls
with no Save between the user and the device; flat navigation without stacked
modals; strict JSON chat contract with visible repair/malformed states; a
single motion path with no per-source divergence; trace export as one click.

## Deferred And Open

- The framework choice stays deferred per ADR 0004 (default: none); revisit at
  Phase 8 only if the minimal modular approach strains.
- The pattern authoring UI (Phase 14) is the largest UI risk and may remain
  partial at the Phase 17 parity review; it must still obey these principles.

## Relationship To Other Docs

- ADR 0004: frontend strategy (fresh, minimal, backend-driven).
- ADR 0009: React frontend migration, keeping backend-authoritative state and
  static embedded output.
- ADR 0002: stop contract and the semantic/transport split the UI reflects.
- `docs/ui-navigation-redesign.md`: the newer sidebar-shell information
  architecture that supersedes this document's current-build layout notes.
- `docs/ui-design-guidelines.md`: the token, component, and visual-language
  details for implementing the sidebar shell.
- `docs/controller-dispatch-semantics.md`: active-controller lease, read-only
  clients, motion SSE, and dispatch-owner switch behavior.
- `docs/goals-and-guardrails.md`: maintainability norms for `web/`.
- `docs/risk-register.md`: R9 (UI regression) and R12 (frontend debt).
