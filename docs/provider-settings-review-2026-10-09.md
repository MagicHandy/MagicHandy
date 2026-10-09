# Provider settings and chat routing review — October 9, 2026

This follow-up is on `codex/chatgpt-motion-planning`, after `c5e5240e`.
It addresses provider separation, visible routing, and the distinction between
adult vocabulary, a valid refusal response, and a failed provider request.
It does not change motion generation, local prompts, or the shared engine.

The subsequent same-day request adds an **opt-in local retry after an explicit
hosted refusal**. This intentionally amends the earlier default no-rerouting
behavior. It is not enabled automatically.

## Settings behavior

Chat model directly offers Local model, ChatGPT, OpenRouter, OpenAI API and
Other compatible provider, including providers without a saved connection.
Each selection exposes only its own connection editor. ChatGPT account controls
are not part of the OpenRouter editor. API credentials remain bound to the
connection ID, provider and normalized endpoint. Provider identity stays visible
even if a user gives a connection a misleading name.

Saved model routing is a read-only backend projection from the same role
resolvers used by inference. It reports Chat and Autopilot provider, model,
endpoint host and context policy. It describes the saved destination, not
credential or inference readiness. Draft choices cannot change this summary.
A missing connection is explicitly unavailable and cannot silently fall back
to a local endpoint. Saving an assigned hosted connection without a model is
rejected; unassigned configurations can be retained for later setup. Settings
can retain a model choice before credentials are available, but setup still
requires a successful real generation before completion.

Autopilot has its own selector. Assigning a different hosted connection defaults
to motion-details-only context. Full conversation sharing remains an explicit
choice. A connection shared by both roles is labelled accordingly. Adding a
named connection does not assign either role. Discard restores model fields
without deleting immediately saved credentials or erasing unrelated persona,
voice or prompt drafts. Detailed context/refusal wording is a disclosure, and
labels remain accessible without duplicate visible headings.

The extracted hosted editor fences stale results when its connection changes,
keeps newer readiness results when initial status arrives late, and clears a
catalog when credentials or endpoints change. ChatGPT account generations cannot
be replaced by an older status response. Decisions is a separate disclosure;
its status request is lazy for a local-only setup, and an older parent status
cannot overwrite a fresh Decisions key update.

Long explanations use the existing dismissible-notice preferences, including
provider credentials, routing/context sharing, response-speed recommendations,
manual IDs, retry behavior and Decisions billing. Users can hide them for this
visit or persist the choice, then restore them through Settings > Access >
Informational notices. The close control remains usable in a read-only provider editor while
credential/model controls stay disabled. Saved destinations, unsaved changes,
active errors, readiness and the retry toggle itself cannot be hidden as guidance.

## Independent consultation

Claude CLI 2.1.280 completed a consultation with `claude-opus-5-5` (Opus 5.5),
confirmed in the response's model usage. The successful call used high effort,
supplied source, and no tools. It took 122.8 seconds. An earlier source-reading
call stalled and was stopped; it is not counted as a completed review.

The consultation identified the frontend's inferred routing summary, missing-ID
display, separate hosted Autopilot privacy default, Add changing Chat, incomplete
assigned models, draft comparison and stale status problems. Those findings
were corrected and regression tested. Existing backend credential-binding tests
already cover changed endpoints. The UI keeps a flat, provider-labelled list
with advanced named connections instead of adding nested groups. The review
covered supplied source, not an independent live UI or security audit; it does
not close the earlier broader implementation-review limitation.

## Live language evidence

The review app's available chat history contained neutral diagnostics, not a
representative explicit user conversation. No private historical conversation
was sent to an outside reviewer. A reproducible, opt-in test runs six sequential
technical inputs through the app chat path, using Utility voice, Motion Off,
and a fake transport. Cases cover a neutral baseline, profanity, neutral adult
anatomy/vocabulary, a literal slang token, a technical future motion preference
with motion still off, and a request to write a brief refusal without roleplay.

| Provider/model | Valid replies | End-to-end request times, ms | Median |
| --- | ---: | --- | ---: |
| Signed-in ChatGPT, GPT-6 Sol / Low | 6/6 | 2005, 2248, 2942, 2089, 2739, 3796 | 2493.5 ms |
| Local Gemma 12B, installed heretic Q4_0 variant | 6/6 | 9230, 386, 724, 334, 690, 1007 | 707 ms overall; 690 ms warm |

All twelve turns used one provider call, valid app replies, no repair, no
semantic fallback and no device commands. Gemma's first request included cold
loading; these small sequential samples are not a general model speed ranking.
The explicit refusal wording was returned inside a valid reply, which is
distinct from a provider's typed refusal outside the expected response schema.

This establishes that adult vocabulary alone did not break these technical
requests. It does **not** establish support for explicit sexual roleplay,
long erotic conversations, every persona, or an unrestricted content policy.
No model was asked to produce erotic text or to bypass a provider's safeguards.
OpenAI documents that refusals can be returned outside a supplied structured
schema: [structured output refusals](https://developers.openai.com/api/docs/guides/structured-outputs?api-mode=responses).

A deterministic OpenRouter integration fixture checks the actual destination
(`openrouter.ai/api/v1/chat/completions`), selected model, and verbatim user
content while ChatGPT is configured as the separate Autopilot. A typed-refusal
fixture asserts one request, a visible error, no leaked provider details, no
repair/rerouting/fallback, and zero motion commands. These are adapter tests,
not live OpenRouter inference: no OpenRouter credential was available for this
review. OpenRouter host fallback remains its own explicitly configurable
availability policy. MagicHandy only retries a refusal through the explicitly
enabled local path below; it never chooses another hosted connection.

## Opt-in local refusal retry

`llm.retry_refusal_locally` defaults to false and applies only to interactive
chat. Enabling it permits one attempt using the saved local model when the
selected hosted adapter returns a typed refusal (including a typed content-filter
finish). Authentication, quota, transport errors, malformed/incomplete output,
and refusal wording inside an otherwise valid reply do not trigger it. There is
no classifier call, text-keyword heuristic, recursive retry or automatic download.
Autopilot retains its independently assigned provider and existing failure policy.

The retry remains in the original chat turn and canonical history. It uses the
original user request and conversation, the common saved prompt/persona profile,
and the normal local model contract and budgets. Hosted Creative v2 reach guidance
is excluded, as verified by captured request bodies. It takes the existing local
request lane after releasing the hosted lane. It makes one local generation and
disables malformed-output repair/salvage for this attempt. Both results still go
through the existing parser, capability checks, motion engine and publication
guards. Stop cancels both in-flight requests and rejects a request admitted
between stages; settings/session/account changes fence late results.

Settings shows the option under the hosted Chat editor, the configured local
destination, and a disclosure for configuring the same saved local model. The
saved-routing summary gains an On refusal row only after Save. An inactive local
editor does not show the hosted Chat model's readiness or invoke its Load/Test
actions. A local retry is announced during streaming and recorded on the reply.
Host diagnostics retain the declined provider/model, time before the retry,
actual local provider/model and total provider calls. Provider diagnostics remain
host-only for remote clients. The ordinary local-only path adds no inference,
prompt, model initialization or external request.

Deterministic tests cover default-off, one typed-refusal retry, valid refusal
wording, unavailable hosted/local endpoints, malformed hosted/local output,
no leaked hosted authorization, single history insertion, persisted two-call
provenance, actual HTTP cancellation in both stages, the between-lanes Stop epoch,
and the separate local motion prompt. All use fake transports. The HTTP
cancellation fixture drains request bodies before waiting so the test server can
observe disconnects; merely rejecting a late response was not accepted as proof
of request cancellation.

An opt-in live test uses a simulated hosted refusal followed by real Ollama Gemma
12B inference through the app chat path. The reply was `7`, with two provider
calls, no repair, no semantic fallback and zero motion commands. Total request
time was 8800 ms, including 3926 ms model load and 4602 ms prompt processing.
This is a cold-path example, not a latency guarantee. The ignored report is
`.scratch/provider-local-retry-live.json`; repeat with
`TestLocalRefusalRetryLiveChat` under the same live-eval build tags.

Opus 5.5 completed a second supplied-source CLI consultation in 137.3 seconds.
It prompted stronger cancellation tests, an explicit Stop epoch check immediately
before the local provider runs, capabilities constructed from local settings,
and tests for local prompt isolation and persisted provenance. Several proposed
issues depended on code outside the supplied excerpt: prompt sets are common to
providers, runtime identity excludes motion-owner overrides, and normal Stop
also cancels the enclosing chat turn. Those were checked rather than treated
as confirmed defects. Neither consultation is a full security audit.

To repeat the language capture, opt into
`go test -tags 'liveeval magichandy_labs' ./internal/httpapi -run '^TestHostedLanguageLiveTextConversation$' -count=1`
with `MAGICHANDY_LIVE_MODEL`, the existing host auth directory in
`MAGICHANDY_HOSTED_DATA_DIR` for ChatGPT, and `MAGICHANDY_EXPERIMENT_CAPTURE`
pointing to an ignored report path. Do not commit captures, auth data, prompts from private
history, or raw provider output.

## Validation and scope

The final source passes TypeScript, five-locale audit, all 104 frontend files
(798 tests), and the frontend build,
`go test ./...`, `go test -race ./...`,
`go vet ./...`, golangci-lint, and the `CGO_ENABLED=0` production build.
The existing main-bundle size advisory remains. No dependency was added. One
parallel frontend run failed an unchanged video-clock test by 0.3 ms beyond its
50 ms tolerance. The full rerun with two workers passed; no test or gate was
weakened. The original log is retained in `.scratch/provider-notices-web-final.log`.

Desktop interaction verified the first-class provider list, scoped OpenRouter
editor, unchanged saved ChatGPT routing while editing, disabled Save for a
missing model, and restored readiness. At 390 px the measured document and
body widths both equal 390 px; the 320 px check also has no horizontal overflow.
Stop remains fixed and accessible, including read-only mode. A stalled
browser tab was recovered using a fresh review tab. Screenshots and live
reports are retained only in `.scratch/provider-settings-*` and
`.scratch/provider-language-*`.

The review app uses simulation and the existing signed-in review profile,
without copying credentials or connecting physical hardware. Its exact selected
ChatGPT model passed `scripts/check-review-llm.ps1`; a text-only production
chat returned `3 + 4 = 7.` in 1831 ms with one provider call, no repair,
fallback or motion. The final embedded build was rechecked after the toggle and dismissible guidance
changes: real ChatGPT readiness passed, and chat returned `4 + 3 = 7.` in
2326 ms with one call, no repair/fallback/motion, even with local retry enabled.
The review profile retains ChatGPT / GPT-6 Sol / Low as primary, with the installed
Ollama Gemma model configured for the demonstrated retry. The factory default
for retry remains off. In the final UI review, dismissing retry guidance for this
visit removed its text while the checked toggle and destination remained visible.
The Chat model explanation also stayed hidden after reload with "Don't show
again"; restoring it through Access > Informational notices made it visible
again without changing model settings.
The simulator is running at `http://127.0.0.1:50218/#/settings/chat/model`.

Binary and browser payload measurements are in the
[scorecard](goal-scorecard.md). The optional new-process memory comparison was
rejected by automatic approval review with only `blocked by policy` as its
stated reason; it is not recorded as a passing measurement. Local inference
contracts and call counts are unchanged. No new runtime memory or p95 latency
claim is made. This change does not alter motion character or mapping, so the
prior shared-engine atlas remains the relevant motion evidence.
