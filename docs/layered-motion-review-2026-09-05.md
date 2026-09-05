# Layered motion review — September 5, 2026

Implemented for draft PR review. No merge, release or physical audition was
performed. This follows the [conversation Lab review](
lab-conversation-review-2026-09-05.md) and records the additional production
Layered mode, continuation behavior, model comparisons and visual limitations.

## Findings from the user's conversation

The captured Lab session contained four human turns and 15 automatic turns.
All 15 automatic turns left the score unchanged. The eight-message inference
window allowed assistant continuations to displace the human requests.

The requested alternation between the ends changed the center layer but left
broad strokes. The stronger follow-up also altered pace. A gentler request
changed turn softness without lowering the speed setting. The later request
to alternate full strokes with short tip strokes moved the center instead of
providing the necessary width alternation. Those outputs look different, but
their geometry does not implement the requested action. Five distinct baseline
plots were inspected; the 19 original turns remain in the atlas.

The useful abstraction is independent, persistent reach, location and pace,
plus explicit operations for changes that couple them. More texture names
alone would leave the same ambiguity. Ordered sections also impose obvious
phrase boundaries; they remain available as a Lab comparison, while Layered
uses one continuous carrier. Softness is absent from its model-facing fields.

## Implemented behavior

- Production chat adds **Layered** alongside Creative, Pattern library and Off.
  The selector has three columns and two rows, with two blank, noninteractive
  reserved cells. Labels wrap instead of truncating. Layered is also the first
  choice in the LLM Lab title-bar list; both paths use the same contract.
- A required `edits` object distinguishes actions from conversational prose.
  Partial edits preserve unmentioned layer attributes. Paired stroke-width
  bounds and explicit layer-period deltas avoid common distance/position and
  "by"/"to" ambiguities. Contradictory geometry and numeric edits are rejected
  transactionally. No repair request, text-derived command or pattern fallback
  conceals a failed output.
- Named geometry operations implement alternating ends, full-and-tip,
  full-and-base, anchored reach and wandering reach/location. They preserve
  pace unless explicitly edited. Alternating ends now permits local width
  variation, normally 15–30 semantic points within the outer band.
- Drift, periodic wave and unequal smooth alternation can independently
  modulate range, center and pace. New runtime scores and `evolve` choose fresh
  nonzero seeds. The chosen seed stays in the score for faithful replay;
  fixed fixtures remain deterministic. Randomness changes the realization
  inside the requested constraints and never changes safety limits.
- Four recent human requests remain available separately from automatic
  replies. Layered Autopilot requests fresh variation while preserving the
  current character. Exact-repetition requests take priority and are checked
  by the host. Lab continuations add 0–50% of the configured quiet interval as
  random delay; the configured interval remains a minimum. The score itself
  remains finite, normally 64 cycles, and will repeat without an accepted
  evolution edit. This is not an endless procedural stream.
- Flow scores pass through cloned semantic motion targets and mode segments,
  the existing compiler, conditional retargeting, sampler, sanitizer and
  transport. A changed speed limit recompiles the score. No additional motion
  loop or runtime dependency was introduced.

## Live model comparisons

Ollama on the review host served
`igorls/gemma-4-12B-it-qat-q4_0-unquantized-heretic:Q4_0` and
`huihui_ai/granite4.1-abliterated:3b`. The following are **intent passes**, not
merely valid JSON. The early 15-case sequence reused the user's wording; the
later eight independent formulations were added to probe generalization.
After inspecting their failures, those cases also became tuning cases. These
small sets do not estimate broad model reliability.

| Candidate | Gemma intent | Granite intent | Decision / observed failure |
| --- | ---: | ---: | --- |
| v1, flat optional edits | 11/15 | 3/15 | Missed widest stroke and unsolicited pace changes. |
| v2, shorter prompt and paired widths | 3/15 | 3/15 | Rejected; frequent prose without the required action. |
| v3, required edits object | 4/9 | 2/9 | Rejected; coupled geometry still missing. |
| v4, named geometry operations | 14/15 | 14/15 | Geometry improved; exact repetition lost to Auto evolution. |
| v5, explicit continuation policy | 15/15 | 15/15 | Original sequence and exact hold passed. |
| Added independent formulations | 7/8 | 5/8 | Relative timing and contradictory geometry exposed. |
| v6, explicit period delta | 23/23 | 21/23 | Reference result before fresh runtime variation. |
| Natural variation candidate | 23/23 | 11/23 | Granite width conflict caused eight cascading character failures. |
| v8, added explanatory prompt guidance | 22/23 | 9/23 | Rejected, including a Gemma regression. |
| **Final v9, simpler wording plus natural variation** | **23/23** | **21/23** | No observed regression against v6 on either model. |

The final run contains 46 replies: Gemma accepted 23/23; Granite accepted
22/23, with 21 intent passes. Both models preserved the requested full/tip
character and speed while producing eight distinct scheduled continuations.
Both held ordinary questions and honored exact repetition through a later
automatic turn. The final two Granite failures are retained: one contradictory
width override was rejected, and one accepted negative period delta made pace
variation develop faster when the request said slower. Valid output can still
be semantically wrong, and reply prose can misdescribe the emitted values.

The extra v8 guidance was removed under the user's requirement not to improve
Granite at Gemma's expense. No automatic model-specific prompt downgrade is
introduced. A separate limited-model option remains possible after evidence
shows a benefit; the default does not silently switch by model name.

## Shared-engine and captured output review

The retained atlas has **509 cases, 266 distinct steady plots, 17 overview
sheets and two captured transport timelines**. It includes failed prompt
iterations, 45 early envelope/profile cases, and 63 current geometry cases:
seven operations × speeds 10/45/85 × Original/2 Standard/2 Pro. These wider
speed bounds belong to explicit engine review fixtures, not an override of
the user's saved limits. All overview sheets were inspected. Detailed review
covered every operation at low/middle/high speed on Original, the earlier
wave/drift/alternate comparisons, the 2 Pro outliers, failed selections, and
both startup/retarget/Stop captures.

Observed character:

- Anchored drift keeps one endpoint fixed while the other develops smoothly.
  Centered variation changes both endpoints; wandering variation adds slower
  location movement. Their phase portraits and planned/wire overlays confirm
  distinct geometry without jumps at the inspected seams.
- Full/local alternation gives clear short anchored clusters interleaved with
  full travel. The base and tip forms are distinct directional placements.
  Irregular dwell times vary the clusters; the overall alternation remains
  recognizable. Periodic waves visibly repeat more evenly than drift.
- Alternating ends produces two local stroke regions joined by longer
  transfers. Local width variation survives instead of returning to broad
  center sweeps. The large transfer arcs in the phase portrait warrant
  physical evaluation; round curves alone do not establish comfort.
- The current 63-case matrix remains below the existing budgets: maximum
  acceleration 2,399.942 %/s², finite-segment jerk 23,999.995 %/s³, and largest
  acceleration discontinuity at a knot 0.000048514 %/s². Bounds and numeric
  tests complement, rather than replace, inspection.

**Effective pace remains a material limitation.** End transfers can introduce
small reversals that engage the shared minimum-reversal-spacing gate. It slows
the whole score, sometimes far below the requested speed. With fixture seed
17, current alternating ends produces mean travel 39.53 %/s at requested 10,
37.08 at 45, and 37.13 at 85. A retained random-seed outlier (atlas case 345,
from the rejected v8 prompt run) reaches only 11.20 %/s at requested 25 and a
roughly 300-second loop. This issue is in the generated geometry/timing, so
rejecting that prompt does not eliminate it. Fresh seeds can therefore change
effective pace considerably even when the speed setting is preserved. The
shared gate has not been relaxed. The next motion-design work should prevent
incidental reversals in the source geometry, for example by coordinating the
center transfer with carrier direction and accounting for that travel in the
local clock, before claiming consistent physical pace.

Full/tip at 45, by comparison, produces mean travel 135.88 %/s and peak velocity
290.04 %/s. Increasing its requested speed to 85 reaches only 152.79 %/s mean:
the kinematic ceiling still limits it. These examples are why model intent
scores refer to semantic edits, not measured carriage behavior. A higher
selected speed is not guaranteed to make every shaped score feel faster.

## Production-path validation and physical limits

Both final models passed `TestLayeredLiveProductionConversationAndAutopilot`.
Four real HTTP chat replies retargeted one shared-engine run, preserving one
transport Play call; the gentler edit changed requested speed from 25 to 20.
One real production Autopilot decision refreshed the score and was compiled.
That particular fixture does **not** dispatch the Autopilot decision through
the production scheduler. Separate unit/lifecycle checks cover Flow routing,
ownership, late mode changes, history persistence and cancellation.

An earlier production test found that "Stop motion now" missed the literal
Stop fast path: the model said it had stopped while emitting no stop action.
The matcher now recognizes this wording and related direct commands without
inference. Both final captures confirm the engine stops and no new Play starts.
The shaded timeline tail contains queued points canceled at Stop, not executed
motion. The captures last about 7.1 seconds (Gemma) and 5.6 seconds (Granite)
before Stop; model and host timing differ between runs.

Transport mode for these tests is the in-process captured fake transport.
Its latency is synthetic; no hardware/network latency claim is made. Raw
commands and trace rows are retained in the production capture files. A
read-only snapshot of the earlier review instance reported `fake_handy`, idle,
zero commands and zero last latency; it is not physical-device evidence.
No connection key was copied, device was connected, or hardware motion issued
by this evaluation. Transport mappings/retries, numeric safety budgets and the
shared sampler/sanitizer were intentionally retained. Physical feel and live
device latency remain unmeasured for this addition.

Full Go tests, full race tests, vet, lint (zero issues), architecture checks,
frontend typecheck, five-locale audit, 472 frontend tests and the pure-Go build
passed. The final CSS-only label fix was rebuilt and reviewed at desktop and
narrow width. [Footprint measurements](goal-scorecard.md) include the final
embedded UI. The isolated review app uses working Gemma through Ollama, passed
the real readiness-generation probe, and completed a nonempty text-only
production chat request. LLM explanations may still overstate non-repetition;
the finite-score limitation above and in Help remains authoritative.

## Evidence and reproduction

Generated artifacts remain ignored under `.scratch/`; none ship in the app:

- Baseline: `layered-user-lab-before.json`, `layered-user-atlas-before/`.
- All model iterations: `layered-live-v1.json` through the named v6 reports,
  `layered-heldout.json`, `layered-natural-live.json`,
  `layered-natural-v8-live.json`, and final `layered-natural-v9-live.json`.
  The two interrupted v4 reports are retained; controller lease loss caused
  their interruption, and the harness now renews the lease while waiting.
- Engine matrices: `layered-engine-matrix.json` and
  `layered-current-engine-matrix.json`.
- Final visuals: `layered-complete-atlas.json` and
  `layered-iteration-atlas/index.html` / `manifest.json`.
- Final live capture: `layered-production-gemma-natural.json` and
  `layered-production-granite-natural.json`, including commands and trace rows.
  Earlier failed production captures and logs remain alongside them.
- Repository and footprint logs: `layered-final-*`; final UI footprint uses
  `layered-final-ui-budget.json`.

```powershell
python scripts/evaluate-layered.py --base-url http://127.0.0.1:49849 `
  --model 'igorls/gemma-4-12B-it-qat-q4_0-unquantized-heretic:Q4_0' `
  --model 'huihui_ai/granite4.1-abliterated:3b' `
  --output .scratch/layered-next-live.json

$env:MAGICHANDY_LIVE_MODEL = 'igorls/gemma-4-12B-it-qat-q4_0-unquantized-heretic:Q4_0'
$env:MAGICHANDY_EXPERIMENT_CAPTURE = Join-Path (Get-Location) '.scratch/layered-next-production.json'
go test -tags 'liveeval,magichandy_labs' ./internal/httpapi `
  -run '^TestLayeredLiveProductionConversationAndAutopilot$' -v -count=1

go run -tags magichandy_labs ./cmd/motion-atlas -catalog=false `
  -llm '.scratch/layered-next-live.json,.scratch/layered-next-production.json' `
  -output .scratch/layered-next-atlas.json
python scripts/render-motion-atlas.py .scratch/layered-next-atlas.json `
  .scratch/layered-next-atlas --captured .scratch/layered-next-production.json
```

Use an isolated app for the evaluator: it takes controller ownership, resets
the Lab and issues Stop. Its sessions are preview-only. See the
[visual review procedure](motion-visual-review.md) for optional plotting
dependencies and complete inspection requirements.
