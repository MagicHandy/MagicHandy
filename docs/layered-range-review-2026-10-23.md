# Layered range and counted-flow review — 2026-10-23

Candidate above alpha.68 (`7117327`), on `codex/layered-range-flow`.
No physical device was connected or commanded during this work. The installed
instance was inspected read-only; private conversation, credentials, network
addresses and downloaded reports remain outside git.

## Installed evidence

Six hosted replies used `gpt-6-sol` with Low reasoning. Each completed in one
provider call, without repair/fallback, in approximately 5.9–8.3 seconds. The
complete retained trace contained 256 rows with no dropped rows. This report
separates selected scores, dispatched samples, and physical behavior:

- The first three targets commanded both 0 and 100 repeatedly.
- A brief pace follow-up also changed geometry. Once its handoff completed,
  its strokes were predominantly about 15–85. That was an unintended loss of
  reach, not evidence that the transport always clipped the endpoints.
- The next base correction reached almost 0 but its upper turns were around
  86. A claim of full travel was therefore incomplete.
- The last target commanded 0 and 100 but produced variable groups of short,
  intermediate and full strokes. It did not encode the requested two-to-one
  sequence. An alternating range envelope cannot guarantee exact counts.

The startup device window was 0–100, with reported physical endpoints of about
5.00–102.83 mm. Per-target median Cloud append latency was about 326–335 ms.
There is no evidence here to justify changing physical end buffers, calibration
or the Cloud protocol. Commanded 0 is not proof that the carriage reached the
user's expected position; physical shortfall remains an observed-device check.

## Compared approaches

| Candidate | Finding | Decision |
| --- | --- | --- |
| Existing field grammar | Valid replies could lose geometry on a pace follow-up; exact counts were unavailable | Retain for uncounted variation, not exact motifs |
| Prompt-only guidance | Better coverage and preservation, but still no exact count representation | Keep useful guidance; insufficient alone |
| Anchored turn prototype | Counted one-end motifs worked; mixed-region grammar was awkward and one ambiguous anchor failed | Replace with explicit lower/upper pairs |
| Explicit counted pairs in shared Layered fitter | Exact two-plus-one, mirrored base, mixed regions, restricted bands and per-group pace | Selected |

Named geometry accepts compatible model-selected widths. The phrase carries
absolute lower/upper turns, counts and relative rates, with no timestamps or
private playback path. Widths remain chosen by the model; there is no fixed
regional preference or speed target. Ordinary field motion remains available
when variation is uncounted or the user releases the phrase.

Hosted current context includes commanded bounds and typical turn coverage,
including lower/upper percentiles and endpoint-visit fractions. A whole-loop
minimum alone no longer stands in for how often the base is visited. Local
contracts/schema and ordinary local context remain unchanged. There is no
language classifier, reply rewriting, keyword-based motion rule or extra model
pass. Structured output constrains grammar, not intent; the native operation
branches follow the existing adapter and the [official Structured Outputs
contract](https://developers.openai.com/api/docs/guides/structured-outputs).

## Claude consultation and resulting corrections

Consulted `claude-opus-5-5` through CLI with `--effort max`, read-only tools for
the architecture pass and a bounded source-only final pass. No private session
history or credentials were sent. Claude supported using the shared fitter but
requested changes to the first candidate:

1. A narrowing splice could accelerate beyond either selected plan. Added a
   regression that reproduced this on all three device profiles, then fixed
   remaining-distance timing and old/new-leg velocity bounds. Difficult joins
   finish the old stroke or pair instead of forcing a faster splice.
2. Slow full-sweep rates were asymmetric for base motifs. Shared descent rates
   now belong to the narrowest adjacent group containing the complete leg;
   both sweep directions use the selected sweep rate for mirrored motifs.
3. Removed new `pace.follow` operations from phrase grammar and reject them
   while a phrase is active. Existing follow was already materialized into an
   independent envelope, so it is preserved without a dangling reference.
   Verified nested phrase validation already runs before numeric unmarshal.
4. Preserve a shared tip/base anchor when returning to field motion; validate
   that a phrase's band equals its actual endpoint hull.
5. Round the compiled horizon up to whole motifs rather than shortening the
   ordinary 64-cycle horizon. This retains longer pace development.

The final source was corrected and tested after that review; Claude's review
was not an approval of the unmodified candidate.

## Real-model and end-to-end checks

All fixtures use synthetic technical requests, the real signed-in Responses
provider, and no hardware. Credentials stay in the existing protected store.

| Final fixture | Result | Generation median |
| --- | --- | --- |
| `gpt-6-sol`, None/Fast reasoning, 12 independent scenarios | 12/12 intent checks; one call each, no repair/fallback | 5,283.5 ms |
| `gpt-6-sol`, Low reasoning, same 12 scenarios | 12/12 intent checks; one call each, no repair/fallback | 5,235 ms |
| Low reasoning continuation: count, widen, faster, more, release, exact hold | 6/6 | See retained per-turn report |
| Production HTTP: text-only, counted start, widen, speed-only, release, Stop | Passed; one Play for the motion run, four one-call motion replies | 25.60 s total fixture |

The HTTP fixture uses None reasoning and a fake dispatch owner. It verifies
backend orchestration, the actual shared engine, captured command stream and
Stop. It does not establish network or physical feel. A strict-schema adapter
round trip also passes with a 32k context budget and 1536 output tokens.

Failures are retained, not hidden in a headline pass rate. Earlier trials
included a rejected ambiguous anchor, inherited 5–95 endpoints, and an exact
hold that removed pace variation. A later apparently correct two-to-one result
also changed speed 32→45 and removed pace texture. Stronger intent checks caught
three mirrored-base pace-preservation failures. A subsequent fast-mode coverage
case broadened reach correctly but embellished pace despite a preservation
request. The final guide distinguishes count/reach, smooth fitting and pace
texture; both final 12-case runs passed after that clarification. These finite
runs do not prove universal language compliance or statistical superiority of
Low over Fast. Existing user model/reasoning settings are not changed.

## Numerical and visual review

Core tests cover 60 profile/speed/coverage combinations: Original, 2 Standard
and 2 Pro at 7/22/46/83/100 with upper widths 13/38 and lower widths 28/47.
They check exact turn order/counts, C2 knots and seam, nonzero turnaround
acceleration, the Layered 2400 acceleration / 24000 jerk budgets, and buffered
endpoint preservation. Additional checks cover pace/reach handoffs, 288 severe
narrowing cases with the saved speed maximum equal to the selected speed,
mirrored slow sweeps, full-motif horizons, transactional rejection and cloning.

Ignored evidence is under `.scratch/layered-range-flow/`:

- `installed-motion.png`: all six retained installed command streams.
- `atlas-reviewed/`: 205 evaluation records, 140 distinct plots, nine contact
  sheets and a manifest retaining failed selections. Original anchored-prototype
  records are explicitly marked as recompiled into the final pair representation;
  their new plots are not claimed to be historical timing captures.
- `phrase-handoffs-1.png` through `-3.png`: 18 detailed profile/speed/anchor
  handoffs, including low-speed severe narrowing.
- `atlas-reviewed/captured-01.png`: production HTTP fake-transport sequence;
  samples shaded beyond Stop are queued points that were canceled.
- `counted-comparison.png`: installed final command excerpt beside the compiled
  accepted two-to-one candidate, with different selected speeds clearly not
  presented as a matched physical performance test.

All nine overview sheets were inspected. Detailed review includes the accepted
counted/mixed/restricted motifs, mirrored slow sweep, low/high-speed outliers,
wrong-pace model selections and captured transitions. The accepted steady
motifs preserve rounded turns, full endpoint visits and exact counts. Buffer
quantization follows the planned positions without invented rests. Severe
narrowing can show a temporary deceleration shoulder and a delayed join; it
stays bounded rather than accelerating to force the shorter destination.
These plots complement, and do not replace, an observed Cloud comparison.

## Compatibility, checks and remaining acceptance

All 47 pre-existing non-phrase Layered comparison records are exactly equal to
an alpha.68 source build, including timing and wire output. Five idle benchmark
repetitions give ordinary Layered compile medians of 3.504 ms before and
3.495 ms after, within run variation. No dependency, worker or model pass is
added. See the [scorecard](goal-scorecard.md) for comparable binary/UI budgets.

Local `go test ./...`, `go vet ./...`, golangci-lint, architecture/goroutine
checks and the CGO-free build pass. Frontend typecheck, 119 test files / 929
tests and production build pass; committed assets are unchanged. Race and
packaging gates are run in PR CI; local Windows has no C compiler configured.
The isolated current-source simulator at `http://127.0.0.1:50264/#/chat`
passed `check-review-llm.ps1` and a real local Gemma text-only chat in 320 ms
(123 ms to first token), with no repair/fallback or motion. Layered is selected,
Autopilot is off, and no hardware credentials were copied into that profile.

Still required before release: observe the counted pattern and reach on the
Original Handy over Cloud, including ordinary and broader tip coverage and
live retargets. Retain a matched trace and physical feedback. The installed
instance's reported shortfall is not declared physically solved by a plot.
Count/order changes retain the general transition rather than the specialized
same-slot splice. No release is published as part of this candidate review.
