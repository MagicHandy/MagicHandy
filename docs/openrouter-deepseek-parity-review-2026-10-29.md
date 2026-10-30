# DeepSeek and OpenRouter review — 2026-10-29

Release review of Claude's work from `claude/deepseek-parity`, based on
`1f89a3b9` (alpha.80). See [ADR 0065](decisions/0065-advanced-no-reasoning-and-openrouter-limits.md)
and the [alpha.81 release review](alpha81-release-review-2026-10-29.md).

The optional Advanced (no reasoning) level improves the recorded DeepSeek
results without a second generation. It does **not** establish parity with GPT.
Keep Terra as the quality recommendation. The default model, existing capability
selections, and ordinary Advanced prompts are unchanged.

## Independent evidence audit

The original report counted Go leaf-subtest passes as successful motion intent.
That overstates quality: some live suites record `intent_pass: false` without
failing a subtest. For example, CompoundMotionComparison finished 24/24 Go
subtests, but the reviewed DeepSeek note met only 10/24 recorded intent checks.
The original 290-case quality totals and alternative-variant percentage tables
are therefore superseded by this audit; they must not be cited as accuracy.

The table below counts actual `intent_pass` records for production variants:
`current` for constraint lifetime, `guidance` for phrase joins, `candidate` for
pace, and `baseline` for range. It excludes injected initial rejection fixtures
and the nine Edge wording rows that have validity results but no intent score.
This leaves 343 turn-weighted records across 23 suites. They are not 343
independent conversations; multiple turns share state. Episode-level development
checks are not fully represented by turn scores. Existing automatic intent checks
can themselves be too strict, so these are recorded checks, not human quality ratings.

The two Terra columns combine partial runs, filling missing suites from the other
run. Raw Go leaf results were 242/257 and 173/189; neither is a full independent
baseline. The two planning-note columns use different revisions. The reviewed
column is Claude's final note before the release follow-up's reach clarification.

| Configuration | Recorded intent checks | Median request |
| --- | ---: | ---: |
| Terra composite 1 | 318/343 (92.7%) | 4.894 s |
| Terra composite 2 | 315/343 (91.8%) | 5.004 s |
| DeepSeek Advanced 1 | 270/343 (78.7%) | 0.828 s |
| DeepSeek Advanced 2 | 280/343 (81.6%) | 0.673 s |
| DeepSeek note, prior | 302/343 (88.0%) | 0.797 s |
| DeepSeek note, reviewed | 297/343 (86.6%) | 0.890 s |

| Suite | Terra C1 | Terra C2 | DS A1 | DS A2 | DS note prior | DS note reviewed |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| ConstraintLifetimeChatLive | 7/7 | 7/7 | 7/7 | 7/7 | 7/7 | 6/7 |
| ConstraintLifetimeLive | 80/80 | 80/80 | 73/80 | 78/80 | 78/80 | 78/80 |
| HostedAutopilotDevelopsSession | 11/12 | 11/12 | 10/12 | 12/12 | 11/12 | 12/12 |
| HostedCompoundMotionComparison | 14/24 | 14/24 | 11/24 | 12/24 | 13/24 | 10/24 |
| HostedConversationDepthLive | 10/10 | 8/10 | 10/10 | 10/10 | 10/10 | 10/10 |
| HostedCreativePaceLive | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| HostedLayeredLiveOperations | 8/8 | 8/8 | 6/8 | 6/8 | 7/8 | 6/8 |
| HostedLayeredLivePlanning | 18/18 | 18/18 | 17/18 | 17/18 | 17/18 | 17/18 |
| HostedLayeredPhraseContinuation | 6/6 | 4/6 | 4/6 | 5/6 | 6/6 | 6/6 |
| HostedLayeredRangeComparison | 10/12 | 11/12 | 10/12 | 5/12 | 12/12 | 10/12 |
| HostedLayeredSessionLive | 9/9 | 9/9 | 9/9 | 9/9 | 9/9 | 8/9 |
| HostedPaceLiveComparison | 12/12 | 12/12 | 5/12 | 6/12 | 9/12 | 12/12 |
| HostedPhraseJoinChoices | 2/5 | 3/5 | 1/5 | 1/5 | 1/5 | 1/5 |
| HostedPhraseTimingScope | 6/6 | 6/6 | 6/6 | 5/6 | 4/6 | 5/6 |
| HostedResponseRepairLive | 7/12 | 8/12 | 0/12 | 1/12 | 5/12 | 3/12 |
| HostedStoppedMotionIntentLive | 23/25 | 23/25 | 20/25 | 22/25 | 22/25 | 23/25 |
| HostedUnappliedHistoryLive | 3/4 | 3/4 | 0/4 | 0/4 | 1/4 | 1/4 |
| IndependentVibrationAutopilotLive | 3/4 | 4/4 | 4/4 | 4/4 | 4/4 | 3/4 |
| IndependentVibrationLive | 24/24 | 22/24 | 17/24 | 22/24 | 24/24 | 23/24 |
| LayeredDirectionalLive | 9/9 | 9/9 | 9/9 | 9/9 | 9/9 | 9/9 |
| NaturalMotionConversationLive | 22/22 | 21/22 | 20/22 | 20/22 | 21/22 | 22/22 |
| NaturalVibrationConversationLive | 20/20 | 20/20 | 17/20 | 15/20 | 18/20 | 18/20 |
| OutputAwarenessLive | 11/11 | 11/11 | 11/11 | 11/11 | 11/11 | 11/11 |

DeepSeek remains weak on phrase joins, changes after an unapplied request and
repair (3/12 in the reviewed note capture, versus Terra's 7/12 and 8/12).
Some compound checks also demand an exact endpoint for short upper strokes
without the request requiring that endpoint. Their failures need individual
inspection; no parser boundary was relaxed to make a benchmark pass.

## Fresh production-path checks before release

`TestNoReasoningProductionContractLive` uses the real capability-to-schema-to-
parser path, a fresh provider per turn, and the actual 1,024-token answer floor.
It does not use the legacy provider wrapper that inserts and removes the note.
Every intent failure fails the test and is retained in the JSON capture.

The eleven cases cover Layered and Creative v2 speed-only changes, exact resume,
explanation while stopped, explicitly increased timing variation, two-short/one-
full phrases, a human-restricted 20–80 band, and one repair after an injected
invalid response. No transport exists. DeepSeek uses reasoning Off and throughput
routing; Terra uses low reasoning through the existing protected sign-in profile.

| Final matched fixture | Intent checks | Median request |
| --- | ---: | ---: |
| DeepSeek Advanced | 11/11 | 0.739 s |
| DeepSeek Advanced (no reasoning) | 11/11 | 1.048 s |
| Terra Advanced, low reasoning | 11/11 | 4.962 s |

This small matched rerun shows parity on these cases, not statistical superiority.
Two preceding runs of the final guide also passed all eleven cases. Earlier
failures remain retained: both models sometimes kept the current 5–95 working
band on a full-range request. The opt-in note now distinguishes the current band
from a human restriction and explicitly describes semantic 0–100 full travel.
It never changes saved motion limits or parses human language with deterministic
rules. The bounded case verifies that an explicit restriction still wins.

Two early fixture checks were corrected rather than training the model to them:
"upper-end" short strokes need not touch exactly 100, and "vary timing gently"
did not necessarily require increasing an already-varying pattern. The final
fixture asks explicitly for an increase and allows a reasonable upper region.
The original captures remain available for audit.

## Routing, limits and reasoning

The original evaluation used utility reply voice, normal production prompts,
strict output discovery, no provider fallbacks and denied provider data collection.
Most quality runs pinned Together. These tests do not measure explicit roleplay,
other hosts, or real-device behavior.

In 26 Planning/Operations calls per route, unsorted OpenRouter routing selected
OpenInference fp4 at an 8.65 s median; throughput selected Together at 0.89 s,
and latency selected Together at 0.91 s. This is an observed snapshot, not a claim
that default routing always selects the cheapest host. OpenRouter's October 28
[routing update](https://openrouter.ai/blog/announcements/how-we-rebuilt-provider-routing/)
describes balancing price, latency, throughput and reliability; explicit
[`provider.sort`](https://openrouter.ai/docs/guides/routing/provider-selection) remains supported.
New connections default to Fastest output. Existing saved connections keep their
routing. The UI explains the potential price tradeoff.

Connection output floors now apply per request: Intermediate 512, both Advanced
levels 1,024, preserving larger saved limits. Original completion lengths indicate
roughly 40–45% of DeepSeek Advanced replies exceeded the factory 256-token cap,
about 1% exceeded 512 and under 1% exceeded 1,024. This is a truncation-risk estimate
from completed replies, not a replay measuring actual failure at every cap.
ChatGPT sign-in sends no output cap; API-key connections benefit from this fix.

OpenRouter reasoning now reserves minimal 2,048, low 6,144 and medium/high/default
8,192 tokens in addition to the answer, bounded by one quarter of known context.
Off sends only the answer cap. Original reasoning runs had 5.1–5.4 s median
latency, 37–38% of completions above 2,048 tokens and maxima around 6.0K/7.4K.
They did not establish overall parity; improved repair came with much higher
latency. The original mixed quality percentages for these runs were not retained
as audited accuracy claims.

## Release corrections and limits

- Both Advanced levels now expose automatic response repair. The new level no
  longer exposes a second-opinion control the backend cannot use for it.
- The capability hint says the planning step applies to chat motion and does not
  switch provider reasoning. Autopilot, reviewer, Simple, Intermediate and ordinary
  Advanced contracts remain unchanged.
- HTTP 429 becomes a temporary rate-limit message; known quota codes and HTTP 402
  retain quota handling. No raw provider prose is retained or retried automatically.
- Evaluation traces retain numeric error codes only, including errors in HTTP-200
  streams. Keys, prompts and arbitrary error prose never enter trace output.
- The planning note is inert: stripped before motion validation and never used
  as speech, memory, a language rule or an independently executed motion command.
- Natural motion, count, reach and repair quality remain model-dependent. No new
  physical smoothness or safety claim follows from these text evaluations.

Original OpenRouter usage increased from about $0.120 to $2.367 during Claude's
broader experiments. The focused release checks added about $0.03. Historical
cached Together calls without reasoning cost about $0.0006 each; host prices and
cache behavior vary. Terra used the existing ChatGPT subscription. Credentials,
private analyses, raw replies and generated plots stay outside Git.
