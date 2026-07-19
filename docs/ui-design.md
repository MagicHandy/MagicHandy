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
> the rail footer on every page**, a compact status-led top bar (dot+text
> readouts, stopwatch, mini visualizer, and the connection disclosure), and
> workspaces as routed pages under a hash router. The shell-owned connection
> manager stays available on every route and owns live provider actions plus
> speed/stroke limits. Chat keeps
> motion behavior, manual test, and the visualizer; Settings is a routed page with sibling sections
> (`#/settings/device|model|voice|prompts|diagnostics` — provider-scoped
> compaction implemented in Slice 13.5; see
> [settings-compaction.md](settings-compaction.md)). Token and component
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
keepalive), reverse/style motion behavior, the testing-badged Manual motion
group, and the motion visualizer. The shell-level connection manager owns the
saved dispatch owner's connect/check/discover actions and immediate speed/stroke
limits on every route. `#/modes` hosts Preset Modes
(Freestyle now, Autopilot when its planner lands), `#/library` hosts the
Phase 14 Browse / Programs / Import / Author / Training workspace, and
`#/settings/device|model|voice|prompts|diagnostics` are sibling sections of
the routed Settings page — deep-linkable, no window, no stacked overlays.
Stop lives in the nav-rail footer on every route (plus Escape), outside the
route tree so no navigation state can unmount it; backend loss shows the
persistent banner and locks backend-required controls while Stop stays
enabled. Nothing renders as a dashboard; new surfaces join an existing
workspace or become a nav destination. Only compact safety and status tools,
including Stop and the connection manager, remain route-independent.

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
- Uses a restrained vertical Handy 2-inspired body and sleeve rather than an
  abstract progress bar. The configured stroke envelope surrounds the moving
  sleeve, which follows the backend sample position. Detailed telemetry names
  state, target speed, and active target without adding controls to the artwork.
- Is never itself a click target: controls are separate, labeled elements
  (limits live in the connection manager and behavior lives in Chat), not
  artwork turned into a mystery button.

## Quick Controls

Speed limits, stroke range, reverse direction, and motion style.

- Apply immediately to active motion (ADR 0002, invariant 9). No save step.
- Speed and stroke limits live in the persistent floating connection manager;
  reverse and style stay in Chat's motion-behavior group. The collapsed manager
  remains visible on every route and clears the reserved mobile Stop footer.
- Speed and stroke each use one dual-thumb control. Thumb changes send only the
  changed backend field; Stroke always keeps at least one percentage point
  between its minimum and maximum to match backend validation.
- Reflect engine state live; if the engine clamps or resolves a value, the
  control shows the resolved value inline rather than via a status line
  elsewhere on screen.

## Settings

> **As built: Settings is a routed page**, reached from the profile lockup,
> with sibling sections under `#/settings/*` — the window-over-chat decision
> below (2026-07-03) is historical. The value the window protected — never
> leaving the current task for a quick change — is kept by the floating
> connection manager. The immediate-apply, single-layer, and
> confirmed-destructive rules below are unchanged and still enforced.
>
> **As built:** provider-scoped disclosure is implemented for Model and Voice.
> Fields render only when the selected provider/mode makes them meaningful;
> status readouts remain visible. Model imports expand inline within the routed
> page rather than opening a second modal layer.

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

- Connection/transport state is always visible in the floating manager trigger.
  Expanding it exposes actions for only the saved dispatch owner: Cloud REST
  check, browser-owned Bluetooth session, or Intiface connect/discovery/select.
  Cloud REST also exposes a compact write-only connection-key field and the
  active API v3 ID source; owner choice, key clearing, developer ID override,
  and server addresses remain in Settings.
- The connection artwork uses a transparent hand isolation derived from the
  reviewed conductor reference. It renders at its intrinsic square ratio with
  no runtime mask or clip. The scaled frame contains the hand, three
  intense-blue SVG arcs, and the poster's tall capsule, domed body, LED, and
  square marker. The arcs cascade toward the device while connecting and remain
  visible when connected. Disconnected has no signal and keeps the square red;
  only a failed connection attempt adds a briefly shaking red X. Reduced-motion
  renders connection feedback statically.
  [Connection artwork](connection-artwork.md) records the generated asset
  provenance, SVG coordinates, state table, and refactor checklist.
- Exactly one client may command the device. Additional clients open read-only:
  they can watch state and trigger Stop, but cannot send motion, rather than
  racing and showing a warning banner after the fact.
- Controls that require a backend or a connection are disabled with a visible
  reason (inline text/tooltip), never a silent no-op.

## Accessibility

- Interactive elements are real semantic controls. Decorative artwork stays
  `aria-hidden`; the visualizer's controls are separate, labeled elements, not a
  role bolted onto live-updating text.
- Full keyboard operability, visible focus, logical order. The connection
  manager is a non-modal disclosure: opening focuses its Close button and
  closing restores the trigger; it does not capture Escape, which remains Stop.
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

### Historical post-Phase-9 snapshot — all rows now closed

The following rows were open on 2026-07-01. The closure evidence appears in the
sections below; none remains an active parity regression:

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

### Phase 14 library implementation

The library closes the planned pattern/training/player surface without copying
the reference app's visual density or creating a second motion model. Browse
exposes enabled state and visible weights; Programs keeps finite funscripts
separate from loops; Import owns file intake — funscripts render a client-side
raw-source trim timeline with explicit zoom/pan/fit controls, action-snapped
dual-thumb selection, precise time readouts, and a persistent selected-length
readout. The zoom viewport never changes the trim or payload. A program/loop-
pattern choice is made before the trimmed selection is submitted through the
normal validated import endpoint, while MagicHandy share files import as-is;
Author reduces
freehand input to editable knots; Training
auditions the same enabled catalog with original/smooth/crisp feel choices and
reversible ratings. Every playback preview comes from backend samples produced
by the playback interpolator; the Import plot is explicitly a raw source-action
inspection view, not a playback preview. Playback controls remain controller-
gated, Stop remains outside the route, and program intensity is capped by the
backend settings envelope.

### Model-manager implementation

Settings > Model separates the saved runtime snapshot from the editable form.
Load/Unload is shown only for managed llama.cpp and locks whenever the visible
form differs from saved settings. Managed mode shows backend-authoritative
runtime version/backend/build state and offers an explicit CPU/CUDA/auto source
build; it never renders runner or GGUF path settings. Build and cancellation are
controller-gated, flat controls rather than a setup card. The inventory is also
backend-authoritative: managed GGUF rows expose source, size, quantization, file
state, ID-based selection, and guarded removal. External llama.cpp and Ollama
both show server-reported model rows with the same **Use/Selected** behavior.
Import GGUF and Import from Ollama are inline disclosures;
the latter accepts a persisted library path, scans bounded manifest metadata,
shows incompatibility reasons, filters the list, and reports copy progress and
cancellation. Repeated rows stay flat and compact on desktop/mobile; they are
not nested cards or oversized download tiles.

Generation optimizations are a compact three-field row: maximum output,
thinking/reasoning, and timeout. Maximum output uses backend-advertised reviewed
choices; reasoning exposes only `Disabled when supported` (the recommended
small-model default request) and `Automatic / provider default`. Inline notice text
explains that low caps can still truncate JSON, the current managed automatic
reasoning path is bounded to half the cap, and repair requests reasoning off to
leave more budget for JSON. Unsupported external models may ignore or reject the override. These
controls never claim an unmeasured general speedup or expose unproven
threads/GPU/context/cache knobs.

Device requirements and app-managed voice modules are status/notice surfaces,
not fake form fields. Cloud REST firmware v4/API v3 appears as a semantic note.
Parakeet separates **MagicHandy module** from **Custom local server**: the former
shows backend-inspected installation state and no paths; only the latter exposes
server/model paths. With voice off, text explicitly says Enable, Save, then
Start. A successful Start includes model load, and microphone input remains
disabled until the backend reports the ASR model ready.

The Chat composer is one compact row: microphone split control, flexible text
entry, then Send. The graphite microphone uses the existing mic artwork with
inset physical depth, not a decorative glow. Its primary action starts or stops
a continuous hands-free session; silence ends a phrase for transcription but
does not turn off the microphone. The adjacent moving triangle opens upward to
hold-to-talk, browser input selection, sensitivity, end-of-speech delay, noise
suppression, level, and queue status. Active/ready states use steel azure, never
Stop red or running green. Emergency Stop closes capture and invalidates queued
transcription and speech playback before recognized text can enter Chat; stale
request generations cannot dispatch motion afterward.

NeuTTS reference generation is a focused modal rather than another permanent
settings block. It lets the controller select a source WAV, enter the exact
spoken transcript, generate codes locally without Python, preview the stored
audio, and correct the transcript before applying managed paths. The UI must
show generation as a bounded in-progress action and retain manual pre-encoded
paths under Advanced. Active ASR and TTS work shares one labeled, boxed voice
queue; provider sections show worker state but do not repeat request rows.
NeuTTS sampling stays in that same collapsed Advanced section: a segmented
**Consistent / Varied** choice, fixed-seed number field, and **New seed** command.
Consistent seed 3 is the default. The control reports repeat-cache availability;
Varied is never presented as a quality improvement because it can reintroduce
measured pacing and intelligibility variance.

### Not yet built — planned, not regressions

Still planned rather than regressed: the Autopilot planner and its richer mode
events (including "I'm close"), the Phase 15 importer and Phase 16 setup
wizard,
curated model downloads/hardware-fit recommendations (`docs/model-management.md`),
the bounded speed-test button, raw-model diagnostics, and live device-position layer
(post-parity backlog). Modes, memory/prompts, voice controls, multi-tab
controller enforcement, and the Phase 14 pattern/training/player surface have
landed.

### Second parity sweep (2026-07-09)

A re-read of the legacy working notes and StrokeGPT-ReVibed PRs #319–#333
after Phase 13.4 added a small UI backlog. The Chat speak-replies quick toggle
has since landed; the remaining follow-ups have design-conforming placements
(`docs/legacy-parity-sweep-2026-07.md` §E/§F):

- a scrolling motion-history readout fed by the trace ring — a readout,
  never a control; the library kept this out of the authoring surface, so it
  remains a diagnostics follow-up;
- a per-slider "test this speed for ~3 s" affordance on the speed bounds;
- raw-LLM-output visibility at the highest diagnostics verbosity
  (display-only, never persisted into history).

### Improved — keep these wins

One authoritative visualizer driven by engine state; Stop in a persistent bar
(the old app hid it in a collapsible sidebar); immediate-apply quick controls
with no Save between the user and the device; flat navigation without stacked
modals; strict JSON chat contract with visible repair/malformed states; a
single motion path with no per-source divergence; trace export as one click.

## Deferred And Open

- Real-device validation must confirm that the 6.6 s routine floor and backend
  preview produce the intended physical feel; browser screenshots and fake-
  transport tests cannot establish that.
- Advanced authoring history/transforms and multi-pattern sequencing remain
  deliberately out of Phase 14. Any future editor must preserve backend-owned
  sampling and shared-engine playback.

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
