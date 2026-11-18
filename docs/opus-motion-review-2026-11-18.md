# Independent review of motion prompts and conversation replay

Claude Opus 5.5 reviewed the changes from the architecture PR at `d8188233`
through `9a4c36cf`, then reviewed the proposed fixes. It received the source
diff, relevant routines and aggregate study results. Original conversations
and credentials stayed out of the review packet.

## Changes confirmed against the app

1. **Account for the dialogue a local model actually receives.** Gemma 4 E4B
   previously learned about a missing definition only when message retrieval
   counted an omission. Long messages can also evict the definition during
   provider admission. The prompt now counts both, and a full or compact hint
   must fit beside exactly the same retained dialogue. An Advanced local
   guide can also use space left after truncation. At 16,384 tokens, regression
   tests cover two byte-evicted messages, 19 earlier omissions plus those two,
   and a guide that would evict one extra message.
2. **Keep instructions, schema and parser in agreement.** The English Simple
   Layered prompt now shows the empty list required by its grouped schema on
   a conversational response. The real engine target determines the edit
   format once for prompt construction, the first proposal and any repair.
   Terra's short pace operation is available on a running Layered score,
   including counted and regional phrases. A library or Creative target
   cannot acquire it from a starter score constructed for the prompt. The
   maximum-speed example during live Autopilot also carries the current
   standing hold. Tests cover the normal chat entry, prompt inspector and
   preservation of rhythm, geometry, counts, layers and directional pace.
3. **Replay the current device calibration.** The reader projects the saved
   motion settings and participating Handy model names in one read-only
   transaction. Each simulated session verifies the effective calibration.
   A synthetic profile set the physical window to 19–77 in reverse, speed
   limits to 18–67, gentle style, and selected an Original and a Standard.
   The common timeline used Standard. The signed-in Terra run completed all
   six turns first pass. A four-point adjustment changed 45 to 49 and kept
   both endpoints. The full sweep reached logical 0 and 100 within the
   reversed physical window. Recompiling its four accepted plans reproduced
   all four in-run trajectory summaries exactly. The profile's synthetic
   connection key and device ID were absent from the export.
4. **Make comparisons auditable.** Each v2 capture identifies its executable,
   selected input, model, calibration, inference of missing Stops, requested
   sampling and seed policy. A reply emitted before a dispatch failure is
   distinguishable from a completed turn. A public scorer checks directional
   or numeric requests, every other score field, prior-speed restoration and
   counted groups. It reports paired gains and losses and flags unscorable
   cases. It rejected a comparison of the same six prompts with different
   stop reconstruction until the differing assumption was explicitly allowed.
   The owner's local labels reproduced all six original baseline/selected
   pace totals. [Capture format, annotations and reproduction commands](motion-visual-review.md#reproduce-an-atlas)

## Fresh original-wording replays

Each row covers all 339 original requests in the seven conversations. The 136
pace requests require a first-pass response, the requested direction, maximum
or hold, and preservation of the rest of the accepted score. Sixty occurred
after turn 32. An exact previous-speed restoration is a separate, numerical
check. Each model generated its own history through the normal streaming chat
path and ended each session with a newly acknowledged simulated Stop.

| Setup selection | First pass | Completed | Pace /136 | Later pace /60 | Previous-speed restores | Median / p95 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Gemma 4 E4B, Simple | 327 | 333 | 116 | 54 | 0/4, one unscorable | 964 / 1,575 ms |
| Gemma 4 12B, Intermediate | 330 | 336 | 132 | 57 | 3/5 | 2,040 / 3,102 ms |
| Signed-in GPT-6.1 Terra, Advanced Low | 338 | 338 | 136 | 60 | 5/5 | 3,742 / 7,161 ms |

A second full signed-in Terra replay through the versioned CLI completed
338/339, with 337 first-pass responses, one repair and one failed turn. It
again passed 136/136 on pace, 60/60 after turn 32, 5/5 previous-speed
restorations and 6/6 counted-end requests. The median was 3,574 ms and
p95 was 7,360 ms. The capture recorded a 131,072-token context and Low
reasoning. The CLI used the source-bound private annotations.

Terra also passed all six counted-end requests. The local capability profiles
have no explicit counted-phrase representation. On 12B, four pace misses
occurred when the simulator was idle. On E4B, six involved idle playback,
nine missed the requested direction or ceiling, two changed an unrelated
score field and three involved a retry or failed completion. A focused E4B
test on two wordings scored 42/50 before the empty-list correction and 43/50
after. Both preserved all 14 exact holds. The conflict between the old
instructions and the schema was reproducible independently of sampling.

The hosted median for pace was 3,107 ms and 52 generated tokens, and for the
other 203 turns it was 4,834 ms and 137 tokens. The original baseline-to-PR
measurements were 4,613 to 2,983 ms and 126.5 to 51 tokens on pace, and 5,074
to 4,516 ms and 161 to 135 tokens on other turns. The smaller speed response
accounts directly for fewer output fields. Provider load and evolving
conversation state also affect elapsed time.

### Repeated E4B comparisons

Each arm ran the same 339 requests with its own newly generated history.
Session order was shuffled for each repeat and the first build alternated
by session. Both arms used the same b11533 runner, catalog file, application
sampling, saved motion calibration and inferred Stops. The replay executables
had SHA-256 prefixes `3eda1f3d4bfb` (previous PR head, `9a4c36cf`) and
`a93a89f0ee1a` (reviewed generation path). The provider chose its seed on
each run. The comparison uses the same 136 annotated turns per repeat.

| Repeat | First pass /339, previous → reviewed | Pace /136, previous → reviewed | Gained | Lost |
| --- | ---: | ---: | ---: | ---: |
| 1 | 320 → 320 | 114 → 114 | 9 | 9 |
| 2 | 322 → 331 | 120 → 119 | 12 | 13 |
| 3 | 326 → 322 | 122 → 121 | 8 | 9 |

The earlier 128/136 and the first follow-up 116/136 were separate
non-interleaved runs of different builds. The three matched comparisons
gave differences of zero or one turn. These data do not establish an overall
E4B improvement. One short continuation caused a seed refresh on three of
three reviewed runs and one of three controls. The old published run also
refreshed that seed. The numerical endpoints and speed stayed the same.

A candidate prompt reminded E4B that a request only to continue playback
leaves the current plan running. With the same preceding dialogue and three
fixed starting scores, it preserved 3/3 continuations versus 2/3. Both
variants preserved 3/3 explicit holds and applied 3/3 four-point increases.
Both missed the compound full-band request on all three starting scores.
When replayed in the original 52-turn conversation, both scored 21/22 on
pace. The existing wording preserved the plan on the continuation and the
reminder refreshed its seed. The review kept the existing 32K wording.

## Default limits and the supported 16K window

Fresh setup allocates 32,768 context tokens for both catalog Gemmas. E4B
uses 256 output tokens and 12B uses 512. A signed-in Terra connection uses
131,072 context tokens when advertised, 256 history messages and 96,000
history bytes. ChatGPT manages its output limit. The local byte allowance
for retained dialogue is 24,000. Provider admission also reserves the
schema, output and reasoning budgets before dropping the oldest dialogue.

| Reported tokens on completed installed turns | Input median / p95 / max | Output p95 / p99 / max |
| --- | ---: | ---: |
| E4B Simple | 3,667 / 3,835 / 4,048 | 103 / 139 / 211 |
| 12B Intermediate | 3,574.5 / 3,771 / 4,078 | 137 / 178 / 228 |
| Terra Advanced Low | 9,219.5 / 10,326 / 10,873 | 278 / 287 / 291 |

No completed local reply in these captures reached the output cap. Failed
turns could lack provider token counts, so the table cannot establish their
cause. The longest completed request took 3.41 s on E4B, 5.22 s on 12B and
13.19 s on Terra against a factory request timeout of 120 s.

Doubling E4B's output allowance to 512 on the 73-turn conversation gave
71 first-pass and 72 completed turns, versus 72 first-pass and 73 completed
at 256. The longest reported completion used 136 tokens at the higher cap
and 141 at the original cap. Pace was 33/35 versus 32/35 and median latency
was 916 versus 912 ms. Setup keeps the 256/512 local output allocations.

At the supported 16,384-token Simple minimum, a default start with no history
exceeded admission before native inference. It needed 11,446 bytes against
a 10,789-byte allowance. Eight verbose user lines could also reappear in
mandatory engine state after their original dialogue had been trimmed.
The app now checks the full local reference against the actual schema and
reserved space for a complete current user turn. When space is tight it
bounds duplicate timeline entries by their encoded size and pairs each
retained line with its original age. The ordinary dialogue still carries the
current request. Simple can use a compact reference with the same operation
grammar. The inspector shows which reference was selected.

At the normal 32K allocation, eight 1,950-byte notes pushed the old essential
prompt over its limit on both E4B and 12B. The revised composer admitted the
request on both. An eight-note control at 200 bytes per note retained the full
reference and every dialogue row. The early 16K real-model trial admitted
six of six starts and six follow-ups. On two missing-definition cases it
held the motion but claimed to use the name. It read two visible definitions
back correctly and applied both marks in both cases, restoring the specified
speed in one. Successful admission and accurate execution are scored separately.

The final compact reference includes one example carrying both endpoints,
full-band geometry and overall speed together. The example values differ
from all the requested values. In two six-session runs per wording, the
selected example applied the visible corrected score exactly on **4/4**
requests, versus **0/4** without the example. Both wordings produced all
12 requested starts, read the visible correction back on 4/4 and preserved
the active plan on 4/4 missing-definition requests. The selected wording
completed 23/24 turns first pass versus 21/24. On two missing-definition
requests the selected model still said it was using the name. The simulator
confirmed the hold in each case.

### Comparing local dialogue limits

In the archived conversations, 139 of 339 requests had more than 64 previous
messages and room for the next older message under the 24,000-byte cap. At
96 rows this happened on 57 requests, and at 128 on eight. These counts use
the original archived dialogue. Each replay generated new assistant replies.

Two synthetic wordings placed the full definition of a named sweep 72 messages
back. At 64 rows, neither Gemma received it. Both missed both numerical
read-backs and both applications. At 96 and 128 rows, both models saw the
definition, read back the values on 2/2 and applied the exact marks and speed
on 2/2. Generic continuation and a deliberately missing name were evaluated
separately. A model could preserve the plan and still claim to use a missing
definition.

The two longest installed conversations supplied 132 original requests,
including 65 scored pace turns and 34 scored pace turns after turn 32.
Every row used the same 32K allocation, 24,000 history bytes, GGUF, runner and
output ceiling within its model. The sessions ran independently for each row.

| Model | Message limit | First pass /132 | Completed /132 | Pace /65 | Late pace /34 | Median / p95 | Median prefill |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| E4B | 64 | 128 | 132 | 61 | 31 | 905 / 1,425 ms | 485 ms |
| E4B | 96 | 130 | 132 | 62 | 33 | 986 / 1,378 ms | 501 ms |
| E4B | 128 | 126 | 129 | 54 | 28 | 1,017 / 1,946 ms | 514 ms |
| 12B | 64 | 129 | 130 | 65 | 34 | 2,076 / 3,364 ms | 1,117 ms |
| 12B | 96 | 125 | 129 | 62 | 31 | 2,067 / 3,327 ms | 1,167 ms |
| 12B | 128 | 128 | 131 | 64 | 33 | 2,308 / 3,453 ms | 1,229 ms |

Managed llama.cpp Layered chat now retains up to **96 messages within 24,000
bytes** at the Simple and Intermediate levels when the local allocation is
at least 32K. This is the smallest tested window that recovered all four
delayed-definition checks on each model. The 16K Simple path keeps its
64-message window. Named external connections use their saved history
choice. Neither the model's native context allocation nor its output cap
needs to rise for this change.

The final serving path replayed all seven conversations again with the
selected defaults. The provider admitted more than 64 previous messages on
144 E4B requests and 145 12B requests, reaching 96 on both. Neither local
run had an essential-input overflow. The larger window restored a specific
ability to use older definitions. Short pace edits still showed misses, and
the 12B result on the two longest sessions illustrates the tradeoff.

| Selected default | First pass /339 | Completed /339 | Pace /136 | Later pace /60 | Median / p95 |
| --- | ---: | ---: | ---: | ---: | ---: |
| Gemma 4 E4B, Simple, 96 messages | 324 | 336 | 122 | 52 | 968 / 1,763 ms |
| Gemma 4 12B, Intermediate, 96 messages | 332 | 338 | 132 | 57 | 2,019 / 3,131 ms |
| Signed-in Terra, Advanced Low, 256 messages | 337 | 338 | 136 | 60 | 3,574 / 7,360 ms |

Factory motion settings allow 20–80% speed and 0–100% travel. Both the
original owner's 1–100% speed window and the narrower reversed test window
were respected by the simulator. A higher speed ceiling would change the
physical operating range. These trials measured commanded motion and supplied
no physical evidence for raising that ceiling.

For 12B the local pace/other medians were 3,638.5/3,534 prompt tokens and
1,103/1,089 ms of prefill. E4B used 3,710/3,619 tokens and 519/505 ms. The
median cache counter was zero for both models in these conversations. A
separate b11533 native control on the same 12B model processed all 360 tokens
cold, then processed 5 and reused 355 on an identical prompt. With a usage
block, the reported total was 360 and the cached count was 355. The shorter
local layouts tested in the original study still cost first-pass accuracy.
[Runtime and latency trials](installed-chat-replay-2026-11-18.md)

With 32 short history messages and an ordinary 32K motion request, the
complete in-process chat preparation and parse took a median 299 → 286 µs
for E4B and 384 → 364 µs for 12B on the previous and reviewed code. Allocated
bytes per operation were 300,487 → 300,776 and 299,192 → 299,480. Five
one-CPU benchmark samples used the same stubbed provider and score. These
numbers exclude generation time.

## Commanded trajectories

The follow-up gallery retains 5,646 evaluated turns and Stops. The shared
engine compiled 3,897 distinct score and calibration combinations for 82
overview sheets. There are 421 simulated dispatch timelines. Across the
compiled positions, the minimum was 0% and the maximum 100%.

The largest stroke-width variation alternated clusters of short strokes at
the ends with long sweeps between them. The largest acceleration occurred
on rapid reversals at 100% requested pace. In the reversed-window control,
speed-only edits preserved the endpoints and changed the cycle time.
The dispatch trace distinguishes submitted points from the queued tail
canceled by Stop. These are commanded trajectories without carriage
feedback. The local gallery is
`.scratch/opus55-review-374-20261118/atlas/index.html`.

## Comparison provenance and scope

The original baseline's replay executable had SHA-256 prefix `fd9a2a07ce08`
(`bcbd7358` application path). The selected local executable was
`e1a5d3fb92bb` and included the prior-speed change and stream counters. The
selected hosted executable was `88ac7ec1beb4`, including the repair wording
in `0a6af591`. Those historical captures used a test-only recorder. The
versioned recorder now emits its own full binary digest. Different
executables can contain the same generation path and different diagnostics.

All local runs used the same catalog files on an RTX 5070 Ti with managed
CUDA b11533, 32,768 context tokens and thinking off unless a test explicitly
selected 16,384. The E4B output limit was 256, the 12B limit 512. Terra used
131,072 context tokens and Low reasoning. The normal interactive request
asked for temperature 0.3, top-p 0.95, repeat penalty 1.2 and a 40-token
repeat window. The provider chose the seed. The application tests evaluate
commanded motion on a simulated transport. The database supplies its current
saved settings and the recorded conversation, with incomplete evidence of
historical playback and scheduler timing.

The seven conversations belong to one installation and were also used to
select prompt variants. Replaying them can expose a regression in those
requests. It cannot establish a population accuracy rate. Previous-speed
and counted-end samples have only five and six cases. The repeated two-wording
synthetic suite is a regression set. A separate, previously unseen set of
conversations would be needed to measure generalization.
