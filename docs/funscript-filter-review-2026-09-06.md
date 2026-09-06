# Funscript filter quality review — 2026-09-06

Parent: `72f8a9be` (video seek review). Scope: apply the continuous LLM modes'
lessons about compiled geometry, independent edits, honest effects and retained
failures to paired-video filters. No model contract or physical transport changes.

## Findings and implementation

1. **Rounded points were still abrupt motion.** The old filter emitted five
   quadratic fillet points into a linear curve. The shared engine now evaluates
   local Hermite windows with continuous position, velocity and acceleration.
   Zero settings preserve authored output. Direct reversals alone are rounded;
   holds and too-short corners remain exact. See [ADR 0027](decisions/0027-media-filter-interpolation.md).
2. **Jitter removal could delete the major reversal beside a tiny dip.** Media
   uses the existing efficient reversal bookkeeping with both amplitudes below
   the chosen threshold and both flanks at most 250 ms. Pattern import retains
   its established behavior. Deliberate slow subtle strokes survive.
3. **Measurements could be absent, stale or pre-cap.** Results now describe the
   compiled curve after the optional authored-delta speed cap. They include
   zero results, rounded/limited/skipped counts, reduced reach and local peak
   timing shift, paired with the applied settings. A compact derived value
   survives snapshots without copying up to 100,000 media points per poll.
4. **Controls could disagree with saved state.** Writes serialize, successful
   responses reconcile values, backend snapshots update idle controls, newer
   drafts survive older completions, and failed writes roll back and cannot
   auto-resume playback. An old player's queued save cannot manipulate a new
   player. Existing Stop and controller-loss fences remain in force.

## Numerical and visual review

The development-only atlas exports six synthetic authored shapes (wide,
asymmetric, subtle, chatter, dwell, dense), four policies (off, smoothing 3%,
rounding 60 ms, smoothing 3% + rounding 200 ms + speed cap 40%), playback rates
0.5/1/2 and Original/Handy 2 Standard/Handy 2 Pro profiles: **216 cases**.
Each candidate is compiled by the shared plan and buffered fitter. There are
no compile/fitter failures; all durations match baseline, positions stay in
0–100, all 54 filters-off controls match baseline samples exactly, and
rounding-only peak rates never exceed the authored rates.

All ten baseline/candidate overview sheets were inspected, followed by detailed
rounding plots for every shape at all three rates and the strong asymmetric
speed-cap and combined chatter outliers. Actual quantized wire points are
overlaid. At 1x on Original Handy, representative results are:

| Shape, rounding 60 ms | Old largest velocity jump (%/s) | New largest velocity jump (%/s) | Old / new highest position (%) |
| --- | ---: | ---: | ---: |
| Wide | 40 | <0.00001 | 87.600 / 88.200 |
| Asymmetric | 62.5 | <0.00001 | 87.563 / 88.114 |
| Subtle | 2.5 | <0.00001 | 51.850 / 51.888 |
| Chatter | 45 | <0.00002 | 89.514 / 89.645 |
| Dwell | 100 | 100 | 90 / 90 |
| Dense 8 ms legs | 1,000 | 1,000 | 51.878 / 51.909 |

Tiny nonzero measured jumps in rounded windows come from sampling immediately
on either side of a join. Tests directly verify the polynomial boundary values.
For wide 1x rounding, peak acceleration is 2,000 %/s² and finite-segment jerk
66,667 %/s³. Dwell shoulders and dense skipped corners still jump in velocity;
zero finite-segment acceleration on a line never establishes smoothness.

Visual conclusions and accepted limitations:

- Wide and asymmetric bodies retain their slopes while velocity turns
  continuously near the apex. Peak reach is slightly better than the old coarse
  fillet at equal windows. Asymmetric apex time can shift locally; this is
  measured rather than described as unchanged timing.
- Smoothing preserves the early major peak beside a small dip. The resulting
  short plateau remains visible. Slow 4-point subtle motion survives; quantized
  tips can still flatten, and wire segment velocity can differ from the plan.
- Deliberate high/low holds remain exact. Dense 8 ms legs are rounded at 0.5x
  where the local window permits it, but skipped at 1x/2x. These failures to
  smooth remain in the atlas and are counted in the app.
- Applying the optional speed cap before rounding makes the controls
  independent. It can cost additional reach: the asymmetric combined 1x case
  peaks at 69.857% versus 73.482% previously. Total clock duration remains exact.
  This accepted tradeoff is documented in the panel contract and ADR.
- C2 applies locally to rounded windows, not every possible authored join.
  This is not a global acceleration/jerk guarantee, and whole-percent buffered
  interpolation is not a measurement of physical carriage motion.

Reproduce the candidate from the repository root:

```powershell
go run -tags magichandy_labs ./cmd/motion-atlas -catalog=false -media-filters -output .scratch/funscript-filter-quality/candidate-final.json
python scripts/render-motion-atlas.py .scratch/funscript-filter-quality/candidate-final.json .scratch/funscript-filter-quality/candidate-final
```

Ignored evidence under `.scratch/funscript-filter-quality/`:
`baseline-atlas.json`, `baseline-final/index.html`, `candidate-final.json`,
`candidate-final/index.html` (216 cards, 73 distinct plots and five overview
sheets each), `comparison.json`, and `benchmarks*.txt`. Baseline export was
captured before replacing the filters; the candidate retains every same case.
Render tooling used Python 3.14, matplotlib 3.10.9 and NumPy 2.4.6 on the review
host; none ships in the app.

## App, lifecycle and validation

The isolated `--simulate-motion` review at `http://127.0.0.1:49947/#/videos`
uses a 90-second synthetic color-bar video with 133 authored actions. Browser
review exercised Play, rounding, smoothing, a backward seek to 12.050 s,
1.5x playback, the optional 80% cap, completion, replay and Emergency Stop.
The panel showed pending writes followed by actual applied results. One active
report showed 19 actions removed, 56 corners rounded, peaks up to 3 percentage
points lower and peak shifts up to 19.6 ms. The settings panel remained readable
with the longer report and Stop reachable.

`browser-trace.json` retains 178 rows / 155 simulator commands / eight streams,
with zero dropped rows and final command Stop. `browser-transition-trace.png`
plots actual dispatched semantic points; regions queued beyond Stop are shaded
as canceled. The transition plot was inspected: filter restarts use separate
streams, authored holds remain visible, and old queued work ends at Stop.
The three filter Stop-to-Play gaps were 189.520, 184.641 and 186.365 ms,
including the intentional 180 ms write debounce. The backward-seek and rate
restart gaps were 15.340 and 4.203 ms. Fake transport ACKs were 0 ms; these are
local lifecycle observations, not physical acquisition or network latency.

`scripts/check-review-llm.ps1` completed a real generation with local Ollama
`huihui_ai/granite4.1-abliterated:3b`, replying `ready`. LLM motion was off.
Previous review processes/data and browser tabs were preserved.

Validation passes: full `go test ./...`, `go test -race ./...`, `go vet ./...`,
golangci-lint, import-boundary/goleak tests, `CGO_ENABLED=0` stripped build,
Labs atlas/motion tests, frontend typecheck, all **497 tests in 67 files**, all
five locales, and the production frontend build. New regressions include 1,000
deterministic asymmetric/mirrored rounding cases, the 100k source cap, dwell and
too-short windows, all three device speed profiles, the real HTTP report, zero
and stale effects, save ordering/readback/failure, and failure without rearm.

No physical device was connected or commanded. Hardware feel and alignment
remain R25/M3 acceptance work. Shared Stop, physical startup acquisition,
transport ownership, default filters, global limits, LLM modes and the authored
timeline display retain their established responsibilities. See the
[performance and budget record](perf-baseline.md#2026-09-06--funscript-filter-quality).
