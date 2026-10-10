# Creative v2 reach guide — October 9, 2026

## Question

Creative v2 models often answered "stay in the shallow upper quarter" by moving
the focus toward the tip while leaving broad strokes enabled, so the compiled
motion still swept most of the slider. A short guide written for hosted models
on the hosted-connections review branch fixed this there. This review checks
whether the same guide helps local models on `main`, and what it costs.

## Change

The Creative v2 contract and its Lab prompt now open with the guide: resolve the
requested reach before texture; a request that confines all motion to a region
needs the outer range to bound it, since focus alone cannot exclude broad
strokes; inside a restricted range both broad and shorter strokes stay within
its endpoints; a new request can replace an earlier restriction; and the edits
must actually realize the promised reach. It is model guidance, never a
text-triggered rewrite. The text is byte-identical to the hosted-review branch.

## Evidence

Local Gemma 12B (`igorls/gemma-4-12B-it-qat-q4_0-unquantized-heretic:Q4_0`
through Ollama), response schema enforced, existing parsers, compiler and
engine review; no device connected.

| Fixture | Without guide | With guide |
| --- | ---: | ---: |
| Eight-turn steering fixture, Lab path, 3 repeats | 18/24 (reach 9/15) | 24/24 (reach 15/15) |
| Same fixture, 6 repeats, measured before the port | 36/48 | 48/48 |
| Creative v2 live request suite, cases and holdouts, production chat path, 3 repeats | 96/102 | 96/102 |
| Warm median, steering fixture | 1,158 ms | 1,319 ms |
| Warm median, request suite | 742 ms | 742 ms |

Without the guide, both upper-quarter requests failed every repeat; with it they
compile to about 75–95 instead of sweeping roughly 5–95, as the rendered atlas
of the earlier run showed. On the request suite, `pace-where` improved by one
repeat and `evolve` lost one: that reply promised a refresh with `action:none`
and no edit, unrelated to reach. `mixed-base` failed every repeat either way.
The contract grows by 713 bytes.

## Limits

Reach-steering fixtures use neutral technical phrasing at fixture speeds with
one local model. They do not establish physical comfort, behavior with other
models or natural conversational requests. Reports and probe sources are
retained under `.scratch/reach-guide-main-20261009/`.
