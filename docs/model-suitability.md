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
