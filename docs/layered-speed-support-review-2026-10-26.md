# Layered speed and device support review — 2026-10-26

Status: all three physical A/B assessments complete; the accepted candidate is
integrated for alpha.75. The reporter confirmed an Original Handy over Cloud in
Layered mode. The initial findings and test-only evaluation below describe the
pre-alpha.75 behavior and are retained as historical evidence. The separately
authorized physical audition and its limitations follow them. Production
integration, visual review and release checks are recorded at the end and in
the [alpha.75 review](alpha75-release-review-2026-10-26.md).

## Findings

### 1. Layered's quiet motion envelope dominates the device calibration

`internal/motion/plan.go` maps 1–100 to the selected model's nominal sustained
velocity range. Original is 110 mm / 32–400 mm/s, Standard 125 mm / 32–400 mm/s,
and Pro 125 mm / 32–450 mm/s. Those normal velocity endpoints agree with the
[manufacturer's FAQ](https://www.thehandy.com/faq/) and
[current Handy 2 support documentation](https://intercom.help/ohdoki/en/articles/9524417-faq).
The values are reference calibration, not carriage telemetry or a guarantee
that a reversing curve can maintain the requested mean.

The Layered field and counted-phrase paths use `flowAccelerationBudget=2400`
and `flowJerkBudget=24000` (semantic percent/s² and percent/s³), while Creative
v2 uses the shared runtime envelope of 7500 and 150000. `FlowTarget` explicitly
retains the quieter historical field budget; it was not a device-reported
limit. The same low budgets apply to Layered handoffs.

Before exact fitting, `flowClock` combines the requested duration, a cosine
acceleration floor and a jerk floor using an eighth-power smooth maximum.
`fitLegCurves` then preserves the resulting duration or lengthens it. Thus
increasing the selected speed stops materially changing the trajectory well
below 100%, especially on short strokes. This is independent of model quality
and occurs before network dispatch. Rounded strokes also spend less of each
cycle near peak velocity than a cruise-shaped stroke.

### 2. The UI hides the existing backend pace estimate for Layered

`Engine.snapshotLocked` publishes `pace` for all continuous curves, including
Flow/Layered and Creative v2. `MotionVisualizer.tsx` only reads it when
`target.dynamic` is present (original Creative). Its other branch renders
`target.speed_percent` as `Speed`. Layered therefore displays 100% while its
backend explicitly reports a much smaller effective pace and `limited=true`.

The limiter explanation is also incomplete: `summarizeMotionPace` compares
final derivatives with the larger runtime envelope, not the actual Flow
budget or the conservative clock that preceded fitting. These cases are
labeled `curve_geometry`, concealing the lower mode-specific envelope. Merely
changing its thresholds would still miss a smooth combination of constraints;
the compiler should retain the reason it extended the requested clock.

### 3. Cloud does not apply a second speed-percentage mapping

Cloud HSP receives position/time points. `BuildHSPAdd` preserves timestamps,
rounds positions to the native percent step, and `BuildHSPPlay` sets
`playback_rate=1`. Native and browser Handy Bluetooth also dispatch HSP points.
Intiface uses timed linear commands with device timing-gap admission; it has
additional update-spacing constraints, but is not involved in this report.

The 350 ms simulated Cloud run retained one Play across full strokes and a
compound retarget, mapped sampled timestamps unchanged, and stopped without
later commands. This does not rule out a real firmware or buffering problem;
it establishes that the large steady-pace deficit is already present upstream.

### 4. The reference's percentage is a different clock

Read-only inspection of the public [reference app](https://thehandyapp.ddns.net/)
on October 26 found `Handjob (simple)` authored with 350 ms per half-stroke at
its 100 setting. The slider spans 10–130 and scales time by `(200-setting)/100`:
100 gives a 700 ms cycle; 130 gives 490 ms. In contrast, the controlled
production Layered full-stroke fixture at 100 has a 946 ms cycle.

This is a 35% difference in full-cycle rate at the two controls labeled 100.
It is not evidence that either percentage measures motor speed. Reference
HSSP playback is not automatically a faster firmware path: the manufacturer
states that [firmware 4 HSSP uses HSP internally](https://www.thehandy.com/blog/firmware-4-is-here/).
No reference source or dependency is incorporated into the application.

## Controlled results

Fixtures use the Original profile, full physical-window setting, saved speed
limits 1–100, zero pace variation, neutral turn softness and group rates 100.
The compound motif is two 72–100 strokes and one 0–100 sweep. Values below
describe compiled commands. Conversion to mm/s uses the nominal 110 mm profile;
the actual calibrated travel, user stroke window and firmware behavior can
make measured carriage velocity differ. Equivalent percent uses the existing
affine calibration, not simply `mean / 400`.

| Requested | Full stroke mean / equivalent | 80–100 stroke mean / equivalent | Two short + full mean / equivalent |
| ---: | ---: | ---: | ---: |
| 25 | 121.0 mm/s / 24.9% | 89.4 mm/s / 16.5% | 112.8 mm/s / 22.7% |
| 50 | 203.3 mm/s / 47.1% | 90.5 mm/s / 16.7% | 156.7 mm/s / 34.6% |
| 75 | 229.6 mm/s / 54.2% | 90.5 mm/s / 16.7% | 166.4 mm/s / 37.2% |
| 100 | 232.6 mm/s / 55.0% | 90.5 mm/s / 16.7% | 167.4 mm/s / 37.4% |

At requested 100, the corresponding peaks are 365.3, 142.2 and 372.0 mm/s.
Short strokes are not exhausting the device's velocity ceiling. Full strokes
are already near it briefly, but have low mean travel because of their shape.

The existing broader 72-case review covers three model profiles, 25/50/75/100,
and six Creative/Creative-v2/Layered fixtures. Its varied-width Layered fixture
reaches only 40.3% equivalent at requested 100 on Original. This is distinct
from the fixed full-stroke fixture's 55.0%, not an inconsistent measurement.

Handy 2 profiles also saturate. Their requested-100 full/short/compound means
are 254.1/102.9/185.2 mm/s (Standard) and 264.3/102.9/190.2 mm/s (Pro).
Selecting a faster model profile cannot remove the inherited shape limits.
Mixed-Handy sessions intentionally use the most conservative participating
normalized profile because they share one timeline. Pro overclock is not
supported or exercised by this evaluation.

## Inert alternatives

All alternatives retain destinations, stroke counts and the normal device
peak ceiling. The fixtures have no intended slow phases, rests or modulation;
this is not a general algorithm for removing such timing from real scores.

| At requested 100, Original | Full mean | Short mean | Compound mean |
| --- | ---: | ---: | ---: |
| Production Layered | 232.6 | 90.5 | 167.4 |
| Exact fitter seeded from requested pace, same quiet envelope | 241.8 | 93.2 | 158.7 |
| Same rounded strokes, Creative runtime envelope | 254.0 | 171.9 | 223.7 |
| Creative runtime envelope plus existing cruise candidates | 320.7 | 171.9 | 258.0 |

Units are nominal mm/s. The first alternative is not a general improvement:
its coupled fitting makes the compound fixture slower. The strongest candidate
increases full-stroke mean by 38%, short-stroke mean by 90%, and compound mean
by 54%. Full strokes have a 686 ms cycle, close to the reference's 700 ms at
100. Its measured analytical peaks remain below 400 mm/s, acceleration 7500
percent/s² and jerk 150000 percent/s³, with continuous velocity/acceleration
at knots. It uses **higher acceleration and jerk than shipping Layered**;
those are existing Creative software limits, not a newly verified hardware
capability. It is not enabled in production.

## Visual and runtime assessment

Rendered all 99 baseline/control/alternative cases into 75 distinct plots and
five overview sheets. Inspected every overview plus detailed Original full,
short and compound production/candidate plots and the same-envelope compound
regression. Identical high-speed narrow cases share a plot and retain separate
manifest records. The plots show:

- real speed saturation, with high selected speeds sharing an identical
  narrow-stroke trajectory;
- the same extrema and counted rhythm in the faster alternatives;
- a longer near-constant-velocity body in the cruise candidate, plus higher
  turnaround acceleration; rounded positions alone do not prove equal feel;
- no analytical velocity or acceleration jumps at the inspected joints;
- the shipping sparse Cloud approximation retaining turn locations/times.

The captured simulated runtime includes startup, a full-to-compound retarget
and Stop. Queued points beyond Stop are explicitly shaded as canceled. The
renderer displays semantic test-transport commands, not measured firmware
interpolation. No physical-smoothness claim follows from these checks.

## Recommended implementation order

1. Make Layered and Creative v2 display the backend's effective/selected pace,
   clearly marked as a commanded estimate. Record actual compiler constraints
   in diagnostics instead of reporting only a generic geometry limit.
2. Separate deliberate motion character from the historical quiet envelope.
   Reuse the shared stroke fitter and bounded cruise shaping for high pace;
   keep softness, explicit rate contrast, rests and longer-horizon modulation.
   Unconstrained low-speed trajectories should remain unchanged. Apply the
   selected envelope consistently to steady plans and all Layered handoffs.
3. Verify calibration against usable device travel and the configured physical
   window. Keep model profiles and shared multi-device limits explicit; do not
   add a transport-local speed multiplier or silently enable overclock.
4. Validate physically before enabling faster timing. Compare current and
   candidate on the same Original/FW/Cloud/window with full, narrow and counted
   motion at low/mid/high requests, plus pace/reach changes and Stop. Match
   reference-app **cycle timing**, not just slider labels. Record latency,
   `motion_trace.v3`, pauses/turn feel and actual observed Stop. Repeat Native
   BLE, Intiface and other model profiles separately before widening acceptance.

## Requested Claude second opinion

At the user's request, Claude Code was invoked with model `claude-opus-5-5`
and effort `xhigh`, restricted to Read/Glob/Grep for an independent assessment.
The run read the compiler, transitions, calibration, sampler, prompts and
evidence, but reached its account session limit before returning findings.
It reported a reset at 09:00 America/New_York. No completed Claude second
opinion is available, and the conclusions in this report are the parent
agent's analysis, not Claude's endorsement.

The prompt, transcript and resume metadata are retained under the ignored
evidence directory as `claude-prompt.txt`, `claude-review.jsonl` and
`claude-resume.json`. A continuation can reuse the completed source reading
after availability returns. Claude made no code changes or hardware calls.

## Reproduction and evidence

`TestLayeredSpeedSupportReview` and `TestLayeredSpeedSupportRuntime` are opt-in
under `magichandy_labs`, with ignored report paths in
`MAGICHANDY_SPEED_SUPPORT_REPORT` and
`MAGICHANDY_LAYERED_SPEED_RUNTIME_REPORT`. The runtime harness reuses the
existing simulated Cloud transport, never a real connection key. The normal
application binary, dependencies, UI bundle and memory budgets are unchanged.

Evidence in `.scratch/speed-support-review/`: `baseline.json/log`,
`controlled.json/log`, `runtime.json`, `race-final.log`, `transport.log`,
`vet.log`, `atlas.json`, `atlas/`, public-reference inspection files and
`speed-summary.png`. The first controlled fixture used an invalid three-cycle
loop and was corrected to six cycles before evaluation. Focused race-enabled
compiler/runtime tests, vet and Cloud/Bluetooth transport contract checks pass.

Standard motion-package lint passes. An additional lint run with the optional
Labs build tag reports 11 existing findings in other review fixtures (complexity,
file-path checks and one staticcheck suggestion); none are in the new or
modified files. The report is retained in `lint.log`, and those unrelated
fixtures were not changed or suppressed for this evaluation.

The existing current review build remains at `http://127.0.0.1:50301/#/chat`,
with motion stopped and its controls visible. Its fresh real-generation
readiness probe passed (`review-llm.log`). Changes in this evaluation are docs
and build-tagged tests, so the running application source/bundle is unchanged.

## Authorized physical audition

On October 26 the user authorized real Cloud playback, confirmed that the
Original Handy was clear and free to move, and stopped other playback. A
separate isolated app at `http://127.0.0.1:50302/#/chat` was built from this
worktree with an **ignored Go build overlay**, not a shipping behavior change.
The overlay adds an explicit experimental variant to three restricted,
previously evaluated Flow fixtures. Unknown variants and unevaluated scores
are rejected. Both variants enter the ordinary HTTP motion-start path,
shared engine, sanitizer, sparse sampler, Cloud protocol and Stop gate.
Other devices, LLM motion and Autopilot are disabled for this audition.

The candidate was checked against the original inert experiment at four
speeds for full, short and counted fixtures: sampled positions/timestamps,
period, acceleration and jerk match exactly. Race-enabled startup, retarget
and Stop tests pass with simulated Cloud latency, including no commands
escaping Stop. The isolated app passed actual model generation and one
text-only chat request with a non-empty reply, one provider call and no
repair/fallback/motion.

### Full-range result

Both runs requested 100%, physical window 0–100, no authored pace variation,
no additional rate reduction, 12 seconds each. Current Layered A completed a
full cycle in 946 ms; candidate B completed one in 686 ms. Runtime snapshots
reported mean travel of 211.4 versus 291.5 semantic percent/s (+38%). These
are commanded estimates, not carriage telemetry. Cloud startup took 2378 ms
for A and 6391 ms for B, including preparation/positioning. Stop requests
completed in 328/332 ms. Both runs completed without transport errors, then
the app stopped and disconnected.

User feedback: **“B was faster and still smooth”** and **“Yes, both Stops were
clean.”** The user also noted that A was faster than a real GPT-generated
maximum-speed pattern in the installed app. Acceptance is limited to this
full-range scenario; it does not establish general motion-quality parity.

### Installed-session distinction

Read-only inspection of the installed app's most recent retained trace
(captured 2026-10-26 10:03:16 UTC) found two 100% targets. The earlier target
used moving shorter strokes, traversing approximately 19–81 overall but
mostly only 35–45 points per leg; its steady sampled mean was 128.1 percent/s.
The later target used full 0–100 sweeps every 473 ms per half-stroke and had
a sampled mean of 211.4 percent/s, matching A's commanded timing. The user
explicitly clarified that **this full-range installed-app motion also felt
slower than A**, so the shorter-stroke segment does not explain that report.
Matching commanded timing is not evidence of matching physical execution.

The installed executable identifies itself as alpha.74 (`ef6f88b5`); the
shipping motion/transport source used by A is unchanged from that release.
The installed trace has no failed transport results (median 343 ms, maximum
436 ms). For its final full-range segment, an estimate based on Cloud Play
acknowledgment and appended timestamps leaves at least 1139 ms of previously
queued points when each append is acknowledged. No obvious buffer starvation
appears in this evidence. That estimate is not device telemetry and cannot
exclude firmware execution differences. The installed-versus-A discrepancy
remains **unresolved**; a matched replay through the running installed chat
and retarget path is still needed. The retained trace stores emitted commands
and target summaries, not the full original FlowSpec.

### Short and compound results

The same 100% request, 0–100 physical window and 12-second duration were used
for short 80–100 strokes and a counted two-short/one-full phrase.

| Fixture | A mean (%/s) | B mean (%/s) | B increase | Stop A/B (ms) |
| --- | ---: | ---: | ---: | ---: |
| Full range | 211.4 | 291.5 | 38% | 328 / 332 |
| Short, 20-point width | 82.3 | 156.2 | 90% | 319 / 317 |
| Two short, one full | 152.2 | 234.6 | 54% | 328 / 307 |

These are runtime commanded means, not measured motor velocity. The short
fixture's physical feedback was **“B was much faster than A this time, and
was still smooth.”** For the compound pair the user confirmed **“Yes, faster
and still distinct and smooth.”** All three tested fixtures therefore have
positive physical feedback for the candidate. This acceptance does not yet
cover arbitrary Layered modulation, live retargeting, other transports or
other Handy models. All six runs completed without a reported transport
error and the app confirmed Stop.
The app disconnected after each pair. The user explicitly confirmed physical
Stop for the full-range pair; the other pair records contain backend Stop
confirmation, which must not be substituted for physical observation.

Rendered and inspected all six actual Cloud-bound command excerpts against
their compiled curves (`physical-command-comparison.png`). Full reach and
the counted short/long sequence are retained. Captured samples track the
compiled motion, with the existing sparse approximation visible most clearly
on narrow strokes. This visual inspection complements the user's feedback;
it does not establish physical tracking or explain the installed discrepancy.

Evidence is retained under `.scratch/speed-support-hardware/`: restricted
overlay and preparation script, provenance with binary/source hashes,
race-test log, runtime capture, previews, installed trace and timestamped
physical A/B captures. Each physical capture includes model-independent
motion settings, runtime snapshots, transport diagnostics, `motion_trace.v3`,
latencies and user feedback. The user connection key is confined to the
isolated app's private settings and is absent from these artifacts.

## Production integration and visual review

The user then authorized implementation and release. Alpha.75 promotes the
accepted candidate through the existing Flow compiler, with regression tests
for the exact auditioned periods, bounded velocity/acceleration/jerk, unchanged
slow pace and explicit softness, and retained phrase rate contrast. Historical
test-only alternatives freeze the old 2400/24000 budget so later comparisons
remain reproducible. Large slow-to-fast edits and one catalog transition
exposed failures during integration; both were fixed without changing the
device velocity or wire position-error limits. See the
[release review](alpha75-release-review-2026-10-26.md) for implementation detail.

The production matrix renders 132 combinations (11 fixtures, four speeds and
three profiles) from the shared engine and Cloud sampler: 122 distinct plots
and eight overview sheets, all inspected. Detailed checks cover full-speed
full reach, roaming geometry, explicit maximum softness, 15% phrase contrast,
very slow authored pace layers, section changes and Pro combined modulation.
Bounds, turns and counts remain distinct. Slow dwell from explicit softness
or authored low rates remains visible rather than being erased by a global
minimum speed. These plots are commanded curves, not physical measurements.

The simulated Cloud runtime uses 350 ms transport latency for a 7-to-100 pace
edit of the compound motif, then Stop. Its rendered command capture finishes
the inherited stroke and joins the new plan without a forced pause or lost
count. Unplayed buffered remainder is marked canceled; teardown assertions
pass. The replay atlas retains all 24 earlier provider outcomes: 12 successful
GPT-6 Sol outputs compile under the new timing, and 12 GPT-6.1 provider failures
remain explicit. The replay's nine distinct curves and overview were reviewed;
this is reuse of recorded model outputs, not a new provider evaluation.

Production evidence is ignored under `.scratch/alpha75/`: matrix/report and
atlas, model replay and atlas, runtime capture, full Go/race/frontend/lint/vet
logs, vulnerability scan, installer checks, compilation measurements and the
working review-app generation/chat probe. No connection key or runtime file
is committed. The original physical evidence remains unchanged.
