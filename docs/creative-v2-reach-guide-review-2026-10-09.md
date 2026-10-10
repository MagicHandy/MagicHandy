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

## October 10 amendment: servers that drop the schema

The evidence above enforced the response schema. Before release, the
14-request × 3 probe from the
[unconstrained-edits review](creative-v2-unconstrained-edits-review-2026-10-09.md)
was rerun on the production chat path with the same model through Ollama, in
one session, on `main` and with the guide:

| Output constraint | `main`, no guide | With the guide |
| --- | ---: | ---: |
| Response schema enforced | 42/42 | 42/42 |
| JSON mode, schema dropped | 42/42 | 38/42 |
| No constraint | 40/42 | 31/42 |

Without the schema, the guide made the model write malformed replies: code
fences, a bracket closing `edits` inside an item, a reply string inside
`edits`, and a repeated `focus` group. The parser correctly rejects each. The
schema prevents all of them.

Ollama and the managed llama.cpp runtime always send and enforce the schema,
so they keep the guide. An external llama.cpp server is any OpenAI-compatible
endpoint, and some ignore structured output, so its Creative v2 contract and
Lab prompts omit the guide and match `main`. With that split the probe
measured 42/42, 42/42 and 40/42; the two remaining rejections are the
`mix_percent` partial group the earlier review kept rejected. The steering
fixture stayed 24/24 with the guide and 18/24 without, and the request suite
measured 96/102 either way, all 102 replies valid.

The same rerun found that Ollama's OpenAI-compatible endpoint ignored
`chat_template_kwargs.enable_thinking=false`. With reasoning off, Gemma 12B
still reasoned until the output budget ran out: 11 of 28 Creative v2 replies
were valid, and the rest came back empty or cut off after about 14 s each. Requests
with reasoning off now also send `reasoning_effort: "none"`, which Ollama
honors: 28/28 valid at a 755 ms warm median, without the guide, as an external
server is now prompted. The managed llama.cpp runtime (b11149) accepts the
field; with the guide it returned 28/28 valid replies for each request form, at
a 1,037 ms warm median.

The five-turn live conversation fixture
(`TestCreativeV2LiveProductionConversation`) fails on `main` as well as with the
guide. Over eight non-stopping repeats each, 16 of 40 turns met their intent
either way, at an 850 ms median: the model never placed the rebounds at the base
or answered "keep varying" with an edit. With the guide it raised inertia when
asked in 5 of 8 repeats instead of 0, and put the first tip focus at 95 instead
of 100 in 5 of 8.

Windows amd64, Go 1.26.9, `CGO_ENABLED=0`, `-trimpath -buildvcs=false -ldflags
'-s -w'`: 21,789,184 B before both fixes and 21,798,400 B after (+9,216 B).
Reports are retained under `.scratch/alpha58-review/`.
