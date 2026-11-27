# Inkling 2 Mini as a local motion model

## Hardware recommendation

Keep **Gemma 4 12B Intermediate** as the automatic selection on the tested
16 GB RTX 5070 Ti. On 100 matched requests it delivered 99 exact results.
Inkling's tuned 32K Basalt connection delivered 97 exact results, including
one recovered by a retry. Both respected all 44 requests to hold. Inkling
handled a lower-endpoint edit that Gemma missed. The full comparison supports
offering it as an opt-in reasoning model on this machine.

For that option, select an **OpenAI-compatible** connection to the local
Basalt server, use the served ID `inkling-2-mini`, choose **Intermediate**
and a **32,768-token context**, and select **Prompt contract and strict parser**.
Enable **Endpoint supports standard token and sampling parameters** and set
the app's output allowance to **4,096 tokens**. Start Basalt with
`--reasoning-budget-tokens 512 --answer-room auto`. MagicHandy automatically
applies its local range and recall guidance to this model and connection
policy on loopback. The managed E4B and 12B hardware thresholds are listed
under [runtime compatibility](#runtime-compatibility).

## Evaluation setup

The host was a Windows PC with a Ryzen 9 9950X3D, 63.6 GiB of system RAM
and an RTX 5070 Ti reporting 16,303 MiB of dedicated memory. The evaluated
servers ran one at a time alongside the desktop. Tests went through MagicHandy's
streaming chat handler, synthetic SQLite conversations, the production parser
and the shared engine. A fake transport recorded the movement and Stop
acknowledgements. Every profile used the English Utility voice and the default
20–80% speed limits.

| Model and runner | Weights | Configuration |
| --- | --- | --- |
| Gemma 4 12B Heretic, managed llama.cpp b11533 | Catalog Q4_K_M, 7.38 GB | Intermediate, 32,768 tokens, 512 output tokens, reasoning off |
| Inkling 2 Mini, Basalt 0.5.5 | NVFP4/Q8 pack, 19.73 GB | Intermediate, 32,768 tokens, 512 output tokens, reasoning off |
| Inkling 2 Mini, upstream llama.cpp b12620 | Unsloth mainline UD-IQ4_XS, 15.73 GB | 32,768 tokens, one slot, looped experts preferentially on GPU |
| Inkling 2 Mini, Unsloth CUDA b12588 | Unsloth UD-IQ4_XS, 15.73 GB, with a 0.40 GB DSpark draft | 32,768 tokens, one slot, speculative decoding |

The Inkling GGUFs differ between upstream and the Unsloth fork. The Basalt
pack also has a different quantization. Measurements apply to each combination
of weights and runner. [Unsloth files used in the evaluation](https://huggingface.co/unsloth/Inkling-2-Mini-GGUF/tree/ae9327c5fbc7333116ba32eee78bb5d2bbefae4f) · [Basalt model pack](https://huggingface.co/jesdga/Inkling-2-Mini-Basalt/tree/426fea7456c37c102be8a30d16e4f6bf0dcd892d)

Basalt 0.5.5 requires an NVIDIA Blackwell SM 12.0 primary GPU. This card
reported compute capability 12.0 and driver 610.88. The 32K Basalt launches
loaded with roughly 0.7–1.0 GiB of dedicated memory still free on this
desktop. A hardware rule for this engine would need to check the GPU
architecture as well as memory. The experiment covers one card and host RAM
configuration. [Basalt platform support](https://github.com/jesdga95/basalt/blob/v0.5.5/docs/platforms.md)

During an additional run a foreground GPU workload reduced free dedicated
memory to about 314 MiB. Basalt's decode rate fell from roughly 55 to 9
tokens/s. We stopped the evaluator, excluded the interrupted capture and
released its VRAM. Available memory and concurrent GPU load materially
affect this model on a 16 GB desktop.

## Response quality

The main comparison covers 90 model-backed turns across two wordings. It
includes cold starts, edits to a running pattern, independent changes to pace
and range, readbacks and recalls after intervening chat. A pass requires the
first reply to complete and the engine to apply the requested action and
trajectory. A separate five-turn conversation in each wording checks the
explanation of range and pace, a proposed pattern, a correction and a spoken
readback with motion idle.

| Intermediate at 32K | Exact model turns | Unrequested motion on holds | Direct Stops acknowledged | Delayed corrected readback while holding | Bounded cold starts |
| --- | ---: | ---: | ---: | ---: | ---: |
| Gemma 12B, managed | 89/90 | 0/34 | 22/22 | 2/2 | 4/4 |
| Inkling, Basalt | 42/90 | 1/34 | 22/22 | 0/2 | 0/4 |

For a request to sweep between 16 and 57 at speed 35, Basalt's Inkling reply
said the sweep was starting. Its action was `none` and the simulator stayed
idle. In a different conversation the user asked only for the corrected
settings. The model moved the running pattern from 5–95 at 46 to 24–66 at
31. Gemma correctly spoke those three values and held the existing trajectory.
Gemma's miss was a request to raise the lower mark from 21 to 26 while
preserving the upper mark of 75. It also moved the upper mark to 63.

All ten separate conversation turns stayed idle on both profiles. Gemma
answered both explanations and the numeric readback in each wording. Basalt
returned variations of “A brief answer” and ellipses on all six. It did
acknowledge the proposed patterns and their corrected speeds. These examples
are why a nonempty reply and an idle engine earn separate checks from a useful
spoken answer.

The existing **OpenAI-compatible** connection offers **Prompt contract and
strict parser**. With this selection Basalt can reason before composing JSON
and MagicHandy validates every returned proposal. The tested connection
declared max tokens, temperature and top-p support. Its model ran over
loopback at 32K. Including both five-turn conversations yields 100
model-backed requests and 44 explicit holds.

| Profile used for all 100 requests | Exact first pass | Exact result after any retry | Delivered a model reply | Unrequested changes on holds |
| --- | ---: | ---: | ---: | ---: |
| Gemma 12B, managed Intermediate | 99/100 | 99/100 | 100/100 | 0/44 |
| Inkling, Basalt with decoder schema, 512 output tokens | 46/100 | 46/100 | 100/100 | 1/44 |
| Inkling, saved Basalt prompt-contract connection, 4,096 output tokens and unrestricted reasoning | 92/100 | 93/100 | 94/100 | 0/44 |
| Inkling, saved Basalt connection with local guidance, 4,096 output tokens and a 512-token reasoning budget | 96/100 | 97/100 | 98/100 | 0/44 |

The saved connection preserved the upper endpoint on the edit Gemma missed.
It also held the running pattern while reading back the corrected values.
Two acknowledgements came back as plain prose and were rejected by the
strict parser. Several longer generations spent the output budget before
producing a usable answer.
The connection-test prompt returned the exact readiness object in all three
64-token probes, both 256-token probes and the 512-token probe. The existing
readiness allowance was sufficient for this machine and model.

The tuned profile passed the six independent fresh-session requests and
delayed readbacks in both wordings. Its remaining misses clustered around
reapplying a newly restated definition and preserving it through the next
edit. One wording recovered the restated pattern after a retry. One separate
conversation turn failed to deliver its explanation. All 22 direct Stops
were acknowledged on both Inkling connections and on Gemma.
Gemma and tuned Inkling each delivered one final result that missed the
request. The two additional misses in Inkling's delivered-outcome score
came from turns that produced no usable reply.

### A prompt change that looked promising on a small sample

Removing “A brief answer” and the ellipsis reply from examples raised Basalt
from **5/14 to 13/14** on a focused sample. The variant sometimes answered a
conceptual question by saying that the simulator was idle. We then asked it
to explain the control or report the saved values that the person requested.
Across the full 100 model-backed turns, that version scored **83/100** versus
**46/100** with the original wording. It answered all six explanation and
readback questions in the separate conversation. It also made **5 unrequested
changes on 44 holds**, up from **1/44**. Examples include an active Creative
hold and a newly restated future definition. We kept the original production
prompt and retained the tests for both spoken content and playback.
The initial capture scored 50/100 before the spoken-content check existed.
Applying the same check to its saved replies marked four echoed explanations
as misses and gave the 46/100 comparison above.

The external route retains up to 64 messages here and the managed Gemma route
retains up to 96. The primary positive long recall and readback comparison
used 26 filler pairs on both. Gemma also passed them at its ordinary 40-pair
test depth. The outside-window controls deliberately placed the definition
beyond each route's own retention limit.

### Giving Inkling room to reason

A matched 19-turn subset includes the delayed cue, held-out edits, five
conversation turns, two fresh starts and two requests to leave the simulator
idle. On Unsloth/DSpark, switching to Auto reasoning and allowing 4,096
output tokens produced **18/19** exact decisions and zero unexpected motion
on ten holds. Gemma 12B at its default settings also scored **18/19** on
these turns. Inkling answered the range-versus-pace questions and recalled
the corrected numbers. It sometimes added a second reassurance sentence
when the person asked for one sentence.

Both models missed the lower-endpoint-only edit. Inkling exhausted all 4,096
tokens after 114.7 seconds and the app rejected the incomplete response.
Gemma delivered a response and changed an extra endpoint.

Two further trials separated reasoning from decoder constraints. On upstream
llama.cpp, a template that prefilled Inkling's final text channel when thinking
was off produced **18/19 first pass and 19/19 after one repair**, with the
ordinary 512-token cap. On Basalt, simply raising the allowance to 4,096 and
selecting Auto reasoning with the full schema produced **7/19**, with one
unrequested change on ten holds. As a simulator diagnostic, we kept the
identical local chat prompt and full backend validation and omitted the
decoder's format constraint. That run produced **19/19 first pass**, including
raising 21 to 26 while preserving the upper mark of 75. All ten holds were
respected.
On nine identical fresh-start and conversation requests with Auto reasoning,
the full schema scored 4/9, JSON-object decoding scored 6/9 and the
prompt-only diagnostic scored 9/9.

The saved connection and the native local role compose different motion
instructions. We tested the existing local range and recall guidance on the
saved Inkling connection while keeping its server, weights, decoder policy,
sampling and output allowance fixed. On 26 requests, including all six
misses from the first wording of the saved-connection run, first-pass accuracy
moved from **20/26 to 26/26**. Exact outcomes after retries moved from
**21/26 to 26/26**. Both kept all 12 explicit holds. Gemma scored 25/26.
The guide puts the current range and the shape of the requested edit near
the latest request when it fits the existing history budget.

On a separate 19-turn control using the same loaded Basalt server with a
512-token reasoning budget, the original named-connection prompt scored
**17/19**. The local guidance scored **19/19**, with zero unwanted changes
on ten holds in both runs. The two recovered requests were the single-endpoint
edit and an acknowledgement of a revised proposal. The guide also scored
19/19 on these requests with unrestricted reasoning.

### Capability levels

At 32K with the Simple capability and 256 output tokens, Inkling using
Basalt's schema decoder scored **5/13** model-backed turns. The managed E4B
default scored **13/13** on the same requests. Both acknowledged all five
direct Stops and kept all five holds.

Basalt also allocated a **65,536-token slot** on the same GPU. With the
named Advanced prompt-contract connection, 4,096 output tokens and the
512-token reasoning budget, Inkling scored **8/17** exact first responses.
It delivered nine replies, held all four explicit holds, and scored **4/10**
on counted-phrase turns. The two wordings requested three or four short
strokes followed by one wide sweep, then changes to individual groups.

For a matched control we manually configured Gemma 4 12B on b11533 with
the 65,536-token slot, the named Advanced prompt contract, Auto reasoning
and a 4,096-token output allowance. All 17 model calls reached the allowance
with an empty visible answer, taking a median of 51.43 seconds. The five
direct Stops still succeeded. The managed Gemma profile that setup selects
uses the Intermediate schema at 32K and delivered all 100 replies in the
broad comparison.

On the 12 requests shared with the native **Advanced (no reasoning)**
trial, the named Advanced route scored **5/12**. The native route used the
decoder schema, a private planning note and 1,024 output tokens, and scored
**9/12**. It kept all three holds. It missed the initial three-short,
one-wide figure in the first wording and passed the mirrored counted
sequence. These tests exercised the Advanced vocabulary within the 64K
allocation. Intermediate achieved the strongest broad result for Inkling.

### Compiled motion

We inspected whole-loop positions, quantized transport points, velocity and
acceleration for the selected edits, together with the failed decisions and
Stop timelines. In the lower-endpoint case, both Gemma and tuned Inkling
kept the requested pace at 38%. Their upper reversals were 63% and 75%
respectively. In the readback case, the before and after curves coincided
for both models. The decoder-schema Inkling trace instead showed a new
24–66% loop. The study's local atlas is
`.scratch/inkling/atlas/index.html`. The paired overview is
`.scratch/inkling/tuned-trajectories.png`.

## Latency

| Same 90 model turns | Delivered | End-to-end median | End-to-end p95 | Median on the 42 turns both got right |
| --- | ---: | ---: | ---: | ---: |
| Gemma 12B, managed | 90/90 | 2.06 s | 3.28 s | 1.88 s |
| Inkling, Basalt | 90/90 | 1.81 s | 2.83 s | 1.98 s |

Timing starts at the chat request and ends after the handler returns. Direct
Stops, model loading, separate conversation checks and the early rejected-schema
probes are outside this table. Neither model retried a turn in it.
The overall median includes quick acknowledgements that missed the request.
Restricting timing to the same correct answers makes that difference visible.

Across all 100 requests, including the conversation checks, the median / p95
for delivered replies was **2.00 / 3.26 s** on Gemma, **1.79 / 2.76 s**
with Basalt's decoder schema, and **5.86 / 37.77 s** on the saved Basalt
prompt-contract connection with unrestricted reasoning. The six undelivered
turns on the saved connection took a median **71.03 s** and a maximum
**85.21 s**.
The named compatible adapter checks the stream's finish status before
publishing the accumulated response text. Its first text callback therefore
arrives close to completion even when the provider signaled activity earlier.

With local guidance and the 512-token reasoning budget, Inkling's median /
p95 was **6.24 / 13.94 s** on 98 delivered replies. The two undelivered
turns took a median **15.52 s** and a maximum **25.91 s**. This profile
reduced long waits and delivered more correct replies. The unrestricted
profile had the shorter median. The tuned run began after a sustained idle
GPU window and used a larger VRAM reserve of 4,816 MiB. The figures describe
the complete configurations on this desktop.

On the 18 identical answers that both got right in the larger-budget trial,
Gemma's median was **1.85 s** and DSpark's was **19.11 s**. Their sample p95s
were 4.35 s and 80.78 s. The 114.7-second unsuccessful DSpark request is
reported separately above.

The answer-prefill run on upstream llama.cpp had an **11.51 s** median across
19 delivered replies. The Basalt diagnostic with Auto reasoning and backend
validation had a **5.98 s** median and **15.68 s** p95 across 19. The Basalt
full-schema Auto run had a **1.92 s** median across 19. The response-quality
results distinguish the configurations that earned those timings.

For the 26-turn local-guidance comparison, delivered replies had a median
of **6.23 s** with the original saved-connection prompt and **5.73 s** with
local guidance. Sample p95 moved from **59.04 s** to **10.76 s**. The
original route delivered 22 replies and the guided route delivered all 26.

On the 19 matched requests with local guidance, the sample median / p95 was
**5.86 / 27.77 s** with unrestricted reasoning and **6.72 / 9.86 s** with
the 512-token reasoning budget. Both runs delivered 19 exact first responses.

On the same 18 first-pass successes, median prompt evaluation took **0.87 s**
for Gemma, **10.07 s** on upstream Inkling, **11.27 s** on DSpark and
**0.98 s** in the Basalt diagnostic. The corresponding times to the first
response-content delta were 1.01, 10.08, 18.32 and 4.82 seconds. The first
delta can be opening JSON syntax. Median application preparation was at
most 1 ms on each path. Basalt's faster prompt processing and the time each
model spent thinking account for substantial differences visible to a user.

Earlier general-purpose tests on this machine used an 8,192-token slot and
counted generated tokens including reasoning. They measured about 45–48
tokens/s on tuned upstream llama.cpp, 57–66 on Unsloth with DSpark and 64–73
on Basalt. A 1,358-token document prompt reached its first output in about
5.0, 3.2 and 0.56 seconds respectively. The app comparison above measures
the time to a completed reply and any admitted simulated edit.

## Runtime compatibility

The shipping b11533 runner exits on the mainline GGUF with
`unknown model architecture: 'inkling2_loop'`. Upstream merged Mini support
on November 20. Promoting the weights in the model catalog alone would leave a
fresh installation unable to start them. The Gemma CUDA regression on a
previous newer build is relevant when choosing a shared runner.
[Upstream Mini support](https://github.com/ggml-org/llama.cpp/pull/32694) · [Gemma runtime comparison](installed-chat-replay-2026-11-18.md)

The managed catalog recommends the E4B Simple profile from 6,144 MiB of
detected NVIDIA VRAM and the 12B Intermediate profile from 10,240 MiB.
Basalt uses its own model-pack format and server. An existing Basalt server
can be selected through a loopback connection without replacing the shared
Gemma runner. [Hardware fit rules and curated models](../internal/llm/model_catalog.go)

Basalt accepted three small object schemas, then returned HTTP 400 on all
seven Intermediate and all thirteen Simple model-backed requests in the
initial 8K probes. MagicHandy constrains each legal action with a separate
object branch. Declaring that their union also has `type: object` lets Basalt
compile the schema while preserving the action and edit constraints. The
app supplies that declaration for loopback Inkling 2 Mini on the native
local route and on saved compatible connections, including Automatic.
The Intermediate range and recall guide on a saved connection applies only
to the explicit prompt-contract setting. Basalt
chose a legal branch on all three contradictory union probes after that
change. E4B on b11533 scored 18/18 on the same focused checks on both sides
of the schema change, including direct Stops.

On b12620, the app's top-level `reasoning_effort: "none"` rendered as
“Thinking effort level: 0.9” in Inkling's actual template. Placing `none`
only in `chat_template_kwargs.reasoning_effort` rendered zero. With fixed
user input, schema and seed, the latter completed the 16–57 start that had
exhausted all 512 output tokens. The long corrected-cue request still used
the entire budget with an empty visible reply. The publisher also used a
prefilled answer channel when measuring direct answers.
[Thinking Machines Lab's model evaluation](https://thinkingmachines.ai/news/inkling-2-mini/)

## Gemma and GPT regression review

On November 27 we compared alpha.90 (`4286ae43`), the submitted Inkling
change (`e29b1341`) integrated with alpha.90, and the corrected build. Both
32K Gemmas used the catalog weights and managed b11533 CUDA runner on an RTX
5070 Ti. GPT-6.1 Terra used the signed-in ChatGPT provider, Advanced, 128K
and Low reasoning. Each build received both wordings of the full synthetic
conversation suite through streaming chat and the shared engine. A simulated
transport recorded commands. Each model generated its own conversation
history during the live runs.

The submitted change added 16 bytes to every native action schema. The
admission budget counts those bytes and removes whole messages. In a
32K fixture a user recorded the word `lapwing`, exchanged 30 unrelated
question-and-answer pairs, then asked for the word. With seven bytes of
headroom on E4B and six on 12B, alpha.90 retained the cue and the submitted
PR dropped it. The correction retains it on both profiles. At two fixed
inference seeds, each Gemma gave the same answer on alpha.90 and the fix.
Neither retrieved this arbitrary word correctly at that distance.

We also captured the outgoing HTTP bodies for idle, running and long-history
requests in both Layered and Creative v2. All 18 corrected Gemma and Terra
bodies matched alpha.90. Basalt 0.5.5's request validator accepted all six
corrected Automatic-connection schemas. It rejected all six from the
submitted PR at the root type check. The native Inkling schemas and the
tuned prompt-contract bodies matched the submitted PR across the same cases.

| Default profile | First-pass checks, alpha.90 | Submitted PR | Corrected PR | Unrequested changes on holds, corrected PR | Confirmed direct Stops, corrected PR |
| --- | ---: | ---: | ---: | ---: | ---: |
| Gemma 4 E4B, Simple | 109/122 | 108/122 | 107/122 | 0/52 | 28/28 |
| Gemma 4 12B, Intermediate | 121/122 | 121/122 | 121/122 | 0/52 | 28/28 |
| GPT-6.1 Terra, Advanced Low | 132/132 | 132/132 | 132/132 | 0/54 | 30/30 |

Direct Stops bypass inference and are counted separately. Endpoint checks
allow two percentage points in the compiled trajectory. On alpha.90, E4B
changed the running plan on one of 52 holds. On the correction it respected
every hold and read back both short-conversation corrections, but still
missed requests including a single-endpoint edit and applying a restated
definition. The 12B miss on the corrected build was a restated definition.
All three models explained the difference between range and pace in both
wordings and kept the simulator idle.

To distinguish model variation from a change in the inputs, we replayed the
baseline answers and fixed the procedural motion seed on both builds. Across
376 turns, every admitted provider request matched alpha.90 byte for byte
after serialization, including history, schema, sampling and output budget.
This covered 128 E4B attempts, 122 12B attempts and 132 Terra attempts,
including retries. All 376 replay verdicts matched. The fresh live runs
above used the normal sampling and independently generated histories.

On 22 additional pairs of local inferences, we held the admitted messages,
sampling seed and runner constant, disabled prompt-cache reuse, and changed
only the schema root type. Every pair returned identical raw output. Visual
review of the corrected build showed why spoken acknowledgement is
insufficient. Asked to apply the restated 18–61 range, both Gemmas kept
commanding 0–100, while Terra commanded 18–61. Asked to raise only the lower
endpoint to 20 and keep the top at 77, the observed ranges were 26–83 on E4B,
21–77 on 12B and 20–77 on Terra.

Run `TestLocalMotionContractsForInklingAndDefaultModels` and
`TestGemmaKeepsTheLastFittingHistoryMessage` in `./internal/httpapi` for
the offline regression checks. The default live matrix is
`python scripts/evaluate-default-motion.py --profiles gemma-e4b gemma-12b terra`
with the catalog store, managed runner and signed-in profile described in
the [motion review guide](motion-visual-review.md).

## Reproducing the comparison

Start a loopback server with one slot and a known context size. For the Basalt
32K run the alias was `inkling-2-mini`, the KV cache was FP16, and vision was
loaded on demand. Upstream used build `660be5506` and the Unsloth fork used
`883286076`. For upstream we placed the repeated expert layers 6–17 on the
GPU first, used a 384-token batch and 128-token microbatch, and kept the
server's normal prompt cache. The Unsloth run used a 512-token batch and a
three-token DSpark draft on the GPU. These were text-only app requests.

```powershell
$env:MAGICHANDY_LIVE_LLAMA_URL = 'http://127.0.0.1:8080'
$env:MAGICHANDY_LIVE_MODEL = 'inkling-2-mini'
$env:MAGICHANDY_QUALITY_CAPABILITY = 'intermediate'
$env:MAGICHANDY_QUALITY_MAX_OUTPUT = '4096'
$env:MAGICHANDY_QUALITY_REASONING = 'auto'
$cases = 'creative|layered|recall_long|recall_readback|recall_outside|holdout|restated|acknowledge_idle|conversation'
python scripts/evaluate-default-motion.py --profiles compatible-local --cases $cases
```

Use the `start_bounds`, `start_reverse_order`, `start_middle`,
`start_short_band`, `start_question` and `start_future` cases with
`--variants a` for independent fresh sessions. Each uses the same wording
in both variants, so the main table counts each once. Give each backend its
own `--output` directory. For Gemma, select `--profiles gemma-12b` and supply
the catalog store and managed executable. To match the positive long-context
cases, set `MAGICHANDY_QUALITY_HISTORY_PAIRS=26`. Captures contain the
accepted semantic target, compiled samples, fake transport and failures.
[Harness and atlas instructions](motion-visual-review.md)

Use `--profiles external-local` with a 512-token output allowance and
reasoning off to evaluate the decoder-schema local role. The diagnostic that held the local-role
prompt fixed while omitting the format constraint was a separate experiment.
For the upstream direct-answer experiment the embedded Jinja template emitted
effort zero when `enable_thinking` was false and appended
`<|message_model|><|content_text|>` when adding the generation prompt in
that mode.
