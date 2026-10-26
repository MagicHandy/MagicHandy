# Layered follow-up to the alpha.75 independent review

Baseline: alpha.75, `37775586`. This follows Claude Opus 5.5 xhigh's independent
review recorded on PR #352. Its findings were hypotheses from code inspection;
the reproductions and measurements below were performed separately. Four
changes showed a clear benefit and are implemented. No hardware was connected
or moved during this follow-up, and these changes have not been released.

## Accepted changes

| Finding | Reproduction and result | Implementation |
| --- | --- | --- |
| Intentional pace variation called a physical limit | Selected 10%, variation 25% compiled to 7.78% equivalent mean pace with no extra fitting delay, but reported `limited: true`. Variation, pace layers, phrase rates and cadence hold now report their planned average separately. | Preserve the authored duration in the compiled Layered curve. Compare the fitted mean with the planned mean when deciding whether physical limiting occurred. The selected speed is unchanged. |
| Fast fitting reshaped feasible slow legs | Mixed full/full fixtures changed an unconstrained slow leg by up to 4.53 position points relative to its original joint fit. The accepted version preserves its normalized shape to numerical precision. | Only consider cruise shaping for legs whose original fit exceeded the requested duration. Refit their common turning state through the existing joint fitter. |
| Slow joins could use the full fast-motion jerk allowance | Ten of 180 Edge/pace transitions change with a plan-relative envelope. Affected gentle Edge entries reduce peak jerk by about 47–84%, without adding a fallback. | Use the greater of the established gentle limit and twice the two plans' own derivative peaks, capped by the existing global limits. The existing velocity checks remain. |
| A completed buffered bridge forced an oscillator crossfade | In a 35→85→7% rapid-edit fixture, the older bridge is still playing but ends before the next queued handoff. The alpha.75 fallback adds two extra direction changes. The bounded join removes them. | Decide eligibility at the future handoff, while retaining the previous transition for already-queued playback and snapshots. |

The UI still shows **Estimated pace**, with both effective and selected values
when they differ. Its tooltip explains intentional variation, for example:
“Effective 8%; requested 10%. The pattern plans an average pace of 7.8%.”
The commanded-estimate qualification remains; this is not motor telemetry.
Genuine fitting limits still show the existing limiter explanation. Other
motion families without authored-duration metadata retain their prior logic.

The slow-leg change has a small timing tradeoff in the mixed fixtures: the
largest observed increase is 6124→6144 ms per loop, about 0.33%. Full, short and
two-short/one-full selected-100 fixtures previously accepted on Original Handy
Cloud retain **4116/1536/2660 ms** six-cycle periods exactly. Explicit softness,
phrase counts, destinations, rate contrast, calibration, sparse Cloud sampling
and transport playback rate are unchanged. No prompt or language heuristic is
introduced, and the shared compiler applies regardless of model provider.

## Alternatives and limits

The first relative envelope used the plans' peaks without headroom (1×). It
changed 47/180 joins and delayed some fast reductions until another turn; for
example, one full 100→5% case added 289 ms before beginning the changed stroke.
The 2× candidate preserves those tested fast joins while correcting the gentle
outliers. Both alternatives and alpha.75 remain in the comparison captures.

The accepted envelope is not cost-free in every case. At short-stroke 25%,
phases 0.07 and 0.57, it finishes the old turn for **30 ms**, then makes a
gentler joining stroke. The complete bridge grows from 39 ms to 368/367 ms;
that is not 368 ms of waiting. The rendered path has no extra reversal or
inserted hold, and the acceleration spike is smaller, but physical feel needs
confirmation. At short-stroke 5%, phase 0.07, the immediate join grows from
61 to 86 ms and peak jerk falls from 137541 to 23149 %/s³.

The existing Cloud buffer can still delay the first unsent change. This work
does not erase queued device points or change Edge's buffering semantics.

## Tests and visual review

- **19 steady cases**, compared before and after: three mixed geometries at
  five relative rates, plus four deliberate-variation levels. All four overview
  sheets were inspected, with detailed full/full 15% and wide/short 35% plots.
  Slow legs retain rounded profiles; rapid legs retain their cruise body;
  exact seam velocity/acceleration remains continuous.
- **180 transition cases**, each rendered for alpha.75, the rejected 1×
  envelope and the accepted 2× envelope. Full, short and compound motion at
  1/5/25/100%, ten handoff phases, Edge entries and 100→1/5% reductions.
  All ten overview sheets and gentle, delayed and fast-drop outliers were
  inspected. All 180 use bounded joins; no fallback is hidden by the report.
- **108 additional fast-drop regression cases** cover all three Handy
  profiles, three geometries, two destination speeds and six phases. Incoming
  and outgoing position, velocity and acceleration, exact polynomial limits
  and shared-sampler wire velocity are checked.
- **Direct interior-join regression** requires a nonzero outgoing velocity and
  verifies that the next authored turnaround remains. Wire knot timestamps are
  integer milliseconds, so its positional check allows 0.01 point rounding.
- **Rapid buffered retarget regression and plot** preserve every sampled point
  of prior history while replacing the unnecessary crossfade. The plotted
  extra reversals after the second handoff disappear in the candidate.
- **132-case release matrix**, 122 distinct plots/eight inspected overview
  sheets, covers all Handy profiles, low/middle/high pace, softness, cadence,
  pace fields, regional drift, compound motifs and section changes. All exact
  kinematic, seam and Cloud-sampler checks pass.
- **Runtime capture** uses the real shared engine with a fake transport and
  350 ms command latency: compound 7→100%, one playback stream, retarget and
  Stop. The in-flight append cancellation at Stop is retained in the trace;
  the engine ends stopped without restarting.
- **18 immediate-transport cases** cover the three fast accepted fixtures,
  three Handy profiles and hard 50/110 ms minimum command intervals. Their
  commands respect both timing and velocity constraints. A 200 ms minimum
  cannot represent the Original Handy's 128 ms short legs; a regression
  requires explicit rejection instead of silently dropping reversals.
- **Contended Stop** begins while an expensive Edge retarget owns the planning
  lock. It completed in 11.26 ms on this host with a zero-latency fake transport;
  no stale planner revived motion or sent a subsequent device command.
- Full Go tests, full race tests, vet, lint, architecture/lifecycle checks,
  pure-Go build, TypeScript/localization checks and **1065 frontend tests** pass.
  The canonical embedded UI was rebuilt.

The current-source isolated review app uses simulated motion and Ollama
`huihui_ai/granite4.1-abliterated:3b`, with LLM motion and Autopilot off. The
provider readiness probe completed real generation. A text-only app chat
returned a non-empty answer in **106 ms**, one provider call, no repair,
fallback or motion. The visible simulated pace readout and its tooltip were
checked against the backend's 7.77996% planned/effective estimate and
`limited: false`; playback was then stopped. No user credentials were copied.

## Cost and Stop assessment

Three 300 ms benchmark repetitions, GOMAXPROCS=8 on the review Ryzen 9950X3D,
compare the alpha.75 fitter via an ignored Go overlay against the candidate.
Selected-100 full/short/compound compilation medians are **0.850/0.201/1.088 →
0.873/0.189/1.118 ms**. Selected-5 medians are **0.135/0.149/0.062 →
0.136/0.139/0.064 ms**. Transient allocations are effectively unchanged.
These small differences are not a measured end-to-end response regression.

The expensive compound Edge join benchmark has median **9.85 ms**, about
2.05 MB transient allocations. A deliberately impossible zero-jerk fitting
budget exhausts one duration search in **0.076 ms** median; it is a stress
fixture, not a valid physical-device scenario. Neither benchmark proves a
universal worst-case Stop bound or a low-end CPU guarantee. The measured
contention does not justify changing the engine's locking architecture in this
patch. A future move outside the lock must account for handoff advancement and
stale plan commits; it should not be introduced merely to remove a lock.

Binary/UI measurements are in [the scorecard](goal-scorecard.md). No dependency,
worker, timer, inference round trip or second motion loop is added.

## Remaining physical checks

Before describing these new transitions as physically accepted, compare the
gentle Edge outliers, rapid repeated edits and maximum-to-gentle reductions on
Original Handy Cloud. Record the physical stroke window, latency, trace,
direction-change feel and both Stops. Follow with Intiface/native BLE and
Handy 2/Pro when those devices are available. The prior installed-app versus
matched-run full-range speed discrepancy remains unresolved; these fixes do
not establish its cause.

Automatic retiming for a device whose minimum command interval exceeds a true
stroke duration remains separate work. Simply dropping turns or globally
stretching a partially buffered plan would damage pattern identity and can
break synchronized playback. The current explicit failure is retained.

## Reproduction and retained evidence

Ignored captures under `.scratch/layered-followup/` include
`baseline-expanded.json`, `candidate-1.json`, rejected `candidate-gentle.json`,
accepted `candidate-gentle-2.json`, `rapid.json`, `release-matrix.json`,
`runtime.json`, all test/benchmark logs, `budgets.json`, atlas directories and
`transition-plots/`. Initial harness-only failures (unsupported variation 80
and an excessively strict integer-knot assertion) are retained separately.

```powershell
$env:MAGICHANDY_LAYERED_FOLLOWUP_REPORT = Join-Path (Get-Location) '.scratch/followup.json'
$env:MAGICHANDY_LAYERED_RAPID_REPORT = Join-Path (Get-Location) '.scratch/rapid.json'
$env:MAGICHANDY_LAYERED_RELEASE_REPORT = Join-Path (Get-Location) '.scratch/matrix.json'
$env:MAGICHANDY_LAYERED_RELEASE_RUNTIME_REPORT = Join-Path (Get-Location) '.scratch/runtime.json'
go test -tags magichandy_labs ./internal/motion -run 'TestLayered(FollowupReview|RapidFollowupReview|ReleaseSpeedReview|ReleaseRetargetRuntime)' -count=1
python scripts/render-motion-atlas.py .scratch/followup.json .scratch/followup-atlas
python scripts/render-motion-atlas.py .scratch/matrix.json .scratch/matrix-atlas
```

The transition reports retain the shared engine's sampled paths, including
failed selections if any, for overlay plotting. Every plot depicts commands,
not measured carriage motion. This follow-up changes no model-selection or
prompt contract and claims no new model-response-quality result.
