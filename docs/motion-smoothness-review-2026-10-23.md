# Installed alpha.70 motion and interruption review

## Evidence and limits

The installed Windows process reports alpha.70, commit `d6a829ca`. Its current
conversation contains 27 human messages and 26 completed GPT-6 Sol replies
between 14:14 and 14:40 UTC on October 23. Completed replies used one provider
call each, with a median 5.560 s request time (2.742–8.959 s). The retained
failure table is empty; ordinary cancellations were previously omitted.

One unanswered turn began at 14:20:24.945 UTC. The access audit records its
completion at 14:20:26.391 and a mode operation completing six milliseconds
later. No control-loss event occurs there. This supports a mode-change
interruption, not the configured 120 s model timeout; it does not establish
the cause of every cancellation the user saw. Earlier heartbeat loss occurred
before this conversation. Private chat and the exported diagnostic ZIP remain
outside version control and were not sent to Claude.

The motion trace retains only its latest 512 rows, starting at 14:32:43 UTC;
1,238 earlier rows were overwritten. All 504 retained Cloud commands succeeded:
median round trip 144 ms, maximum 1,191 ms. These acknowledgments do not prove
physical smoothness or exclude problems in the omitted interval. The active
device profile was Original Handy, with a saved 0–88 physical stroke window
and speed limits 1–100. Semantic full-range requests still map through that
saved window. The installed process and its hardware were left alone.

## Interruption behavior

An ordinary failed turn exposed the provider/context error directly. Another
late-completion path labeled any cancellation as Emergency Stop. The corrected
path keeps typed cancellation causes through the model coordinator and the
session/controller lifetime, then presents application-owned explanations:

- Unknown interruption: “The reply was interrupted before it finished. Please
  try again.”
- Model deadline: “The model took too long to reply. Please try again.”
- Changed model or motion settings: explain the changed settings and ask for a
  new request.
- Lost controller or sign-in: explain which authority ended and how to recover.
- Actual Stop: retain the existing explicit Emergency Stop message.

A reply already committed to history stays visible; its interrupted motion or
speech is reported separately. Interrupted output never becomes a motion
target or a repair offer. Technical errors now survive in the existing bounded,
redacted diagnostic store under `kind: interrupted`, even after cancellation.
That persistence uses a separate 500 ms deadline and cannot recreate deleted
conversations. No conversation text is classified or rewritten by these rules.

## Layered versus Creative v2

`TestHostedCompoundMotionComparison` sends six synthetic mechanical requests
twice through each real production chat service, using GPT-6 Sol, None reasoning,
32K context and the existing 512-token output cap. There is no transport or
hardware engine. All raw proposals, including unsupported requests, are retained.

| Request group | Layered | Creative v2 |
| --- | --- | --- |
| Five full strokes, then five lower-region strokes | 2/2 | 0/2 |
| Two upper-region strokes, then one full sweep | 2/2 | 0/2 |
| Slower full sweeps mixed with faster upper-region strokes | 2/2 | 0/2 |
| More free variation, pace-only edit, gentle syncopation | 6/6 | 6/6 |
| Valid structured responses | 12/12 | 12/12 |
| Median response time | 6.314 s | 3.897 s |
| p90 response time (nearest rank) | 7.706 s | 4.829 s |

These are control-intent checks, not physical acceptance. Creative v2 correctly
disclosed that its mix/focus grammar cannot guarantee exact counts, order or
separate rates for regional stroke groups. A higher-capability model cannot
express a feature the contract does not expose. Rendered outputs confirm that
its mixed-range results are distributions rather than the requested sequence;
permission bounds 0–100 also do not guarantee every broad stroke reaches both
ends when variation remains enabled. Layered's counted phrases represent the
three compound examples directly. Both ultimately use the same joint fitter
and motion engine.

This is a small comparison on one model, not a general capability guarantee.
Output token counts were not retained by this harness; elapsed time includes
the complete provider request and existing parsing/validation.

## Handoff experiment

Changing count, geometry or phrase layout could fall back to a fixed 750 ms
blend between independently moving curves. Opposing velocities can create a
braking shoulder or an extra small reversal even though both source curves
have continuous derivatives. Steady-loop smoothness does not test this path.

The accepted candidate first keeps the existing ordinal pace/reach splice.
For a changed Layered grammar it searches nearby authored turns for a monotone
quintic join, matching position, velocity and acceleration at both ends. Its
velocity ceiling respects the selected neighboring legs, and the existing
acceleration/jerk limits still apply. When momentum makes a direct join
impossible, the current authored stroke finishes before the join begins.
The new score, counts, endpoints and speed are not rewritten.
Phase-near entry can begin partway through the new phrase. The first complete
cycle after entry preserves its counts; no claim is made that the connecting
stroke itself restarts every requested sequence at its first group.

Visual review rejected an initial version that could slow almost to rest
mid-stroke without reversing. The final search also rejects an interior speed
minimum, trying other durations before deferring or using the existing fallback.
An active earlier transition or a different spatial projection keeps its
existing handling. Creative v2's transition path was evaluated but not changed.

The inert matrix covers three Handy profiles, speeds 15/35/94, four phases and
three edit classes. Of the 72 Layered edits, 66 select the bounded turn join;
six retain the existing fallback. All 36 changed counted phrases join, removing
the four reproduced sub-1% extra reversals. Across Layered cases the total goes
from five to one; the remaining case is retained as a limitation. Maximum sampled
transition acceleration falls from about 3,105 to 2,130 percent/s². The longest
tested connection is 2.372 s at low speed. Those numbers describe commanded
paths, not carriage feedback. A hardware A/B remains necessary before claiming
the user's perceived roughness is resolved.

The shared engine selects the join for every motion source using Layered
targets. Transport scheduling, steady curves, sampler tolerances, stroke limits,
and Stop authority remain unchanged. No second motion loop is introduced.

## Claude second opinion and follow-up

A focused Claude Opus 5.5 Max CLI review completed against public source and
synthetic cases. It supported a bounded C2 join after addressing direction at
nearly stationary turns, scope, sampled continuity and diagnostic persistence.
It agreed that the comparison does not justify a separate Advanced mode yet.
An earlier broader invocation exhausted its session limit without a final report;
no conclusions are attributed to that invocation.

The candidate now checks the authored outgoing direction even when velocity is
nearly zero, and preserves the existing ordinal path/fallback for same-slot
phrase edits. The evaluated non-phrase geometry path remains covered by the
36-case comparison and 60 additional geometry regressions. Tests also cover
fully soft turns and actual sampled derivatives on both sides of each join,
with the unblended prior path before a deferred join. The final evaluated paths
are unchanged by these edge-case guards. All nine overview sheets were inspected;
detailed outliers include the extra-reversal case, the slow deferred join,
the geometry fallback and the accepted geometry join after shoulder rejection.

Cancellation evidence now preserves a typed context cause even when an SDK
returns an unrelated stream error. Specific sign-in/controller causes take
precedence over a stale repair, and the repair outcome identifies changed
settings as stale. A deletion regression confirms detached persistence cannot
recreate a failure row for a removed conversation; the datastore enables foreign
keys per connection. Stable event codes are a possible later UI improvement;
the current shared error allowlist contains only application-owned messages.

Remaining work includes physical A/B validation, reducing the six fallback
geometry cases, and handling a new edit while an earlier join is still active.
Neither minimum reversal counts nor a schema-valid model reply alone establishes
physical comfort or faithful first-cycle sequencing.

## Steady motion and rejected changes

Forty-five inert cases compare representative model selections at 15/35/94,
with Layered turn softness 0/25/50 and Creative v2 references. More softness
does not uniformly reduce wire velocity error. At 94%, increasing softness to
50 reduces mean travel rate by roughly 14–18% in the tested Layered examples.
Some mid-speed cases gain quantization variation. This is not a suitable global
default change; none of these steady-shaping experiments are promoted.

Whole-percent buffered points still approximate curved travel. Short intervals
in a trace alone do not establish a defect, and additional sampling can make
some transports worse. Keep physical comparisons transport-specific. The current
change addresses reproduced transition artifacts without changing established
steady motion or the previously accepted Intiface spacing.

## Advanced mode recommendation

Keep improving Layered first. A new label would not fix common fitter, sampling
or transport behavior. Creative v2 remains useful for compact, freely varying
motion; replacing Layered with it would lose expressive power demonstrated above.

An eventual Advanced planner could add real value by composing ordered stroke
groups with independent travel timing, turn character and bounded variation.
The backend would compile the typed semantic score through the same fitter,
sampler, transport and Stop path. Planning would happen on requests or at the
existing Autopilot cadence, never once per physical stroke. A compact local
contract would remain separate. Eligibility should be based on demonstrated
schema and intent capability, rather than provider branding. A capable local
model should not require sending its conversation to a cloud provider to use
the richer contract. Initial evaluation should include at least two models,
including a local model, against requests current Layered cannot represent.

Promote such a mode only after it beats Layered on mixed intent, preserves exact
counts/range when requested, avoids added normal-call latency and passes both
compiled and physical handoff comparisons. Existing response repair can handle
invalid proposals; it cannot substitute for those acceptance checks.

## Reproduction and validation

Ignored evidence lives under `.scratch/motion-smoothness-review/`:

- `compound-comparison.json`, `compound-atlas/`: 24 model outputs, 20 distinct
  plots and two overview sheets, including all six unsupported Creative cases.
- `steady-cases.json`, `steady-probe.json`, `steady-atlas/`: all 45 experiments,
  quantized-wire metrics, 45 plots and three overview sheets.
- `handoff-comparison.json`, `handoff-final.json`, `handoff-single-crest.json`,
  `handoff-shaped-search.json`, `handoff-reviewed.json`: original and rejected/accepted transition
  iterations, including fallbacks.
- `handoff-reviewed-plots/`: 108 final comparison pairs and nine overview sheets.
- `installed-audit-summary.json`: bounded event metadata without identity data.
- `review-chat-final.json`, `budgets.json`: final review-app generation and matched builds.

The full Go suite, frontend suite (121 files, 949 tests), typecheck, localization audit, vet, lint and
pure-Go Windows build pass locally. Targeted regressions cover cancellation
causes, retained technical evidence, discarded drafts, remote error visibility,
exact join derivatives, encoded velocity bounds, rejected braking shoulders,
one shared playback stream and no post-Stop device work. Windows race execution
is unavailable because this host has no GCC; Linux CI remains required.

The isolated review app uses a working local Gemma 12B endpoint. The required
generation probe passes, and a text-only request through the app chat returns
one non-empty reply in one call without repair/fallback or motion. No real
device was connected or moved for this review.
