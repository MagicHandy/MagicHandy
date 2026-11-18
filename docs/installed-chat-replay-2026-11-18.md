# Retained chat replay and llama.cpp comparison

On the 339 retained requests, Terra's median response fell from 4.90 to 3.44 s and exact pace edits rose
from 107/136 to 136/136. The two local models showed different tradeoffs. The Windows release comparison
identified a major E4B regression on the stock b12503 CUDA runner when Flash Attention was selected
automatically.

## Conversations and scoring

The installed database had seven nonempty conversations, 339 user turns, 320 interactive replies, 78
Autopilot remarks and one empty tab. The earlier interactive replies came from ChatGPT (313) and another
hosted provider (7). Its 22 diagnostic failure records included 12 response repairs, of which six failed,
nine interruptions and one account connection error. For the 320 recorded interactive replies the median
preparation was 2 ms, first provider activity was 1.63 s, first visible token was 4.81 s and completion was
4.86 s. This sample excludes attempts without a recorded reply.

We read a single SQLite transaction opened read-only. Each replay used the original user wording, in order,
with its own SQLite session, the live app's streaming handler, a real model, the shared engine and a
simulated transport. Each subsequent user turn saw the newly generated replies. Stored Autopilot remarks
were inserted at their original positions. The installed speed limits were 1–100% with the Original Handy
calibration. Fifteen later recorded starts supplied evidence of an intervening playback stop. The stored
evidence contains dialogue and diagnostic failures. The complete live scheduler timeline and physical
carriage trajectory were unavailable. The interactive replay compressed idle time and started the seven
conversations independently. A separate, paired two-conversation run activated the real Autopilot mode at
recorded remarks with a holding planner to exercise interactive edits during Autopilot. The owner authorized
the signed-in ChatGPT replay. Published results contain aggregated measurements and synthetic examples.

The first-pass count requires an accepted response and an expected provider call count with no repair or
fallback. A separate sample of 136 short, unambiguous pace or continuation turns checks direction or the
requested maximum, every other field in the accepted flow, and plan identity when already at a limit. Turns
at reconstructed restart boundaries are excluded from this subset. The source included five explicit returns
to the speed of the immediately preceding faster stretch and six requests specifying two or three strokes at
one end of the travel. Each turn retains the previous and resulting scores, including misses. Estimates and
plots come from the shared sampler.

## Results on original wording

Each column pairs the PR baseline at `bcbd7358` with the selected application path. The first two counts
include all 339 requests. The exact-pace column covers the same 136 turns for every model.

| Setup-selected model | First pass /339 | Delivered after repairs /339 | Exact pace and preservation /136 | Median | p95 |
| --- | ---: | ---: | ---: | ---: | ---: |
| Gemma 4 E4B · Simple | 329 → 323 | 338 → 335 | 122 → 128 | 908 ms → 903 ms | 2,214 ms → 1,674 ms |
| Gemma 4 12B · Intermediate | 329 → 327 | 337 → 338 | 131 → 131 | 2,058 ms → 2,089 ms | 3,235 ms → 3,389 ms |
| ChatGPT GPT-6.1 Terra · Advanced Low | 335 → 337 | 339 → 338 | 107 → 136 | 4,900 ms → 3,435 ms | 7,739 ms → 7,010 ms |

The catalog Gemmas ran locally on b11533, an RTX 5070 Ti and 32,768 context tokens, with thinking off.
Simple used the E4B QAT file (`063b60c663f5…`) and 256 output tokens. Intermediate used 12B Q4_K_M
(`239ec3629639…`) and 512. The signed-in Terra profile used 131,072 context tokens and Low reasoning. Simple
and Intermediate express variations with the controls and layers allowed by their profiles. Advanced can
express counted phrases explicitly.

Terra’s median completion on the 136 pace turns fell from 4,613 to 2,983 ms, and median generated tokens
fell from 126.5 to 51. Each replay ended every conversation with a newly acknowledged Stop. One compound
request to recall and vary a multi-region figure produced no accepted Terra update after repair on the final
run. Overall delivered responses were 338/339 there and 339/339 on the baseline. An independent run with the
same speed-limit cue, before the repair feedback edit, also scored 136/136 on pace, with a 3,247 ms overall
median.

Terra recovered the exact previous speed and the rest of the active pattern on 5/5 original follow-ups, up
from 0/5. The 12B result was 1/5 in both runs. For six counted end motifs the conservative check required
the right number of short strokes and a distinct group spanning the full range. Terra scored 5/6 before and
6/6 on the final run. In the baseline miss, transit between regions still reached the requested end. Both
Terra runs passed 5/5 full-device reach checks and 5/5 full-pass checks. On six separate ordered-region
probes the existing instructions and an additional reminder both produced all six requested sequences. The
extra reminder was dropped.

### Later turns

The longest retained conversation had 73 user turns. Across all seven sessions, 122 requests came after turn 32. Terra completed 120/122 of those on the first pass before and 121/122 after. Exact pace and preservation went from 50/60 to 60/60 there, and from 57/76 to 76/76 in turns 1–32. The 12B runs each scored 120/122 first pass and 57/60 exact after turn 32. In the independent 230-message tests, 12B and Terra could apply an explicitly restated correction. E4B could read the new values back but still applied the wrong band. [Long-context fixture and commanded trajectories](default-model-motion-review-2026-11-18.md)

A separate 111-turn run included 74 interactive turns while the real Autopilot scheduler was active in each
version. Its holding planner preserved controlled comparisons. This run gave the speed grammar and
previous-pace fact 52/52 exact pace turns, up from 35/52. Both versions completed 110/111 first-pass turns.
Median completion fell from 5.38 to 3.33 s. In six frozen-state counterfactuals, the earlier speed came
entirely from the engine and was absent from dialogue. Terra restored 6/6, up from 0/6. For six requests at
the ceiling, adding the explicit current-limit cue preserved 6/6 active patterns versus 4/6 without the cue.
Both forms still changed the pattern in all three separate requests that explicitly called for new
lower-region strokes at maximum speed.

In the six frozen-state speed restorations, E4B improved from 0/6 to 2/6 and 12B from 0/6 to 4/6. These
probes deliberately omitted the original speed from dialogue. In the retained conversations, five local pace
follow-ups found the simulator idle after reconstructed boundaries. Timing reflects replayed turns and host
load. The simulator measures the commanded score. Physical carriage feedback and the original time between
turns were unavailable.

The full E4B comparison combined the same two independent session batches on each build (57 and 282 user
turns). Exact pace went from 122/136 to 128/136. E4B delivered three fewer replies after retries and lost
one of the five full-device reach requests. It still passed all five full-pass requests. Of the 60 scored
later pace turns it got 57, up from 56. Only four previous-pace follow-ups were scorable on E4B because a
prerequisite slowdown did not change the commanded speed in the fifth. Exact restores went from 0/4 to 1/4.

### Local latency trials

| 12B variant on the same 339 original turns | First pass | Exact pace and preservation | Exact previous-pace restorations | Median all turns | Median prefill |
| --- | ---: | ---: | ---: | ---: | ---: |
| PR baseline | 329/339 | 131/136 | 1/5 | 2.06 s | 1.10 s |
| Selected, original message order | 327/339 | 131/136 | 1/5 | 2.09 s | 1.13 s |
| Current state after dialogue | 326/339 | 131/136 | 5/5 | 1.51 s | 0.56 s |
| Current state inside the existing guide | 322/339 | 127/136 | 4/5 | 1.47 s | 0.54 s |
| Move state only with intact history | 326/339 | 131/136 | 5/5 | 1.65 s | 0.57 s |

Moving the volatile state let llama-server reuse a larger input prefix. The variants lowered latency but
produced more first-pass misses on these conversations. The merged version also lost exact pace edits. The
selected local path keeps its original message order. On a matched 57-turn subset, enabling native KV chunk
reuse at 128 tokens took 2.27 s versus 2.05 s for the same selected binary. The two model packages and their
output and context budgets were held fixed within each comparison.

The original-wording 12B runs had 4/5 → 5/5 full-device reach requests and 5/5 → 5/5 full passes. For
open-ended descriptions the larger table reports accepted replies. Exact geometry is scored only on turns
where the request specifies it.

In the public two-wording synthetic suite, the current E4B, 12B and Terra paths scored 126/140, 139/140 and
152/152 on first-pass intent. The preceding default-profile study scored 127/140, 138/140 and 151/152
respectively. None of the three new runs issued an unrequested change on an exact hold. These suites include
motion-free acknowledgements, delayed corrections, geometry edits and Stop.

### Correcting invalid counted movements

Five archived model proposals failed because a downward join between separate stroke groups was impossible,
either inside the figure or at its loop seam. Each proposal, original user turn and preceding dialogue was
replayed twice with the same commanded starting score. Both versions validated a real Terra repair in all
ten trials. The old feedback kept every original count and endpoint in 5/10. The selected feedback explains
that separate regions can carry the same counts and endpoints while the shared engine connects them. It kept
all groups in 10/10. The repaired proposal still passes the regular phrase validator and shared sampler.

## Changes

Active positional Advanced hosted chat can encode a pace-only set or adjustment in a short disjoint schema
branch. Large strict schemas otherwise require the provider to generate many nullable fields even for a
single edit. The model chooses its operation, and the ordinary parser and lifecycle checks apply. The
speed-only edit retains geometry, rhythm, seed and layers. The full compositions remain available for
compound requests. The live chat also gets the previous distinct commanded flow speed from the engine. It is
scoped to the current stream and to the plan at the handoff, including when several unsubmitted targets
coalesce.

The local provider now reports token counts on streams that return llama.cpp timings without a usage block.
Its processed and cached prompt tokens are counted separately. The prompt builder also stopped constructing
and discarding a second interactive schema. In four matched one-CPU benchmark samples the Intermediate
assembly median fell from 283 to 211 µs and allocated 297,190 to 230,502 bytes per call. Advanced assembly
fell from 1.114 to 0.844 ms and from 493,096 to 317,145 bytes. The isolated assembly comparison produced
identical instructions and schema bytes.

## Upstream runtime

Every historical run used the original E4B Q4_K_M GGUF (`d0027dd3a912…`), the app’s closed-thinking
template, 16,384 context tokens, a 256-token output budget and CUDA 12.4. The October prompt and harness
came from `ef6f88b5`. A second application binary used the current prompts on the same file. Each row covers
22 intent checks with each of the same five seeds, 10252026 through 10292026. The control used the
app-managed b11533, and downloaded CUDA archives were checked against their upstream SHA-256 digests.

| Runtime | October fixture by seed | October total | Current prompts by seed | Current total |
| --- | --- | ---: | --- | ---: |
| b11533, app pin | 18, 17, 17, 16, 16 | 84/110 | 18, 19, 20, 19, 20 | 96/110 |
| b11874, previous rejected candidate | 15, 16, 13, 15, 17 | 76/110 | — | — |
| b12359, binaries linked from v1.0.0 | 17, 15, 14, 15, 16 | 77/110 | 19, 19, 19, 19, 20 | 96/110 |
| b12503, newest Windows nightly at selection | 4, 2, 2, 2, 3 | 13/110 | 1, 4, 1, 3, 1 | 10/110 |
| b12503 with Flash Attention off | 17, 16, 16, 14, 16 | 79/110 | 19, 18, 19, 19, 20 | 95/110 |

The frozen October run medians, excluding the first request after each runtime loaded, were 1.03 s on
b11533, 0.99 s on b11874, 0.90 s on b12359 and 2.99 s on stock b12503. The initial pilot omitted the
closed-thinking template and is excluded from the table. After restoring it, the two older builds reproduced
their published October scores exactly.

### Current default models on the same 57 retained turns

This comparison held the app prompt, catalog file and capability constant within each model. Medians include
inference and any repair. The table uses the original prompt baseline at `bcbd7358` to isolate the runner.

| Windows CUDA runtime | E4B first pass /57 | E4B exact /17 | E4B median | 12B first pass /57 | 12B exact /17 | 12B median |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| b11533 | 55 | 13 | 1,140 ms | 54 | 15 | 2,027 ms |
| b12359 | 57 | 14 | 773 ms | 55 | 15 | 2,344 ms |
| b12503 | 22 | 1 | 4,041 ms | 53 | 15 | 3,758 ms |
| b12503, Flash Attention off | 55 | 14 | 824 ms | 54 | 15 | 3,107 ms |

The stable bundle was faster and more accurate on the current E4B subset. It took longer on 12B and scored
lower on the frozen October prompt. Over all 339 retained turns the stock b12503 12B path completed 326
first pass, versus 329 on b11533. Both scored 131/136 for exact pace. The median / p95 moved from 2,058 /
3,235 ms to 2,204 / 4,526 ms.

### Isolating the runtime change

For a synthetic request specifying 65–100, the original, current and compact prompts produced identical
input token IDs across the tested runners (2,600, 3,017 and 3,065 tokens respectively). The pin and b12359
selected the requested bounds on all three. Stock b12503 missed all three, including contradictory widths on
the current prompt and 0–100 on the compact one. Forcing reasoning off missed all three again. Disabling the
chat parser exhausted the output budget on every trial. With Flash Attention off, the same b12503 binary
selected 65–100 in all three.

A grammar-free native greedy completion on the same 2,600 IDs diverged from the pin at generated token three on stock b12503. With Flash Attention off, b12503 matched the pin through token 19, as did b12359. This isolates a Flash Attention execution-path difference on the tested card. A specific faulty kernel or first bad commit would require a bisect. The app keeps its existing managed pin. [October release comparison](alpha74-release-review-2026-10-25.md) · [v1.0.0 Windows release](https://github.com/ggml-org/llama.cpp/releases/tag/v1.0.0) · [b12503 nightly](https://github.com/ggml-org/llama.cpp/releases/tag/b12503)

### Standalone reproduction with Google's weights

A native three-digit table lookup reproduced the failure outside MagicHandy. The input used only synthetic
bin labels, Gemma turn markers and an answer prefix. The probe sent the token IDs to `/completion` with
greedy sampling, no grammar and three output tokens. On Google's published E4B Q4_0, the 19-row case
tokenized to the same 262 IDs on all three configurations. Bin 009 contained `K650`.
[Pinned Google GGUF](https://huggingface.co/google/gemma-4-E4B-it-qat-q4_0-gguf/tree/4b4a2c1d584be7264f87aac328a1bc739ce81b6c)

| Windows CUDA build and flag | 3 rows | 19 rows | 73 rows | 211 rows |
| --- | ---: | ---: | ---: | ---: |
| Expected digits | 354 | 650 | 652 | 214 |
| b12359, auto | 354 | 650 | 652 | 214 |
| b12503, auto | 354 | **655** | **651** | **213** |
| b12503, off | 354 | 650 | 652 | 214 |

The checked-in probe reproduced all four outputs on a fresh launch of b12503 in each mode. The app's E4B
catalog GGUF also missed the last three cells on auto and got all four with Flash Attention off. In a second
control we omitted the answer prefix and allowed 384 tokens. Both models exhausted the budget on all four
stock-nightly lookups. Both returned all four correct terminal codes on b12359 and b12503 with Flash
Attention off.

The test host ran Windows 11 Pro build 26300, a Ryzen 9 9950X3D, an RTX 5070 Ti and NVIDIA driver 610.88.
The b12503 Windows CUDA 12.4 server reported commit `4577932fa` and b12359 reported `b1b770d90`.
The Google model SHA-256 was `676c35070db6dbe52f93e9c864ee0fba4eddea94b9c875d9cb10daff453fbaee`.

Start the server below, then run the Python command in another terminal. Repeat with `-fa off`.
The probe prints the input hash, exact answer and generated token IDs. It returns a nonzero status for
an incorrect lookup. [Standalone probe](../scripts/probe-gemma-flash-attention.py)

```powershell
.\llama-server.exe -m .\gemma-4-E4B_q4_0-it.gguf -c 16384 -np 1 -ngl 99 -fit off -fa auto --offline --no-ui --host 127.0.0.1 --port 8080
python scripts/probe-gemma-flash-attention.py --url http://127.0.0.1:8080
```

## Visual review

The local atlas retains 7,735 evaluated turn and Stop records across the study, including failed
selections. Recompiling the 4,896 distinct flow specifications and limit settings produced 102 overview
sheets. Every sampled position stayed within 0–100%. The gallery also contains 608 transport timelines.

Each accepted score was compiled with the shared engine and plotted with the initial quantized transport
points, velocity and acceleration. The separate dispatch charts mark Stop and the canceled tail. In the
speed-restoration control the original 18% score held its longer period, while the selected 43% score
matched the previously commanded cycle. In a frozen ceiling case the older response changed a sub-group’s
timing. Its trajectory drifted out of phase even though the overall speed and the maximum endpoint were
identical. The ceiling cue preserved the entire score. The largest acceleration examples show steep
reversals at high commanded speeds. The large stroke-width variation example alternates short strokes at the
ends with long travel between them. The study figure is
`.scratch/installed-replay-20261118/motion-replay-comparison.png` and the local gallery is
`.scratch/installed-replay-20261118/atlas/index.html`.

## Reproduction

Set `MAGICHANDY_QUALITY_MODEL_DIR` to the app model store and `MAGICHANDY_QUALITY_RUNNER` to the selected managed llama-server. For ChatGPT, select `--profile terra` and supply an existing signed-in `MAGICHANDY_HOSTED_DATA_DIR`. Selecting that profile sends original user turns and retained Autopilot remarks to ChatGPT. The source instance can stay running. Use a fresh output filename for each run. [Replay and atlas commands](motion-visual-review.md#reproduce-an-atlas)
