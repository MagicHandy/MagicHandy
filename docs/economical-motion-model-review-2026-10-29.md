# Economical hosted motion models — 2026-10-29

This is the baseline screen before reasoning controls were implemented. The
[follow-up implementation review](openrouter-reasoning-review-2026-10-29.md)
records the new controls, response-format changes, supplied-example trials and
cumulative spending. Current-app limitations below describe that baseline.

GPT 6.1 Terra with low reasoning remains the quality recommendation. DeepSeek
V4.1 Flash 1017 on Together is the most promising economical alternative tested:
it is faster, but does not yet match Terra's consistency on the Advanced Layered
contract. No production defaults, prompts, model connections, or device settings
were changed by this evaluation.

Latency is an acceptance criterion, not a tie-breaker. A cheaper route that
regularly takes substantially longer than Terra is not a good interactive choice.
The MiniMax/SambaNova route and Qwen/Alibaba screen failed that criterion here.

## Method and scope

Source: `claude/handy-top-speed` at `9247dc54` (application code `a97ddbe0`).
Seven fixed cases covered an indirect start request, an explicit gentle start,
three short upper strokes followed by one full sweep, asymmetric down/up speed,
natural upper-region variation, broader upper-region coverage, and maximum pace.
Three Terra cases and six fast DeepSeek cases received repeats.

The existing explicit persona/input fixtures used in the Vanessa register
evaluation were reused alongside one saved persona and six saved user utterances.
Their wording was preserved. The gentle-start case combines two existing inputs.
Vanessa is an installed model, not a saved persona; it was not loaded or queried
during these tests. Private inputs and outputs remain in ignored scratch files.

Every provider received the production Advanced Layered prompt and response
contract, plus the same instruction to analyze the adult context and return a
brief neutral technical description. **This evaluates motion planning with adult
input context, not unrestricted erotic dialogue quality.** Some models ignored
the neutral-output instruction; that behavior is not scored as a benefit. No
prompt rewriting or repeated attempts to overcome a policy refusal were used.

The harness called `chat.Service.Complete` to compose requests and validate
responses. Responses using the strict wire schema were normalized with
`llm.DomainOutput` exactly once; already-normalized ChatGPT responses were not
normalized again. Original state, limits and parser remained fixed. Automatic
repair was excluded from first-attempt results. Three separate, single-attempt
validation repairs used the production `Service.Repair` feedback and unchanged
contract. Refusals were not repair candidates.

The shared `motion.FlowTarget` / `ReviewMotionOutput` path rendered accepted
Layered targets without constructing a transport. No hardware was connected or
commanded for this evaluation. The already-running review app stayed idle with
LLM motion off.

There were 59 OpenRouter requests and 10 ChatGPT subscription requests. Final
OpenRouter account usage and the per-request cost ledger both reported
**$0.050914287**, against a $2 authorization. Two refused requests lacked individual
cost fields; final account reconciliation showed no additional charge. The harness
reserved conservatively before each call and disabled provider fallback. It used
an output limit of 2,048 tokens and a 22-second OpenRouter response deadline.

Timing below is the full response time, not first-token time. Motion cannot use
an incomplete, unvalidated JSON stream. Local validation added roughly 0–1 ms.
These are small sequential samples on this host, with mixed cache warmth and
unequal case counts after screening. They are neither a service guarantee nor
a statistical tail-latency study. Reasoning tokens can consume the output budget;
OpenRouter documents that behavior in its [reasoning guide](https://openrouter.ai/docs/guides/best-practices/reasoning-tokens).

## First-attempt results

“Valid” means the production response validator accepted the result. It does
**not** mean the requested motion was correctly followed. Valid no-action replies
are included, even when motion was expected. Repair calls are excluded here.
Latency includes unsuccessful attempts; fast refusals are not useful responses.

| Model / provider / reasoning | Attempts | Valid | Median | Observed range | Practical finding |
| --- | ---: | ---: | ---: | ---: | --- |
| GPT 6.1 Terra / ChatGPT sign-in / low | 10 | 10 | 4.12 s | 2.81–7.07 s | Best consistency; no policy refusals; one unrequested pace-modulation choice |
| DeepSeek V4.1 Flash 1017 / Together / provider default | 7 | 6 | 2.25 s | 1.16–3.68 s | Best candidate using settings the current app can express; semantic misses remain |
| DeepSeek / Together / none | 13 | 11 | 1.14 s | 0.42–2.42 s | Fast, but missed starts and some range/focus requests; app does not expose this setting yet |
| DeepSeek / Together / low | 6 | 4 | 5.03 s | 1.29–8.98 s | Two length-limited incomplete outputs; no quality/latency case for this setting |
| MiniMax M3.1 Flash / ModelRun / low | 2 | 2 | 0.65 s | 0.56–0.75 s | Very fast, but a no-action start and incomplete full-range compound motion |
| MiniMax / ModelRun / medium | 6 | 5 | 0.79 s | 0.58–2.09 s | Better compound output, but invalid range expansion and no action for maximum pace |
| MiniMax / SambaNova / medium | 3 | 2 | 11.13 s | 4.21–15.26 s | Too slow; still produced a range validation error |
| MiniMax / Fireworks / low | 1 | 1 | 3.05 s | One sample | Explicit gentle start worked; insufficient evidence for a recommendation |
| GPT 6 Luna / Azure / low | 7 | 6 | 3.78 s | 2.42–9.12 s | Reasonable inexpensive option, but range failure and a slow variation request |
| GPT 6 Luna / Azure / none | 4 | 1 | 2.78 s | 2.47–7.76 s | Three content-filter responses; unsuitable in this configuration |
| Qwen 3.8 Flash / Alibaba / thinking disabled | 2 | 2 | 6.77 s | 5.19–8.35 s | Slower than matched Terra cases; compound output missed full endpoints and changed speed |
| Dolphin Mistral 24B Venice Edition / Venice / default | 2 | 1 | 1.68 s | 1.64–1.73 s | Fast, but failed the Advanced Layered contract |
| Mercury 3 / Inception / low | 3 | 1 | 0.82 s | 0.73–3.79 s | Two explicit content-policy refusals; successful response took 3.79 s |

MiniMax's ModelRun responses reported zero reasoning tokens even when low or
medium was requested. That is an observation, not proof of what the provider did
internally. Provider identity matters: treating all MiniMax routes as equivalent
would hide a roughly subsecond versus 11–15 second difference on this screen.

## Motion quality and refusal findings

- **Terra:** both compound repeats preserved three upper strokes and one complete
  0–100 sweep. Both broader-coverage repeats expanded the outer band from 75–100
  to 55–100 and selected widths of 30–45, with the upper endpoint anchored. The
  directional request used down/up rate factors of 100/25. Both start cases
  started. One maximum-pace response selected speed 100 but added an unrequested
  pace layer and variation; the repeat selected a clean 100% plan. Schema success
  is therefore not a perfect intent score, even for Terra.
- **DeepSeek, default:** the explicit gentle start, directional request and maximum
  pace worked. Broadening selected the existing 75–100 band at its full width,
  rather than meaningfully expanding coverage. Natural variation retained roaming
  geometry instead of establishing the requested upper focus. The compound
  response contained an axis-only layer edit and was rejected. An indirect start
  received a valid conversational response without a start action.
- **DeepSeek, none:** both compound attempts had the correct count and a full
  0–100 sweep. The first upper segment was 80–95 and the repeat 70–100. Maximum
  pace and directional asymmetry worked. The natural-variation outputs introduced
  alternating movement between the lower and upper regions; the second also
  changed base speed and directional bias. These are interesting valid patterns,
  but weaker matches for the requested upper focus. Broadening was minimal in
  one attempt and geometrically invalid in another. The explicit gentle-start
  response selected a width larger than its own outer interval and was rejected.
- **MiniMax:** the medium ModelRun compound output correctly reached 0 and 100,
  unlike its low-effort result. Its variation plan retained the upper endpoint but
  repeatedly covered almost the full range. It returned no motion for the maximum
  pace case. That prevents recommending it solely on its excellent latency.
- **GPT content handling:** Terra low had no refusals in ten technical-output
  trials. Luna low had none in seven; Luna none returned three `content_filter`
  outcomes in four. Those are different models/settings/routes, not evidence
  that all GPT providers behave alike. This test does not establish unrestricted
  roleplay support or causally prove that reasoning effort changes moderation.
- **Other explicit refusals:** Inception returned two 403
  `content_policy_violation` errors. They were retained and no circumvention
  attempts were made. Other invalid outputs were parser/geometry failures,
  truncations or wrong decisions, not automatically labeled policy refusals.

## Single-attempt validation repair

| Rejected response | Additional response time | Outcome | Total model time |
| --- | ---: | --- | ---: |
| DeepSeek none, broader coverage repeat | 1.03 s | Valid expansion to 50–100 at width 50 | 3.45 s |
| DeepSeek none, explicit gentle start | 0.96 s | Same incompatible width remained; rejected | 2.23 s, no applicable motion |
| Venice Dolphin, compound | 1.01 s | Still invalid | 2.74 s, no applicable motion |

Repair can fit the latency budget, but it did not reliably fix the weaker
responses. It cannot detect every valid-but-wrong plan either. These data do not
justify weakening the geometric validator or silently changing the user's words.

## Price and current-app compatibility

The exact candidate identifier is `deepseek/deepseek-v4.1-flash-1017`, tested on
**Together**. The fetched endpoint catalogue listed $0.30 per million input tokens
and $1.20 per million output tokens for that route, with cheaper cache reads. The
model-wide headline price belongs to a different endpoint and should not be used
to estimate these trials. Rates and available routes can change; consult the
[endpoint catalogue](https://openrouter.ai/api/v1/models/deepseek/deepseek-v4.1-flash-1017/endpoints).

At those rates, an illustrative uncached 4,000-input/500-output turn costs
$0.0018 (about $1.80 per 1,000 turns). This excludes retries and longer history;
actual cost depends on the full prompt/schema, output and cache usage. The seven
default-setting trials cost $0.00982 in total. ChatGPT sign-in consumed subscription
usage rather than OpenRouter credit; it was not treated as zero economic cost.

For a provisional current-app trial: choose this model, allow Together only, use
strict structured output, keep the existing full Advanced guidance, and retain
normal response validation. An optional single repair may help but is not a
quality guarantee. The routing behavior and controls are documented in
[OpenRouter provider selection](https://openrouter.ai/docs/guides/routing/provider-selection).

The experimental `reasoning: {effort: "none"}` request is **not currently expressible
in MagicHandy's OpenRouter connection settings**. `ModelConnection.validateReasoning`
accepts reasoning effort only for ChatGPT/OpenAI; `CompatibleOptions` and the
compatible completion body do not send it. The runner also requested latency-first
routing, whereas pinning Together is the relevant supported app control. Do not
promise the 1.14-second result by merely selecting DeepSeek in today's UI.

If an OpenRouter reasoning control is implemented later, discover model/provider
support, retain an explicit provider-default choice, and test its effect per
model. Do not globally disable reasoning: MiniMax declares it mandatory, and the
quality/moderation observations above are not uniform across models. A deployment
candidate still needs longer conversations, Autopilot, repair/staleness handling,
other motion modes and independent-device control checked through the app.

Venice's logged-in web account showed no available credits. The evaluated Dolphin
model was Venice-hosted **through OpenRouter**, not a test of Venice's newer direct
models or API subscription. Its failure does not establish that those other models
would fail. Direct Venice remains unevaluated; its current offerings are listed in
the [Venice model catalogue](https://docs.venice.ai/models/overview).

## Visual review and retained evidence

Ignored local evidence is under `.scratch/model-economy-2026-10-29/`:

- `fixtures.json`, `requests.json`, individual response/validation files and the
  one-shot repair requests preserve exact inputs and failures privately.
- `decisions.json` and `statistics.json` separate parsed motion from timing and
  provider outcomes; `ledger.json` and `spending-final.json` reconcile spending.
- `atlas/index.html` has a record for all 69 attempts, 36 distinct motion plots
  and three overview sheets. Rejected, truncated, refused and no-action results
  retain their own cards. Plots omit private persona and reply text; raw evidence
  remains in the separate ignored response files.

All three overview sheets were inspected, followed by detailed compound,
directional, maximum-pace, DeepSeek alternating-focus and MiniMax range-variation
plots. The plots confirm the semantic differences above. The inspected accepted
curves have continuous planned turns; model choice primarily changed geometry,
relative timing and coverage here. These are commanded estimates from the shared
engine, not measured device motion. No physical smoothness comparison, startup or
live retarget test was performed as part of this model screen.

The review app at `http://127.0.0.1:50314/#/settings/chat/model` runs the current
application source. `scripts/check-review-llm.ps1` completed a real generation
against `review-12b`; the same build also has a successful text-only app chat
request with motion disabled and no repair/fallback. The selected local review
model is a readiness configuration, not a change to the recommended production
ChatGPT default.
