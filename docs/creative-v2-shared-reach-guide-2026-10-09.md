# Shared Creative v2 reach guide — October 9, 2026

Follow-up to the [hosted latency and intent review](cloud-motion-latency-review-2026-10-08.md).

## Question

The October 8 review added a reach guide to the Creative v2 contract for hosted
models only and kept local prompts byte-for-byte unchanged. Its local evidence
came from the first draft of the guide (14/16 against 12/16), not the final
text. This review asks whether the final guide also helps local models, and
whether it costs anything elsewhere.

## Evidence

All runs used local Gemma 12B (`igorls/gemma-4-12B-it-qat-q4_0-unquantized-heretic:Q4_0`
through Ollama), the response schema and the existing parsers, compiler and
engine review. No device was connected.

| Fixture | Without guide | With guide |
| --- | ---: | ---: |
| Eight-turn steering fixture, `cmd/cloud-motion-review`, 3 repeats | 18/24 (reach 9/15) | 24/24 (reach 15/15) |
| Same fixture, independent probe, 6 repeats | 36/48 (reach 18/30) | 48/48 (reach 30/30) |
| Creative v2 live request suite, cases and holdouts, production chat path, 3 repeats | 96/102 | 97/102 |
| Warm median, harness run | 1,103 ms | 1,267 ms |

Without the guide, the two upper-quarter requests failed every repeat: the model
moved focus toward the tip but left broad strokes enabled, so compiled motion
still swept roughly 5–95. This is the miss the October 8 review recorded for
GPT-6 Luna. With the guide, the same requests compile to about 75–95. The
rendered atlas (48 cases, 11 distinct plots) shows no new excursions or reversal
artifacts; deepest-quarter, 35–65 and full-stroke requests compile alike in both
variants.

On the request suite, `pace-where` improved (0/3 to 2/3) and `evolve` dropped
one repeat (3/3 to 2/3): that reply promised a refresh with `action:none` and no
edit, unrelated to reach. `mixed-base` failed every repeat with and without the
guide. GPT-6 Sol passed the steering fixture 16/16 at a 3.37 s median with the
same guide on October 8, so on this fixture the local model now matches the
hosted one.

## Decision

The guide leads every Creative v2 contract, local and hosted, in production and
in the Lab. The hosted-only prompt variant and capability flag are removed; the
local refusal retry still rebuilds its own contract. `cmd/cloud-motion-review
-reach-guide=false` reproduces the earlier baseline for either kind of model.
Hosted and local prompts can still diverge where a measured difference calls for
it; none does today.

## Limits

These are reach-steering fixtures with neutral technical phrasing at fixture
speeds, one local model, temperature 0.1 in the Lab and the production sampling
in the request suite. They do not establish physical comfort, behavior with
other local models or natural conversational phrasing. Reports, the atlas and
the probe source are retained under `.scratch/shared-reach-guide-20261009/`.
