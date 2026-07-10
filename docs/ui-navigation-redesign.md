# UI Navigation Redesign — Permanent Sidebar Shell

## Status

Proposed 2026-07-06; **implemented 2026-07-08** with the React migration
(ADR 0009, PRs #37/#38; mobile-footer refinement in #46). This is now the
as-built shell specification: the nav rail with pinned Stop, the status-only
bar, and the four routed workspaces all exist as described, with two labeled
exceptions — Autopilot renders as coming-soon until its planner ships (shell
step 3), and Pattern Library is a labeled empty state until Phase 14. This
document specifies the **shell and information architecture**; it does not
restate the safety, accessibility, and parity rules in
[ui-design.md](ui-design.md), which stay in force unchanged. Settings-page
internals are being revised separately in
[settings-compaction.md](settings-compaction.md) (Slice 13.5).

## Why Change The Shell

Today MagicHandy is a two-surface app: one control view (chat + a single
control sidebar) and a Settings **window** layered over it. That shape was
correct while Settings was the only second surface (product decision
2026-07-03). It stops being correct now that the app is growing into several
distinct workspaces:

- **Chat** — the conversation plus live control.
- **Preset Modes** — autonomous motion (the renamed "Hands-free").
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
bug that forced the sidebar to be non-sticky in the current build).

## Navigation Sidebar

Fixed width `clamp(212px, 18vw, 236px)` on desktop; full height; its own
scroll only if the link list overflows (Stop stays pinned regardless).

### Identity (top)

A compact profile lockup: a modest avatar (34–36px, `--radius-sm` squircle —
not a glowing gradient orb), the app/session identity beneath it
(`local · llama.cpp · cloud`, driven by real provider/transport state), and the
active persona/prompt-set name. The lockup is a button: activating it navigates
to **Settings** (`#/settings`). This is the "click your profile to configure"
affordance from the Python app, now a page rather than a window.

### Page links (middle)

One link per workspace, each an icon + label row:

| Order | Label | Route | Icon intent |
| --- | --- | --- | --- |
| 1 | Chat | `#/chat` (default) | conversation |
| 2 | Preset Modes | `#/modes` | autonomous motion |
| 3 | Pattern Library | `#/library` | saved patterns |
| 4 | Settings | `#/settings/*` | configuration |

Row spec: 40px tall, `0 12px` padding, 18px icon + 0.9rem label, radius
`--radius-sm` for default and hover states. Hover uses `--surface-2`. **Active
state is an edge marker, not a fill blob**: a 3px `--accent` (steel azure) left
border plus a one-step surface raise (`--surface-2`), `--text` label,
`--accent` icon, and square row corners (`border-radius: 0`) so the single-side
edge marker reads cleanly. Do not paint the active row as a saturated pill —
that is the exact "oversized bubble" quirk this redesign avoids (see
"Anti-Vibecoding Rules").

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
layout — a control column beside the conversation:

- **Control column** (left, ~300px): the live **quick settings** (speed,
  stroke, reverse, style — immediate-apply), the **manual test motion** group
  (still explicitly badged **"testing"**, still labeled as driving the device
  directly to check the connection), and the **detailed visualizer** with
  commanded-estimate labeling. These are the controls that currently live in
  the single sidebar panel; they move here because the sidebar becomes a nav
  rail.
- **Conversation** (right, fills remaining width): the chat log (grows with the
  viewport, keeps near-bottom scrollback stickiness and the jump-to-latest
  affordance) and the composer.

Pause/Resume and the chat-keepalive toggle live in the control column too,
adjacent to the run readout. Nothing in this page is a stacked modal.

Rationale for keeping quick settings here rather than in Settings: mid-session
speed/stroke/style/reverse changes must not force the user out of chat — that
was the value behind the old settings-window decision, and it is preserved by
docking the live controls on the Chat page. Deep, rare configuration is what
moves to the Settings page.

## Workspace: Preset Modes (formerly Hands-free)

The autonomous-motion workspace. Renames "Hands-free" to "Preset Modes" because
the page is about *choosing a motion behavior*, of which hands-free autopilot
is one. All behaviors here are **clients of the one motion engine** (ADR 0002,
Phase 11); none is a second motion pathway.

Contents, top to bottom:

1. **Autopilot** — the headline control (full contract in the next section).
   A single prominent on/off control that hands motion direction to the LLM.
2. **Freestyle** — the deterministic autonomous mode: start/stop plus the style
   selector (gentle / balanced / intense) that biases the seeded scoring. This
   is today's Freestyle, relocated here from the sidebar.
3. **Preset arrangements** — named, bounded segment sets (the Phase 11
   arrangement contract: ≤8 segments, 4–120s each) the user can start with one
   click. Built-in presets ship read-only; user presets are editable later.
4. **Session shaping** — mood/intensity and an optional timed session length,
   and the **"I'm close"** affordance (a chat/UI signal that biases the planner
   down or toward finish, a proven StrokeGPT behavior).

Stop and Pause interrupt any behavior here immediately; quick settings and the
style selector apply live during a running behavior. Every planner decision is
a trace row, so a still device is always diagnosable as planner-wait vs
transport failure.

## Autopilot — Behavior And Safety Contract

Autopilot lets the LLM "take over and change direction as it sees fit based on
context." Because it puts the model in the motion driver's seat, it needs a
tighter contract than any other control. Autopilot is **off by default** and
always visibly indicated when on (status-bar phase = "autopilot" plus an active
state on the button).

**What it does.** While on, the model continuously proposes motion — pattern,
intensity, focus region, direction, and how long to hold each — from the
conversation and session context, and may change its mind as context evolves.
The user does not have to issue per-turn commands.

**How it routes (no separate pathway).** Autopilot emits the same bounded
**arrangement segments** as Freestyle (Phase 11 contract), or — once the
library exists — picks `{pattern_id, intensity}` from **enabled** library
entries (the Phase 14 curation contract). Deterministic code compiles those
into engine `ApplyTarget` retargets. The model **never** triggers low-level
stream replacement per turn and **never** imports `transport`; the existing
depguard import boundary keeps `internal/modes` off transport. If nothing
matches, the deterministic semantic-target path is the fallback so the model is
never silenced.

**Bounded by the user's envelope.** Autopilot cannot exceed the live quick
settings: speed and stroke limits clamp its segments, style biases its scoring,
reverse is applied at the transport boundary. Segment bounds hold (4–120s, ≤8
segments); variation comes from changing targets over time within the envelope,
never from rapid oscillation around one target.

**Interruptible and pause-aware.** Stop and Pause take effect immediately.
Pause preserves phase; resume continues the plan. Autopilot's keepalive never
restarts motion the user paused or stopped — the same rule as chat keepalive.
Stop is always the safety path and is never replaced by turning Autopilot off.

**Traceable.** Every autopilot decision records a planner trace row (seed,
inputs, chosen segment/pattern, score table, cadence) exactly like Freestyle,
so autonomy is auditable and a stall is diagnosable.

**Fails safe.** A malformed or errored model response never drives motion:
autopilot holds its last safe segment and surfaces the malformed-response
indicator, and if the model stays unavailable it winds down rather than acting
on garbage. Model errors never enter motion, history, or (later) TTS.

**Consent and clarity.** Turning Autopilot on is an explicit, single action
with a clear on-state; turning it off returns motion control to the user (chat
or manual). It requires a connected controller and is disabled read-only with a
visible reason.

## Workspace: Pattern Library

The motion-content workspace (Phase 14). It does not invent a second playback
path — everything plays through the shared motion engine.

- **Browse and toggle** built-in and user patterns; enable/disable is the
  primary control because enabled patterns are the model's curation vocabulary
  (Autopilot and chat pick only from enabled entries; disabled are never
  selectable — tested).
- **Programs / funscripts**: import and a **program player** (the reference's
  "Reprodutor" folds in here — programs are library content, not a separate
  top-level tab). Import hygiene strips long inactive gaps.
- **Authoring canvas**: freehand draw with simplification/interpolation and a
  backend-sampler preview (never a client-side guess).
- **Feedback**: thumbs adjust weight/enablement **only visibly and reversibly**;
  auto-disable is opt-in.

Until Phase 14 lands, the sidebar link may show the page in a clearly-labeled
"coming in Phase 14" empty state rather than a dead link, so the shell can ship
first.

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
   Concretely: the current `.status-pill { border-radius: 999px }` filled chips
   with glowing dots (`box-shadow: 0 0 6px …`) are the anti-pattern; replace
   them with compact chips at `--radius-sm`, a hairline (`--line`) or no fill,
   grouped with spacing/dividers, and drop the dot glow. Fully-round (`999px`)
   is reserved for the state dot itself and toggle thumbs — never for status
   chips, buttons, or nav rows.
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
   **No purple, no blue-green decorative tones.** The nav active state is an
   edge marker + surface raise, not a saturated fill.
7. **Motion is subtle and reduced-motion-aware.** Page transitions and the
   streaming cursor are quiet; `prefers-reduced-motion` disables non-essential
   animation. No bouncing, no pulsing glows.

## Safety Invariants Preserved

Every ui-design.md safety property maps to a home in the new shell:

| Invariant | Home in the redesign |
| --- | --- |
| Stop always visible/reachable | Pinned bottom of the permanent sidebar; mobile fixed bar |
| One authoritative visualizer | Compact in status bar, detailed on Chat page — one component |
| Immediate-apply quick controls | Chat page control column, no Save step |
| One feedback channel, never occluded | Toast + backend banner above all pages |
| Single active controller | Unchanged; extra clients read-only with Stop |
| Backend-loss lock | Banner in status bar; `data-requires-backend` still locks |
| Motion through engine only | Preset Modes / Autopilot / Library are engine clients |
| No `100vw`/`100vh` sizing | Popovers positioned from measured geometry |
| Flat navigation, no stacked modals | Router mounts one workspace; no overlay windows |

## Migration

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
- **Step 2 — Preset Modes + Autopilot.** Add the **Preset Modes** workspace;
  relocate Freestyle/style there; add the **Autopilot** control wired to a new
  autopilot mode in `internal/modes` (LLM-driven arrangement segments through
  the engine, per the contract above). Trace rows and Stop/Pause interruption
  are part of the definition of done.
- **Step 3 — Pattern Library.** Build the **Pattern Library** workspace with
  Phase 14 (browse/enable, import, player, authoring, curation, feedback). Until
  then the link shows a labeled empty state.

## Test Hooks That Change

The current `web/assets_test.go` asserts the settings-**window** shape
(`#settings-overlay`, `#settings-window`, `#settings-close`, profile button
`aria-haspopup="dialog"`, and no Stop inside `<header>`). The shell refactor
changes these deliberately (not silently):

- The status bar stays status-only; the `<header>` still must not contain Stop,
  so the existing `barSection` containment check holds — Stop simply moves from
  the sidebar panel to the sidebar's pinned footer.
- `#settings-overlay`/`#settings-window`/dialog semantics are replaced by a
  routed `#/settings` **workspace** with `aria-current` navigation; the profile
  button becomes a link to `#/settings` rather than a dialog opener. The tests
  assert the new nav/router hooks instead.
- New assertions cover: the four nav links and their routes, the pinned Stop in
  the sidebar footer (not in `<header>`, present in every workspace), the Chat
  control column, the Preset Modes/Autopilot hooks, and the Library empty-state
  or content.

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
