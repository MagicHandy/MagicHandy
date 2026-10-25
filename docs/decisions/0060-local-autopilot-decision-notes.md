# ADR 0060: Local Autopilot decision notes and their latency cost

Status: accepted, 2026-10-25. The owner explicitly requested that the latency
compromise be easy to trace later.

## Decision

Simple and Intermediate **Autopilot motion** generate a `basis` string of at
most 240 characters before edits, within the same request. It names applicable
human constraints, released/expired restrictions, and remaining freedom. The
backend validates the note's type/size and discards it before the normal motion
parser. Its prose never commands motion, creates a constraint lock, or forces
speed, range, pattern rotation or a minimum change.

The bounded, aged human-request timeline is placed beside the decision. Simple
moves it out of the system snapshot to avoid duplication at 16K. Intermediate
also retains its original system timeline, the arrangement tested at 32K.
Positive output budgets below 384 are raised to 384 for this request only;
Intermediate's existing 512 minimum already supplies that room. Context
allocations are unchanged. Interactive chat, speech check-ins and Advanced
contracts do not get the note. No additional model request or motion loop is
introduced; scheduling priority, Stop and late-result admission remain intact.

## Explicitly accepted latency compromise

Measured on RTX 5070 Ti, reasoning off, two sampling seeds. Times are median
elapsed generation/validation per decision, not physical device latency or a
guaranteed service level. These are small scenario sets; caching and host load
affect timing.

| Model / seed | Intent passes before → after | Median before → after | Extra time accepted |
| --- | --- | --- | --- |
| E4B / 10252026 | 14/30 → 26/30 | 1,652.5 → 1,754 ms | +101.5 ms, about 6% |
| E4B / 10262026 | 10/30 → 25/30 | 1,761 → 1,828 ms | +67 ms, about 4% |
| 12B Intermediate / 10252026 | 23/30 → 25/30 | 1,390 → 1,815 ms | +425 ms, about 31% |
| 12B Intermediate / 10262026 | 25/30 → 27/30 | 1,402.5 → 2,047.5 ms | +645 ms, about 46% |

**We accept extra Autopilot latency for better constraint-following:** 24/60 →
51/60 for E4B, and 48/60 → 52/60 for Intermediate 12B. This applies to background
motion decisions, not ordinary chat. The cost is materially larger on 12B:
roughly half a second per decision here. Do not describe it as latency-free.

Keep these ignored captures together in `.scratch/e4b-simple-2026-10-25/`:

- E4B controls: `fixed-baseline-autopilot.json`,
  `seed2-baseline-autopilot.json`; production: `verified-10252026-autopilot.json`,
  `verified-10262026-autopilot.json`.
- 12B controls: `transfer-seed1-12b-baseline-autopilot.json`,
  `transfer-12b-baseline-autopilot.json`; production:
  `transfer-12b-production-10252026.json`, `transfer-12b-production-10262026.json`.
- Failed, shorter-note and extra-arithmetic variants remain beside them.
  Valid JSON with a changed hold or unrequested edit remains an intent failure.

## Revisit conditions and implementation

Re-evaluate if planning intervals become short enough for two-second decisions
to overlap, slower hardware makes queueing visible, interactive latency rises,
or a newer local model preserves constraints with less output. Compare quality
and latency over multiple seeds, including exact holds, fixed-speed/free-shape,
released constraints, natural development and timed scope. Do not remove the
note on latency alone or retain it after its quality benefit disappears.

Implementation: `internal/chat/local_motion_plan.go` contains the prompt,
schema, note parser, capability predicate and output floor.
`AutopilotService.Complete` captures capability before changing Layered's local
dialect; `completeLayeredAutopilot` and `continuousPrompt` assemble the request.
`TestNaturalAutopilotContinuityLive` is the production evaluator. Rollback of
this policy should not alter the engine or transport. Regression tests check
that the note cannot command motion or become the spoken reply.

See the [optimization and transfer review](../e4b-simple-review-2026-10-25.md)
and [capability architecture](0058-model-capability-levels.md).
