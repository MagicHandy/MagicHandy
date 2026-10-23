# ADR 0052: Counted phrases within the shared Layered engine

Status: Accepted in PR #341, 2026-10-23. Extends ADR 0023.

## Context

A hosted model could vary Layered width and location, but could not express an
exact sequence such as two upper-region strokes followed by one complete sweep.
An alternating envelope produces intermediate reaches and unequal run lengths;
changing its period does not make it a stroke counter. The installed-session
review also found a pace follow-up changing geometry, and a reply claiming full
travel after only one endpoint had been restored.

## Decision

Add an optional `FlowSpec.Phrase` with one to six ordered groups of absolute
`bottom_percent`, `top_percent`, `count` and `rate_percent`. Each group supplies
one to eight lower/upper turn pairs, with at most 24 strokes per motif. Every
bottom must be at least ten semantic points below its own and the preceding
top, including the seam. The phrase's band must equal its endpoint hull.
Invalid scores are rejected atomically, without moving or repairing a turn.

A phrase uses the existing Layered joint fitter, velocity/acceleration/jerk
budgets, immutable plan, sampler, sanitizer, buffered transport and Stop path.
It creates no timer, worker, transport command or second motion engine. Whole
motifs repeat to cover at least the ordinary 64-cycle horizon; at most 23 extra
cycles are needed. Independent seeded pace variation and pace layers remain.
No range/center modulation or other stroke grammar can coexist with a phrase.

Rate is relative to the selected base travel rate. Each ascent uses its group's
rate. A shared descent uses the narrowest adjacent group containing that entire
leg; equal spans or neither-containing use the destination group. This gives
both legs of a full sweep the same selected rate for tip- and base-anchored
motifs. It is a geometric compilation rule, not interpretation of user prose.
The joint fitter may lengthen individual legs to satisfy physical limits.

For compatible phrases, handoffs retain the ordinal stroke. Reach changes use
monotone quintic joins bounded by the incoming and destination legs' actual
peak velocities as well as the existing hardware envelope. A narrowed leg gets
time for the distance actually remaining. If the inherited state cannot fit,
finish the old stroke, or the old pair, before joining; do not create a speed
burst to force the new reach. Other handoffs retain the established transition.

Hosted models get an exclusive `phrase` composition branch. Named field
geometries also accept compatible model-chosen stroke widths. Local-only
contracts and schemas are unchanged. Geometry choices, counts, pace and
follow-up intent remain model decisions: no user-text classifier, keyword rule,
preferred speed or preferred regional width is introduced. Saved limits and
structured-output validation remain authoritative.

Pace-only edits retain a phrase; geometry, width, range/center or shape-control
edits return to field motion. Shared endpoint anchoring survives that exit;
mixed-region phrases retain the preceding anchor. Existing `pace.follow`
operations already materialize independent timing, so that timing survives a
phrase. New follow operations are unavailable/rejected while a phrase has no
range/center source. Returning to field motion restores normal follow support.

Hosted context includes the existing compiled summary plus typical lower/upper
turn percentiles and endpoint-visit fractions. This distinguishes an isolated
full sweep from typical reach. These are commanded estimates, never carriage
telemetry. They are calculated once with the plan, not through additional model
calls or continuous analysis. Factory geometry and local context remain intact.

## Alternatives and consequences

Prompt-only guidance helped geometry preservation but could not encode exact
counts. An anchored single-turn representation could not describe mixed upper
and lower groups cleanly. Promoting the experimental stroke mode would bring a
different grammar and timing behavior into production; ordered sections cannot
represent individual counted strokes without section transitions. Explicit
turn pairs fit the existing joint fitter and retain ordinary field motion for
uncounted variation.

A phrase deliberately repeats its selected geometry until edited; ordinary
Layered remains available for freely evolving reach. Changes to group counts
or ordering can use the existing general crossfade rather than the specialized
ordinal splice. A slow sweep and a short stroke can have different fitted
rates even when their selected rates match, because the same safety limits
still apply. Physical Cloud feel remains a separate acceptance check.

See [the evaluation](../layered-range-review-2026-10-23.md) for failed candidates,
real ChatGPT trials, kinematic/transport tests, visual review and limitations.
