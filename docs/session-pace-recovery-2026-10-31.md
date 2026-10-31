# Layered pace and Cloud restart review — 2026-10-31

The installed alpha.81 session exposed separate compiler and transport issues.
The app and physical device were not restarted, connected or commanded during
this investigation. Inspection used a redacted diagnostic report and saved
conversation evidence. A discarded conversation was recovered locally from
validated committed SQLite WAL frames; no runtime data belongs in this PR.

## Evidence and limits

- One retained Layered planning context selected 78% base speed, an 85/100
  directional rate contrast, an alternating regional field and an independent
  pace envelope. Its authored effective pace was 50.03%, but the fitted result
  was 19.18%, with jerk reported as the binding limit. Directional field fitting
  stretched the entire 64-cycle loop to accommodate its worst reversal.
- Other frames demonstrate real authored slowdowns as well as physical limits.
  A 100% request over a 10%-wide stroke compiled to 21.40%; a subsequent full
  stroke compiled to 86.63%. Those are commanded mean-travel equivalents, not
  motor telemetry. Increasing the global speed multiplier would conceal the
  difference and affect already-correct motion.
- The first retained transport failure was a 10,000 ms point-upload timeout at
  07:55:57 UTC. Fail-stop and subsequent operations also failed despite HTTP
  200 transport status. This is response-processing failure, not a pre-dispatch
  motion-envelope rejection. The exported error text is redacted: `ok:false`
  is MagicHandy's result field and does not prove that Cloud returned a literal
  negative acknowledgement. It can represent an API error envelope, invalid
  JSON or a response-body read/size failure. The exact cause of these HTTP 200
  failures cannot be reconstructed from this export.
  The retained 284 steady append operations had a 449 ms median, 657 ms p95
  and 10,000 ms maximum; two failed. The 282 successful appends had a 656 ms
  p95 and 1,343 ms maximum. This is a retained trace window, not a complete
  session latency distribution.
- Later accepted startup commands failed measured arrival checks. In the last
  attempt, the projected target was 25.37 mm and repeated stationary reports
  were 28.67 mm. Another attempt moved away from a 5 mm target. The owner
  reports no manual movement, mode change or competing app control. These are
  real unresolved positioning discrepancies; they are not justification to
  broaden the 1% arrival tolerance or assume a constant calibration offset.
- Subsequent controller-heartbeat expiry happened after the positioning failure;
  it does not explain the original upload timeout. Two retained chat failures
  were cancellations caused by model-setting changes, not malformed output.

## Changes retained

Directional **fields** now constrain the slowdown locally at every reversal.
The shared joint fitter first applies the existing velocity, acceleration, jerk
and reversal bounds. If a fast-direction leg needs extra time, its two slow
neighbors receive at least the same proportional slowdown. This preserves the
authored direction contrast without imposing the worst leg's slowdown on distant
strokes. A field's local timing can therefore change under physical constraints;
it does not promise one uniformly stretched clock for all 64 cycles.

Counted **phrases** retain their common clock, including region transfers and
the seam. That fit now evaluates the common scale directly instead of first
distorting adjacent timings through a local fit and using the largest resulting
ratio. This produces a small improvement without changing the phrase's timing
relationships. Symmetric fields use the previous fitter unchanged. Authored
slowdowns, endpoints, counts, sparse sampling, transport timing, calibration and
all runtime bounds remain in force. There are no language rules, new prompts,
model calls, dependencies or additional motion paths.

Accepted lead-ins that do not settle now return `ErrStartupArrival`. Autonomous
mode retry stops on this physical failure, as it already does for unsafe
geometry. It cancels pending work before asynchronous teardown, including when
the lifecycle lock is busy. A fresh explicit user start is still allowed.
Transient transport errors retain their existing handling. Trace annotations
distinguish `arrival_not_reached` from `unsafe_geometry` without exposing raw
error text. This prevents repeated unverified positioning attempts; it does
**not** claim to repair the original Cloud outage or the device's arrival error.

## Comparison

The captured prompt did not retain the random seed. Replays use its numeric
geometry over the full 64-cycle horizon with three explicit seeds; they are
reconstructions, not exact copies of the historical trajectory. The matrix has
108 cases across Original Handy and both Handy 2 profiles, four speeds, three
seeds and field/full-directional/regional-phrase scenarios. All 108 were run
against alpha.81, direct common-clock fitting, and the retained local field fit.
No case became slower. The direct common-clock alternative alone made only a
small difference and did not resolve the field bottleneck.

Representative Original Handy field, seed 17:

| Base setting | Alpha.81 effective pace | Retained effective pace | Mean-travel increase |
| --- | ---: | ---: | ---: |
| 25% | 14.24% | 14.24% | 0% |
| 60% | 18.22% | 28.63% | 40.3% |
| 78% | 18.13% | 31.99% | 53.9% |
| 100% | 18.56% | 38.39% | 75.7% |

The remaining gap includes requested modulation and short reversing travel.
The low-speed sample stays identical. These estimates are not a promise of
matching a manual application's percentage scale in every pattern.

## Validation and artifacts

`TestDirectionalFieldLimitsSlowdownToNeighboringTurns` checks both direction
orientations, no acceleration discontinuity, kinematic limits, no authored leg
speedup, and local contrast preservation around a constrained short stroke.
Existing directional tests retain both directions, all three Handy profiles,
1/25/55% slow-leg rates, widths 10/30/100, and field/stroke/region grammars.
The regional whole-clock ratio regression remains unchanged.

The 375-case `TestMotionPathReview` passes its encoded velocity, missing-turn
and wire-error checks, including Cloud, generic immediate and Handy Intiface
sampling. Shared Stop/lifecycle tests and the fake transport remain the runtime
validation path. Startup tests assert the typed arrival failure; mode tests
cover both failure categories while teardown is blocked and verify a fresh
explicit start can still work afterward.

Ignored local artifacts are under `.scratch/pace-recovery-20261031/`:
`pace-before.json`, `pace-direct.json`, `pace-local.json`, `path-final.json`,
the source overlays used for comparisons, the redacted diagnostic report, test
logs and `visual-review/`. The visual atlas retains all 699 evaluated cases
(including the insufficient direct-clock alternative), with every distinct
trajectory on an overview and detailed before/after outliers. Curves are
commanded estimates; wire interpolation is not carriage telemetry.

All 26 overview sheets and eight detailed plots were inspected. The captured
field keeps its endpoint sequence and intentional slow sections, with less
unnecessary time stretching and no added rests. Regional phrases keep their
counts and transfers, and full directional strokes retain rounded reversals.
The detailed curves show no velocity or acceleration discontinuity at knots;
their extrema remain within the existing budgets. Sparse wire approximations
continue to track the intended position curves. The more pronounced acceleration
changes in faster examples still need physical assessment; plotted continuity
alone cannot establish perceived smoothness.

Final validation passed: `go test ./...`, `go test -race ./...`, `go vet ./...`,
`golangci-lint run` (zero issues), the pure-Go production build, frontend
typecheck, all 1,082 frontend tests, and the canonical frontend production build.
Architecture and goroutine lifecycle checks are included in the Go suites.
All 108 cases passed for each of the three timing implementations, and all 375
shared motion-path cases passed. No evaluated selection was dropped from the
atlas because it was slow or unhelpful.

The isolated current-source review app is `http://127.0.0.1:50337/#/chat`, with
simulated hardware and LLM motion off. `check-review-llm.ps1` completed real
generation on local Gemma 12B through Ollama. A separate application chat turn
completed in 486 ms with a non-empty reply, one provider call, no repair or
fallback, and no motion. This is readiness evidence, not a model latency study.

## Required physical follow-up

Before publication, compare alpha.81 and the candidate with identical seeds,
Cloud, the correct Original Handy profile and saved 0–97% window. Include the
captured alternating field at 25/60/78/100%, both directional orientations,
full strokes, counted regional transfers, a live pace edit and Stop. Record
turn quality, cadence contrast, append latency and the actual trace. Keep
unrelated devices disconnected.

Investigate the startup discrepancy separately with authorized read-only state
capture and a bounded lead-in audition. Retain the actual Cloud API failure
code, stroke calibration, commanded endpoint and repeated slider readings.
Do not remove fail-stop, widen tolerances or automatically compensate positions
to make an unexplained physical mismatch disappear. The installed app remains
unchanged until that test or an update is authorized.
