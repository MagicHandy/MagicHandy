# Layered session repair and capability evaluation — 2026-10-28

## Scope and evidence

Based on alpha.78 (`69a33dc0`) and the two reported installed-app conversations.
The user reported brief endpoint stops, slow/jerky Layered motion, malformed
responses, rough Edge motion, and inability to request fast downward travel with
a slow return. The real device is the Original Handy on Cloud. The temporary
Handy 2 Standard setting was an attempt to increase speed, not a hardware change.

The initial read-only diagnostic capture contains seven malformed responses:
three repaired and four failed, plus two canceled requests. Later captured
Creative v2 output contains an actual 166 ms stationary segment. Two distinct
Cloud recoveries stopped the stream after append reported a stopped state
(22:32:53.400 and 22:34:05.544 in the captured log). Those recovery events are
separate from a planned pause or near-stationary rounded turn. Their upstream
cause is not established by this review; recovery and Stop are not suppressed.

After the user said the installed session was in use, investigation continued
only against saved evidence, synthetic requests, isolated inference workers and
fake transports. No installed settings, playback, controller ownership, browser
tab or hardware connection was changed. Private diagnostics, credentials and
generated artifacts remain outside git under `.scratch/layered-session-repair`.

## Root causes and repairs

### Edge shape persisting into Layered

`HoldingTarget` set 85% turn softness. Subsequent Layered edits cloned that score,
but the model could neither see nor change this shaping parameter. A faster base
setting therefore hit the same fitted timing ceiling. An Original Handy full
sweep with inherited softness compiled to **173.010 semantic percent/second** at
45%, 75% and 100%, matching the approximately 173–174 plateau in the captured
Layered trace. Neutral rounded full travel at 100% compiles to **291.545%/s**.
These are commanded mean estimates, not measured carriage velocity.

Edge now uses the shared normal rounded turns, with its existing reduced speed
and reach. It no longer seeds this hidden softness in later Layered edits. At
the reviewed 20% base speed and 25..75 travel, mean travel remains **93.284%/s**;
peak velocity falls from **195.43 to 146.53%/s**, with a rounder phase portrait and
less concentration of acceleration around the endpoint. Authored Lab/library
softness is still honored and is described as read-only context when present.

The model now also receives compiled pace facts, alongside reach facts, and
traces record the selected Handy profile, softness, cadence and direction rates.
This makes a low effective pace distinguishable from a low base setting.

Selecting Handy 2 Standard for Original hardware does not unlock more speed:
the profile has a different physical travel mapping (about 12% lower semantic
velocity for the same physical limit here) and does not use the Original-only
accepted sparse Cloud sampling policy. No profile was changed automatically.

### Unrequested Creative v2 rests

Creative v2's phrasing logic could randomly insert 110–280 ms rests. Those rests
are removed from the production gesture path, while its range, rate and phrase
variation remain. Prompt/help/localization copy no longer promises these rests.
The separate Lab-only Stroke experiment retains its own authored linger behavior.
No wire filter or hardware acceleration limit was relaxed.

### False rejection versus real grammar limits

- Phrase plus recall was advertised but rejected by a composition allowlist.
  It now applies edits to the recalled score and reports that base when an edit
  is invalid.
- Relative-control failures now identify the resolved field/value and allowable
  range rather than giving a generic capability error.
- Full-and-tip/base guidance explains that the widest stroke must equal the
  outer band; copying a previous narrower width contradicts full travel.
- Removing a nonexistent range/center layer is a no-op. Redundant removals do
  not discard or invalidate an otherwise valid phrase.
- A paired stroke still has to reverse by at least ten points, including across
  the loop seam. Removing this check would accept contradictory geometry.

## Regional cycles instead of explicit reversal lists

The original paired-stroke grammar cannot express every ordered sequence of
disjoint regions. The Advanced contract now also accepts `phrase.regions`, with
bottom/top, count, rate and cycle direction for each region. It describes
complete local out-and-back cycles plus straight travel to the next region.
Same-direction intermediate boundaries merge into one leg without an extra
reversal, detour or rest; merged timing preserves total requested travel time.

Bounds remain one to six regions, one to eight cycles each, at most 24 local
cycles, with at least ten points between real reversals. Incompatible small
reversals receive an actionable geometric error, not silent clipping. Handoffs
preserve ordinal strokes only when retained semantic boundary slots match.

Trials kept both failed representations and accepted outputs:

| Trial | First-pass intent result | Decision |
| --- | ---: | --- |
| Original hosted contract, 64K | 6/9 | Reproduced composition/region limitations |
| Explicit reversal-list prototype | 16/18, then 25/27 | Removed: plausible output with wrong local counts |
| Repairing the reversal-list count failures | 0/2 | Stronger repair text did not solve the representation |
| Initial region contract | 27/27 | Kept the vocabulary; replaced its first connector compiler after review |
| Final compiler, first rerun | 23/27 | Three equal-rate floating-point rejections and one upstream timeout retained |
| Final compiler after exact equal-rate fix | **27/27** | Adopted |
| Production one-repair path on the two original invalid motifs | **2/2** | Valid regional repairs; no deterministic text rewrite |

The discarded connector compiler could invent extra local cycles. The final
compiler builds the selected cycles and straight travel directly, then merges
waypoints. A regression covers five lower cycles followed by three upper cycles
in either orientation. A separate regression covers the 100% merged-rate
roundoff failure; equal input rates remain exactly equal.

## Direction-specific pace and capability decision

Advanced Layered accepts `directional_pace:{up_rate_percent,down_rate_percent}`.
Up is semantic travel toward 100/tip; down is toward 0/base. Each 1..100 rate
multiplies the selected base travel rate, together with existing group rates
and pace modulation. A fast downward stroke and slow upward return therefore
no longer require alternating time-based pace waves or a geometry substitution.
There is no preferred speed, fixed ratio or user-text classifier.

Omission preserves the current rates; 100/100 clears them in both chat and the
motion target constructor. Reach, counts and unrelated pace stay independent.
For unequal rates, one fitted clock scale preserves relative leg durations
across the entire motif and seam. Arbitrary neighboring index pairs were not
sufficient for mixed regions. The same velocity/acceleration/jerk limits and
rounded-turn fitter remain authoritative. A leg stretched only to preserve
rhythm does not become eligible for a flattened cruise profile.

This deliberately trades some average speed for the requested contrast when a
fast, short leg constrains the motif. For example, at base 100 with up 30/down
100, the full-range fixture compiles to about **112.30%/s mean, 335.88%/s peak**;
the two-short/one-full fixture to **58.29%/s mean, 234.79%/s peak**. Symmetric
Layered retains its accepted timing. Low group and direction multipliers can
compound into very slow travel; they stay explicit in context/trace rather than
being replaced with an arbitrary speed floor. These are not speed telemetry.

Each capability level was exercised with the production chat parser and shared
compiler. Local candidates temporarily added a compact direction block to that
tier's normal schema; they were not enabled in the review app. Cases include
the original request, its reverse, swapping directions, removing contrast,
reach-only and speed-only follow-ups, exact continuation and a question.
Advanced also includes counted compound motion. Each case ran three times.

| Capability / tested model | Context/output | First-pass valid and intended | Shipping decision |
| --- | --- | ---: | --- |
| Simple / local Gemma E4B Q4 | 16K / 256 | **17/24** | Do not expose direction control |
| Intermediate / local Gemma 12B | 32K / 512 | **21/24**, unchanged after a guidance retry | Do not expose direction control |
| Advanced / ChatGPT GPT 6.1 Terra, Low reasoning | 64K / 1024 | **27/27** | Expose and accept direction control |

Simple had three prompt-budget rejections, three wrong reach selections and one
geometry substitution in place of direction control. A shorter-guide follow-up
fixed all three budget cases but did not establish a solution to the intent
failures. Intermediate handled the direction changes but consistently omitted
the width edit needed when narrowing travel; additional guidance still failed
the same three follow-ups. Those trial promotions were reverted. The existing
Simple and Intermediate direction schemas/prompts remain unchanged.

A legacy Simple pilot used the wrong capability flag and a separate 24-case
admission probe used an oversized output reserve; both are retained but excluded
from the capability comparison. No schema admission failure is counted as a
model response. All successful final Advanced trials used one provider call
without repair or fallback. A capability selector is not a certification of
every model: arbitrary local models manually set to Advanced were not tested.

Final hosted direction median generation was **3,504 ms** (sample p90 4,581 ms),
and final region median **3,922 ms** (sample p90 5,464 ms). Local models ran in
separate CPU-only llama.cpp workers with four threads, avoiding the active
session's GPU allocation. Their medians (~5.0 s E4B, ~14.9 s 12B) are not GPU
benchmarks or comparable provider-latency recommendations.

The parser rejects unsupported-tier direction edits. Switching to Simple or
Intermediate preserves already-playing asymmetric motion and labels its rates
as read-only context, rather than silently changing the running pattern.

## Independent review

Claude Opus 5.5, Max effort, performed three read-only reviews. The final review
caught extra connector cycles, pair-index timing errors, cruise eligibility of
stretched slow legs, unadvertised local edits, ordinal-slot ambiguity and a
misleading empty-regions error. All were addressed and tested as described above.
This records findings and dispositions, not a claim that Claude approved the
subsequently revised implementation.

The suggestion to require one direction rate to be 100, or impose an arbitrary
minimum effective speed, was not adopted: legitimate authored timing would be
rejected or changed. Instead 100/100 canonicalization, prompt guidance, compiled
pace facts and trace visibility address ambiguity. Additional physical testing
of slow asymmetric turns remains necessary.

## Visual, runtime and automated validation

Generated evidence remains ignored under `.scratch/layered-session-repair`:

- `motion-figures/index.html`: 502 retained records, 287 distinct motion plots,
  18 overview sheets. Includes baseline, inherited-hold reproduction, rejected
  reversal prototype, final field/phrase/directional matrix and 24 Edge handoffs.
- `model-figures/index.html`: 270 retained trial records, 30 distinct compiled
  outputs, two overview sheets. Includes failed selections and admission errors,
  so 270 is not a count of successful model calls.
- `session-runtime.json` and `motion-figures/captured-01.png`: shared-engine
  regional-to-directional retarget and Stop using an inert buffered transport
  with 350 ms append latency. One stream/play is retained through the retarget;
  queued points beyond Stop are marked canceled in the plot.

All 20 overview sheets were inspected. Detail inspection covered the inherited
softness speed ceiling, old versus normal Edge turns, Creative v2 rest removal,
low/middle/high direction contrast and both orientations, compound counts,
separate-region connecting travel, wrong local model selections and the runtime
retarget/Stop timeline. Rounded Edge removes the flattened endpoint shape at
unchanged mean pace. Removed Creative rests no longer appear as flat plan
segments. Region transfers have no extra detours, and directional plans retain
the selected slower return. Exact knot tests complement the plotted derivatives.

The whole-percent wire overlay is still piecewise linear and can approximate a
slow rounded turn coarsely. The accepted transport sampling policy is unchanged;
plots and fake transport cannot establish physical smoothness. The new
asymmetric fitter also makes slow endpoint portions more visible in some
compound patterns. These are explicit physical follow-up items, not hidden
behind a claim that numerical continuity guarantees feel.

Automated coverage includes 486 profile/speed/reach/grammar/direction combinations,
120 seeded regional cases (half with direction contrast), C2 joins and seam,
bounds, acceleration/jerk/velocity, preserved relative timing, unchanged symmetric
timing, exact counts, canonicalization, tier isolation, repairs and teardown.
Full Go tests, full race tests, vet, lint (zero issues), architecture checks,
CGO-disabled build, frontend typecheck/build and 1,069 frontend tests passed. An
initial race build collided with the frontend replacing embedded hashed assets;
the full race command was rerun after the frontend build had finished.

## Budgets and review app

Windows amd64, Go 1.26.10, `CGO_ENABLED=0`, `-trimpath -buildvcs=false -ldflags
'-s -w'`: embedded core **24,337,408 -> 24,376,832 bytes** (+39,424). Canonical
UI **2,856,409 -> 2,856,207 bytes** (-202); main JS **1,313,547 raw / 353,078 gzip**,
down 34/10 bytes. CSS remains 210,037 raw / 36,604 gzip. No production dependency,
background worker, polling loop or extra model call was introduced. No RSS or
cold-start improvement is claimed; test inference workers are not shipping assets.

The isolated review build is at `http://127.0.0.1:50309/#/chat`, with simulated
devices, Intermediate local Gemma 12B and LLM motion Off. A real generation via
`scripts/check-review-llm.ps1` passed, followed by a non-empty app chat reply in
one call (5,034 ms total, 1,214 ms first token in the final rebuild), without
repair/fallback or motion.
The browser shows that reply with motion Idle. No user credentials were copied.
Advanced provider behavior was tested through the isolated hosted fixtures above,
not by promoting the review app's 12B model to an unvalidated capability level.

## Before physical acceptance or release

1. Confirm the actual Original Handy profile, saved limits and orientation. Keep
   other sessions stopped only when the user authorizes this next test.
2. Compare prior and new Edge shape at equal reduced pace/reach, then return to
   full-range Layered at 75% and 100%. Record trace and observed endpoint behavior.
3. Compare low, middle and high base speeds for both direction contrasts, paired
   counted phrases and separate-region motifs. Assess counts, turn hesitation,
   short-to-full travel and the average-speed tradeoff of asymmetric timing.
4. Test live reach/rate changes, Edge return and Stop with normal and delayed
   Cloud appends. Distinguish planned slow travel from actual transport recovery.
5. Retain the transport mode, latency summary, trace and physical observations.
   Sampling, saved stroke windows, hardware envelopes and recovery behavior are
   intentionally unchanged by this work.

No real-device test was run in this task. Publication and physical acceptance
remain separate from the isolated model/compiler results.
