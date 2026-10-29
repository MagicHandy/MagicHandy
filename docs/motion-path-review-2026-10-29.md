# Motion pathway and top-speed review — 2026-10-29

This reviews the unfinished `claude/handy-top-speed` changes against alpha.79
`f21853ab`. It follows the owner's clarification that older filters **may** now
hurt motion and may be unnecessary after the successful sparse-point audition.
The inert review changed no installed-app settings, controller state, GPU
allocation or real hardware. The review app and CPU-only model worker use
isolated state. A subsequent explicitly authorized
[Cloud hardware comparison](top-speed-cloud-audition-2026-10-29.md) accepted
full-range, short and compound patterns at 100% on the Original Handy.

## Findings and adopted corrections

The main upper-speed bottleneck is the generated velocity profile, not a
transport multiplier. A rounded stroke reaches the peak-velocity limit before
its mean travel rate reaches the requested rate. Shorter cruise ramps can use
more of the stroke for travel. Merely increasing the acceleration/jerk budget
or removing old filters does not provide the same improvement.

The initial proposal did introduce a sparse-output regression. It estimated
acceleration by differentiating reference points that also contain the previous
rounded, retimed append anchor. That anchor need not lie exactly on the
mathematical curve at its new timestamp. A one-millisecond gap to the next real
probe can therefore imply an acceleration that the trajectory never has.
This consumed approximation tolerance and inserted unnecessary wire points.
133 ordinary-speed matrix outputs changed despite unchanged trajectories.

The sampler now carries an explicit position/acceleration tolerance. Its
acceleration reserve comes from the compiled polynomial, including an active
fitted join, with the old 7,500 %/s² reserve as a floor. Expired fast transition
history stops affecting the destination. All **198 cases at or below 75%** now
match alpha.79 exactly in sampled plan and transmitted points. The Original
Handy matrix's mean point density drops from **10.11 to 9.49 points/s** relative
to the initial proposal, without removing mandatory turns or relaxing the
position error bound. This is not a physical smoothness measurement.

The initial four ramp candidates also left coarse shape-selection steps.
The retained search uses nested 0.05 ramp-share increments above the knee.
Every eligible profile remains available at higher settings. A single moving
candidate was rejected: it discarded previously better joint fits and caused
larger speed regressions. Creative v2 keeps the original low-speed selection;
at high speed its minimum worthwhile improvement is measured against the
original rounded fit, not repeatedly against intermediate winners.

Pace diagnostics now use the actual content envelope. Legacy content and
content with tighter limits no longer get labeled against a prepared
generator's wider top-speed limits.

## Controlled alternatives

Every steady comparison used the same 375 fixtures and the shipping sampling
policy, including Original Handy Cloud's accepted sparse preference. An earlier
review helper sampled the conservative policy first and could reject output
before trying the intended transport policy. The helper now selects the policy
before sampling. The matrix also covers generic 50 ms immediate dispatch and
Handy Intiface's 110 ms constraint. A separate 315-case matrix uses the actual
native/browser Handy BLE capabilities: buffered HSP with whole-percent points,
not immediate dispatch. The final Cloud pass uses the actual 100-point append
cap and produces identical output to the initial 128-point review ceiling.
No alternate motion loop, hardware dispatch or language classifier was added.

| Alternative | Evidence | Decision |
| --- | --- | --- |
| Shorter ramps, old derivative budget | Full-range 100% remained 78.7% effective; compound 61.8% | Insufficient on its own |
| Wider derivative budget, ordinary ramps | Full range remained 78.7%; compound reached 70.4% | Insufficient on its own |
| 1.5× acceleration / 2× jerk plus ramps | Full range reached 80.2%; compound 69.9% | Retained as a more conservative physical comparison candidate |
| Original 3× / 8× proposal plus shorter ramps | Full range 86.6%; compound 74.1% | Retained as a development candidate with the corrections above; physical acceptance open |
| Remove field-lobe cleanup | 30 trajectories changed; some combined patterns became about 2.4 effective percentage points slower and acquired tiny reversals | Rejected; region-transfer regression also failed |
| Remove final quantized refinement | 114/375 outputs exceeded their position-error budget, including roaming patterns | Rejected |
| Remove crossing-time alignment | No mean-speed gain; mean Cloud RMS position error rose from 0.628 to 0.692 percentage points in the initial-proposal comparison | No clear improvement; retained alignment |
| Remove transition chatter cleanup | Steady output unchanged, but Full sweeps → Traveling window produced an unwanted 1.438% reversal with a 50 ms flank | Rejected |

Field cleanup and transition cleanup act on different defects from excessive
point density. Their continued usefulness was tested; it was not assumed from
their names or age. Media-specific filters do not run in the Layered path and
were not removed as a supposed Layered fix.

## Final estimates and limitations

Original Handy, requested 100%, identical authored fixtures:

| Pattern | Alpha.79 effective pace | Final effective pace |
| --- | ---: | ---: |
| Full range | 78.67% | 86.63% |
| Two upper strokes / one full stroke | 61.81% | 74.10% |
| Default Layered field | 67.86% | 78.03% |
| Creative v2 | 60.48% | 71.19% |
| 20%-wide stroke | 38.63% | 41.30% |
| 10%-wide stroke | 21.40% | 21.40% |

These are calibrated equivalents of **commanded mean travel**, not motor
telemetry. No reversing stroke can average its peak velocity for an entire
cycle. Very narrow strokes still meet the reversal-spacing floor. Increasing
their request beyond that floor cannot create additional speed. This work
does not increase transport capacity, change device calibration, alter the
saved speed/range limits, remove authored softness or rest requests, or weaken
Stop. The 3×/8× envelope is a software turn-quality choice, not a published
manufacturer acceleration/jerk rating.

## Validation and artifacts

The opt-in development test `TestMotionPathReview` covers full/short/minimum
width, compound, regional/mirrored, directional, pace-wave, roaming/combined,
sections, explicit softness/cadence, Creative v2, experimental stroke score and
Freestyle. Speeds are 5/25/60/75/85/92/100 on Original and 25/75/85/100 on both
Handy 2 profiles. Selected fixtures also exercise generic 50 ms immediate and
110 ms Intiface sampling; buffered native/browser BLE has its own matrix.
It checks encoded velocity, mandatory turns, and buffered error at one-millisecond
increments; failures remain in the report. No missing turns, velocity-limit
violations or position-budget failures were found in the retained 375 cases or
the 315 additional native/browser BLE cases. The existing release and session
review matrices add another 244 passing cases; their three overview sheets
were also inspected.

Additional checks include a 390-fit 75–100 integer-speed sweep, the full
existing compiler/transport/Stop lifecycle suite, 180 Edge/slowdown join cases
(all used monotone joins), rapid retarget history, and a 350 ms fake-transport
append-latency run from slow to fast compound motion followed by Stop. The
fake-transport capture is command evidence, not physical response time.

Ignored artifacts live under `.scratch/top-speed-followup/`:

- `alpha79.json`, `claude.json`, each named ablation, `final.json`: scores,
  full-loop plan samples, wire points and numerical metrics.
- `followup.json`, `rapid.json`, `runtime.json`: joins and runtime/Stop traces.
- `visual-review/index.html`, `manifest.json`, `overview-01.png` through
  `overview-19.png`: 4,125 comparison records, including rejected variants.
- `visual-review/detail-*.png`: 30 detailed position/velocity/acceleration
  comparisons. All overview sheets and representative low/high, compound,
  directional and filter-removal outliers were inspected. No extra rests,
  lost counts, overshoot or C2 seam jumps appeared in the retained output.
  Unfiltered field lobes visibly introduced near-stationary transfer shoulders.
- `native-ble.json`, `visual-review/ble-overview-*.png`: all 315 additional
  actual buffered BLE-policy cases, grouped by authored shape.
- `release-matrix.json`, `session-matrix.json`,
  `visual-review/supplement-*.png`: 244 supplemental release/session cases.
- Test, race, vet, lint, frontend, build and review-model logs accompany them.

Validation passed: `go test ./...`, `go test -race ./...`, `go vet ./...`,
golangci-lint, formatting/diff checks, the CGO-disabled production build,
frontend typecheck/build and all 1,069 frontend tests. Architecture and
goroutine-lifecycle tests remain enabled. No hard gate was relaxed. The
[scorecard](goal-scorecard.md) records binary size and compile/sampling costs.

Reproduce the steady final matrix without connecting a device:

```powershell
$env:MAGICHANDY_PATH_REVIEW = Join-Path (Get-Location) '.scratch/motion-path.json'
go test -tags magichandy_labs ./internal/motion -run '^TestMotionPathReview$' -count=1
python scripts/render-motion-atlas.py .scratch/motion-path.json .scratch/motion-path-atlas
```

The isolated current-source app is at `http://127.0.0.1:50312/#/chat`, with
simulated devices and LLM motion off. `check-review-llm.ps1` completed real
generation; a separate application chat request completed in 5.043 s with one
provider call, no repair/fallback and no motion. This CPU-only readiness probe
is not a production inference latency comparison.

## Physical acceptance before release

The [first hardware comparison](top-speed-cloud-audition-2026-10-29.md) is now
complete: at 100%, B was faster and remained smooth in the three matched
patterns, with the compound rhythm retained. The remaining coverage below is
still open; the result is specific to those scenarios and one Original Handy.

Keep the installed session unchanged until a dedicated audition is authorized.
Compare alpha.79, the conservative 1.5×/2× variant, and the final candidate on
Original Handy Cloud at 75/85/92/100 with the same 0–100 window and identical
full, 20%-wide and counted compound requests. Include explicit slow return,
Edge entry/exit, a high-to-low edit, a narrowed range, and Stop between runs.
Record append/start latency, exported motion trace, settings and actual user
observations of turnaround harshness, hesitation, pattern distinctness and Stop.
Keep COYOTE disconnected. Subsequent Handy 2, native/Intiface and simultaneous
device acceptance is separate. The old sparse-output audition does not approve
these newly sharper top-speed reversals.
