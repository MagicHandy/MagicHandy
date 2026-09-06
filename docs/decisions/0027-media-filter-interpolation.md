# ADR 0027: Local media rounding inside the shared curve

Status: Accepted for implementation, 2026-09-06. Physical acceptance remains open.

## Context

The continuous LLM modes established that motion must be reviewed through the
compiled engine, including velocity, acceleration and quantized output. Media
rounding instead inserted quadratic fillet points into a linear timeline. Its
outline looked rounded but each segment still changed velocity abruptly. Dense
scripts could silently disable rounding to stay within the source point cap.

## Decision

Keep authored media linear by default. An opt-in rounding policy travels with
the media timeline and compiles into sparse local Hermite polynomial windows
inside `motion.Curve`. Reuse the existing continuous-motion polynomial evaluator
and shared sampler/fitter/sanitizer. No transport or separate motion loop is added.

Apply smoothing, optional authored-delta speed limiting, then corner rounding.
The media smoothing predicate requires both neighboring excursions to be small
and short, using the shared efficient reversal bookkeeping; pattern import keeps
its established predicate. Each control changes only its named property.

Rounding joins the existing line slopes with zero acceleration at both ends.
Symmetric windows produce cubic smoothstep velocity between the two rates,
therefore no new peak rate or local overshoot. Window sizes respect adjacent
legs; dwell shoulders and windows below 4 ms remain exact. The video clock and
source-point budget are preserved. Additional internal fitter landmarks describe
the compiled geometry rather than pretending inserted points are source data.

Publish compact compiled measurements independently of the large media payload:
corner counts, reduced reach and local apex-time shift. Filter reports carry the
applied policy, and the UI distinguishes stale, pending and measured-zero results.
Save responses and backend snapshots reconcile controls; failed saves cannot
auto-resume playback.

## Consequences

Only rounded windows are C2; plateau shoulders, skipped corners and unrelated
authored slope changes remain linear. Quantized wire points remain piecewise
linear and may flatten tiny tips. No blanket media acceleration/jerk cap or
physical smoothness claim follows from this change. Combining the speed cap
and rounding can reduce reach more than the old operation order; local peak
timing can shift while total duration stays fixed. Both costs are explicit.

Rounding now works at the 100k source limit and costs O(n) additional compilation
and storage. The measured 50k remaining-action case takes about 3.3 ms and
9.22 MB total allocations, versus the old 1.1 ms path that skipped rounding.
There are no new shipping dependencies. See the
[review](../funscript-filter-review-2026-09-06.md) and
[budgets](../perf-baseline.md#2026-09-06--funscript-filter-quality).
