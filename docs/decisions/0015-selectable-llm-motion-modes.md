# ADR 0015: Selectable LLM Motion Modes

## Status

Accepted and implemented; amended for alpha.29 to make Creative the default.
The persisted/API identifier remains `dynamic`; the UI labels this mode
`Creative` beginning with alpha.24. Pattern Library and Off remain explicit
persisted alternatives. The default change is a product choice informed by
iterative installed-session feedback and model evaluation, not a claim that the
remaining real-device A/B acceptance is complete.

## Context

MagicHandy's original LLM contract only let the model choose enabled library
patterns, speed, and named area focus. That is safe and inspectable, but it also
makes conversational motion depend on a finite catalog and on deterministic
continuity/variation policy. Users reported that the result felt mechanistic,
especially when a request called for subtle positioning, gradual variation, or
natural changes that did not map cleanly to a saved loop.

StrokeGPT-ReVibed's default non-library schema gave the model more direct
semantic control of position, stroke range, speed, timing, and action/no-action.
Its useful lesson is model ownership of bounded motion intent. Its implementation
is not suitable for direct copying: it mixed client timers, mode-specific motion
paths, and transport assumptions that produced poor physical behavior and made
safety fixes inconsistent.

The design question is whether MagicHandy can offer comparable model freedom
without adding another motion loop, exposing raw device positions, or letting a
mode bypass the shared sampler, retargeter, limits, controller ownership, and
Stop lifecycle.

## Decision

MagicHandy has three persisted, mutually exclusive LLM motion modes:

1. **Creative** (`dynamic` in settings/API): the model emits bounded semantic geometry (`center_percent` plus
   `span_percent`, an ordered route through named anchors, or a compact phrase
   of two to four complete semantic sections),
   an optional stroke-length envelope, `speed_percent`, slow center/rhythm
   `variation_percent`, and a `segment_seconds` decision horizon. The backend
   compiles this to ordinary loop content and a `MotionPlan`.
2. **Pattern Library**: the model selects enabled opaque pattern handles, speed,
   and optional named area focus. It remains an explicit selectable alternative.
3. **Off**: the model is chat-only. Model-authored starts and updates are
   rejected, but user and model Stop paths remain unconditional.

The Chat Controls sidebar exposes a segmented choice with all three values.
Settings > Model exposes the same categorical list and shows area-focus and experimental-content controls
only for Pattern Library. The permanent navigation rail remains navigation, not
a mixed global control surface. Selection persists through a controller-gated
scoped settings endpoint. A mode change stops Autopilot planning so an
old-vocabulary decision cannot arrive late, but it does not stop already-running
engine motion. Dispatch rechecks the saved mode before applying every non-Stop
command.

Dynamic and Pattern Library use separate prompt vocabularies. Dynamic prompts
contain no pattern catalog, persisted pattern IDs, area-focus fields, transport
fields, phase values, or device commands. Pattern prompts contain no Dynamic
geometry. The strict parser rejects mixed shapes.

Dynamic geometry is bounded as follows:

- speed is 1–100 and is clamped again to the user's motion limits;
- center is 0–100 and span is 20–100;
- anchor routes contain 2–6 names from base/lower/middle/upper/tip, cannot
  repeat consecutively, and must cover at least 20% of travel;
- `span_percent` or the anchor bounds are the outer reach;
  `span_min_percent` is a 20–outer-span floor and `span_profile` selects
  `steady`, `breathe`, `wander`, or `contrast`;
- an explicit variable span profile produces a deterministic, loop-closed
  phrase of at least about 30 seconds at maximum reference speed. It changes
  the complete anchor route coherently rather than adding sample jitter;
- variation is 0–100 and independently produces loop-closed multi-harmonic
  center drift plus bounded leg-time breathing over a route-sized phrase of at
  least about eight seconds at maximum reference speed;
- the decision horizon is 4–120 seconds and does not stop motion at expiry.
- a section phrase contains 2–4 complete movement ideas; each uses the same
  bounded geometry/texture vocabulary plus 2–12 cycles. Sections compile to
  one long engine curve, never a command queue or a second motion loop.

The empty span profile preserves alpha.25's bounded implicit span swell for old
in-memory targets. New model responses use the explicit vocabulary. A variable
profile without a usable floor is rejected or normalized steady at non-chat
boundaries; the backend never invents the amount of range variation. The
backend-derived phrase seed is diagnostic/runtime state, not a model field.

Interior route anchors are pass-through knots with a non-zero tangent. Only
the true route endpoints reverse direction. Creative uses a monotone C2
quintic-Hermite profile whose neighboring legs share velocity and acceleration
at every knot; its runtime plan also has a perceptual jerk budget. Dynamic content enters the same
`MotionTarget`, normalization, sampler, transition, transport stream, and Stop
generation as every other source. Content identity excludes the decision
horizon so a timing-only update preserves phase. Cross-geometry handoff phase
selection considers position, direction, and bounded velocity mismatch before
the existing C1 continuity transition. Its phase search scales with authored
curve complexity, so a long Creative phrase is not searched at the same coarse
resolution as a short catalog motif.

While Dynamic motion is active, any non-empty interactive turn may carry a
model-selected `update` or `none`. A negative qualifier applies to its semantic
axis: preserving pace does not cancel an explicit range request, while an
unscoped refusal remains inert. Deterministic code does not map taste phrases
to geometry and does not force variation after a valid `none`. Starting motion
still requires explicit current-turn authority. Autopilot has its
existing autonomous authority; it clamps the model horizon to the user's motion
cadence range. If Dynamic generation fails, it holds or waits rather than
falling back to a deterministic pattern.

The setting is additive inside the existing versioned settings JSON. Beginning
with alpha.29, documents without it adopt Creative; a previously saved chat-only
capability maps to Off. Explicit saved Pattern Library and Off choices remain
unchanged. No database schema migration is required.

## Consequences

Positive:

- The model can create continuous conversational motion without depending on
  filenames, catalog coverage, or deterministic pattern-selection taste.
- Safety fixes, transport behavior, limits, diagnostics, and Stop remain shared
  with patterns, programs, media, and preset modes.
- The user can switch control philosophies beside the conversation or in Model
  settings, and the backend snapshot exposes the selected mode and Dynamic
  geometry honestly.
- Dynamic failures cannot silently introduce a library pattern the model did
  not choose.

Negative:

- The strict output schema is larger, increasing small-model malformed-output
  risk and the need for provider-specific acceptance.
- Model-authored geometry can still feel poor even when it is valid; simulation
  proves continuity bounds, not physical quality.
- The extra Chat control consumes some of the already dense motion sidebar.
- Dynamic state adds trace and frontend types that must stay synchronized with
  the engine snapshot.

## 2026-08-20 calibration follow-up

Initial physical feedback found Dynamic motion still robotic and reported that
73% felt materially slower than comparable controls. Inspection showed that
Dynamic's nonzero variation repeated a four-cycle sine and that all loop speeds
were calibrated to 180% travel/s at 100%, then constrained by the catalog's
450 ms authoring reversal floor.

The shared planner now maps 1–100 through an explicitly selected Original /
Handy 2 Standard / Handy 2 Pro travel and normal-speed profile, then applies a
distinct exact-curve runtime envelope; catalog authoring keeps its quieter
quality limits. Handy publishes enough travel and speed data for this physical
calibration, but not per-model acceleration limits, so the runtime ceiling stays
shared and Pro overclocking is not exposed. Dynamic variation uses a longer deterministic harmonic phrase
and bounded temporal asymmetry, and fractional phase sampling removes authored-
millisecond plateaus. The Chat sidebar uses visible set-point sliders for
ordered cadence/style settings and segmented radio choices for categorical
modes. The comparison and formulas are recorded in
[`../motion-calibration.md`](../motion-calibration.md).
The cross-device percentage mapping is separately governed by
[`ADR 0016`](0016-handy-model-speed-calibration.md).

## 2026-08-20 range-envelope follow-up

The first Creative variation field coupled center, span, and timing into one
small texture. That kept the schema compact but did not let the model express
phrases such as alternating tight and broad strokes independently of pace or
center drift. Creative now treats outer geometry, span floor/profile,
center/rhythm variation, pace, and decision horizon as separate semantic axes.

The installed managed 12B Gemma model passed 16/16 repeated range-envelope
cases and the broader 9/9 Creative intent matrix on the first response, with no
engine or transport instantiated. Cases covered variable start, legacy-to-
breathe, contrast, steady-to-variable, variable-to-steady, pace-only
preservation, unchanged continuation, conversation-only hold, start, focus,
negation, and Stop. This closes the managed-provider prompt/parser portion of
acceptance, but not the Ollama or real-device feel portions.

This is a refinement of the accepted one-engine decision, not a new motion
path. The initiating report lacked transport, latency, and trace evidence, and
no post-change device run has yet been captured, so the acceptance gate below
remains open.

## 2026-08-20 natural-turn and local-variability follow-up

Review of the latest installed trace separated two remaining defects. Its
contrast range spent more than 30 reversals inside an approximately 31–35%
span, while its velocity profile remained nearly linear through most of each
leg and compressed braking into a short endpoint guide. Creative now uses
whole-leg shape-preserving PCHIP easing for true reversals; stored patterns keep
their short acceleration-budgeted guides so this does not silently rewrite the
catalog. Both profiles compile through the same curve, exact acceleration
limiter, engine, sampler, transport stream, and Stop generation.

The long loop-closed phrase remains useful for avoiding an obvious seam, but
wander and contrast now make correlated seeded span choices at roughly
two-cycle cadence and deliberately visit low, middle, and high portions of the
allowed band. Center and leg timing receive independent seeded texture, with a
square-root response so ordinary model-selected variation is perceptible.
Determinism is retained for diagnostics; determinism no longer means a short
repeated table or a long same-span plateau.

The conversation review also found a model/validator mismatch: the assistant
could claim that it reached the tip or full length while emitting no accepted
update or geometry that omitted an endpoint. Terse follow-ups now inherit the
last user position request for validation, explicit reach claims must cover the
requested effective window, and a position-only correction is scoped to
geometry. Unsolicited pace, span-texture, variation, and horizon fields are
stripped as unauthorized model noise instead of requiring a growing catalog of
phrase-specific instructions. The managed model passed 9/9 repetitions of the
reproduced correction sequence plus the broader 25/25 Creative matrix without
constructing an engine or transport.

## 2026-08-22 continuous-turn and phrase-variety follow-up

Alpha.27's whole-leg PCHIP removed the short endpoint brake, but PCHIP is C1:
velocity is continuous while acceleration can still jump at a reversal. Felt
output remained abrupt, and a single long envelope could still express many
variations of one route rather than a larger movement idea.

Creative now renders its semantic knots with a monotone C2 quintic Hermite
profile. True reversals share one bounded acceleration state across both legs;
pass-through anchors retain a bounded nonzero velocity. The planner evaluates
the exact quintic acceleration and jerk extrema and lengthens only the Creative
period when either runtime quality envelope is exceeded. Catalog and imported
curves retain their existing profiles.

The model may also return two to four complete `sections`, each with geometry,
texture, and 2–12 cycles. Deterministic code expands them into one loop-closed
curve with at least 60 seconds of maximum-reference travel (or the longer
decision horizon), uses C2 smootherstep for slow control envelopes, and derives
new micro-timing for later occurrences. Replacing a phrase advances one bounded
seed from the authoritative current target; no unbounded history or hidden
random decision policy is introduced. Speed-only updates preserve every
section, while a scalar geometry correction deliberately returns to the
currently effective section.

Cloud's required whole-percent wire encoding remains unchanged. The Creative-
only fitter now scores both position error and inverse time-at-position error,
then removes redundant quantized edges. This preserves a short eased stroke's
timing shape without changing catalog, imported, or media simplification.

The installed Ollama model passed the new four-case no-transport phrase matrix:
multi-section start, running phrase replacement, pace-only preservation, and a
failed-tip correction. The last case used the existing single repair turn to
produce a real geometry change. Broader provider behavior remains stochastic,
and no post-change hardware command was issued, so real-device acceptance is
still open.

## 2026-08-22 elapsed-phrase and default follow-up

Read-only diagnosis of the installed alpha.28 max-rate session showed that the
scheduler was already reconsidering at roughly 12-second intervals inside the
configured 8–16 second range. The model chose six consecutive holds because it
could see session age and speed age but not the age of the effective semantic
phrase, its reconsideration count, its hold streak, or the current horizon. The
autonomous contract also omitted the already accepted span-envelope and section
fields.

Autopilot now supplies and traces those facts. Semantic phrase identity excludes
speed, horizon, and backend phrase seeds; only geometry or texture changes reset
the clock. Motion change rate is described as a preference for more or less
frequent meaningful differences, not a deadline. The active Gemma llama.cpp
model kept 3/3 short-age holds, changed 3/3 otherwise identical long-age/six-
hold cases, and completed a 12-decision transport-free run with one hold, 11
semantic changes, 11 range envelopes, four horizons, and no repair/fallback.
Sections remained optional.

Creative becomes the fresh/pre-selector default on that evidence and the user's
explicit product direction. This amendment does not alter the one-engine path,
authorization, configured limits, transport boundary, or Stop semantics. It
also does not treat a prompt harness as physical acceptance; the matched device
comparison below remains required evidence.

## Rejected Alternatives

- **Remove Pattern Library control.** Rejected; authored content remains useful,
  predictable, and explicitly selectable even though Creative is the default.
- **Expose raw timed points or transport commands to the model.** Rejected by
  ADR 0002 and the one-motion-path safety invariant.
- **Copy StrokeGPT-ReVibed's control loop.** Rejected because its parallel
  motion/timer behavior is the defect this Go rewrite is intended to remove.
- **Use a backend randomizer to decide whether to act.** Rejected because it
  creates a second hidden taste policy. The model owns valid action/no-action;
  deterministic code owns authorization and safety.
- **Infer the mode per turn.** Rejected because it makes capabilities and
  diagnostics ambiguous and can let stale responses cross vocabularies.

## Remaining Acceptance

Run the same prompt matrix through the installed managed llama.cpp model and the
supported Ollama path, then compare Dynamic and Pattern Library on the same real
device below the agreed test speed.
Include slow narrow loops, ordered-anchor pass-throughs, true reversals,
repeated conversational updates, Autopilot handoffs, mode changes with an
in-flight request, and Emergency Stop. Record `motion_trace.v3`, transport,
latency, and subjective continuity. A failed feel check requires a Creative or
default-policy correction; it does not justify a transport-specific bypass.
