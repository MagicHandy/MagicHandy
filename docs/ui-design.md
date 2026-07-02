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

### Persistent control bar (all viewports, never hidden)

A slim, always-on bar owns the safety- and state-critical elements:

- connection/transport status, live, with the reason available on expand
- the one device visualizer in compact form
- the emergency Stop control
- active mode/label and the quick-settings entry point

This bar is fixed to the viewport. It is never inside the sidebar, never inside a
collapsible panel, and is not hidden when the keyboard opens. Stop and device
state survive every UI state.

### Primary content

- The conversation is the main scrollable region and should use available
  desktop width. Do not reintroduce the narrow-chat regression from the old app;
  constrain line length inside message bubbles rather than constraining the
  entire chat column unnecessarily.
- One navigation affordance opens Settings as a routed full-region view, so
  deeper screens (prompt library, pattern authoring, diagnostics detail) are
  sibling routes, never stacked modals.

### Sidebar (optional, desktop only)

The sidebar holds only non-critical, browse-style content such as history and
library lists. Nothing safety-critical lives here, because it can be collapsed.
Stop, connection, and the visualizer are in the persistent bar.

### Mobile

Same persistent control bar. The keyboard never hides device state or Stop.
Settings and library are routed views, not an off-canvas overlay competing with
the control bar.

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
- Exposes quick settings through an explicit, labeled control, not by turning
  the artwork itself into a mystery button.

## Quick Controls

Speed limit, stroke range, reverse direction.

- Apply immediately to active motion (ADR 0002, invariant 9). No save step.
- Reachable from the persistent bar in every viewport.
- Reflect engine state live; if the engine clamps or resolves a value, the
  control shows the resolved value inline rather than via a status line
  elsewhere on screen.

## Settings

Preserve the familiar grouping (Persona, Model, Voice, Device, Motion,
Diagnostics) but fix the structural flaws:

- Settings is a routed view, not a modal; sub-areas are sibling routes. No
  modal-in-modal.
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

- Always rendered in the persistent bar; large touch target; high contrast.
- Has a documented, visible global keyboard shortcut.
- Backed by ADR 0002's stop contract: it cancels motion, stops planners, and
  marks stopped even if the transport call fails. The UI shows "stopped"
  immediately, then reconciles with engine state.
- Never disabled by a transient UI state. If the backend is unreachable, Stop
  still attempts and the UI reports whether it succeeded.

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
- Emergency Stop in a collapsible sidebar, and the motion strip hidden when the
  mobile keyboard opened -> Stop and device state live in a persistent bar that is
  never hidden.
- Mixed checkbox / toggle / save-on-change idioms -> one consistent, predictable
  immediacy model.
- Status scattered across many spans and occluded by the settings modal -> one
  layered notification system, never occluded, severity not color-only.
- Modals launched from inside the settings modal -> flat, routed navigation.
- `aria-hidden` artwork turned into a button, and `role="button"` on
  live-updating text -> real semantic controls; decorative stays decorative.
- Multi-tab hardware hazard handled by a warning banner -> an enforced single
  active controller; extra clients are read-only.

## Deferred And Open

- The framework choice stays deferred per ADR 0004 (default: none); revisit at
  Phase 8 only if the minimal modular approach strains.
- The pattern authoring UI (Phase 14) is the largest UI risk and may remain
  partial at the Phase 17 parity review; it must still obey these principles.

## Relationship To Other Docs

- ADR 0004: frontend strategy (fresh, minimal, backend-driven).
- ADR 0002: stop contract and the semantic/transport split the UI reflects.
- `docs/goals-and-guardrails.md`: maintainability norms for `web/`.
- `docs/risk-register.md`: R9 (UI regression) and R12 (frontend debt).
