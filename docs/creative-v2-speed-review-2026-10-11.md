# Creative v2 speed and Cloud sampling review — 2026-10-11

The user reported that 75% felt slow with **Cloud and Creative v2**, despite
the model selecting the intended speed. The Handy generation was not supplied.
This review fixes two reproducible software defects: premature saturation of
the rounded Creative v2 profile, and rejection of valid fast curves at an
artificial sampling partition. It does not establish physical feel on the
user's hardware. Changes remain on `codex/asynchronous-motion-review`.

## Reproduction and results

Tests use the shared compiler and sampler, all three saved Handy profiles,
0–100% physical-window settings, and a 1–100% allowed speed range. The table
shows the Original Handy profile at requested 75%. Effective pace is the
backend's calibrated **commanded mean travel**, not measured carriage speed.

| Creative v2 phrase | Before effective pace | After effective pace | Before mean travel (%/s) | After mean travel (%/s) |
| --- | ---: | ---: | ---: | ---: |
| Full range, no focus mix or variation | 56.72% | 74.82% | 217.39 | 278.55 |
| Narrow 20% range | 29.85% | 29.85% | 126.58 | 126.58 |
| Default varied gesture | 50.63% | 53.64% | 196.81 | 206.99 |
| Strong directional contrast and inertia | 24.59% | 29.66% | 108.81 | 125.94 |

Full-range mean travel increases about 28%; default varied travel about 5%;
the directional fixture about 16%. Layered and the original Creative compiler
have identical numerical output in this matrix. The narrow fixture is exactly
unchanged, as are unconstrained full strokes through 50% in the regression matrix.

The old full-range gesture produced the same 56.72% effective pace at requested
75% and 100%. Its rounded, inertia-warped velocity crest exhausted the device
ceiling while much of the stroke remained substantially slower. Cloud preserved
that slow curve's timestamps. Changing the LLM's speed selection or multiplying
HSP playback speed would not fix the profile while preserving its limits.

At 75%, the new full-range curve has peak velocity 345.34%/s, peak acceleration
about 6,207%/s² and peak jerk about 145,860%/s³. These remain below the unchanged
Original profile velocity ceiling (363.64%/s), acceleration ceiling (7,500%/s²),
and jerk ceiling (150,000%/s³). Position, velocity and acceleration remain
continuous at joins. Reversal spacing retains the existing 100 ms floor.
At requested 100%, this fixture reaches 75.28% effective pace before the jerk
limit binds. Rounded reversals, narrow travel, rests and directional contrast
still prevent arbitrary mean-speed targets; the backend continues reporting
effective pace and limiting reasons.

## Compiler correction

The existing rounded profile remains the starting point. Only when physical
fitting extends the phrase beyond its requested duration does the compiler
try four bounded cruise blends. Longer strokes can spend more of their body
at a useful velocity, with cubic velocity ramps into and out of that body.
Inertia shifts the ramps; it does not create another velocity spike in the
cruise. Shared endpoint acceleration is incorporated into the ramps, preserving
C2 joins without a whole-stroke correction that would recreate the original
crest. The fitted curve includes the ramp/body join times explicitly.

Selection checks both the individual leg's exact kinematic floor and the
assembled phrase. A candidate must compile, preserve valid monotonic legs, and
reduce total duration by at least 0.5%. It cannot alter reversal positions,
turn count or intentional rest durations. The existing leg weights continue
expressing breathing and directional contrast. Strokes no wider than 20% keep
the original profile. The normal shared sanitizer, physical budgets and Stop
path still apply. No LLM prompt, schema, model selection or transport payload
contract changes.

Three early prototypes are retained in the visual evidence. Whole-stroke
acceleration correction recreated a velocity hump; asymmetric native endpoint
acceleration weakened the cruise benefit; unconditionally selecting a cruise
amount slowed some directional phrases. These were rejected. Final selection
evaluates the complete fitted phrase instead of assuming a locally faster
profile is always globally better.

## Shared sampler and Cloud contract

Whole-percent rounding can make two fixed sample points require an impossible
interval near the velocity ceiling. The buffered sampler had treated an
**unsent batching boundary** as if it were a required motion knot. This could
reject both the old varied gesture and the faster candidate.

The ordinary fit still prefers the established partition. If that fit fails,
it retries the same bounded solver without the artificial partition anchors.
The already-submitted tail, true motion knots and reversals stay fixed; the
velocity bound is unchanged. The next append boundary is chosen from the
accepted fitted samples. A separate refinement reserves the analytic
between-probe interpolation error (`A × dt² / 8`), so the existing 0.8% wire
position-error bound also holds between 5 ms probes. A 1 ms regression check
caught an earlier 0.802% outlier; the tolerance was not relaxed.

The [current manufacturer API v3 specification](https://www.handyfeeling.com/api/handy-rest/v3/docs/spec.yaml)
was inspected on October 11. HSP points carry millisecond timestamps relative
to the script start, and playback rate 1 preserves their speed. The existing
Cloud builder preserves sampled timestamps, rounds positions once, and caps
adds at 100 points. No extra velocity multiplier belongs at this boundary.
`PointPosition` currently has a conflicting numeric `maximum: 50` alongside
its 0–100 description; it was not used to introduce a 50% clamp. The existing
[pinned manufacturer RPC/SDK review](bluetooth-intiface-review-2026-09-20.md)
and transport tests retain the 0–100 integer-percent contract.

Physical stroke-window scaling also affects millimetres travelled. It remains
a transport envelope, not an extra point scaling or a reason to silently
increase speed during a range edit. The user's actual profile, stroke window,
network timing and carriage response still require a physical trace to assess.

## Verification and visual review

- Normal regression coverage checks three device profiles, four gesture
  characters and ten requested speeds for geometry, intentional rests,
  low-speed identity, physical limits and monotonic pace.
- The buffered regression emits 20 seconds of both the new full-speed curve
  and the previously failing old varied curve on every profile. It checks
  positive timestamp spacing, whole-percent wire velocity and position error
  at every millisecond, including append boundaries.
- The final inert atlas contains 72 cases: three profiles × four speeds
  (25, 50, 75, 100) × six characters (four Creative v2 fixtures, Layered,
  Creative). All compile and sample successfully. There are no model calls
  or model-selection claims in this timing experiment.
- A shared-engine runtime capture uses Cloud's real sampling/timing
  capabilities with a fake owner and **simulated 350 ms append latency**.
  It starts at 75%, applies 90% during the same run, and Stops. The capture
  records eight appends, one Play, one stroke-window command and one Stop;
  no command follows Stop. Thirteen trace rows and pure Cloud request-builder
  mapping examples are retained. This is not a physical Cloud session or a
  measurement of the user's current network latency.
- `go test ./...`, `go test -race -timeout 20m ./...`, `go vet ./...`, normal
  and labs-tag lint, the pure-Go build, frontend typecheck/build and all 847
  frontend tests pass. The opt-in atlas/runtime tests also pass under race.

All runtime artifacts remain ignored under `.scratch/speed-review/`:

| Evidence | Local artifact |
| --- | --- |
| Original behavior, including wire-fit failures | `baseline.json`, `baseline.log` |
| Rejected compiler prototypes | `cruise-candidate.json`, `joint-cruise-candidate.json`, `continuous-cruise-candidate.json` |
| Selected compiler before the partition correction | `fitted-cruise.json` |
| Final 72 cases | `final.json`, `final-labs-race.log` |
| Failed strict wire check retained for comparison | `wire-fidelity.log` |
| Start / retarget / Stop commands and trace | `runtime.json` |
| Combined visual review | `atlas.json`, `atlas/index.html`, `atlas/manifest.json` |
| Compilation cost | `compile-benchmark.log` |

The combined atlas retains 432 entries, including failures, with 183 distinct
detailed plots and ten overview sheets. All ten sheets were inspected, followed
by detailed baseline/full-75, final/full-25, final/full-75, final/varied-75,
final/directional-75 and Standard/full-100 plots. Representative detailed files
are `motion-1c7da79e43aab8d4.png`, `motion-241c0b202c7fe5bb.png`,
`motion-da0f882dedd53494.png`, `motion-5ea3b0af5a0bc398.png`,
`motion-0eabf529a243415b.png` and `motion-0c58c7778daf84d8.png` in `atlas/`.
Identical sampled curves share images; the manifest retains each requested speed.

The final plots show a broader useful velocity body in fast wide strokes,
rounded turns without extra reversals, and zero velocity/acceleration knot
jumps. Low-speed full strokes retain their previous curve. Varied reach and
holds survive; deliberate directional imbalance remains visible. Quantized
output follows the planned curve closely. Higher acceleration and finite jerk
are expected from faster travel and remain within the original bounds. These
plots validate commanded shape, not subjective comfort or actual motor tracking.

## Cost and review build

The constrained full-range fixture takes 13.8–15.3 ms to compile versus
1.3–1.5 ms before, allocating about 6.77 MB cumulatively versus 0.87 MB.
That bounded cost occurs when preparing a constrained phrase, not per sample
or through an extra model call. At 25%, allocation stays unchanged and compile
time changes from about 0.89 to 1.00 ms. See the
[scorecard](goal-scorecard.md) for binary and process measurements.

The isolated review app is `http://127.0.0.1:50226/#/chat`, with the motion
simulator, LLM motion Off and Autopilot Off. It uses the available local Ollama
Gemma 12B model without copied account credentials. The readiness script passes;
a real text-only HTTP chat request returns a non-empty reply in 322 ms with
one provider call, no repair, no fallback and no motion. Evidence is in
`review-llm.log` and `review-chat.sse`. The review app does not connect hardware.

To regenerate the inert captures, from the repository root:

```powershell
New-Item -ItemType Directory -Force .scratch/speed-review | Out-Null
$env:MAGICHANDY_SPEED_REPORT = Join-Path (Get-Location) '.scratch/speed-review/final.json'
$env:MAGICHANDY_SPEED_RUNTIME_REPORT = Join-Path (Get-Location) '.scratch/speed-review/runtime.json'
go test -tags magichandy_labs ./internal/motion -run '^TestSpeedReview(Runtime)?$' -count=1 -v
python scripts/render-motion-atlas.py .scratch/speed-review/final.json .scratch/speed-review/final-atlas
```

Use the optional plotting dependencies documented in
[motion visual review](motion-visual-review.md). Add `-race` with a configured
Windows C compiler to repeat the instrumented runtime test; the shipped core
still builds with `CGO_ENABLED=0`. Report paths are restricted to `.scratch`.
Matched real-device A/B evidence remains the acceptance follow-up in R1.
