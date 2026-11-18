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

For 12B the local pace/other medians were 3,638.5/3,534 prompt tokens and
1,103/1,089 ms of prefill. E4B used 3,710/3,619 tokens and 519/505 ms. The
median cache counter was zero for both models in these conversations. A
separate b11533 native control on the same 12B model processed all 360 tokens
cold, then processed 5 and reused 355 on an identical prompt. With a usage
block, the reported total was 360 and the cached count was 355. The shorter
local layouts tested in the original study still cost first-pass accuracy.
[Runtime and latency trials](installed-chat-replay-2026-11-18.md)

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
