# E4B Simple optimization and prompt transfer — 2026-10-25

The owner requested Simple optimized for E4B, then evidence-based transfer to
other prompt sets. Full motion guidance and the semantic engine remain. No
classifier of user wording, deterministic language repair, forced variety,
preferred speed or fixed coverage band was added.

## Environment and evaluation

E4B: installed `gemma-4-e4b-it-heretic-q4-k-m`, llama.cpp b11149 CUDA,
16,384 context, reasoning off, 256 interactive output, seeds 10252026/10262026.
The evaluation uses the app's existing `gemma4-close-thinking.jinja` fix. Early
standalone runs accidentally omitted it; they remain labeled PRE-TEMPLATE-FIX
in the atlas and are excluded from production comparisons. No new template
workaround was introduced.

Transfer models: installed Gemma 12B Q4_0 through Ollama at Intermediate's
32K/512 settings; `gpt-6.1-sol` with low reasoning and a 64K application budget.
GPT used the existing credential store in place. No credentials or user chat
history were copied into review data or evaluation messages.

Intent checks inspect actual semantic changes and preservation of unrequested
state, rather than JSON validity or convincing prose. A repair counts as a
first-attempt miss. Rejected and wrong choices remain in captures. Integration
uses only a simulator/in-process Buttplug; no real device was connected.

## Adopted changes

Simple Layered/Creative v2 chat gets a short sparse-edit reminder. While motion
is running it also checks full-device reach, broader short strokes and exact
repetition. Stopped/paused requests retain their lifecycle guidance without
the running-only reminder, keeping startup inside the conservative 16K budget.
Prompt inspection shows the same sections as production.

The reminder is retained for English/default, Spanish and Simplified Chinese.
Portuguese and Japanese keep their existing interactive guidance: two seeded
comparisons and Japanese adaptations showed no sufficient quality benefit.
This follows the saved prompt-set ID, never detected user language. Stored
custom profiles and personas are not rewritten.

Simple and Intermediate Autopilot motion get a private decision note before
edits. **[ADR 0060](decisions/0060-local-autopilot-decision-notes.md) records the
accepted latency compromise**, exact measurements, source captures, code
locations and revisit criteria. Context remains 16K/32K respectively. The
Intermediate/GPT chat prompts and all Advanced contracts remain unchanged.

Simple automatic correction drops only the optional first-attempt reminder,
leaving the complete contract, original request and exact validator feedback.
This leaves correction room at 16K. The existing single-correction limit and
strict validation remain; no model choice is silently replaced with a pattern.

A short stopped-state cue from the 12B work also helped E4B distinguish current
backend state from earlier playing replies. Simple receives a backend-only
restart-eligibility fact without the detailed multi-output contract. The cue
grants no authority: the one-use permit, Stop sequence, connection/ceiling
checks and dispatch admission remain authoritative.

## Matched results

| Scenario | Before | Selected version |
| --- | --- | --- |
| E4B chat, seed 10252026 | 15/22; median 921.5 ms | 18/22; median 925 ms |
| E4B chat, seed 10262026 | 16/22; median 945.5 ms | 17/22; median 927 ms |
| E4B Autopilot, seed 10252026 | 14/30; median 1,652.5 ms | 26/30; median 1,754 ms |
| E4B Autopilot, seed 10262026 | 10/30; median 1,761 ms | 25/30; median 1,828 ms |
| E4B held-out start/resume/explanation, Layered + Creative v2 | 19/20 | 20/20 |
| E4B quoted/negated/future requests across four modes | 24/24 | 24/24 |
| Intermediate 12B chat, reminder transfer | 21/22; median 860 ms | 21/22; median 930.5 ms; rejected |
| GPT chat, reminder transfer | 22/22; median 5,585 ms | 22/22; median 5,266.5 ms; no demonstrated quality gain, rejected |

Native-language five-turn conversations in both modes, across two seeds:

| Set | Baseline | Shared reminder trial | Decision |
| --- | ---: | ---: | --- |
| English | 7/20 | 11/20 | Keep |
| Spanish | 8/20 | 12/20 | Keep |
| Portuguese (Brazil) | 8/20 | 8/20 | Preserve original |
| Simplified Chinese | 9/20 | 10/20 | Keep; modest gain |
| Japanese | 13/20 | 12/20 | Preserve original |

A full Japanese adaptation scored 14/20 but exceeded the conservative 16K
budget in a lifecycle check. The shorter version fit and scored 13/20, matching
baseline, so neither was adopted. Replies retained the selected language, but
some promised full travel without selecting it; those remain intent failures.

The E4B HTTP → model → engine → fake-Buttplug transfer trial improved from 7/8
to 8/8 first-attempt Stop/resume decisions and described stopped state correctly.
The integration recheck caught missing cue delivery to Simple, whose detailed
output context is deliberately absent. The separate internal eligibility fact
fixes that wiring and has an HTTP-level regression. Longer vibration prose did
not improve E4B's explanation of the intensity ceiling and was rejected. Simple
still lacks reliable independent-output knowledge; Intermediate supplies output
awareness, and independent control needs a tested Advanced model.

## Rejected directions and limits

Shortening the full guide degraded E4B (14/22 chat, 10/30 Autopilot). The grouped
Intermediate dialect scored 16/22. An interactive planning note scored 12/22
and increased latency. Extra recency/arithmetic/corrective prose was inconsistent,
and a shorter Autopilot note lost some quality. Full guidance, local context
allocation and number of model requests are therefore preserved.

Some released regional bounds still persist; width can be confused with an
endpoint, and timed holds may last too long. E4B occasionally reseeds an exact
repeat or changes an unrequested field. Older Creative/Pattern library paths
retain their previous lifecycle limitations. These small seeded evaluations
are not reliability guarantees or evidence of physical smoothness.

## Reproduction and visual review

Ignored artifacts: `.scratch/e4b-simple-2026-10-25/`. The fixtures require
`-tags 'liveeval,magichandy_labs'`; none connects physical hardware:

- `TestNaturalMotionConversationLive`: 22-turn reach/preservation conversation.
- `TestNaturalAutopilotContinuityLive`: 30 hold/release/development decisions.
- `TestSimplePromptSetsLive`: five language sets, two continuous modes.
- `TestLocalMotionAdaptationsLive`: lifecycle and negative/intent holdouts.
- `TestManualVibrationResumeProductionChatLive`: HTTP and simulated Buttplug;
  `MAGICHANDY_RESUME_LEVEL=simple` selects the E4B contract.

Use `MAGICHANDY_NATURAL_EVAL=1`, `MAGICHANDY_LOCAL_MODEL`,
`MAGICHANDY_NATURAL_LEVEL`, `MAGICHANDY_NATURAL_OUTPUT`,
`MAGICHANDY_NATURAL_SEED`, and `MAGICHANDY_EXPERIMENT_CAPTURE`. E4B can use
`MAGICHANDY_NATURAL_LLAMA_URL`; its final Simple flag is
`MAGICHANDY_SIMPLE_PRODUCTION=1`. Clear `MAGICHANDY_SIMPLE_VARIANT`,
`MAGICHANDY_TRANSFER_GUIDE` and `MAGICHANDY_PROMPT_SET_VARIANT` for production
runs. To reproduce Intermediate's historical no-note baseline, use the parent
revision; the capability now enables the note automatically.

The initial atlas retains 958 records, 347 distinct outputs and 22 inspected
overview sheets. Detailed full-range mixtures, retained-region failures and
fixed-speed/free-shape decisions were inspected. Planned and whole-percent
output follow the same curves, with knot jumps near zero; wrong region/hold
choices remain visible. The follow-up atlas retains another 684 records,
312 distinct outputs and 20 inspected overview sheets for final integration,
native-language comparisons and larger-model transfer. Across the two atlases
there are 1,642 records and 42 inspected sheets; outputs can repeat between
atlases. The follow-up's 601 compiled records have maximum knot jumps of
0.00001238 percent/s velocity and 0.00029742 percent/s² acceleration.

Detailed follow-up inspections include:

- `motion-10925cc4c0577137.png`: production 12B keeps the requested 43% base
  speed while developing location and width; span CV 0.249, balanced travel
  and no visible flattening introduced by quantization in the excerpt.
- `motion-0c4dc2a1a2decbed.png`: production 12B's timed-hold failure. The shape
  is continuous, but the model changes pace too early. It remains an intent
  failure despite valid output and a smooth-looking curve.
- `motion-5371be9ae70ccb79.png`: E4B's retained upper-region failure. The
  requested full-device strokes remain around 65–95%; fluent reply text does
  not make that a successful selection.
- `motion-3022bf56ef6d6478.png`: GPT baseline Creative v2 full-device request
  retains broad travel and its existing timing character; no extra reminder
  was needed. Its excursions and occasional dwell still reflect Creative v2's
  selected gestures, not proof of identical full-length strokes every cycle.

The resume capture previously used “moving output exists” as its `valid`
flag, mislabeling successful no-motion answers as rejected structure. The
fixture now records response validity separately from motion intent. Retained
raw SSE and `intent_pass` are authoritative for those historical rows; this
reporting correction does not change any success count above.

The sampler, turn shapes, device mapping, limits and transport cadence are
unchanged. Graphs are commanded estimates, not carriage feedback.

## Final validation and review app

Full `go test ./... -count=1`, full race testing followed by race rechecks of
the changed chat/HTTP/transport packages, `go vet ./...`, lint and the
`CGO_ENABLED=0` shipping build pass. The existing frontend checks remain green;
this follow-up changes no frontend files or embedded bundle. Opt-in model
quality suites retain their semantic failures separately from these CI checks.

The current-source isolated review app runs at
`http://127.0.0.1:50294/#/settings/chat/model`, with simulated motion stopped,
Gemma 12B through the available Ollama endpoint, Intermediate and 32K context.
`scripts/check-review-llm.ps1` completed a real generation. A text-only request
through this exact app returned a non-empty answer in one provider call, with
no repair, fallback or motion. The settings screenshot is retained at
`.scratch/e4b-simple-2026-10-25/final-review-settings.png`.

The isolated E4B runner was stopped after its live evaluations to free GPU
memory for 12B. Its last backend cue-wiring correction has an HTTP regression
and the successful live cue-transfer trial; a fresh E4B app-path generation
after that final wiring correction has not been repeated. The running review
app uses the available 12B model rather than an offline E4B endpoint.
