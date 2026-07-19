# UI Navigation Redesign — Permanent Sidebar Shell

## Status

Proposed 2026-07-06; **implemented 2026-07-08 through Phase 14** with the React
migration (ADR 0009, PRs #37/#38; mobile-footer refinement in #46). Updated
2026-07-19 for PR #101: Autopilot is an assistant session on Chat, not a preset
mode. This is now the as-built shell specification: the nav rail with pinned
Stop, status-only bar, and four routed workspaces all exist. Pattern Library's
Phase 14 workspace is implemented; Chat Autopilot's initial curation loop is in
review, with the remaining autonomy work called out below. This
document specifies the **shell and information architecture**; it does not
restate the safety, accessibility, and parity rules in
[ui-design.md](ui-design.md), which stay in force unchanged. Provider-scoped
Settings compaction is implemented in Slice 13.5; see
[settings-compaction.md](settings-compaction.md).

## Why The Shell Changed

Before the React migration, MagicHandy was a two-surface app: one control view (chat + a single
control sidebar) and a Settings **window** layered over it. That shape was
correct while Settings was the only second surface (product decision
2026-07-03). It stops being correct now that the app is growing into several
distinct workspaces:

- **Chat** — the conversation, assistant Autopilot, and live control.
- **Preset Modes** — deterministic autonomous motion (the renamed "Hands-free").
- **Pattern Library** — browse, import, enable, and author motion content.
- **Settings** — device, model, prompts/memory, diagnostics.

Four peer workspaces do not fit a "one view plus an overlay window" model:
overlaying three of them over the chat would stack surfaces, and hiding chat to
show a full window fights the value that motivated the window in the first
place. The right information architecture for peer workspaces is a **permanent
left navigation sidebar that switches pages** — the shape in the reference
"Stroke Orchestrator" layout. This document adopts it.

Adopting page navigation reverses one earlier decision (Settings as a floating
window) but keeps the value behind it: see "Settings Workspace" for how
mid-session tweaks still never force the user out of chat.

## The Shell: Four Persistent Regions

```text
┌──────────────┬───────────────────────────────────────────────┐
│ profile ▸    │  status bar — phase · connection · controller  │
│ (→ Settings) │  · run timer · compact visualizer   [status]   │
│              ├───────────────────────────────────────────────┤
│ ▸ Chat       │                                                │
│   Preset     │                                                │
│    Modes     │        active workspace (routed)               │
│   Pattern    │        one mounted at a time                   │
│    Library   │                                                │
│   Settings   │                                                │
│              │                                                │
│ ──────────── │                                                │
│ [ STOP  Esc ]│  ← pinned to sidebar bottom, on every page     │
└──────────────┴───────────────────────────────────────────────┘
```

1. **Navigation sidebar** (left, permanent): identity at the top, page links in
   the middle, the emergency Stop pinned at the bottom.
2. **Status bar** (top of the workspace column): status only, never controls.
3. **Workspace** (center): exactly one routed page mounted at a time.
4. **Feedback layer** (toast + backend banner): above content, never occluded.

Because the sidebar is permanent, **Stop is visible on every page without any
overlay logic** — the single strongest reason to prefer this shell. Stop leaves
the status bar entirely (top-bar controls read as awkward — a standing
finding), and it is no longer entangled with an overlay's stacking context (the
bug that forced the sidebar to be non-sticky in the pre-migration build).

## Navigation Sidebar

Fixed width `clamp(212px, 18vw, 236px)` on desktop; full height; its own
scroll only if the link list overflows (Stop stays pinned regardless).

### Identity (top)

A compact profile lockup: a modest 34px `--radius-sm` avatar with the restrained
azure-to-steel brand gradient, `MagicHandy`, and `local / {dispatch owner}` from
backend state. The lockup is a link to **Settings** (`#/settings`). This is the
"click your profile to configure" affordance from the Python app, now a page
rather than a window.

### Page links (middle)

One link per workspace, each an icon + label row:

| Order | Label | Route | Icon intent |
| --- | --- | --- | --- |
| 1 | Chat | `#/chat` (default) | conversation |
| 2 | Preset Modes | `#/modes` | autonomous motion |
| 3 | Pattern Library | `#/library` | saved patterns |
| 4 | Settings | `#/settings/*` | configuration |

Row spec: 40px tall, `0 12px` padding, 18px icon + 0.9rem label, radius
`--radius-sm` for every state. Hover uses neutral `--surface-2`. Active uses the
soft `--accent-tint` fill, `--text` label, and `--accent` icon on the rounded row,
with no edge marker. It is neither a saturated pill nor the clipped inset bar
retired during the React polish pass (see "Anti-Vibecoding Rules").

Links are real `<a href="#/...">` elements so they are keyboard-focusable,
middle-clickable, and deep-linkable. `aria-current="page"` marks the active
one.

### Emergency Stop (bottom, pinned)

Full-width, `--danger`, min-height 44px, radius `--radius-sm`, with the visible
`Esc` hint. Pinned to the sidebar bottom with `margin-top: auto` so it holds
the floor on every page. This satisfies the ui-design.md invariant "Stop is
always visible and always reachable" structurally: it is never inside a routed
workspace, never behind an overlay, never collapsible. Read-only clients still
render and can trigger it (`data-allow-readonly`, `data-allow-backend-offline`).

### Collapse and mobile

- **Medium widths** (`≤ 1040px`): the sidebar may collapse to a 64px
  icon-only rail (labels become tooltips/`aria-label`); Stop stays a red icon
  block, still pinned. Collapse is a layout change, not a content change — no
  safety control disappears.
- **Mobile** (`≤ 720px`): the nav becomes a bottom tab bar (Chat / Modes /
  Library / Settings), and **Stop detaches into its own fixed bottom bar above
  the tabs** so the keyboard never hides it — the same rule as today, carried
  forward. The profile lockup moves into the Settings page header.

## Routing

Extend the existing hash router to top-level workspace routes; keep Settings'
sub-sections as today.

- `#/` and `#/chat` → Chat (default)
- `#/modes` → Preset Modes
- `#/library` → Pattern Library
- `#/settings`, `#/settings/device|model|prompts|diagnostics` → Settings

One workspace is mounted at a time; the others are `hidden` (never occluding
overlays). On navigation, move focus to the workspace's `<h1>` (`tabindex="-1"`)
so keyboard and screen-reader users land in the new page. Routes are
deep-linkable and back/forward-navigable. The router stays the single source of
which page is visible — no parallel `display` toggling from feature modules.

## Status Bar (top, status only)

Spans the workspace column, height ~48px. Carries the state the user glances
at, never a control (Stop and everything else live in the sidebar/workspaces):

- **Phase / activity** — idle, chat-driven, freestyle, autopilot, paused,
  recovering (mirrors the reference's `FASE`).
- **Buffer / queue depth** — optional, small (mirrors `BUFFER`), useful for
  diagnosing starvation.
- **Connection / transport** and **controller lease** — the existing
  status readouts, restyled compact (see anti-vibecoding rules).
- **Run timer** — the stopwatch (accumulates across pauses, freezes on pause,
  resets on stop).
- **Compact visualizer** — the one authoritative engine-state visualizer in its
  small form; the detailed form lives on the Chat page. Same component, one
  source of truth.
- **Backend-loss banner** — the persistent alert, still above everything.

A single optional disclosure (a small "Details" / metrics popover, the
reference's `Métricas`) may expose the diagnostics grid without leaving the
current page. It is a popover from measured geometry, not a `100vw` panel.

## Workspace: Chat (home)

The default page. Two columns on desktop, mirroring the reference `Controle`
layout — the conversation beside a compact control column:

- **Conversation** (left, fills remaining width): a compact **Autopilot**
  session strip, the chat log (grows with the viewport, keeps near-bottom
  scrollback stickiness and the jump-to-latest affordance), and the composer.
  Autopilot's state and Pause/Resume live with the conversation; autonomous
  replies render in this same log rather than in a mode card.
- **Control column** (right, ~300px): the live **quick settings** (speed,
  stroke, reverse, style — immediate-apply), the **manual test motion** group
  (still explicitly badged **"testing"**, still labeled as driving the device
  directly to check the connection), and the **detailed visualizer** with
  commanded-estimate labeling. These are the controls that currently live in
  the single sidebar panel; they move here because the sidebar becomes a nav
  rail.
Autopilot Pause/Resume is in its session strip. Nothing in this page is a
stacked modal.

Rationale for keeping quick settings here rather than in Settings: mid-session
speed/stroke/style/reverse changes must not force the user out of chat — that
was the value behind the old settings-window decision, and it is preserved by
docking the live controls on the Chat page. Deep, rare configuration is what
moves to the Settings page.

## Workspace: Preset Modes (formerly Hands-free)

The deterministic autonomous-motion workspace. Renames "Hands-free" to
"Preset Modes" because the page is about choosing a repeatable motion behavior.
Assistant autonomy stays on Chat, where its conversation and generated lines
are visible. All behaviors remain **clients of the one motion engine** (ADR
0002, Phase 11); none is a second motion pathway.

Contents, top to bottom:

1. **Freestyle** — the deterministic autonomous mode: start/stop plus the style
   selector (gentle / balanced / intense) that biases the seeded scoring. This
   is today's Freestyle, relocated here from the sidebar.
2. **Preset arrangements** — named, bounded segment sets (the Phase 11
   arrangement contract: ≤8 segments, 4–120s each) the user can start with one
   click. Built-in presets ship read-only; user presets are editable later.
3. **Session shaping** — mood/intensity and an optional timed session length,
   and the **"I'm close"** affordance (a chat/UI signal that biases the planner
   down or toward finish, a proven StrokeGPT behavior).

Stop and Pause interrupt any behavior here immediately; quick settings and the
style selector apply live during a running behavior. Every planner decision is
a trace row, so a still device is always diagnosable as planner-wait vs
transport failure.

## Chat Autopilot — Behavior And Safety Contract

Autopilot lets the LLM curate motion continuously from conversation context
without requiring a command every turn. Because it puts the model in the
motion driver's seat, it needs a tighter contract than any other control.
Autopilot is **off by default** and
always visibly indicated when on (status-bar phase = "autopilot" plus an active
state on the button).

**What the initial slice does.** While on, the model receives a bounded tail of
the canonical conversation plus style, speed limits, recent pattern ids, and
its last autonomous line. At each segment boundary it may select an **enabled**
pattern and intensity or keep the current segment. Deterministic code chooses
the bounded dwell time. Focus regions, programs, freeform arrangements,
session arcs, and user-configurable speech cadence remain planned; the initial
slice must not be described as already controlling them.

**How it routes (no separate pathway).** Autopilot emits the same bounded
**arrangement segment loop** as Freestyle (Phase 11 contract) and picks
`{pattern_id, intensity}` from **enabled** library entries (the Phase 14
curation contract). Deterministic code compiles those
into engine `ApplyTarget` retargets. The model **never** triggers low-level
stream replacement per turn and **never** imports `transport`; the existing
depguard import boundary keeps `internal/modes` off transport. If nothing
matches or a model call fails, a deterministic planner segment is the visible
fallback so motion does not stall.

**Bounded by the user's envelope.** Autopilot cannot exceed the live quick
settings: speed and stroke limits clamp its segments, style bounds deterministic
dwell timing, and reverse is applied at the transport boundary. Segment bounds
hold (4–120s); variation comes from changing targets over time within the
envelope, never from rapid oscillation around one target.

**Interruptible and pause-aware.** Stop and Pause take effect immediately.
Pause preserves phase; resume continues the plan. Autopilot's keepalive never
restarts motion the user paused or stopped — the same rule as chat keepalive.
Stop is always the safety path and is never replaced by turning Autopilot off.

**Traceable.** Every Autopilot decision records its source, segment index,
chosen pattern, speed, duration, and fallback error when present. Planner
fallback rows retain the deterministic score table and seed, so autonomy is
auditable and a stall is diagnosable.

**Fails safe.** A malformed or errored model response never drives motion. The
deterministic planner supplies that segment and the Chat strip reports
`Planner fallback`; diagnostics retain the reason. A model-requested Stop is
treated as hold because only the user owns Stop. Model errors never enter the
chat log or TTS.

**Chat and speech delivery.** Successful autonomous lines enter the canonical
chat log before optional TTS. The controller browser discovers new speech ids
from that same log and plays them in order; initial history is never replayed.
If TTS is already active or queued, a new autonomous line remains visible but
does not deepen the speech backlog. Stop cancels the mode context and pending
voice work.

**Consent and clarity.** Turning Autopilot on is an explicit action in Chat
with a clear on-state; turning it off stops the session's motion. The control is
disabled for read-only and backend-offline clients. Temporary device/model
failure is reported through status and trace rather than hidden.

## Workspace: Pattern Library

The motion-content workspace (Phase 14). It does not invent a second playback
path — everything plays through the shared motion engine.

- **Browse and toggle** built-in and user patterns; enable/disable is the
  primary control because enabled patterns are the model's curation vocabulary
  (Autopilot and chat pick only from enabled entries; disabled are never
  selectable — tested).
- **Programs / funscripts**: finite imported content and a **program player**
  (the reference's "Reprodutor" folds in here — programs are library content,
  not a separate top-level route).
- **Import**: bounded file intake with a zoomable raw-source timeline,
  action-snapped trim, visible selection length, and explicit program/pattern
  interpretation. Only pattern import strips qualifying long inactive gaps;
  programs preserve selected elapsed timing.
- **Authoring canvas**: freehand draw with simplification/interpolation and a
  backend-sampler preview (never a client-side guess).
- **Feedback**: thumbs adjust weight/enablement **only visibly and reversibly**;
  auto-disable is opt-in.

As built, Browse, Programs, Import, Author, and Training are compact sibling
tabs. Playback-preview curves are backend sampled rather than a second frontend
interpolator; Import's client-rendered source-action plot is inspection only.
Desktop and 390 px mobile checks cover the original four Phase 14 tabs,
including the pinned Stop/footer boundary and scroll reachability; Import has
focused component coverage for trim, zoom, validation, and payload invariants.

## Workspace: Settings

The former settings **window** becomes the Settings **page**, reached from the
profile lockup or any `#/settings/*` route. Its internals are unchanged: the
Device / Model / Prompts & Memory / Diagnostics sections, immediate-apply live
device settings, explicit and confirmed destructive actions (reset, clear
memory), the protected built-in prompt sets, and the redacted connection key
that is never echoed back.

What changes is only that it is a page, not an overlay. The safety substance
that the overlay guaranteed is preserved elsewhere:

- **Stop stays reachable** — it is in the permanent sidebar, so being on the
  Settings page never hides it (the overlay previously had to render Stop above
  its own backdrop; now there is no backdrop).
- **Chat is not "hidden" for a quick change** — the common mid-session tweaks
  are the quick settings on the Chat page; Settings is for deliberate, rarer
  configuration where navigating away from chat is expected and fine.
- **One layer, no stacked modals** — a page cannot stack on a page; sub-sections
  are sibling sections within the one Settings page, as before.

## Anti-Vibecoding Rules

The reference layout is good, but AI-generated UIs drift toward a recognizable
set of quirks. These rules are binding for this redesign; the first is the one
the user called out explicitly.

1. **No oversized round bubbles around status or controls.** Status is a compact
   readout — an 8–9px state **dot + text** — not a fully-rounded filled pill.
   Concretely: the retired legacy `.status-pill { border-radius: 999px }` filled
   chips with glowing dots (`box-shadow: 0 0 6px …`) are the anti-pattern. The
   React shell uses compact readouts with no fill or glow, grouped with spacing
   and dividers. Fully-round (`999px`) is reserved for circular micro-elements,
   never for status chips, buttons, or nav rows.
2. **Cap the radius scale.** Controls and cards use `--radius-sm` (8px) to
   `--radius` (12px). Nothing control-sized is a pill. Avatars are modest
   squircles, not orbs.
3. **Restrained elevation.** Use the existing surface ladder
   (`--bg` < `--surface` < `--surface-2` < `--surface-3` < overlay) with the two
   defined shadows to signal depth. Do not put a drop shadow or glow on every
   element; no neon shadows on dots, chips, or the brand mark. Depth reads from
   the value ladder first, shadow second.
4. **Real icons, not emoji.** One consistent monochrome inline-SVG icon set
   drawn with `currentColor`. Replace ad-hoc glyphs (the `⏱` timer glyph, the
   gradient brand blob) with set members. Decorative marks stay `aria-hidden`;
   icons never become the only label for a control.
5. **Operational density, not a consumer hero.** Left-aligned, information-dense,
   a clear type scale with real hierarchy. No centered hero blocks, no giant
   empty padding, no oversized headings. This is an instrument.
6. **Hue discipline (unchanged from app.css).** `--accent` steel azure is the
   only interactive hue; `--ok` green means running/go only; `--warn` amber is
   warning; `--danger` red is stop. Everything else is neutral graphite.
   **No purple, no blue-green decorative tones.** The nav active state is a soft
   `--accent-tint` fill on the rounded row, not an edge marker or saturated fill.
7. **Motion is subtle and reduced-motion-aware.** Page transitions and the
   streaming cursor are quiet; `prefers-reduced-motion` disables non-essential
   animation. No bouncing, no pulsing glows.

## Safety Invariants Preserved

Every ui-design.md safety property maps to a home in the new shell:

| Invariant | Home in the redesign |
| --- | --- |
| Stop always visible/reachable | Pinned bottom of the permanent sidebar; mobile fixed bar |
| One authoritative visualizer | Compact in status bar, detailed on Chat page — one component |
| Immediate-apply quick controls | Speed/stroke in the connection manager; behavior on Chat; no Save step |
| One feedback channel, never occluded | Toast + backend banner above all pages |
| Single active controller | Unchanged; extra clients read-only with Stop |
| Backend-loss lock | Banner at workspace top below status bar; required controls lock |
| Motion through engine only | Chat Autopilot / Preset Modes / Library are engine clients |
| Viewport-safe sizing | The shell may own `100vh`; no overflow-prone page-wide `100vw`; popovers stay bounded |
| Flat navigation, no stacked modals | Router mounts one workspace; no overlay windows |

## Historical Migration Sequence

Ship incrementally; never lose a safety control mid-migration. The React
implementation handoff is `docs/react-ui-implementation-handoff.md`. Each step
is verified against the live rendered DOM (headless run + inspection), runs the
asset/UI tests, and re-checks the Functional Parity Baseline rows it touches.

- **Step 0 — React scaffold (no behavior loss).** Add the static React build,
  embed its output from Go, preserve existing visible safety behavior, and add
  frontend build/test CI.
- **Step 1 — Shell refactor (no new backend).** Introduce the permanent nav
  sidebar and top-level router; move Stop to the pinned sidebar bottom; move the
  current control panel (quick settings, manual test, visualizer detail,
  pause/resume, keepalive) into the **Chat** workspace; convert the settings
  window into the **Settings** workspace; keep the status bar status-only. This
  is a pure front-end reorganization of surfaces that already exist.
- **Step 2 — Preset Modes + Chat Autopilot.** Add the **Preset Modes** workspace
  and relocate deterministic Freestyle/style there. Add the **Autopilot**
  session strip to Chat, backed by the shared autonomous lifecycle in
  `internal/modes` and semantic targets through the engine. Bounded
  conversation context, trace rows, browser-playable chat/TTS ordering, and
  Stop/Pause interruption are part of the definition of done.
- **Step 3 — Pattern Library.** Build the **Pattern Library** workspace with
  Phase 14 (browse/enable, import, player, authoring, curation, feedback).
  **Implemented on the Phase 14 review branch.**

## Test Hooks

The pre-React `web/assets_test.go` asserted settings-window IDs and dialog
semantics. The migration deliberately replaced those checks. Current
`web/assets_test.go` verifies the generated Vite root and safety-critical strings
in the embedded bundle, including Stop, routes, backend-authoritative labels,
voice controls, and the Phase 14 library; it also rejects the retired
`status-pill` class. `web/src/App.test.tsx` owns behavioral coverage of the four
routes, pinned Stop, status-only top bar, routed Settings, connection manager,
Chat controls, and library workspace.

## Relationship To Other Docs

- [ui-design.md](ui-design.md): the enduring UI principles, safety rules,
  accessibility, visual language, and the Functional Parity Baseline. Its
  "Layout" and "Settings" sections are annotated to defer to this document for
  the shell shape.
- [ui-design-guidelines.md](ui-design-guidelines.md): the token, component, and
  sketch-level visual contract for implementing this shell without introducing
  generic oversized status bubbles or new decorative hues.
- ADR 0002 (motion/transport contract), ADR 0004 (frontend strategy), and
  ADR 0009 (React frontend migration): this redesign obeys all three
  (engine-only motion, backend-authoritative state, static embedded React build).
- [react-ui-implementation-handoff.md](react-ui-implementation-handoff.md): the
  Claude-oriented implementation sequence and acceptance checklist.
- [IMPLEMENTATION_PLAN.md](../IMPLEMENTATION_PLAN.md): the shell refactor is a
  UI phase; Autopilot rides the Phase 11 mode architecture; Pattern Library is
  Phase 14.
- [goal-scorecard.md](goal-scorecard.md): re-score the UI/parity rows when each
  migration step lands.
