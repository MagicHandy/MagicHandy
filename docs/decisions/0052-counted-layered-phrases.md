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


## October 28 extension: complete regional cycles and directional pace

The paired `strokes` representation cannot describe separated regions in an
arbitrary order: every preceding top must remain above the next bottom. This
is a valid reversal constraint, not a rule to weaken. An evaluated alternative
let the model author every reversal; it produced valid-looking motifs with
wrong cycle counts and was not retained.

`phrase.regions` instead explicitly requests complete local out-and-back
cycles plus connecting travel. Each of one to six regions selects bottom/top,
count (1–8), relative rate (1–100), and `cycle` (`up_down` starts at the bottom;
`down_up` starts at the top). Total local cycles are bounded at 24. The compiler
joins neighboring endpoints with straight travel. A boundary passed in the same
direction is a waypoint, not a reversal: merge those legs instead of inserting
a detour or stop. The merged rate preserves summed travel time, weighted by
distance; equal rates remain exactly equal. Connecting travel uses the destination
region's rate. It adds no extra complete local cycles. The periodic sequence,
including its seam, must reverse by at least ten points at each actual turn.
An incompatible small reversal is rejected with a geometric explanation rather
than silently widening reach or inventing another stroke. Geometry, counts and
rates are explicit model choices; no human text is classified.

Both representations expand to the same bounded semantic leg list, then use
the existing fitter, sampler and transport. Before waypoint merging, a region
motif has at most 54 nodes (48 local plus six region starts). Whole motifs repeat
over the ordinary horizon. Regional handoffs retain ordinal slots only when
local counts, cycle directions, retained semantic boundary slots and turn
directions match. An equal total leg count alone is insufficient. The legacy
`strokes` vocabulary and timing remain compatible.

Optional `FlowSpec.directional_pace` chooses `up_rate_percent` (toward semantic
100/tip) and `down_rate_percent` (toward 0/base). Both are 1–100% multipliers of
base travel rate, combined with existing group rates and pace modulation. This
supports rapid travel one way and a slower return without replacing geometry
or inventing an alternating pace wave. Omission preserves the current choice;
both rates 100 restores the legacy symmetric clock. No direction is favored.
The transport still owns the user's physical reversal mapping.

For explicitly unequal direction rates, a common clock scale preserves relative
leg durations around the whole motif, including connecting travel and the seam.
Independent leg fitting, or pairing arbitrary neighboring indices, could invert
the intended contrast on short, fast strokes. This reuses the same exact joint
fitting and cruise candidates, and never raises velocity, acceleration or jerk
limits. Cruise eligibility uses the individual leg's constraint result: a slow
leg stretched only to preserve rhythm keeps its rounded shape. This accepts a
lower average pace to preserve directional rhythm. Symmetric/default scores
retain their physically accepted timing.

Only Advanced capability advertises and accepts model-authored direction rates.
The tested Simple/E4B and Intermediate/12B candidates handled basic contrast but
failed compound follow-ups; their normal prompts and schemas remain unchanged.
Switching to a smaller capability does not erase a playing asymmetric score;
its direction rates are exposed as read-only facts. Two 100% rates canonicalize
to the ordinary symmetric score in the motion layer as well as the chat parser.
Other explicit rates remain model-authored choices, visible in context and traces;
there is no language classifier, forced preferred ratio or hidden pace floor.

See [the session repair evaluation](../layered-session-repair-review-2026-10-28.md)
for failed representations, tier trials, compiled plots, Stop tests and required
physical follow-up. These results do not establish carriage smoothness.
