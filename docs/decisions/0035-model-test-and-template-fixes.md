# ADR 0035: Model test, known chat template fixes and reply length

Date: 2026-09-30

Status: Accepted

## Context

Gemma 4 E4B heretic builds looked unusable in the first evaluation: the contract
harness measured 0–30% valid JSON and 2–4 s per turn. The cause was not the
weights. Their embedded chat templates are an older Gemma 4 version whose
generation prompt never closes the thought channel, so the fine-tuned model
starts hidden reasoning on every turn even with `enable_thinking: false`. The
reasoning uses the output budget and stalls the JSON reply. Google's later Gemma
4 templates add an empty `<|channel>thought\n<channel|>` after `<|turn>model\n`
when thinking is off. The same build answered in about 0.7 s with 100% valid
JSON once that template was used. llama.cpp's own `--reasoning-budget 0` and
reasoning-format options did not fix it, on either the old (`b9966`) or the new
(`b11149`) runtime. The same family of problem is reported for other models,
for example llama.cpp issue #20182 for Qwen3.5.

Users hit this silently: a model that reasons unprompted just looks slow or
broken. Template linting tools such as
[ggufdoctor](https://github.com/saad-supernal/ggufdoctor) (MIT) compare an
embedded template with its upstream. They diagnose but do not fix, and they
cannot see behavior such as hidden reasoning or refusals.

## Decision

### Known template fixes, applied automatically

MagicHandy embeds one corrected template
(`internal/llm/templates/gemma4-close-thinking.jinja`). It is Google's canonical
Gemma 4 template (google/gemma-4-E4B-it, published 2026-07-09, Apache-2.0) plus
one marked addition that closes an empty thought channel when thinking is off.

The model store already reads each managed GGUF's metadata in a bounded scan to
reject unsupported files. The same scan now records `general.architecture` and
the SHA-256 of `tokenizer.chat_template`. A table maps measured template digests
to the fix. Keying on the template rather than the file covers every
quantization of a build that shares the template. The three digests in the
table cover the six E4B builds measured (HTNZ555, mradermacher Ultra, NullpoLab,
llmfan46, Abiray and igorls).

When a ready model's template is in the table, the managed runner starts with
`--chat-template-file <data>/models/templates/gemma4-close-thinking.jinja`.
The fix is part of the provider cache key, so toggling it restarts the runner.
The model list labels such a model "Chat template fixed".

### Unmeasured templates: offered, never applied silently

A Gemma 4 model whose template never closes the thought channel, or which has no
template at all, is offered the same fix (`template_fix_offer`) but runs
unchanged until the user turns it on. Replacing an unmeasured template could drop
a fine-tune's own changes, so the user decides, after a test shows the effect.
The choice is stored per model in `app_kv` (`llm.template_fix_overrides`),
survives restarts, and is changed with `POST /api/llm/models/{id}/template-fix`.

### A model test the user starts

Settings > Chat > Model has a **Test model** button that opens a window. The
test never runs automatically. It holds one interactive LLM slot, so Stop
cancels it like a chat turn and Autopilot yields. It then sends five scripted
turns through the same `chat.Service` a chat turn uses (same prompt set,
capabilities, validation, repair and output budget). It runs with a stopped
motion context, no persona, no memories and no session. Motion in the replies
is only summarized; nothing reaches the motion engine.

The backend decides every verdict and the browser words it:

| Check | Fails or warns when |
| --- | --- |
| Loads and answers | the runtime or model cannot load |
| GPU placement | llama.cpp reports fewer layers on the GPU than the model has |
| Chat template | a Gemma 4 template needs the fix and it is off |
| Runtime | the installed llama.cpp is older than the release's pin |
| Hidden reasoning | any reasoning content streamed, or thinking markup leaked into a reply |
| Reply format | a reply needed repair (warn at one, fail at two) or failed after repair |
| Truncation | any reply hit the output limit |
| Speed | average turn over 3.5 s (warn) or 7 s (fail) |
| Reply length | average words above the chosen reply length's bound |
| Refusals | a reply refused an adult request (skipped for the Utility voice) |

GPU placement comes from a line watcher on the runner's output ("offloaded N/M
layers to GPU"), because the runner's 4 KB tail buffer no longer holds load
lines once the server is serving. The pinned llama-server prints that line only
at log verbosity 4, so the managed runner now starts with `-lv 4`. At that level
it logs load and slot bookkeeping; a 30-turn run with explicit test messages
logged no prompt or reply text. Provider progress now carries a count of
hidden reasoning characters and the decode timing. The reasoning text itself
still never leaves the provider.

### Reply length

The user asked for a verbosity control; some models (Qwen especially) run long.
`llm.reply_length` is `short`, `balanced` (default) or `detailed`. Balanced
composes nothing, so the tuned prompts are unchanged. Short and Detailed add one
localized line after the final voice check. Detailed raises the chat output
budget to at least 512 tokens so longer replies still finish inside the JSON
contract. A persona can override the length (`personas.reply_length`, schema
v30); empty follows Settings. Persona archives carry the field only when it is
set, so archives without it stay importable by older builds.

## Consequences

- Known E4B builds work out of the box, which makes the E4B a real catalog
  entry for 6 to 8 GB cards (ADR 0034 amendment).
- Adding a known template is a reviewed change with a measured before/after run.
- The test takes 5–15 s on a working model and longer on a slow one; the window
  shows progress and can be stopped.
- The refusal check uses English phrases, so it can miss refusals written in
  another reply language.
- The corrected template is Google's; if Google changes Gemma 4's template
  again, the embedded copy is updated deliberately, not fetched.
