# External-model context review — 2026-10-11

Implemented on the separate `codex/asynchronous-motion-review` branch. No release
merge or physical device run is part of this evaluation. See
[ADR 0042](decisions/0042-provider-conversation-windows.md) for the exact policy.

## What was tested

77 live generations used an existing signed-in ChatGPT profile through its
protected token source, with no credentials copied into the evaluation data:

- 36 screening calls: GPT-6 Sol and GPT-6.1 Sol, Low reasoning; three context
  settings; 72 KB and 160 KB synthetic conversations; story recall, interactive
  Creative v2 motion, and Autopilot phase continuation.
- 24 calls testing a history-first prompt arrangement, subsequently discarded.
- 16 genuinely continuing turns: eight per model, appending each actual generated
  reply before the next request. These add a character, transfer an earlier
  object, revisit the original setting, request a previously agreed motion phase,
  revise its pace, and continue talking while paused and stopped.
- One actual HTTP chat-path call after 222 stored messages, including history
  older than the former 200-message storage cutoff.

Fixtures are benign fictional travel conversations, not private user chat.
Most filler is deliberately generated, repetitive scene-journal material; this
is a targeted retention/continuity test, not a broad creative-writing benchmark.
Each early agreement has a correction: Juniper, a copper compass passed to Elian,
and the west quay instead of the east pier; beacon-phase motion changes from
speed 36 to 42 while keeping range 20–65 and zero rebounds. The continuing test
later passes the compass to Neri and revises pace to 30.

The production composer, provider adapter, strict response parser and shared
motion compiler are used. No runtime language filter, phrase replacement,
keyword lock, extra reviewer call or summary generation is added. Exact fixture
facts and numerical bounds are checked by the test harness only.

Both account catalog models advertised context 272000. OpenRouter and API-key
OpenAI credentials were unavailable on this host. Their expanded admission and
metadata paths are covered by protocol/configuration tests; no live paid API
quality or latency claim is made for those routes.

## Results

Each row contains one story, one interactive motion and one Autopilot request.
Times are end-to-end generation; medians across these three different tasks
describe this small run, not a service guarantee.

| Model / Low reasoning | Conversation | Window | Correct continuity | Times: story / chat motion / Autopilot |
|---|---:|---|---:|---|
| GPT-6 Sol | 72 KB | Recent | 0/3 | 4.616 / 2.823 / 3.192 s |
| GPT-6 Sol | 72 KB | Long | 3/3 | 3.252 / 3.023 / 5.752 s |
| GPT-6 Sol | 160 KB | Extended | 3/3 | 4.591 / 5.309 / 4.487 s |
| GPT-6.1 Sol | 72 KB | Recent | 0/3 | 4.988 / 3.701 / 5.036 s |
| GPT-6.1 Sol | 72 KB | Long | 3/3 | 4.659 / 5.881 / 5.599 s |
| GPT-6.1 Sol | 160 KB | Extended | 3/3 | 4.809 / 4.755 / 6.884 s |

Across the complete screening matrix, every request returned a valid contract in
one provider call. All 18 cases whose required history fit passed their factual
or motion checks. All 18 cases whose old agreement fell outside the window
missed it. In particular, Long is still insufficient for the 160 KB fixture.
Those failures remain in the report; valid JSON alone is not a quality pass.

The continuing-story test passed **16/16**, including subsequent corrections and
no motion during paused/stopped conversational turns. Sol's range was
2.929–7.432 s; 6.1 Sol's was 4.496–6.357 s. Some Sol turns reported cached input
(18,304 tokens), but cache hits did not reliably predict lower total latency.

The full HTTP chat test passed in **3.360 s**: 222 prior stored messages, 83,570
admitted prompt bytes, 14,489 actual input tokens, 46 output tokens, one provider
call, no repair or fallback, and zero transport commands. It correctly continued
the scene using the ship, current compass holder/material and corrected meeting
place. This validates the storage-to-provider path as well as direct services.

Recommendation: **GPT-6 Sol / Low with Long history** as the first choice for
longer sessions. Extended makes substantially longer retained stories usable;
its measured Sol requests still completed around 4.5–5.3 s. This improves
the context available to existing modes; it does not require a new mode.

## Cache experiment

Moving changing application instructions after a stable history prefix preserved
all in-window checks but produced only one nonzero cached-input result out of
24 requests and no consistent latency improvement. Different contracts and
automatic cache boundaries matter. The rearrangement was removed rather than
shipping an unproven additional message layout.

The final continuing run used the established order. OpenAI recommends stable
prefixes and growing history for caching, but explicitly does not guarantee hits;
see [prompt caching](https://developers.openai.com/api/docs/guides/prompt-caching).
The application records actual usage when returned and does not claim that a
large context will be free or cached.

## Motion visual review

All 56 motion-related rows from screening, the rejected prefix experiment and
continuing runs were exported through the shared engine. The atlas contains 46
distinct steady outputs and three contact sheets. Failed selections are retained
and labeled. Conversation-only rows show the unchanged score for comparison;
that plot is not evidence of dispatch during pause or Stop.

Inspected all three overview sheets and detailed figures for cases 2, 5, 18 and
54: the wrong short-window Autopilot choice, successful Long mapping, a more
pause-heavy valid mapping, and the final speed-only correction. The accepted
phase plots remain within the requested 20–65 outer window, while missing-context
Autopilot choices visibly span outside it. Planned and whole-percent wire curves
track closely in the excerpts; inspected phase portraits remain balanced and
report zero velocity/acceleration knot jumps. Some pauses belong to the preserved
existing character, not a new context feature. Numerical compilation checks passed.

Engine/sampler/transport code is unchanged. Tests cover current action admission,
pause, Stop and stale-work cancellation through existing paths. There was no
physical device, transport latency measurement, or physical-feel claim. The
curves are commanded estimates, not carriage telemetry.

Ignored evidence directory: `.scratch/long-context/`. Files include `screen.json`,
`prefix.json`, `flow.json`, `app-path.json`, `motion-cases.json`, `atlas.json`,
`atlas/index.html`, all three `atlas/contact-*.png`, and individual figures.
The discarded prefix report is retained; its runtime implementation is not.

## Local performance and delivery

Native/local prompt history stays at 64 messages / 24 KB. The new windows are
per external connection. Technical-only planning still excludes dialogue;
reviewer context is unchanged. Local generation gets no additional calls or
summary work. Regression tests compare the exact composed local messages with
and without hosted-budget fields.

Trimming local speech history before copying makes allocations independent of
the retained transcript: 6,048 B / four allocations for both 200 and 1,000 source
messages in the composition microbenchmark, roughly 1.55–1.81 microseconds on
this host. These are composition measurements, not whole-app inference timings.
The app continues to load only 64 rows for a normal local request. Storage
retention grows to 1,000, but browser reads fetch only the newest 200 rows using
an anchored display window; page byte bounds remain in place. Regressions cover
reload pagination during new appends and late commits inside the visible window.
Pruning now uses a sequence cutoff to keep persistence work small.

UI history choices are shared by Setup and Settings, preserve explicit saves,
invalidate connection readiness when changed, and are localized in all five
supported languages. Existing connections retain Recent. A newly selected model
on ChatGPT/OpenAI/OpenRouter with advertised capacity >=131072 gets Long;
manual/unknown models and generic compatible endpoints get Recent.
Context limits and usage costs are in a closed disclosure instead of permanent
paragraphs in the form. Longer inputs can still hit model or account limits, and
very long chats eventually roll out of the retained window.

Expanded external providers use a dark connection header, a muted steel-blue
form background and stronger dividers between rows. The history field and its
disclosure share one section without duplicate rules or excess vertical spacing.
The same editor styles apply in Setup and Settings.

Full Go tests/race, vet, lint, architecture/lifecycle checks, pure-Go build,
TypeScript, all 847 frontend tests and the canonical frontend build passed.
See [scorecard](goal-scorecard.md) for measured binary/UI budgets.
The isolated review app uses available local Gemma with motion Off, and its real
generation plus text-only chat path are checked. The external model tests above
use the existing signed-in profile without copying its credentials.

Final review at `http://127.0.0.1:50225/#/settings/chat/model`: the readiness
script passed; a subsequent text-only HTTP chat completed in 321 ms with one
provider call, a nonempty reply, no repair/fallback and no motion. The expanded
provider layout was visually inspected in the running production build with
Stop visible. The working tree's application source matches the published build.

## Reproduction

From the review worktree, use an explicitly authorized existing ChatGPT profile
and an ignored report path. The tests skip without these environment variables:

```powershell
$env:MAGICHANDY_HOSTED_DATA_DIR = 'path-to-existing-signed-in-app-data'
$env:MAGICHANDY_CONTEXT_REPORT = '.scratch/long-context/screen.json'
go test -tags 'liveeval magichandy_labs' ./internal/chat -run '^TestLongContextLive$' -count=1 -v -timeout 20m
$env:MAGICHANDY_CONTEXT_REPORT = '.scratch/long-context/flow.json'
go test -tags 'liveeval magichandy_labs' ./internal/chat -run '^TestLongContextFlowLive$' -count=1 -v -timeout 10m
$env:MAGICHANDY_CONTEXT_APP_REPORT = '.scratch/long-context/app-path.json'
go test -tags 'liveeval magichandy_labs' ./internal/httpapi -run '^TestLongContextLiveApp$' -count=1 -v -timeout 3m
```

No evaluation creates a device playback loop. Hosted generation consumes account
usage. Create the output directory before invoking the test; the raw reports
contain synthetic messages and model replies and remain outside Git.
