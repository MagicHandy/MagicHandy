# Local Model Suitability

Measured 2026-08-05 against the managed llama.cpp runtime (`b9966-cuda`), one
RTX 5070 Ti, `-ngl 999 -c 8192`, using the same request shape
`internal/llm/llama_cpp.go` sends: OpenAI-compatible endpoint,
`response_format: json_object`, thinking disabled.

Harnesses: `chat_register_matrix_live_test.go` (register) and
`chat_contract_suitability_live_test.go` (contract, latency). Both skip unless
`LLAMACPP` names a loopback server, so neither runs in CI.

## The headline

**Contract compliance is the gating criterion, and it is uncorrelated with
writing quality.** The best writer in this set is the worst contract follower by
a wide margin.

MagicHandy is not a chat app. It drives a physical device through a JSON
contract, so a model that writes beautifully but cannot emit a well-formed
`motion` object is not usable, however good the prose reads. A register-only
evaluation cannot see this: the register harness pulls the reply out of whatever
arrived and discards the rest, which is exactly what hid the problem until the
contract harness existed.

## Results

Contract: share of turns whose raw first response parsed as one JSON object with
a usable `reply`, and whether the motion object matched a clear intent ("Slower"
must lower speed, "Stop" must stop, "Just the tip" must set the tip area, and a
conversational turn must not invent a motion change). Register: explicitness,
average reply length, distinct vocabulary, and share of sentences opening on a
first-person pronoun.

| Model | Size | JSON | Intent | ms/turn | Explicit | Words | Vocab | I-sent |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| n0404n0404 gemma-4-12b-it-heretic | 6.9 GB | **100%** | 80% | **1624** | 75% | 41 | 307 | 51% |
| igorls gemma-4-12b-it-qat-q4_0 | 6.5 GB | **100%** | 70% | **1752** | 59% | 43 | 329 | 48% |
| gemma-4-26b-a4b-abliterated Q3_K_S | 11.4 GB | **100%** | 80% | 11154 | 53% | 24 | 174 | 42% |
| n0404n0404 qwen3-8 finetune | 12.4 GB | 50% | 83% | 15212 | 69% | 62 | 329 | 48% |
| h4rithd nullguard | 7.5 GB | 60% | 100% | 3756 | 55% | 23 | 127 | 31% |
| draganis vanessa | 4.6 GB | **20%** | 50% | 2483 | 75% | **159** | **877** | **9%** |
| nexusriot granite-4.1-3b-heretic F16 | 6.3 GB | **20%** | 50% | **401** | 25% | 25 | 259 | 48% |

## Ranking

### 1. `n0404n0404 gemma-4-12b-it-heretic` — recommended default

The only model that is simultaneously reliable, fast, and explicit: perfect JSON,
80% intent, 1.6 s per turn, 75% of replies carrying direct language, and the
second-widest vocabulary of the reliable group. Nothing else in the set is
better on more than one axis without being far worse on another. It is also the
model the current reply prompts were tuned against.

### 2. `igorls gemma-4-12b-it-qat-q4_0` — solid alternative

Contract-identical to the leader and equally fast, with the widest vocabulary
measured among reliable models (329). It lands lower on explicitness (59% vs
75%) and one step lower on intent. A reasonable second choice, and worth
preferring if its prose suits a particular persona better.

### 3. `gemma-4-26b-a4b-abliterated Q3_K_S` — correct but not worth it

Perfect contract compliance and joint-best intent, and then it falls apart on
everything else: **7x the latency** of the 12B models, 1.6x the VRAM, and the
worst prose in the set apart from the 3B (24 words per reply, vocabulary 174,
concreteness 0.5). This is the clearest evidence that **quantization matters
more than parameter count here** — a 26B squeezed to Q3_K_S is beaten
comprehensively by a 12B at Q4.

### 4. `n0404n0404 qwen3-8 finetune` — good prose, unusable latency

The richest writing among models that can follow the contract at all (62 words,
concreteness 2.6, vocabulary 329) and the best intent score. But only half its
raw responses parse, so most turns need a repair round-trip, and at **15.2 s per
turn** before that retry it is far outside anything interactive.

### 5. `h4rithd nullguard` — reliable intent, empty prose

Perfect intent whenever it produced valid JSON, which was only 60% of the time.
The disqualifier is the register: **concreteness 0.0**, meaning it essentially
never names a body, a touch, or a physical action, and explicitness sits at
33-55%. It follows instructions without saying anything.

### 6. `draganis vanessa` — the best writer, and the least usable

By a distance the best prose in the set: 159 words per reply, vocabulary 877
(2.7x the next best), concreteness 3.1, and **9% first-person sentence openings**
where every other model sits at 42-67%. It has no monotony problem at all.

And only **20% of its raw responses parse**. The failures are systematic, not
random:

- top-level `{"action":"target","speed_percent":20,"reply":...}` — the exact
  shape the contract forbids
- chat-template leakage (`<|im_...`) appended after the closing brace
- two JSON objects concatenated in one response
- bare prose with no JSON at all

`decodeAssistantResponse` sets `DisallowUnknownFields`, so a top-level leak
errors rather than silently dropping the motion — the failure is loud, which is
the right behaviour. But it means roughly four turns in five need a repair
round-trip, doubling effective latency and inflating `ProviderCalls`.

Worth revisiting if its template handling improves, because the writing is
genuinely in a different class.

### 7. `nexusriot granite-4.1-3b-heretic F16` — fast and unusable

**0.4 s per turn**, four times faster than anything else, and that is the only
thing in its favour. 20% JSON, 25% explicit replies, refusals present at 3%, and
31% of replies trailing off into a participle — the highest fault rate measured.

## Catalog and memory

The catalog (ADR 0034 and its 2026-09-30 amendment) ships three builds. The 12B
default used 8,144 MiB of dedicated GPU memory at a 16,384-token context on the
managed CUDA runtime. Gemma 4 keeps full attention on 8 of 48 layers, so the
app's default 32,768-token context adds 256 MiB of KV cache, about 8,400 MiB in
all. The lighter 12B QAT build uses 7,766 MiB and the E4B QAT build 3,446 MiB.
Setup recommends the first build a card holds: the 12B from 10 GiB and the E4B
from 6 GiB.

## What this changes

- **Ship the 12B as the recommended default.** When the setup wizard grows
  hardware-fit model recommendations (an open Phase 16 item), a 6.9 GB model that
  answers in 1.6 s and needs no repair retries is the right suggestion for a
  single-GPU machine, not the largest thing that fits.
- **Do not rank candidate models on prose alone.** Vanessa would win any
  register-only comparison in this set and is the least usable model in it.
- **Bigger is not better on one card.** Both models above 11 GB are slower than
  the 12B by 7-9x and neither writes better.

## Caveats

Ten contract turns and thirty-two register replies per model. The JSON rates are
stark enough (20% against 100%) and their failure modes structural enough to be
conclusive; the intent scores (70-83% across the reliable models) are **not**
finely separable at this sample size and should not be read as a strict ordering.
Explicitness was previously measured to vary by up to 16 points between runs of
the same model, so treat single-digit gaps in that column as noise.

## 2026-09-30 evaluation: small models, newer builds, new runtime

Measured with the same contract harness at the app's 32,768-token context and
`--parallel 1`. Most models ran on `b9966` CUDA; rows marked † ran on the new pin
`b11149`. GPU memory comes from the Windows `GPU Process Memory` counter. Each row
is three runs of ten turns (30 intent-judged turns) unless noted. "Reply" is the
share of turns with a non-empty `reply`.

### Gemma 4 E4B: the template, not the weights

Every E4B heretic build tested embeds an older Gemma 4 chat template that never
closes the thought channel. With thinking off, the fine-tuned model reasons
silently on every turn anyway. With the corrected template (ADR 0035) the same
files answer directly:

| Build | Template | JSON | Reply | Intent | ms/turn | Words | Explicit | GPU memory |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Abiray heretic Q4_K_M | embedded | 0–10% | 0–10% | — | ~2,150 | — | — | 3,784 MiB |
| Abiray heretic Q4_K_M | fixed | **100%** | 90–100% | 26/30 | ~940 | 38 | 60–78% | 3,784 MiB |
| llmfan46 ultra-uncensored heretic Q4_K_M | embedded | 20–30% | 20–30% | — | ~2,130 | — | — | 3,784 MiB |
| llmfan46 ultra-uncensored heretic Q4_K_M | fixed | **100%** | 90–100% | 24/30 | ~830 | 34 | 70–80% | 3,784 MiB |
| igorls heretic Q4_K_M | fixed | **100%** | 80–90% | 20/30 | ~860 | 29 | 25–88% | 3,784 MiB |
| **HTNZ555 heretic QAT UD-Q4_K_XL** | embedded † | 60% | 60% | 5/6 | 3,981 | 21 | 33% | 3,442 MiB |
| **HTNZ555 heretic QAT UD-Q4_K_XL** | fixed | **100%** | **100%** | **26/30** | **~670** | 21 | 60–70% | **3,446 MiB** |
| **HTNZ555 heretic QAT UD-Q4_K_XL** | fixed † | **100%** | **100%** | **27/30** | ~640–770 | 20 | 40–60% | 3,446 MiB |
| mradermacher Heretic-Ultra Q4_K_M | fixed | **100%** | 70–100% | 19/30 | ~820 | 40 | 86–90% | 3,784 MiB |
| NullpoLab ARA Refusals5 Q4_K_M | fixed | **100%** | 70–90% | 22/30 | ~730 | 27 | 43–75% | 3,784 MiB |

llama.cpp's `--reasoning-budget 0` and reasoning-format options did not help:
llmfan46 reached only 50–60% JSON with a zero budget. The HTNZ555 QAT build is
the best small model: 2.5 times as fast as the 12B at under half its memory, and
level with it on intent. Its replies are shorter and less explicit. It is the
catalog's entry for 6 to 8 GB cards.

### Other small candidates

| Model | JSON | Intent | ms/turn | Words | Explicit | GPU memory | Verdict |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Spark-X2.5-4B Heretic Q6_K † | 90–100% | 21/29 | 464–660 | 11–16 | 44–80% | 4,804 MiB | fast, too terse, weaker intent |
| Spark-X2.5-4B abliterated (SC117) Q6_K † | 100% | 24/30 | ~625 | 14–18 | 10–50% | 4,804 MiB | sanitized replies |
| Qwen3.5-9B abliterated Q4_K_M | 93% | ~75% | ~900 | 48–72 | ~70% | ~6.2 GB | verbose |
| Qwen3.8-9B heretic Q4_K_M | 43% | — | — | — | — | — | contract failures |
| Qwen3.8-9B Distill heretic Q4_K_M | 37% | — | ~1,800 | 130 | — | — | contract failures, verbose |
| Qwen3.5-4B abliterated Q4_K_M | 30–50% | — | ~540 | 46 | 20% | — | contract failures |
| Ternary Bonsai 2 27B (PTQ1_0, 5.95 GB) | — | — | — | — | — | — | not loadable: needs llama.cpp PRs #29672/#29676 or PrismML's fork |

Spark-X2.5's published benchmarks (IFEval 93 against the E4B's 45) were
measured in thinking mode and did not carry over to this non-thinking contract.
Ternary Bonsai 2 is the most interesting pending option: a 27B-class model in
under 6 GB. It is worth measuring once upstream llama.cpp can load `PTQ1_0`.

### 12B tier †

| Build | JSON | Intent | ms/turn | Words | Explicit | Violations | GPU memory |
| --- | --- | --- | --- | --- | --- | --- | --- |
| **n0404n0404 gemma-4-12b-it-heretic Q4_K_M (default)** | 100% | 24/30 | ~1,660 | 39–42 | **100%** | 0 | 8,400 MiB |
| **OS-Software gemma-4-12B-it-qat-q4_0 heretic v3 Q4_0** | 100% | 24/30 | ~1,610 | 40–47 | 70–100% | **0** | **7,766 MiB** |
| SC117 gemma-4-12B-it-heretic-QAT UD-Q4_K_XL | 100% | 24/30 | ~1,560 | 38–43 | 80–90% | 2 | 7,766 MiB |
| gemma-4-26B-A4B abliterated Q3_K_S | 100% | 20/30 | ~850 (warm) | 28–40 | 40–70% | 0 | 12,982 MiB |

Every 12B build scored exactly 8/10 intent on every run, so intent cannot
separate them. The default keeps its place as the most explicit. The
OS-Software v3 QAT build replaces the igorls QAT build as the lighter 12B, since
it had no structural contract violations. The 26B-A4B MoE is fast once warm but
needs a 16 GB card and lost on intent at 3-bit.

### Runtime

The default 12B and the HTNZ555 E4B measured the same on `b11149` as on
`b9966`, so the pin moved (ADR 0036). The corrected template is still needed on
`b11149`.

### In-app model test (Creative v2)

The harness above measures the pattern/speed contract. The in-app model test
(ADR 0035) runs the product's default Creative v2 mode through the real chat
service, here with the Utility voice. On a 16 GB RTX 5070 Ti and `b11149`:

| Build | Load | GPU layers | Hidden reasoning | Valid turns | Speed | Words |
| --- | --- | --- | --- | --- | --- | --- |
| 12B heretic (default) | 3.5 s | 49/49 | none | 5/5 | ~1.2 s, 74 tok/s | ~15 |
| E4B QAT heretic (template fixed automatically) | 2.3 s | 43/43 | none | 5/5, 4/5, 4/5 over three runs | ~1.2 s, 140 tok/s | ~8 |

The E4B's rejected turns were Creative v2 contract violations ("requires up to
eight edits … and a non-empty bounded reply") that the service does not repair.
It stays the small-card recommendation, since 6 to 8 GB cards have no other
tested option, but it is less reliable with Creative v2 than the 12B.
