# GPT-6.1 Terra as a ChatGPT default candidate

Date: 2026-10-28. Base: alpha.77, commit
`892d555ba0e08be6b5134e239e273371c47f9d90`.
Evaluation branch: `codex/chatgpt-terra-evaluation`.

## Recommendation

GPT-6.1 Terra with **Low reasoning** is the selected default for new
ChatGPT connections. In 113 matched requests per model, its median completion
latency was 3.284 seconds versus 4.045 seconds for the previous GPT-6 Sol / Low
recommendation: 18.8% lower. The 90th percentile was 4.955 versus 6.905 seconds,
28.2% lower. Both met the intent checks on 111/113 requests.

Following the user's request, Setup and Settings now prefer Terra for an empty
ChatGPT model choice, with Low reasoning when advertised by the account. If
Terra is unavailable, the previous fallback order remains: GPT-6 Sol, GPT-6.1
Sol, then the first available account model. Existing saved choices are not
migrated. Recommendation text is updated in all five locales. Prompts, parser
acceptance rules, motion generation and local-model behavior are unchanged.
There is evidence for a useful latency improvement, not proof that Terra has
better motion quality or is interchangeable for every workload.

## Model and request path

- Actual ChatGPT sign-in account, production Responses provider and production
  chat service; no API-key substitute or CLI-model proxy.
- The account catalog advertises `gpt-6.1-terra`, with Low as its default
  reasoning effort. Terra and GPT-6.1 Sol advertise 500K context on this account;
  GPT-6 Sol advertises 272K. This is a point-in-time catalog observation.
- All comparisons used the same 64K context admission setting, existing
  production prompt construction, schemas, and parser. Prompts were not padded
  to 64K. This is not a full-window or long-conversation stress test.
- Serial requests prevented our own benchmark from contending with itself.
  Model order reversed between suites. Two follow-up conversation repetitions
  ran Terra then Sol. Provider-side load and cache effects remain uncontrolled.
- Times measure completion through the relevant production service or HTTP chat
  fixture, including parsing, not time to first token. Rejected responses remain
  in latency totals. Quality passes require a usable response without hidden
  repair/fallback; the fixtures also inspect the actual parsed motion.
- Most cases are inert provider/parser/compiler tests. The HTTP resume cases
  use the real chat handler, shared engine, and simulated transports. No physical
  device was connected or commanded.
- Credentials stayed in the pre-existing protected account store. Captures
  contain authored benchmark prompts and outputs, not copied user sessions.

The [official Terra model documentation](https://developers.openai.com/api/docs/models/gpt-6.1-terra)
describes Responses and structured-output support. Its API reasoning choices
include `none`; the ChatGPT account catalog used here does **not** advertise
`none`. A direct experimental request accepted it, but this is not a reason to
add an unadvertised option to the sign-in UI. Model selection follows observed
task quality and latency rather than an assumed advantage from a newer name;
see OpenAI's [model-selection guidance](https://developers.openai.com/api/docs/guides/model-selection).

## Matched results

Each row below contains the same requests for both models. Quality is a focused
intent check, not a blinded human quality score or an estimated production
success rate. Times are medians in seconds.

| Suite | Cases per model | Terra Low correct | Sol Low correct | Terra Low time | Sol Low time |
| --- | ---: | ---: | ---: | ---: | ---: |
| Response boundaries | 8 | 8 | 8 | 3.351 | 3.882 |
| Sequential conversation edits | 10 | 9 | 10 | 3.557 | 4.002 |
| Two additional conversation runs | 20 | 20 | 20 | 3.496 | 4.544 |
| Vibration control, Layered and Creative v2 | 26 | 26 | 26 | 2.895 | 3.329 |
| Layered planning and constraints | 18 | 18 | 17 | 4.183 | 4.772 |
| Stopped/paused motion intent | 10 | 10 | 10 | 2.903 | 3.719 |
| Resume through the HTTP chat path | 16 | 16 | 16 | 2.947 | 4.086 |
| Regional transitions and counts | 5 | 4 | 4 | 4.096 | 7.704 |
| **All matched cases** | **113** | **111** | **111** | **3.284** | **4.045** |

| Completion statistic | Terra Low | Sol Low |
| --- | ---: | ---: |
| Median | 3.284 s | 4.045 s |
| 90th percentile, linearly interpolated | 4.955 s | 6.905 s |
| Slowest measured response | 7.387 s | 10.116 s |
| Structurally accepted responses | 112/113 | 112/113 |

The initial 18-case screen also tested GPT-6.1 Sol / Low and Terra / None.
Compare these only with the same initial 18 cases, not the larger sample:

| Model / effort | Intent checks | Median completion |
| --- | ---: | ---: |
| GPT-6.1 Terra / Low | 17/18 | 3.497 s |
| GPT-6 Sol / Low | 18/18 | 3.959 s |
| GPT-6.1 Sol / Low | 18/18 | 5.896 s |
| GPT-6.1 Terra / None, experimental | 18/18 | 3.642 s |

Terra / Low was 40.7% faster than GPT-6.1 Sol / Low in that smaller matched
screen. Disabling reasoning did not improve its median latency. Four separate
single-word availability probes completed successfully, but their timings are
excluded from the tables.

## Failures and scoring audit

1. **Terra, base-speed edit.** In the first sequential conversation, the current
   base speed was 58 and the request asked for seven percentage points more.
   Terra selected 70 instead of 65 while claiming it made the requested change.
   It preserved the counted phrase but also changed memory, pace variation and
   seed unnecessarily. This is a real model intent error, not malformed JSON.
   Two subsequent complete ten-turn runs both selected 65 and passed all ten
   checks. The original failure remains in the aggregate and visual report.
2. **Sol, replace periodic geometry with irregular drift.** Sol added drifting
   center/range layers but left the global variation mode as `waves`. The engine
   still mixes a periodic base-width field with that range layer, so the request
   to replace periodic reach was only partially satisfied. Keep this failure;
   do not weaken the evaluator merely because the resulting curve looks usable.
3. **Both models, separated counted regions.** Four short upper strokes followed
   by two short lower strokes produced two groups with no return bridge. The
   last-to-first join was invalid and was rejected. Both succeeded when the
   request explicitly included a full sweep bridging the regions. This remains
   a shared model/contract weakness, not evidence against Terra alone. A future
   bridge/repair improvement should preserve requested counts and be evaluated
   separately; do not bypass seam validation or add language keyword rules.
4. **One evaluator defect, corrected.** Terra's Creative v2 plain Start returned
   an all-zero `follow` envelope. Runtime semantics preserve the owner's retained
   vibration level, which was 100 in this case. The old live evaluator treated
   zero-follow as silence and falsely failed it. `chosenLevel` now uses the
   backend-reported retained level for this case, while explicit Off and
   steady-zero still mean zero. A six-case semantic regression check passes.
   The original capture is unchanged; an explicit scoring-correction record
   changes only this one outcome. Both model captures were audited for the same
   condition. No runtime acceptance or safety behavior changed.

## Shared-engine visual review

Normalized all 262 benchmark decisions, retaining errors and wrong accepted
selections. The exporter compiled 203 retained/selected scores into 80 distinct
outputs, with five overview sheets. Fifty-seven decisions had no new compiled
linear action, and the two rejected region joins remain visible as error cards.
Some lifecycle cards show the retained score while stopped; a plotted score is
not evidence that playback started. The HTTP fixture checks actual engine state.

Inspected all five overview sheets, two vibration overview sheets, and detailed
outliers: Terra's wrong 70% edit versus the repeated correct 65% output; Sol's
partially periodic drift versus Terra's drift; and the shared 100% counted
bridge. Coverage, anchored returns, regional rate contrasts and counts remained
visibly distinct. The 100% bridge selected by both models compiled identically.
The wrong speed edit changed the clock rather than losing the counted geometry.
Detailed plots show the shared engine's rounded turns, planned/wire overlays,
velocity and acceleration; they do not establish physical smoothness.

The vibration export retains all 84 vibration/HTTP rows and 20 distinct
envelope/retained-level combinations. Follow is shown as a drive range driven
by shared motion, not a fictitious standalone zero waveform. Explicit Off stays
zero; steady, wave, and follow outputs remain separate. No new out-of-bounds
geometry or compile failure appeared among accepted scores.

Artifacts under ignored `.scratch/terra-evaluation/`:

- `catalog-probes.json`: account metadata and simple request probes.
- `screen-runs.json`, `deep-runs.json`, `repeat-runs.json`: exact model, effort,
  test exit code and capture file for every suite.
- Individual capture JSON and logs, including original failed runs.
- `scoring-corrections.json`, `all-turns.json`, `summary.json`,
  `overall-summary.json`: transparent normalization and aggregate results.
- `motion-atlas.json`, `plots/index.html`, `plots/manifest.json`,
  `plots/contact-01.png` through `contact-05.png`: shared-engine visual record.
- `vibration-atlas.json`, `vibration-overview-1.png`,
  `vibration-overview-2.png`: sampled envelopes and follow semantics.
- `latency-comparison.png`: matched-suite latency chart.
- `review-chat.jpg`: current-source app, successful text-only local reply and
  idle simulator. This readiness check is separate from the ChatGPT benchmark.

## Reproduction and validation

Live test commands use `-tags liveeval,magichandy_labs`, `-count=1`, and a timeout
of at least 10 minutes. Set `CGO_ENABLED=0`, `MAGICHANDY_NATURAL_EVAL=1`,
`MAGICHANDY_HOSTED_DATA_DIR` to an existing signed-in account profile,
`MAGICHANDY_LIVE_MODEL` to the exact model ID, `MAGICHANDY_LIVE_REASONING=low`,
`MAGICHANDY_LIVE_CONTEXT=65536`, and `MAGICHANDY_EXPERIMENT_CAPTURE` to a distinct
absolute ignored filename per suite. Do not copy credentials into a new profile.

| Package | Test | Extra environment |
| --- | --- | --- |
| `internal/chat` | `TestResponseBoundariesLive` | None |
| `internal/chat` | `TestHostedConversationDepthLive` | `MAGICHANDY_EVAL_REPEATS=1`, then 2 |
| `internal/chat` | `TestVibrationLevelLive` | None |
| `internal/chat` | `TestHostedLayeredLivePlanning` | `MAGICHANDY_EVAL_REPEATS=1` |
| `internal/chat` | `TestHostedStoppedMotionIntentLive` | `MAGICHANDY_LIFECYCLE_MODE=layered` |
| `internal/httpapi` | `TestManualVibrationResumeProductionChatLive` | `MAGICHANDY_RESUME_EVAL=1` |
| `internal/chat` | `TestHostedPhraseJoinChoices` | `MAGICHANDY_JOIN_VARIANTS=guidance` |

The `guidance` variant uses the current production prompt unchanged. Follow-up
conversation repeats run the whole sequence, so each later edit sees actual
earlier model choices; do not filter to only the arithmetic subtest.

Validation completed:

- `go test -tags liveeval,magichandy_labs ./internal/chat -run '^TestChosenVibrationLevelUsesFollowSemantics$' -count=1`
- `go vet -tags liveeval,magichandy_labs ./internal/chat`
- `go test ./internal/chat`
- `golangci-lint run ./internal/chat`: zero issues.
- Tagged lint with `--new-from-rev HEAD`: zero findings on changed lines.
- `gofmt` and `git diff --check`.
- `TestCapturedVibrationEnvelopes` with absolute capture paths.
- Shared motion atlas export/render and visual inspections listed above.
- `scripts/check-review-llm.ps1 -BaseUrl http://127.0.0.1:50306`: actual local
  Gemma 12B generation succeeded. One additional text-only chat request completed
  in 2.879 seconds, with a non-empty response, no repair/fallback indication,
  Motion `none`, and the simulator idle.

An extra lint run including every opt-in `liveeval,magichandy_labs` test reports
52 existing findings across the live evaluation harnesses (function size,
complexity, environment-selected capture paths and style). Those are outside
the normal release lint invocation. No lint configuration or gate was changed;
do not describe that expanded invocation as passing.

The default-selection follow-up changes the shared frontend connection editor,
recommendation copy and locales, alongside the opt-in evaluator correction.
See the [goal scorecard](goal-scorecard.md) for rebuilt bundle measurements.
No physical-device, long-running Autopilot, voice, refusal-rate, or
full-context-window comparison was performed. Those claims cannot be inferred
from the aggregate scores. The review app remains at
`http://127.0.0.1:50307/#/settings/chat/model` with its working local model; the ChatGPT benchmark
used the pre-existing account in place, without putting its credentials in the
isolated review profile.

Default-selection follow-up validation: TypeScript checking, localization audit
(3,479 keys across five locales), all 1,069 frontend tests, production frontend
build, and the `CGO_ENABLED=0` application build passed. Existing Setup tests now
exercise a catalog with Terra after Sol and confirm Terra/Low is selected, and
confirm a saved model/Medium selection keeps exact readiness after discovery.
The fresh current-source review app on port 50307 also passes the actual model
generation probe and a text-only chat-path request (622 ms, non-empty reply,
no repair/fallback indication, motion disabled and simulator idle). Previous
review sessions remain running.
