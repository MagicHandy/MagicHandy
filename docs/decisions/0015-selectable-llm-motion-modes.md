# ADR 0015: Selectable LLM Motion Modes

## Status

Accepted and implemented. Dynamic remains opt-in until live-provider and
real-device A/B acceptance supports changing the default.

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

1. **Dynamic**: the model emits bounded semantic geometry (`center_percent` plus
   `span_percent`, or an ordered route through named anchors),
   `speed_percent`, slow `variation_percent`, and a `segment_seconds` decision
   horizon. The backend compiles this to ordinary loop content and a
   `MotionPlan`.
2. **Pattern Library**: the model selects enabled opaque pattern handles, speed,
   and optional named area focus. This remains the default while Dynamic
   acceptance is open.
3. **Off**: the model is chat-only. Model-authored starts and updates are
   rejected, but user and model Stop paths remain unconditional.

The Chat Controls sidebar exposes a list with all three values. Settings > Model
exposes the same list and shows area-focus and experimental-content controls
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
- variation is 0–100 and produces deterministic, loop-closed center/span drift
  over several cycles rather than random per-sample noise;
- the decision horizon is 4–120 seconds and does not stop motion at expiry.

Interior route anchors are pass-through knots with a non-zero tangent. Only
the true route endpoints reverse direction. Dynamic content enters the same
`MotionTarget`, normalization, sampler, transition, transport stream, and Stop
generation as every other source. Content identity excludes the decision
horizon so a timing-only update preserves phase. Cross-geometry handoff phase
selection considers position, direction, and bounded velocity mismatch before
the existing C1 continuity transition.

While Dynamic motion is active, any non-empty, non-negated interactive turn may
carry a model-selected `update` or `none`. Deterministic code does not map taste
phrases to geometry and does not force variation after a valid `none`. Starting
motion still requires explicit current-turn authority. Autopilot has its
existing autonomous authority; it clamps the model horizon to the user's motion
cadence range. If Dynamic generation fails, it holds or waits rather than
falling back to a deterministic pattern.

The setting is additive inside the existing versioned settings JSON. Documents
without it preserve Pattern Library behavior; a previously saved chat-only
capability maps to Off. No database schema migration is required.

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

## Rejected Alternatives

- **Replace Pattern Library control.** Rejected until Dynamic has matched model
  and hardware evidence; authored content remains useful and predictable.
- **Expose raw timed points or transport commands to the model.** Rejected by
  ADR 0002 and the one-motion-path safety invariant.
- **Copy StrokeGPT-ReVibed's control loop.** Rejected because its parallel
  motion/timer behavior is the defect this Go rewrite is intended to remove.
- **Use a backend randomizer to decide whether to act.** Rejected because it
  creates a second hidden taste policy. The model owns valid action/no-action;
  deterministic code owns authorization and safety.
- **Infer the mode per turn.** Rejected because it makes capabilities and
  diagnostics ambiguous and can let stale responses cross vocabularies.

## Acceptance

Before Dynamic can become the default, run the same prompt matrix through the
installed managed llama.cpp model and the supported Ollama path, then compare
Dynamic and Pattern Library on the same real device below the agreed test speed.
Include slow narrow loops, ordered-anchor pass-throughs, true reversals,
repeated conversational updates, Autopilot handoffs, mode changes with an
in-flight request, and Emergency Stop. Record `motion_trace.v3`, transport,
latency, and subjective continuity. A failed feel check keeps Dynamic opt-in; it
does not justify a transport-specific correction.
