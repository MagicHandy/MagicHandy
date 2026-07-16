# LLM Motion Control Surface — Current State and Ideas

This document catalogs how the local LLM can drive the device today, what the
motion engine can already do that the model cannot yet reach, and ideas for
widening LLM control in two families: **capability exposures** (A–G — widen
*what* the model expresses in one turn, mostly by reaching existing engine
fields) and **control paradigms** (numbered — different shapes for how the model
drives the device across a session, each with a phased plan). It is a design
sketch for discussion, not a committed plan; nothing here is scheduled until it
becomes a scoped slice.

It is grounded in two things: MagicHandy's current code
(`internal/chat/contract.go`, `internal/motion/target.go`) and the reference
app's own hard-won motion-control notes
(`StrokeGPT-ReVibed/docs/motion_control_modes.md` and `ROADMAP.md` items #3,
#12, #15, #16). Where the reference app already reasoned about a direction, we
cite it rather than re-derive it.

## The one invariant every idea must keep

Every LLM control path — present or proposed — routes through the shared
sampler/sanitizer and the transport boundary, and the model never sees
transport details. This is [ADR 0006](decisions/0006-drop-legacy-motion.md)
(one motion backend, one neutral frame) and
[ADR 0002](decisions/0002-motion-transport-contract.md) (semantic intent vs
physical transport), and it matches the reference app's explicit guardrail:

> Avoid designing new LLM modes that directly expose transport details like HSP
> replacement, HDSP position frames, morph duration, or phase offsets. Those
> should remain backend implementation details with tests and trace fields.
> — `StrokeGPT-ReVibed/docs/motion_control_modes.md`

So the model emits **semantic intent** (a region, a pattern id, an intensity, a
named arrangement). Deterministic code compiles that into motion and clamps it
to the user's speed/stroke/limit envelope. Speed and stroke limits stay
transport-layer caps, never prompt-only behavior. Emergency Stop stays
independent of every generation, upload, and playback latency.

## What the LLM can emit today

`internal/chat/contract.go` — `MotionCommand`, the only motion shape the parser
accepts:

| Field | Values | Meaning |
| --- | --- | --- |
| `action` | `none` / `start` / `target` / `stop` | start, retarget, or stop motion through the engine |
| `pattern_id` | an **enabled** library id | curate one enabled pattern (rejected if disabled/unknown) |
| `intensity` | 1–100 | playback intensity for the chosen pattern (maps to speed within limits) |
| `speed_percent` | 1–100 | plain speed with no pattern |

Validation already enforces the safe combinations: intensity requires a
pattern, curated patterns require an intensity, intensity and speed are mutually
exclusive, and `none`/`stop` carry no target fields. Disabled or unknown
pattern ids are rejected, and an all-disabled library keeps the deterministic
fallback. This is real curation — the model selects from author-owned content —
but it is a narrow slice of what the engine can do.

## What the engine already supports that the model cannot reach

`internal/motion/target.go` — `MotionTarget`, the app-level semantic intent the
engine actually consumes — is richer than the chat contract that feeds it:

| Engine field | Capability | Reachable from chat today? |
| --- | --- | --- |
| `PatternID` | repeatable pattern | **yes** |
| `SpeedPercent` | speed within limits | **yes** (as intensity/speed) |
| `AreaFocus{MinPercent,MaxPercent}` | constrain sampling to a **stroke region** | **no** |
| `SoftAnchor{PositionPercent,WeightPercent}` | gently bias motion toward a point | **no** |
| `ProgramID` | play a finite **program/funscript** | **no** |

This is the single most useful fact for planning LLM-control work: **stroke
regions, soft anchors, and program playback already exist in the engine and its
tests.** The near-term ideas below are mostly about *safely exposing existing
engine capability to the model through a versioned contract*, not about building
new motion. That keeps them small and low-risk relative to their apparent scope.

## Ideas, ranked by leverage-to-risk

Each idea notes whether it **restores** reference-app parity or is **net-new**,
its main dependency, and an honest disposition. Ordering is a suggestion, not a
commitment.

### A. Stroke-region focus (parity; low risk)

Let the model request a focus region — either a named zone (`tip` / `shaft` /
`base`) or an explicit `depth_min`/`depth_max` — that compiles to the engine's
existing `AreaFocus`. The reference app called this area-focus and localized
`tip`/`shaft`/`base` into bounded local stroke windows before sending them
(`motion_control_modes.md`). This directly answers "LLM selection of stroke
regions."

- Dependency: a small contract addition plus a named-zone→percent table;
  `AreaFocus` and its clamping already exist.
- Disposition: strong candidate. Carry across the reference app's hard lesson —
  **active** focus changes should take the smoother live-stroke path, not a
  flushed morph replacement — so region changes mid-session do not reintroduce
  the "stop/go morph" reports. Named zones are safer to expose than raw numbers
  because they localize to bounded windows.

### B. Program / script selection (parity; low–moderate risk)

Extend curation so the model can pick an enabled **program** (funscript), not
only a pattern — the reference app's item #16 `{script_id, intensity}` shape.
The engine already accepts `ProgramID`; the library already separates finite
programs from loops ([pattern-library.md](pattern-library.md)).

- Dependency: expose enabled program ids to the model as data (same
  enabled-only, curation-gated rule as patterns) and add a `program_id` branch
  to the contract with intensity mapped to playback speed within limits.
- Disposition: good, but respect the reference app's own caution — with a small
  catalog "the LLM keeps picking the same two scripts" becomes the failure
  mode. Worth doing; pair with a note that catalog size gates its value.

### C. Bounded relative deltas (net-new polish; low risk)

Map "a little faster", "deeper", "shorter strokes" to bounded *relative*
adjustments of the current target, clamped **once** at the transport boundary.
The reference sweeps flag a real bug here: clamping speed at multiple layers
compounded. Keep it single-clamp and visible.

- Disposition: cheap ergonomics win; mostly a prompt/normalization concern. Low
  risk if clamp-once is enforced by a test.

### D. Style / mood bias as visible state (parity; low risk)

The model already biases Freestyle indirectly; make the **Motion Style**
(gentle/balanced/intense) a visible, model-readable, model-settable field rather
than hidden prompt drift, surfaced in diagnostics. Reference ROADMAP #4 reached
the same conclusion ("steer model behavior without hidden prompt drift").

- Disposition: aligns with the project's "provider choice is visible state"
  stance; low risk because deterministic scoring already consumes style.

### E. LLM-requested motion arrangement (net-new; moderate risk)

Let the model *request* a bounded arrangement — named styles, focus regions,
durations, repetitions, intensity drift — that compiles through the existing
Phase 11 arrangement contract, instead of nudging low-level targets every turn.
This is precisely the reference app's "Preferred direction" in
`motion_control_modes.md` and its item #16 arrangement note. MagicHandy already
has the compiler (Freestyle uses it); the new part is a safe *request* schema.

- Dependency: a versioned `arrangement` request shape plus validation that it
  stays inside the 1–8 segment / 4–120 s bounds the planner already enforces.
- Disposition: high-value and architecturally aligned, but it is the first idea
  that grows the schema meaningfully — version it, keep each segment long enough
  to establish feel (multiple cycles), and surface the active arrangement in
  diagnostics so users can see what the model changed.

### F. Mode selection + session freestyle/curation toggle (net-new; moderate)

Let the model enter a mode (Freestyle / a future Autopilot) and honor a
**session-level toggle** between "curate authored content only" (`{pattern_id or
program_id, intensity}`) and "freeform arrangement" — the reference app's item
#16 reframe of anchor-loop output as a session Freestyle switch. When curation
is on, the model may only select authored content; when freeform is on, it may
request arrangements.

- Dependency: the mode manager exists; needs a visible toggle and a
  guarded `mode_action` field (the reference app gates this behind explicit
  settings, especially for voice transcripts — ROADMAP #3).
- Disposition: this is the concrete shape of the plan's unstarted **Autopilot**
  slice. Keep model-triggered mode changes behind an explicit user opt-in.

### G. Soft-anchor waypoints (parity; moderate)

Expose the engine's `SoftAnchor` so the model (or a saved preset) can bias
motion toward 2–6 waypoints (tip/upper/middle/lower/base) that motion slides
through without hard stops. Reference ROADMAP #7 designed exactly this as an
inspectable control surface.

- Disposition: attractive but do it after A/B; the reference app deliberately
  put visible *authoring* of soft anchors ahead of letting the model invent
  them. Prefer "model selects a saved soft-anchor preset" over "model emits raw
  waypoints."

## Control-paradigm ideas — each with a plan

The A–G ideas above widen *what* the model can express in a single turn. The
ideas here are different *control paradigms* — different shapes for how the model
drives the device over a whole session. They are not in the StrokeGPT-ReVibed
docs (except where noted) and are offered for discussion, each with a sketched
plan and honest risks. A paradigm usually composes the A–G capabilities rather
than replacing them; none changes the invariant (semantic intent only,
deterministic compile, transport-layer caps, Stop independent of every
generation).

Each plan is written as review-sized slices so no single PR has to land the
whole paradigm.

### 1. Pattern/segment queue — the model builds a live playlist

**What it is.** Instead of one target per turn, the model appends items — a
pattern, a program, or a focused region with an intensity — to a visible
**queue** that plays in order. The model can enqueue several ("warm up on a slow
stroke, then tease the tip, then build") and the device works through them; the
user sees the queue and can reorder, remove, skip, or clear it. This differs
from the arrangement idea (E): an arrangement is one bounded plan compiled at
once, while the queue is open-ended and editable turn by turn.

**Plan.**

- *Slice 1 — backend queue.* A bounded FIFO of semantic `MotionTarget`s owned
  next to the engine; it advances to the next item at a clean segment/cycle
  boundary (never mid-stroke), reusing the same retarget path so there is no new
  motion math. Caps: max depth (e.g. 8), per-item min/max duration, and total
  wall-clock ceiling. **Stop clears the queue**; Pause freezes it.
- *Slice 2 — visible queue strip + user control.* A compact strip in the chat
  control column showing upcoming items with reorder/remove/skip/clear, all
  controller-gated. The current item is highlighted; completed items drop off.
- *Slice 3 — LLM `queue_op`.* A versioned contract op
  `{op: append|insert|remove|clear, item?}` where `item` reuses the A/B/G field
  shapes (pattern/program/region/anchor + intensity). Enqueue-only for the model
  by default; destructive ops (clear, remove) can be model-allowed only in the
  most autonomous control style (see idea 2).

**Disposition / risks.** Strong and genuinely different; it makes "sequences of
patterns" a first-class, inspectable object instead of hidden planner state.
Main risk is the model over-filling the queue or thrashing it every turn —
mitigate with the depth cap, a minimum dwell per item (carry across the
reference app's "keep each segment long enough to establish feel"), and by
defaulting the model to append-only. Depends on nothing new in the engine.

### 2. Control-style selector at the top of the chat

**What it is.** A small segmented control in the chat header that sets **how much
authority the model has over motion** for the session — for example **Manual**
(the model talks; only the user drives motion), **Assist** (the model may adjust
speed/region/pattern within limits), and **Director** (the model may compose
queues/arrangements and switch modes). It is the visible, multi-position form of
idea F's session toggle, placed where the user is already looking.

**Plan.**

- *Slice 1 — the control + session setting.* A persisted per-session
  `control_style` enum with a segmented control in the chat header
  (`WorkspaceHead`), defaulting to the least-autonomous style. Backend-
  authoritative like every other setting; changing it is a visible action.
- *Slice 2 — the parser gate.* `control_style` decides which motion fields the
  response parser accepts: Manual ignores motion entirely (reply-only), Assist
  accepts single-target/region/speed changes, Director additionally accepts
  `queue_op`/`arrangement`/`mode_action`. Rejected fields degrade to "reply
  only," never to an error that blocks chat.
- *Slice 3 — status + trace.* The active style shows in the status bar and every
  motion trace records the style in effect, so "why did it do that?" always has
  an answer.

**Disposition / risks.** High-value UX: it makes autonomy an explicit, one-glance
choice rather than something inferred from prompt wording, and it gives every
other idea here a natural gate. Low motion risk because it only *restricts* the
contract. Keep the default conservative and keep Stop/limits identical across all
styles. This is the recommended home for the idea-F toggle.

### 3. Continuous parameter vector — the model turns dials, not picks presets

**What it is.** Expose a small set of continuous "dials" the model sets or nudges
as one vector — e.g. `speed`, `depth_center`, `range_width`, and a `sharpness`
that biases between smooth and crisp motion — instead of choosing a discrete
pattern. The model steers a point in parameter space ("shift a bit deeper,
narrow the range, keep the speed"); deterministic code maps that vector onto the
engine's existing fields (`depth_center`/`range_width` → `AreaFocus`; `speed` →
`SpeedPercent`; `sharpness` → pattern/soft-anchor choice) and clamps each once.

**Plan.**

- *Slice 1 — vector→target mapping.* A pure function from a validated `params`
  object to a `MotionTarget`, with per-field clamping to the user envelope and a
  single clamp point at the transport boundary (honor the reference sweeps'
  clamp-once lesson). Absolute set and bounded relative nudge both supported.
- *Slice 2 — optional live dials UI.* Read-only-by-default sliders that mirror
  the model's current vector so the user can see the operating point and, if
  they choose, take a dial over (which drops that field to Manual for the turn).
- *Slice 3 — contract + prompt.* A `params` branch in the motion contract and a
  short prompt describing the axes in plain language.

**Disposition / risks.** A clean, expressive paradigm that fits how people
actually ask for changes ("deeper, slower, tighter"), and it is mostly a mapping
layer over existing fields. Risk: too many axes invite fiddly, incoherent motion
— keep the vector to ~4 axes, and prefer named zones over raw depth numbers for
the region axis (idea A). Overlaps idea C (relative deltas); treat deltas as the
one-axis special case of this vector.

### 4. Session goal / director — outcome-directed motion

**What it is.** The user (or the model, on request) sets a high-level **goal** —
a duration, an intensity **curve** (steady / build / waves), and an end behavior
(ease off, hold, or stop) — and a deterministic director shapes a sequence of
arrangements over time to follow that curve, reporting progress. Control is by
*desired outcome over time* rather than by per-turn command. This is motion-only
and bounded; it is the motion spine of the deferred Story Mode without the voiced
scenes.

**Plan.**

- *Slice 1 — goal contract + director.* A `session_goal
  {duration_s, curve, end_behavior}` compiled by a deterministic director into
  timed arrangement segments inside the existing 1–8 segment / 4–120 s bounds.
  The curve only shapes intensity *within the user's limits* — it never raises a
  cap.
- *Slice 2 — visible progress + control.* A progress/time-remaining readout with
  the current curve, plus one-tap "hold here," "wind down now," and "stop." The
  user can adjust or cancel at any moment.
- *Slice 3 — model access, gated.* In Director control style only, the model may
  propose a goal from chat ("edge for a few minutes then ease off"), shown as a
  suggest-then-confirm card before it starts.

**Disposition / risks.** Distinct and appealing, but the highest-design idea
here: an intensity curve is exactly the kind of "hidden escalation" that must be
made explicit and bounded. Only pursue with the curve fully visible, the
duration capped, instant user override, and no coupling to conversation
sentiment (see the non-goal below). Depends on ideas E and 2.

### 5. Explicit feedback-loop controls — a visible closed loop

**What it is.** Small, always-available controls next to the chat — **more**,
**less**, **hold**, **switch it up**, **back off** — that (a) act *deterministically
and immediately* on the current motion within limits, and (b) are recorded as
structured signals the model reads to propose its next change. It is the visible,
explicit counterpart to the tempting-but-hidden "sentiment-paced drift" non-goal:
same closed loop, but driven by buttons the user pressed, not by the model's
guess about mood.

**Plan.**

- *Slice 1 — deterministic buttons.* The controls adjust the live target
  (speed/region/variation) through the normal retarget path, clamped to limits,
  and work even if the model is slow or Manual — they are not model-dependent.
- *Slice 2 — structured signals to the model.* Each press appends a compact,
  visible signal to the chat log (`{signal: more|less|hold|switch|back_off,
  at_target}`) that the model may use to shape its next reply/queue in Assist or
  Director styles.
- *Slice 3 — feedback into preferences (optional).* Repeated signals can nudge
  the existing visible/reversible pattern-preference weights, never silently
  (reuse the Phase 14 training feedback contract and its undo).

**Disposition / risks.** Genuinely useful and safe because the buttons stand on
their own; the model layer is additive. Keep the deterministic path primary so a
stalled or absent model never makes the controls feel dead. Ties into idea D
(visible style) and the Phase 14 feedback system.

## Cross-cutting requirements and non-goals

These apply to every idea above, old and new.

- **Suggest-then-confirm for big changes.** A new arrangement, queue rewrite,
  goal, or mode switch should surface a one-line summary (reusing the library's
  backend preview sampler) that the user accepts before it drives the device —
  an approval step, not a stop.
- **Named user presets as model choices.** Let users save an
  arrangement/region/anchor/vector as a named preset; the model then selects a
  *preset id* like a pattern id. This keeps the model's surface small and lets
  users curate exactly what it may reach — the safest source for queue items
  (idea 1) and scenes.
- **Typed/spoken parity.** Every contract field behaves identically whether the
  turn arrived by keyboard or ASR transcript, except that model-selected *mode
  or control-style changes from voice* stay behind their own explicit gate
  (reference ROADMAP #3).
- **Trace-visible "what changed."** Every model-driven change — region, pattern,
  program, vector, queue op, style, mode, goal — must be legible in
  diagnostics/trace. This is an observability requirement on all of the above.
- **Non-goal: sentiment-paced drift.** Tying intensity to inferred conversational
  escalation is hidden state and easy to overfit; the reference app reached the
  same verdict on fuzzy controllers ("treat as a research spike"). Idea 5 is the
  sanctioned, *explicit* alternative. Recorded here so it is not re-proposed as a
  quick win.

## Cross-references

- Current contract: `internal/chat/contract.go`; engine intent:
  `internal/motion/target.go`; system-prompt assembly: `internal/chat/prompts.go`.
- [motion-retargeting.md](motion-retargeting.md) — the sampler/retarget model
  any new intent compiles through.
- [pattern-library.md](pattern-library.md) — patterns vs programs, enabled-only
  curation, and the LLM catalog rule.
- [IMPLEMENTATION_PLAN.md](../IMPLEMENTATION_PLAN.md) — the unstarted Autopilot
  slice (ideas E/F are its likely shape) and the bounded arrangement contract.
- Reference: `StrokeGPT-ReVibed/docs/motion_control_modes.md` (route policy and
  "Future LLM Control Modes") and `ROADMAP.md` items #3, #4, #7, #12, #15, #16.
