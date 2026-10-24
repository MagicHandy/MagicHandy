# Slow Layered motion and direction changes

Update (October 24): the [physical comparison and production review](cloud-sampling-review-2026-10-24.md)
supersedes the sampler candidate below. The maintainer preferred sparse D,
confirmed the final production runs and Stop, and requested release. Extra
velocity refinement and the shorter-turn prototype are not shipped. Some turn
hesitation remains. The following records the original investigation.

Original October 23 status: **software evaluation complete; physical acceptance pending**. On
October 23 the user asked to defer hardware tests and document them for the next
update. Do not release the motion changes below until the physical checklist
passes. No device was connected or moved during this investigation, and the
installed application was not restarted or reconfigured.

## Installed-session evidence

Switching the installed alpha.71 session from GPT-6 Sol to GPT-6.1 Sol restored
replies. Three subsequent motion replies completed in 8.860, 11.893 and 7.388
seconds. The last response selected the requested two upper-region strokes
followed by a full sweep. The captured commands reach semantic 0 and 100; the
user's configured physical stroke window remained 0–88.

However, the same motif alternated between very different travel rates while
the displayed base speed stayed at 58. Short half-strokes ranged from 276 to
1,053 ms; a slow full half-stroke took 3,759 ms. Its average commanded travel
was 26.6 percentage points/s, with individual endpoint segments around
3.6–4.2 points/s. These are commands, not carriage measurements. The trace
does not retain the accepted original FlowSpec, so its exact layer parameters
cannot be recovered; a 12%-of-base-rate fixture closely matches the slow leg
timing, but is an analogue, not a replay of the original model JSON.

The captured transport results contain 112 successful operations, median
latency 322 ms and maximum 744 ms. The minimum submitted tail lead estimated
relative to Play acknowledgement is about 1.99 s. This is not device-buffer
telemetry, but the capture does not show failed appends, recurring stream
restarts or an obvious starvation interval matching the cyclic slowdowns.

## Reference preset

The user identified **Handjob (simple)** at
[The Handy App](https://thehandyapp.ddns.net/#/presets) as physically smoother.
Read-only inspection of its public application source found:

- Three base actions, 0 → 100 → 0, with 350 ms per half-stroke at its nominal
  100 setting. Its slider spans 10–130 and applies `(200-setting)/100` to time.
- The minimum setting therefore has a 665 ms half-stroke / 1,330 ms cycle.
  It is substantially faster than the captured 7,518 ms slow full cycle;
  matching slider labels would not match physical timing.
- A two-dimensional Catmull–Rom spline over time and position is sampled into
  seven points for that basic loop. At minimum speed, the reconstructed
  time/position pairs are `(0,0), (172,33), (419,78), (665,100), (911,78),
  (1158,33), (1330,0)`. It uses larger segments approaching turns than our
  quantized slow oscillator. This is inspection of the command representation,
  not a measurement of its firmware interpolation or physical acceleration.
- It uploads a script and invokes looping HSSP playback. According to
  [Handy's firmware 4 explanation](https://www.thehandy.com/blog/firmware-4-is-here/),
  HSSP uses HSP internally on firmware 4. A separate transport implementation
  is therefore not justified by the reference alone.

No reference application code, SDK, assets or dependencies are incorporated
into MagicHandy. The downloaded source remains ignored inspection material.

## Changes proposed for review

### Scope timing to the pattern that requested it

The old hosted instruction explicitly preserved materialized pace timing when
a phrase replaced field geometry. Removing the regional center/range layer
could leave its coupled pace cycle operating on the entire new phrase. Merely
asking for explicit pace removal was insufficient: some model responses moved
the obsolete slowdown into a full-sweep group's rate instead.

The revised hosted instruction distinguishes timing tied to the replaced
regional pattern from an independent speed cycle or a still-requested regional
contrast. The model decides whether to remove, retain or translate it. Existing
phrase count/reach edits retain their unrelated rates; explicitly slower sweeps
and independently requested speed cycles remain available. No keyword rule,
minimum chosen speed, fixed regional width, or automatic layer deletion was
added. Local-model prompts are unchanged.

The surrounding hosted description was compacted so the strict Responses
round-trip fixture still fits its existing 32,768 context budget. The final
production wording passed **12/12 live GPT-6.1 Sol Low checks**, two repetitions
of six cases: replace upper/lower coupling, retain independent cycling, edit
counts only, request a deliberately slower sweep, and ask a non-motion
question. Each used one provider call, with no repair or fallback. Earlier
wordings and failed semantic selections are retained in the visual review.
This is a bounded intent sample, not a guarantee for arbitrary conversation.

### Refine representable velocity changes in the shared buffered sampler

The existing fitter primarily bounds position error. A low-speed curve can
meet that bound while its whole-percent wire segments have noticeable changes
in velocity. The candidate adds a point only when a representable split
substantially reduces velocity approximation error and still satisfies the
existing position and maximum-velocity bounds. It keeps original points,
times, reversals, append cutoffs and boundary-segment velocities. A frame is
left unchanged if its largest velocity step would increase or it exceeds its
point budget. Transitions and immediate transports use their existing path.

The final 60-case matrix covers three Handy profiles, speeds 10/25/58/90,
10/28/100-point steady spans and counted phrases at 25% and 12% group rates.
Compared with baseline, 29 cases improve RMS velocity error; none worsen the
measured RMS velocity error, maximum velocity step, maximum position error or
steady-stroke variation. Periods and authored turns are identical. For the
Original Handy slow phrase analogue at base 58 / group rate 12:

| Measure | Baseline | Candidate |
| --- | ---: | ---: |
| RMS commanded velocity error | 3.343 points/s | 3.072 points/s |
| Largest commanded velocity step | 22.219 points/s | 17.626 points/s |
| Submitted points in the comparison window | 95 | 111 |

That is roughly 8% less velocity error and 21% less maximum velocity step in
this fixture. **It does not establish physical smoothness.** Extra points can
affect firmware behavior differently from these numerical measures. Globally
denser spatial sampling was rejected after numerical regressions; an earlier
velocity-fit placement was rejected because it changed append boundaries and
introduced velocity/variation regressions. The final helper runs after the
original natural cutoff and protects each boundary segment.

### Broader turn-shape experiment, not enabled in production

`TestLayeredTurnProfileReview` compares the current rounded oscillator with
three C2 cruise profiles: short rounded acceleration/braking regions and a
more uniform middle. It retains exact leg durations, endpoints and shared
joint acceleration. A proposed leg is kept only within the existing velocity,
acceleration and jerk envelope; otherwise that leg retains the baseline.
The 144 inert cases cover steady and compound motion, four speeds and three
group rates. High-speed tight strokes often cannot adopt a shorter turn
without exceeding the current envelope, which is deliberately not relaxed.

For the base-58/rate-12 compound fixture, the 0.18-ramp profile reduces the
fraction of planned time below 20% of mean travel speed from about 8.8% to
6.6%, with unchanged period/reach. The first 12 seconds use 32 wire points
instead of 57. This is a plausible way to reduce creeping and speed modulation,
but it raises planned acceleration/jerk within the existing envelope and may
feel too abrupt. It remains a test fixture and an ignored comparison build,
not an automatic production profile or a new user-facing mode.

The stronger architectural direction is to improve the shared stroke fitter
and its quantized output, preserving timing intent separately. A cloud-only
mode would not fix the same engine/transport issue for local models, library
patterns or other motion sources. The reported motor minimum makes tiny slow
segments a plausible contributor, but this review does not claim the firmware
enforces that value in HSP or impose a speed floor to work around it.

## Reproducibility and visual review

Ignored evidence lives in `.scratch/slow-layered-61/`:

- `installed-motion.png`, `trace-analysis.json`: original trace and corrected
  fast/slow views. Private diagnostics remain outside git.
- `phrase-scope.json`, `phrase-replacement.json`, `phrase-confirmation.json`,
  `phrase-compact-confirmation.json`: all 42 live selections, including failed
  early interpretations. `model-atlas-final/` renders their shared plans.
- `sampler-{baseline,denser,velocity,stable}.json` and the baseline/stable
  `-extra` files retain all sampling experiments. `sampling-atlas/contact-01`
  through `05.png` cover all 60 cases, including rejected variants.
- `turn-profiles-final.json`, `turn-atlas-final/`: all 144 turn-shape cases;
  seven overview sheets and the distinct detailed plots. `turn-detail.png`
  compares slow full sweeps, the slow compound motif and a faster compound
  case. `reference-comparison.png` compares the public reference at its own
  minimum and at the captured slow cycle period.

Every overview was inspected, followed by detailed slow phrase/full-sweep,
fast tight-stroke and failed-selection plots. The final sampling changes
retain turns and clocks; the dense experiment introduces extra uneven wire
steps; cruise profiles flatten the velocity body but increase turn curvature.
No plot represents physical telemetry or substitutes for the checklist below.

To regenerate, use `TestHostedPhraseTimingScope` with `liveeval` and the
existing protected hosted credential source; the default variant is the final
instruction. `TestLayeredSmoothnessProbe` and `TestLayeredTurnProfileReview`
use `magichandy_labs`. With an output path and no custom input, the sampler
probe generates the complete built-in matrix; set
`MAGICHANDY_SMOOTHNESS_FULL_MATRIX=1` for all three Handy profiles. The turn
fixture takes `MAGICHANDY_TURN_PROFILE_REVIEW` as its ignored output path.
These fixtures construct no transport. Export the
four live reports with `cmd/motion-atlas -catalog=false -llm`, then render with
`scripts/render-motion-atlas.py` as described in
[motion visual review](motion-visual-review.md).

Regression tests check independently sampled wire error, original anchors and
clock, frame edges, capacity, transition exclusion and immediate-transport
exclusion at both 12% and 25% group rates. Full Go tests, vet, lint, pure-Go
build and the existing full frontend suite pass locally. Linux race execution
remains a CI requirement. The current isolated review app also completed a
real text-only local Gemma chat in 3.323 s with one call and no repair, fallback
or motion, and passed `check-review-llm.ps1`.

Claude Opus 5.5 Max was requested for a read-only second opinion, but its CLI
session quota was exhausted. No successful Claude review of this candidate is
claimed. Performance measurements are in the [scorecard](goal-scorecard.md).

## Physical acceptance required before the next update

**Not performed. User explicitly deferred physical tests.** Keep this work out
of a release until these results have been recorded. Resume only after a fresh
confirmation that The Handy is clear and installed-app playback is stopped.
Use the existing hardware-test profile without copying credentials. Do not
take over or stop the user's installed session. Leave COYOTE and additional
Handys disconnected for the first comparison.

1. Record model/firmware, transport, stroke limits, actual selected pattern,
   compiled cycle/leg times, startup latency, append latency and buffer lead.
   Original Handy on Cloud is the primary reproduction. Use identical stroke
   bounds across runs, first the user's 0–88 and then 0–100 if authorized.
2. Compare **A: baseline sampler**, **B: selective velocity refinement**, and
   **C: B plus the experimental 0.18 rounded-turn profile**. Separate builds
   are prepared under the ignored evidence directory. All use the same shared
   compiler, engine, sanitizer and transport. No direct device script loop is
   allowed. C is experimental and must not silently become a release default.
3. Start with steady full sweeps at a cycle time matched to the reference's
   1.33 seconds, rather than matching its slider number. Then test a slow full
   cycle around 7.5 seconds. Run each for 10–15 seconds with an explicit Stop
   and a short stopped pause between runs. Confirm physical Stop each time.
4. Repeat A/B/C with the two-short-upper/one-full-sweep motif at base 58 and
   group rates 12 and 25. Retain endpoints and counts; test both travel
   directions and repeated loop seams. Do not compensate by raising the
   selected speed, narrowing reach or removing intentionally slow sections.
5. Ask for assessment after each pair: creeping/stepping near turns, abrupt
   braking or acceleration, pauses/catch-up, uniformity in the stroke body,
   naturalness of the compound join, and correct reach/counts. Repeat if the
   observer missed a run. Retain negative feedback as well as preferences.
6. Check live retargets between wide/narrow and fast/slow patterns, entry/exit
   from Edge, ordinary Stop and Emergency Stop. Confirm no delayed restart or
   residual commands. Repeat representative cases on native Bluetooth; verify
   the previously accepted Intiface spacing remains unchanged. Multi-device
   failure/Stop behavior remains covered by existing software tests and should
   receive a short regression run if a shared path changes further.
7. Capture each run's trace, physical feedback and exact source revision.
   Accept B only if it improves or matches A without a physical regression.
   Adopt C only after independent physical acceptance and a separate reviewed
   production implementation. If B/C are worse, retain the prompt/feedback
   fixes and revert the unaccepted motion candidate rather than declaring
   success from numerical metrics. CI, race checks and PR review must also be
   green before merge/release.

| Acceptance item | Status |
| --- | --- |
| Cloud A/B steady, including very slow turns | Pending |
| Cloud A/B compound counts, reach and loop seams | Pending |
| C turn-profile comparison and naturalness | Pending; experimental |
| Retarget, Edge and Stop on hardware | Pending |
| Native BLE / Intiface regression | Pending |
| Physical feedback and trace attached to review | Pending |
