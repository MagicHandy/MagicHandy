# ADR 0023: Persistent Layered motion and explicit continuation

- Date: 2026-09-05
- Status: Implemented for review; no merge or release authorized
- Extends: ADRs 0015, 0019 and 0022

## Context

The user's relative/layer experiment preserved motion better than ordered
sections, but missed coupled reach/location requests. It overused turn
softness, and every one of 15 captured automatic continuations held an
unchanged score. Recent assistant turns displaced the human instructions.
Adding more named textures alone does not solve those interpretation and
persistence problems.

## Decision

Add `layered` as a production LLM motion mode and an identically shaped Lab
contract. The production selector has three columns and two rows: Creative,
Pattern library, Layered, Off, and two noninteractive blank slots. Existing
saved modes and defaults remain valid. Labs still use the default-off setting
available in every release.

The model edits one persistent semantic `FlowSpec` with a required `edits`
object and a reply. Partial control and layer edits preserve unspecified
attributes. Paired stroke-width bounds distinguish distance from endpoint
position. Layer period deltas distinguish "by four" from "to four". Explicit
geometry operations handle coupled changes such as alternating short strokes
between ends or alternating full travel with short anchored strokes. They
preserve pace unless the model explicitly requests a separate pace edit.
Turn softness and ordered sections are absent from this contract.

Independent range, center and pace envelopes support irregular drift, smooth
unequal alternation and periodic waves. Runtime initialization and explicit
`evolve` edits choose a fresh nonzero seed, recorded with the resulting score.
This avoids a universal repeated realization without making evidence
irreproducible. The compiler and safety bounds remain deterministic for a
captured score. The finite compiled score can still loop; Autopilot refreshes
its realization through ordinary conditional retargeting. No second motion
clock or sampling loop is introduced. Layered Lab inference waits at least
the configured quiet interval, plus up to half that interval of random delay.
Other Lab methods keep their existing cadence.

The latest four human requests are supplied separately from recent dialogue.
Automatic assistant replies cannot evict them. A continuation explicitly
requests evolution while preserving the current character; explicit requests
for exact repetition instead select a hold instruction, also enforced by the
host. Automatic changes cannot raise speed or widen the current requested
outer band. Invalid, contradictory or unauthorized responses are retained and
rejected in one call, without repair, prose-derived motion or library fallback.
The same prompt must demonstrate no regression on Gemma when refined for
Granite. Small-model shortcomings stay visible rather than being hidden by
implicit corrections.

`MotionTarget` and mode segments carry cloned semantic Flow scores through
the existing plan, sampler, sanitizer and transport. A settings speed clamp
recompiles the score. Retargeting preserves the active run; Stop, cancellation,
controller ownership and mode changes invalidate late work. Existing missing
layer-shape fields preserve their previous behavior. Layered production
Autopilot holds the current score on generation failure and does not apply the
legacy extra pace modulation or pattern fallback.

## Consequences and validation

The controller remains simple, with more complexity contained in the validated
semantic contract. Flow metadata is present in snapshots and trace exports so
plots can reproduce the accepted motion. A saved seed is evidence, not a fixed
runtime default. Model output validity, request mapping, plotted character,
transport dispatch and physical feel are separate acceptance criteria.

Tests cover partial edits, conflicts, schema boundaries, speed relimiting,
serialization, cloned state, continuation history, mode changes and Stop.
Live Gemma/Granite trials retain all iterations, and production HTTP tests
dispatch their real replies through a captured fake transport. Every evaluated
accepted score is rendered from the shared engine alongside failed selections.
See [the review](../layered-motion-review-2026-09-05.md) for results, limitations,
artifacts and reproduction. These plots do not establish physical comfort.
