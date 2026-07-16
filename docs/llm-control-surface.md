# LLM Motion Control Surface — Current State and Ideas

This document catalogs how the local LLM can drive the device today, what the
motion engine can already do that the model cannot yet reach, and a ranked set
of ideas for widening LLM control. It is a design sketch for discussion, not a
committed plan; nothing here is scheduled until it becomes a scoped slice.

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

## New ideas not in the reference notes

These are not in the StrokeGPT-ReVibed docs; offered for consideration with the
same skepticism.

- **Suggest-then-confirm previews.** For a large change (new arrangement, new
  program), the model proposes and the UI shows a one-line summary the user
  accepts before it drives the device. Reuses the backend preview sampler the
  library already renders. Turns "the model surprised me" into an approval step
  without adding a stop.
- **Named user presets as first-class model choices.** Let users save an
  arrangement/region/anchor set as a named preset; the model then selects a
  *preset id* the same way it selects a pattern id. Keeps the model's surface
  small and inspectable and lets users curate what it may reach.
- **Typed/spoken control parity.** Whatever contract fields exist must behave
  identically whether the turn arrived by keyboard or by ASR transcript — with
  the reference app's caveat that model-selected *mode changes* from voice stay
  behind their own explicit gate.
- **Trace-visible "what changed."** Every model-driven change (region, pattern,
  program, style, mode, speed) should be legible in diagnostics/trace, so a user
  can always answer "why did it do that?" This is a documentation/observability
  requirement on all of the above, not a feature.
- **Sentiment-paced drift — flagged, not recommended.** Tie intensity drift to
  conversational escalation. Tempting, but it is hidden state and easy to
  overfit; the reference app reached the same verdict on fuzzy controllers
  ("likely too noisy without large-scale human input; treat as a research
  spike"). Record it here so it is not re-proposed as a quick win.

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
