# Layered steady motion and handoff review — 2026-10-22

The reported roughness occurs both during unchanged Layered playback and when
chat changes it, using Handy Cloud. This review compares the shared compiler
and buffered sampler against `c5d777c8`, the preceding WAN/hosted-pace PR head.
It uses captured **synthetic** model proposals, inert plans and fake transports.
No physical device was connected or moved. Command plots do not establish
carriage smoothness or physical comfort.

## Findings and changes

1. **Carrier half-cycles are not the combined field's turning points.** A
   moving center can cancel a half-stroke at its sampled endpoints without
   canceling the actual excursion. The old compiler gave those endpoints an
   entire half-cycle. One captured 37% proposal produced a 0.316% reversal
   lasting 251 ms; both endpoints round to the same Cloud position. Another
   proposal had a 305 ms stationary wire segment. A first attempt to merge
   only sub-percent reversals removed these symptoms but left larger near-holds.

   The compiler now discovers extrema of the complete center/width/carrier
   field at 32 probes per cycle, refines the brackets, and integrates the
   existing clock between those extrema. Travel compensation accounts for the
   actual cycle interval. Existing monotone joint fitting, velocity,
   acceleration and jerk limits remain authoritative. Only small **and brief**
   reversals use the existing 2% / 250 ms chatter criteria; slow subtle travel
   remains authored content. Geometry discovery is separate from device-clock
   calculation. No wire resolution enters this compiler.

2. **Rounding position at the old timestamp creates speed pulses.** Optional
   whole-percent points now align with their crossing times when monotonicity,
   interval spacing, velocity and trajectory fidelity permit it. A fixed
   12 ms correction limit is removed. Only real anchors and the committed append
   tail are pinned; pinning an unsent cutoff and its predecessor introduced a
   recurring batch-boundary error. A final check rejects retiming that loses
   trajectory fidelity after velocity projection. Immediate transport spacing
   retains its existing path, including Handy Intiface's 110/180 ms settings.

3. **Global phase search can jump to a different part of a long phrase.**
   Layered edits now search near the current stroke. Field cycle coordinates
   survive compilation and chatter merges, so an edit that changes the number
   of turns still searches near the same part of the phrase. Position and
   direction remain relevant to selecting a handoff.

4. **The 750 ms crossfade can blend opposed motions.** For Layered edits with
   identical stroke destinations and spatial projection, a bounded monotone
   quintic joins the inherited position, velocity and acceleration to the next
   destination. If necessary, the engine finishes the current stroke before
   joining the following one. The new plan resumes at that endpoint. The splice
   must meet the same analytic motion envelope; unsupported/chained cases keep
   the established transition path. Geometry edits retain a crossfade with the
   improved nearby phrase selection. This is part of the existing engine path,
   with no new producer, transport owner or playback loop.

5. **A transition previously weakened fitting for the entire append window.**
   Authored knots outside an actual blend are mandatory again. Crossing
   alignment resumes at the blend end, even in the same append. Single-stroke
   splices use ordinary bounded timing throughout.

## Independent consultation

The requested Claude CLI consultation used **`claude-opus-5-5`, effort `max`**,
with safe mode, read-only source tools and no hardware or browser access. Model
identity is recorded in the CLI result. The first review challenged the
sub-percent-only fix, identified the true-extrema and append-boundary problems,
and correctly rejected average-segment velocity error as sole proof of
smoothness. Those findings changed both the implementation and the evaluation.

The broader recommendations are deliberately separated from this change:
replace geometry crossfades with general feasible bridges, and eventually
choose wire levels per stroke rather than against an absolute probe grid.
Neither is silently introduced as another motion model. Source and synthetic
fixtures were supplied; installed private conversation history and credentials
were not sent to Claude.

## Numerical and visual evaluation

The full-loop matrix contains 47 proposals/fixtures on all three Handy profiles
(141 cases), including the loop seam and two seconds beyond it. It includes
23 captured valid ChatGPT proposals and 24 authored width/pace/center variants.
Duplicates are retained as input records; they are not independent evidence.

| Measure | Previous head | Final candidate |
|---|---:|---:|
| Cases with stationary wire edges in the matrix | 39 | 0 |
| Example `live-19`, commanded stationary time | 305 ms | 0 ms |
| Example `live-22`, commanded stationary time | 251 ms | 0 ms |
| `live-19` mean travel | 59.11 %/s | 63.86 %/s |
| `live-22` mean travel | 79.37 %/s | 86.08 %/s |
| Steady 22% full-width loop duration | 128,000 ms | 128,000 ms |
| Steady 22% full-width mean travel | 100 %/s | 100 %/s |

Across 36 steady-pattern/profile records, variation between identical strokes
improves in 33 and worsens in three; the median ratio is 0.863. Across the whole
matrix, instantaneous velocity approximation error improves in 82/141, with
median ratio 0.993. The largest and 95th-percentile command velocity steps do
**not** improve universally. Restoring real small excursions changes the
underlying motion, so these are not interchangeable measures of physical feel.
Maximum sampled wire-position error in this matrix is 0.803 percentage points.

The cutoff experiment is a cleaner timing comparison: with identical compiled
geometry, freeing unsent boundaries improves instantaneous velocity error in
139/141 cases and steady-stroke consistency in 34/36. The committed tail,
reversals, existing velocity ceiling and append point limit remain fixed.

Fast moving-center fixtures can regain excursions that half-cycle sampling had
erased. For `alternate-22-2`, total loop travel grows 5,100→6,528 percentage
points and period grows 65.256→85.206 s, while mean travel changes 78.15→76.62
%/s. This is a change in the represented rhythm, not a blanket slowdown.
It is an explicit physical-review case.

Validation also covers:

- 144 moving-center/profile combinations: geometry bounds, continuous joint
  velocity/acceleration and the existing derivative envelope.
- 90 pace-splice combinations: three profiles, five large/small speed pairs,
  six starting phases, both endpoint derivatives, wire velocity, and deferred
  joins. A production-engine fake verifies one Play and no work after Stop.
- 16 handoff scenarios rendered with original/global, nearby/crossfade and
  bounded-splice variants. Speed reductions lose the old extra braking/reversal
  shoulder; geometry edits retain their existing fallback.
- 144 Cloud/Intiface comparisons across eight motion families and six speeds:
  no sampler errors. Immediate sampling policy is unchanged; curves that use
  the Layered compiler can change. Maximum immediate command-count difference
  over 12 seconds is four; maximum mean-rate difference is 1.84 %/s.
- Real **GPT-6 Sol, reasoning none**, through production HTTP chat and a fake
  transport: text-only greeting, compound alternation, reversed contrast,
  replacement with steady full strokes, pace-only increase, and Stop. Four
  motion turns pass intent checks with one provider call each, no repair or
  semantic fallback, and only one playback stream. Total fixture time: 31.45 s.

The final shared-engine atlas renders **251 cases into 233 distinct plots and
15 overview sheets**, covering every continuous library recipe at 10/25/43,
Flow experiments, Creative/Creative v2 across all profiles, and the 47 fixtures.
All overview sheets were inspected. Six additional detailed sheets cover all
17 continuous library recipes at all three speeds, overlaying planned and wire
position/velocity. Detailed outliers include `live-19`, `live-22`, the fast
moving-center case, steady 45%/30%-width, and the slowdown handoff. The library
retains its different range/center/pace characters; bounded turns and loop seams
remain continuous. Four final live-chat proposals and the captured append/Stop
timeline are retained separately.

All generated evidence is ignored under `.scratch/layered-smoothness/`:
`full-matrix-before.json`, `full-matrix-final.json`,
`full-matrix-pinned.json`, `full-matrix-free-boundary.json`,
`parity-before.json`, `parity-final.json`, `handoffs-final.json`,
`atlas-final/`, `catalog-details-*.png`, `handoff-final-*.png`,
`http-live-final.json`, and `http-atlas-final/`. Earlier sub-percent-only and
incorrect clock-normalization prototypes remain there as rejected evidence.
The prior hosted-pace report retains failed model selections.

## Scope and remaining validation

The full Go suite, vet, lint, CGO-free build, focused lifecycle/architecture
checks and live hosted fixture pass locally. CI supplies the Linux race and
frontend/package gates. The isolated current-source review app uses an
available local Gemma endpoint; both the real generation probe and text-only
chat are required before handoff.

No prompt-language matching, preferred speed band, model call, context growth,
dependency or background worker is added by the smoothness changes. Shared
compiler cost changes are measured in the goal scorecard. Real Handy Cloud
A/B feedback is still needed before claiming the reported physical sensation
is resolved. Geometry crossfades and residual wire-level stroke variation are
explicit further work, not masked by the passing tests.
