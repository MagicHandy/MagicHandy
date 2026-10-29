# OpenRouter reasoning and motion response review — 2026-10-29

Implemented on `codex/openrouter-reasoning`, based on `9247dc54` (application
code `a97ddbe0`). OpenRouter now exposes reasoning controls. The Advanced
Layered format distinguishes travel lengths from endpoint positions and makes
full-band travel an exclusive composition. These remove observed sources of
invalid output without changing model-selected numbers or weakening validation.

GPT 6.1 Terra / Low remains the quality recommendation. DeepSeek V4.1 Flash
1017 / Together / reasoning Off is a fast, economical option, but some valid
responses still misunderstand coverage or timing. No new capability tier or
default-model change is justified by these trials.

## Implementation

- Reasoning choices come from OpenRouter's nested model metadata: Provider
  default, Off when optional, Enabled with provider-selected effort, and
  supported effort levels. Null effort metadata permits the gateway's documented
  efforts; missing/empty lists do not invent an effort selector. Mandatory
  reasoning excludes Off. Unknown effort values are ignored.
- Explicit Off is persisted losslessly. Where `none` is advertised, the UI sends
  that effort; otherwise it sends `enabled:false`. Default sends neither. The
  compatible adapter sends `reasoning:{...}` and rejects unsupported saved
  choices before generation. ChatGPT/OpenAI request construction is unchanged.
- Setup and Settings share the existing Graphite row treatment. Changing models
  keeps compatible choices and clears incompatible ones. Changed reasoning
  invalidates provider/readiness caches. The existing catalogue cache lasts
  fifteen minutes; no per-turn discovery or second inference call is added.
- OpenRouter's Check model now allows 2,048 total completion tokens when
  reasoning may be active. The previous 64-token allowance could expire before
  a reasoning model emitted its tiny readiness answer. Explicit Off retains 64.
  A model still has to return a real successful readiness response.
- Advanced Layered uses `stroke_width:{shortest_percent,widest_percent}`.
  `min_percent/max_percent` remain outer positions. Current and recalled score
  context and code-owned examples use the same vocabulary. Parsing translates
  field names only; duplicate/mixed fields, impossible geometry and invalid
  numbers still fail. Unambiguous historical responses remain readable.
- `composition:"full_band"` fills the chosen outer interval and cannot also
  emit a contradictory width in the constrained schema. Manual, counted phrase,
  named geometry and full-slider choices remain available. Simple/Intermediate
  prompts and schemas retain their existing vocabulary and budgets. The shared
  engine, sampling, device limits, Stop and transports are untouched.

The API behavior follows [OpenRouter's reasoning documentation](https://openrouter.ai/docs/guides/best-practices/reasoning-tokens)
and [model catalogue metadata](https://openrouter.ai/docs/api/api-reference/models/list-all-models-and-their-properties).

## Evaluation method

The baseline and failed variants remain in the private ignored evidence under
`.scratch/model-economy-2026-10-29/`. The harness uses the production
`chat.Service` prompt composition and validator and the shared
`motion.FlowTarget` / `ReviewMotionOutput` renderer. It creates no transport.
Three final DeepSeek trials additionally use the real Go compatible-provider
adapter, including discovery and reasoning emission. Other OpenRouter trials
use exported production requests with strict output and a pinned Together route.

Inputs and enabled adult personas were preserved. Every trial requested a brief
neutral technical description of the motion decision. This evaluates handling
of explicit input and motion planning, **not unrestricted erotic roleplay
generation**. Rejected responses and model decisions that do not match intent
are retained. No refusal was rewritten or retried for circumvention. First-pass
results exclude repair and local fallback.

OpenRouter used `deepseek/deepseek-v4.1-flash-1017`, Together only, reasoning
`none`, 2,048 output tokens and a 22-second deadline. The actual catalogue listed
its provider default as High, making explicit Off materially different from
leaving the setting unset. GPT used `gpt-6.1-terra`, ChatGPT sign-in, Low.
All timing is complete-response latency, which is what validated motion needs.

One attempted snapshot replay counted indentation in the saved schema as input
budget and failed before sending any request. The harness now compacts the JSON
schema on import, preserving its contents. Those fourteen zero-call preflight
errors remain saved but are excluded from model statistics. The successful
matched run alternates before/after for each of seven cases.

## Results

Valid means accepted by the existing validator, not that the model followed
every part of the request. Valid no-action replies are included.

| Trial group | Valid / attempts | Median | Range |
| --- | ---: | ---: | ---: |
| Baseline DeepSeek, reasoning Off | 11 / 13 | 1.139 s | 0.416–2.422 s |
| Final format DeepSeek | 11 / 11 | 1.278 s | 0.642–1.917 s |
| Final Go provider-adapter DeepSeek | 3 / 3 | 1.434 s | 1.045–1.561 s |
| Matched GPT before | 7 / 7 | 3.672 s | 2.160–4.837 s |
| Matched GPT after | 7 / 7 | 3.812 s | 3.010–5.272 s |
| Supplied examples, DeepSeek | 8 / 8 | 0.997 s | 0.534–1.936 s |
| Supplied examples, GPT | 8 / 8 | 3.238 s | 2.047–5.432 s |

These are small exploratory samples with mixed cache warmth, not a statistically
controlled estimate of failure probability or a latency guarantee. The final
DeepSeek variants and baseline include different repeat counts. The GPT paired
comparison uses the same seven inputs, personas and starting states.

### GPT regression check

Both versions retained three short upper strokes followed by one full 0–100
sweep. The after version selected 75–100 for the short group versus 70–100 before;
both preserve the requested count and full sweep. Both broader-coverage replies
selected the same 60–100 outer interval and 30–40 travel lengths, with upper
anchoring and original pace intact. Both start cases started; maximum pace
remained 100 with no added pace modulation.

The revised natural-variation reply preserved base pace and its modulation while
changing the upper coverage. The baseline also changed pace and timing. For
directional travel, the revised result preserved full travel and selected a
100/30 down/up contrast; the baseline changed width and other timing too. No
quality regression was observed in these cases. This does not establish
non-regression for every model, conversation or other motion mode.

### DeepSeek quality limits

The final format eliminated the sampled structural/geometry rejections and kept
latency low. It did not make every decision correct:

- The two final broader-coverage attempts retained the old 75–100 band. One
  filled its width, but neither meaningfully expanded reach. The real-adapter
  repeat over-expanded to 0–100 instead. Both are weaker than GPT's response.
- The compound count remained correct, but two final responses stopped their
  upper endpoint at 95 rather than the requested 100.
- Final variation trials kept upper focus better than earlier alternating-end
  choices, but added unrequested pace changes. The real-adapter repeat still
  chose movement across much of the device range.
- Explicit gentle starts, maximum pace and directional contrast worked. The
  indirect start still sometimes returned no motion.

These are model interpretation errors. The application does not infer a hidden
correction from particular words, choose fixed regions/speeds, or silently turn
a rejected proposal into a different motion.

## Supplied Examples.txt

The user supplied 33 existing generated examples in a file labeled **Gemma**.
It is not attributed to Vanessa. Eight representative lines (examples 1, 8, 10,
14, 18, 22, 26 and 29) were extracted verbatim and sent through the production
prompt/validation path for both models. An ignored source manifest retains the
file hash and line references. The file itself, persona text and responses are
not committed or reproduced here.

Both providers returned eight valid decisions with no provider-flagged policy
refusals. Direct fast-start and stroke requests started; a continuation with full
travel selected 0–100 for both. Requests concerning unsupported physical actions
or speech could correctly return no device action. DeepSeek also translated one
non-device sensory request into a short upper motion, whereas GPT left motion
off; this is a behavioral difference, not automatically a refusal or success.

The chronological slow-then-fast example exposed an intent mismatch: DeepSeek
selected directional 100/35 down/up timing rather than a sequence over time.
GPT chose a varying pace envelope, a better match for temporal variation, but
not a proven one-shot slow-then-fast sequence. A DeepSeek repeat without new
guidance changed width instead, so the problem is not a reliable field-mapping
rule that can be patched deterministically.

## Rejected alternatives and capability tier decision

Additional self-check prose and a shortened contract did not reliably improve
validity. A renamed-length experiment worked substantially better, but live
production trials exposed the contradictory full-band/short-width combination;
the exclusive full-band branch addresses that structural ambiguity.

Removing named choices in favor of manual-only composition did not consistently
improve intent and removed useful expression. Combined or extra reach guidance
sometimes overcorrected into full-device travel. These variants were discarded.

A final small timing-guidance trial explicitly distinguished changes over time
from up/down contrast. Two repeats of the supplied timing request produced one
no-action result and one modest pace-envelope change; the variation case still
added unwanted directional bias. The true directional case remained valid, but
there was no clear consistent benefit. That prose was not adopted or sent on to
GPT as a production change. All failed/weak alternatives remain in the atlas.

This evidence does not justify a fourth capability tier. The accepted changes
clarify the existing Advanced contract while retaining its expressive choices.
Compact local model contracts remain separate and unchanged.

## Cost, visual review and validation

Across the baseline and this follow-up: **144 OpenRouter calls and 40 ChatGPT
subscription calls**. Of those, this follow-up accounts for 85 OpenRouter and 30
GPT calls. Final OpenRouter account/key usage was **$0.120223647** against the
user's $2 ceiling. The per-request ledger totals $0.119276607 for 141 calls;
three Go-adapter requests are accounted for by the account total, and two earlier
refusals omitted individual cost fields. ChatGPT consumed subscription usage.
No paid requests remain queued.

`implementation-atlas-final/index.html` retains all **115** follow-up attempts,
**68 distinct motion plots** and **five overview sheets**, including rejected
and no-action cases. All five sheets were inspected. Detailed review covered
the matched GPT compound and directional outputs, DeepSeek's unchanged-band and
upper-variation results, and both providers' supplied timing example. Planned
and quantized curves preserve the shared engine's continuous joins; the plots
make the wrong geometry and directional interpretation visible. Numerical
zero velocity/acceleration knot jumps in those inspected cases do not prove
physical smoothness. This evaluation did not connect or command hardware.

Validation passed: full Go tests, full race suite plus a fresh chat/httpapi race
run after the final context/readiness changes, vet, lint, import-boundary and
motion/transport lifecycle tests, pure-Go Windows build, TypeScript checks,
**1,073 UI tests across 131 files**, locale audit across five locales and the
canonical frontend build. Added coverage checks discovery, cached metadata,
outgoing reasoning JSON, unsupported/mandatory settings, persisted Off,
readiness budget, duplicate/mixed widths, full-band composition and agreement
between current/recalled context and the advertised schema.

The final isolated app is running at `http://127.0.0.1:50317/#/settings/chat/model`
with simulator motion, LLM motion off and working `review-12b` local inference.
The readiness script completed real generation. A fresh text-only app chat
completed in 5.106 seconds with no repair/fallback and motion remaining idle.
The actual shared reasoning component was visually checked with optional and
mandatory catalogue fixtures at
`http://127.0.0.1:50316/.scratch/reasoning-preview.html`; this is an explicitly
labeled private component preview, not a second shipped frontend or a claim
that an API key has been installed in the review app.
