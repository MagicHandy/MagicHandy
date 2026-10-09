# Hosted motion planning: latency, intent and local isolation

Date: 2026-10-08. Branch: `codex/chatgpt-motion-planning`.
Follow-up to [the connection/setup review](cloud-model-review-2026-10-08.md).

## Decision

Keep the existing Creative v2 mode and shared motion engine. Add a short reach
planning guide only for hosted models, and recommend **GPT-6 Sol with Low
reasoning** for a new ChatGPT connection. Preserve existing model selections.
It was the fastest model to pass the compound production fixture in this review.
GPT-6 Luna is a quicker simple-edit alternative, with a demonstrated compound
request miss; GPT-6.1 Sol with Low reasoning is a slower passing alternative.
These are small same-host samples, not a universal ranking or latency SLA.

Do not add a cloud-only mode, a second model call to judge each response, or a
model call per stroke. The existing engine plays while a bounded semantic
update is planned. Existing Autopilot scheduling and cancellation remain intact.
Provider choice does not change the shared sampler, sanitizer or transport path.

## What was tested

Neutral synthetic technical requests went through the real signed-in ChatGPT
provider, existing parsers and the shared motion compiler. Local comparison used
Ollama's installed `igorls/gemma-4-12B-it-qat-q4_0-unquantized-heretic:Q4_0`.
No physical device was connected. Full-path tests used the production HTTP chat
handler and a fake transport; the comparison tool has no transport or engine loop.

The first screen covered 22 model/effort/tier trials across GPT-6.1 Sol, GPT-6
Sol/Astra/Luna and GPT-5.6 Sol/Terra/Luna. A 112-turn comparison then covered seven
interfaces, two models and two repetitions. Reach, preservation, hold and
question cases were scored separately from JSON validity. Later targeted
steering and production tests refined the recommendation.

| Idea | Evidence | Result |
| --- | --- | --- |
| Improve steering in existing modes | Original Luna Creative v2 reached 4/8 intent checks in the four-turn suite. Explicit reach guidance achieved 24/24 in an eight-turn, three-repeat follow-up. Final GPT-6 Sol guidance achieved 16/16. | Ship hosted-only reach guidance; retain existing modes. |
| Adaptive continuous stream | A prototype mapped model choices onto existing Freestyle tendencies. Both models passed only 4/8 intent checks: holds/questions passed, shallow-only edits still admitted deep excursions. | Keep out of production. Hard reach bounds and mode ownership need a separate design before promotion. |
| Richer stroke vocabulary | Numeric stroke-end and groove interfaces each passed 8/8 for both models. Luna medians were 3.43 s and 2.84 s; GPT-6.1 Sol medians 5.21 s and 5.16 s. Plain-word mapping passed 5/8 and 4/8. | Preserve numeric interfaces in Labs. This suite does not demonstrate a replacement for Creative v2's direction, inertia and rebound controls. |
| Bounded phrase composition | Models returned correct phase fields, initially scoring 8/8. Rendered motion revealed inherited span modulation and failed exact full-stroke geometry. Corrected checks passed only holds/questions, 4/8 per model. | Keep experimental. Exact choreography needs compiler-aware phase semantics, not just schema success. |

Layered also ran as a comparator: Luna passed 6/8 (two invalid edits), GPT-6.1
Sol 7/8 (one incomplete 45-second request). All failures are retained. The
GPT-6.1 comparison used a requested `ultrafast` tier, but a separate metadata
probe showed default service processing; no speed benefit is attributed to it.

## Latency and production path

Five sequential production edits test short tip strokes with directional timing,
added inertia, full strokes mixed with three shrinking base rebounds at width
45/retention 75, removal of rebounds, and continuation of the same character.
Each turn must use one provider call without repair or fallback. A generated
Autopilot proposal and a plain-text Stop follow the edits. The test asserts one
transport Play and captures queued samples and trace rows.

| ChatGPT model, Low reasoning | Five interactive edits | Median / maximum request time | Autopilot and Stop |
| --- | ---: | ---: | --- |
| GPT-6 Luna | 4/5 intent checks | 2.64 / 4.45 s | Passed; compound base request used tip focus |
| GPT-6 Sol | 5/5 | 3.47 / 5.81 s | Passed, one shared run |
| GPT-6.1 Sol | 5/5 | 4.03 / 6.91 s | Passed, one shared run |

The final independent eight-turn steering fixture, repeated twice with GPT-6
Sol, passed 16/16 with median 3.37 s and maximum 5.62 s. It covers upper/base/
middle restrictions, natural variation within a band, hold, a question and
restoring full reach. These small fixtures do not estimate p95, sustained plan
quota, WAN behavior, physical comfort or responsiveness under account contention.

Live production testing exposed a defect that Lab-only tests missed: the
production action schema used a root union rejected by OpenAI. The adapter now
wraps that union in a required `proposal` object, supplies types for constants,
and strictly unwraps before domain validation. The action alternatives and
no-motion restrictions remain intact. Strict compatible/OpenRouter adapters
use the same conversion; local and prompt-only paths keep their original schema.
Initial HTTP 400 rejections and the later Luna intent miss remain in the atlas.

## Model controls and setup

Setup shows a short motion recommendation and a collapsed response-speed control.
Settings shows the full recommendation, account-advertised reasoning choices,
and the last successful connection check's elapsed time. Reasoning is persisted
per connection and participates in readiness invalidation. A new selection uses
Low when the account advertises support; existing compatible choices are kept.
Manual model changes clear potentially incompatible effort. Local settings gain
no hosted controls or extra planning request.

There is no unverified Fast switch. On the signed-in account, `fast` returned
HTTP 400. `priority` and `ultrafast` requests completed but reported
`service_tier: default`. Low reasoning is the verified available latency lever.
The generic API's [Fast guide](https://developers.openai.com/api/docs/guides/fast-mode)
does not establish access through a ChatGPT plan. Account model metadata and
[sign-in inference requirements](https://developers.openai.com/siwc/token-sharing-open-source/models-and-inference)
govern the UI; [preview limitations](https://developers.openai.com/siwc/token-sharing-open-source/preview-limitations)
remain distinct from API-key billing and model support.

## Local-only performance

The first guide experiment was global. That version is not shipped: hosted and
local prompts are now separate. The local Lab planning prompt is byte-for-byte
the original **6,895 bytes**, with regression assertions that hosted reach
guidance does not enter local production or Lab prompts. Local inference keeps
its single call, reasoning-off behavior, schema and existing context budget.

| Gemma 12B local fixture | Intent | Warm median |
| --- | ---: | ---: |
| Original, eight turns | 6/8 | 1,146 ms |
| Final isolated local path, two eight-turn runs | 12/16 | 1,041 ms |

The first baseline request took 9,415 ms while loading the model and is excluded
from the warm comparison. The samples show no warm latency regression; they
do not prove an improvement or rule out small differences. Local remains faster
on this host. A discarded global-guide test passed 14/16 at warm median 1,263 ms;
its record is retained to make the decision to isolate prompts reviewable.

Module profiling also found eager Windows credential-store initialization added
about 15 MB of local-only working set. Credential storage is now initialized on
first actual use. A disconnected status read creates no files and loads no ACL
support DLLs. Once storage is used, its directory protections and serialized
refresh transaction still apply. Repeated matched local startup probes returned
working set to about 23 MB, with an isolated first-launch OS module-load outlier
retained. Full binary/UI and memory measurements are in the [scorecard](goal-scorecard.md).

## Visual review

The complete atlas contains **247 cases, 90 distinct plots, six overview sheets
and three captured production timelines**. Every accepted proposal and retained
failed selection was rendered through the shared engine. All six overviews,
the production captures and detailed outliers were inspected.

- Original Luna shallow-only selection (case 23) traverses most of 5–95 despite
  its reply. Hosted guidance fixes outer reach rather than merely moving focus.
- Phrase case 43 has varying stroke heights and does not realize the promised
  full/upper/full sequence. Field-level success was rescored using compiled span.
- Adaptive case 47 still dips to roughly 17 despite a shallow bias. A preference
  is not a hard bound; the prototype is unsuitable for this request.
- Luna's compound case 230 places accents near the tip. Sol case 236 instead
  anchors at the base with shrinking accents; case 237 removes them while
  preserving directional character. The sampled wire tracks the planned curve.
- Captures show same-stream retargeting and canceled queued remainder at Stop.
  The gray post-Stop region is canceled queued data, not evidence of commands
  continuing after Stop. These are commanded estimates, not carriage feedback.

No motion generator or transport timing changed. This review concentrates on
model mapping at fixture speeds (primarily 25%, with production limits 20–80);
it does not replace the existing low/mid/high generator matrix or physical review.

## Reproduction and retained evidence

The development-only CLI is built with `magichandy_labs`. Supply an existing
signed-in app profile; it uses the protected token source without copying or
printing credentials. Cloud runs consume plan usage. Reports contain synthetic
prompts/model output and must remain outside Git.

```powershell
go run -tags magichandy_labs ./cmd/cloud-motion-review `
  -data-dir .scratch/provider-review-data -models gpt-6-sol/low `
  -phase steering -repeats 2 -output .scratch/cloud-motion-review.json
go run -tags magichandy_labs ./cmd/cloud-motion-review `
  -local-model igorls/gemma-4-12B-it-qat-q4_0-unquantized-heretic:Q4_0 `
  -phase steering -repeats 2 -output .scratch/local-motion-review.json
```

Use `-phase suite` for the seven-interface comparison, and
`-hosted-guidance=false` for the original hosted baseline. The historical
`-steering` option adds the first guide; leave it off for the final production
guide. See [the visual-review workflow](motion-visual-review.md) for full-path
capture and rendering.

Retained local reports under `.scratch/`: `cloud-motion-screen.json`,
`cloud-motion-suite.json` and `.json.scored.json`,
`cloud-motion-steering-{baseline,candidate}.json`,
`cloud-motion-gemma-{baseline,final,isolated-final}.json`,
`cloud-motion-selected-final.json`,
`cloud-motion-production-{typed,final,gpt-6-sol,gpt-6.1-sol}.json`,
`cloud-motion-schema-failures.json`, and `cloud-motion-complete-atlas/`.
The raw suite is unchanged; corrected scores are separate. Generated atlases,
captures, databases, binaries and credentials are ignored runtime artifacts.

## Validation and limits

Go full tests, race tests, vet, lint, architecture/lifecycle gates and the
CGO-disabled build passed. Frontend typecheck, localization (2,871 keys in five
locales), build and 103 files / 788 tests passed. Added regression coverage
checks hosted/local prompt separation, typed union conversion and unwrap,
reasoning/readiness binding, lazy secure storage, and account-driven UI options.

The final review app uses signed-in GPT-6 Sol / Low, Motion Off and no Handy key.
It passed real readiness generation and a text-only HTTP chat request without
repair, fallback or motion. The exact final-build text reply, `7 + 2 = 9.`,
took 18,103 ms in one provider call; this outlier is retained in
`.scratch/motion-final-build-chat.sse` and reinforces that the smaller motion
fixture medians are not a latency guarantee. Earlier same-model text replies
took 3,524 ms. Final setup screenshots cover 1280×720, 390×844 and 320×568;
settings also passed the narrow checks and is left at the browser's restored
1020×956 viewport. Document width matches both narrow viewports and Stop
remains visible. Setup uses a one-line recommendation and collapsed speed
details. Browser input automation stalled during this pass; after inspecting
the enabled, unobstructed button, Continue was activated through the browser
debugger's DOM click. This confirms the page handler and resulting layout,
not a new native keyboard/input certification. Screenshots are retained as
`.scratch/motion-{setup,settings}-final-{desktop,390,320}.png`.
The earlier independent Claude review remains pending its session limit;
this work does not claim that approval.

OpenRouter and compatible conversations preserve included user text, subject
to ordinary whole-message budgets. No adult-content stripping, automatic
rewriting or cross-provider refusal retry was introduced. Technical-only
Autopilot is an explicit context choice. Own-key OpenAI/OpenRouter/Decisions
protocols have fixture coverage; live billing credentials were not supplied.
Decisions remains an optional bounded library selector, not a new real-time
pattern composer or an inferred ChatGPT-plan entitlement.
