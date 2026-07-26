# Prompt Localization Strategy

## Decision

Use hybrid localized prompts for local models:

- Localize behavior, persona, voice, anatomy, mood, memory framing,
  conversation context, recent-reply framing, reply-language reminders, and
  repair-language reminders.
- Keep user-facing `reply` text in the language selected by the built-in prompt
  set.
- Keep JSON keys, enum values, pattern IDs, and the code-owned machine contract
  in English.
- Preserve user-authored memories, persona text, custom anatomy, and canonical
  recent assistant replies verbatim inside bounded context.
- Keep the final output guard last, after every localized or dynamic block.

Do not translate protocol tokens such as `reply`, `motion`, `action`,
`pattern_id`, `speed_percent`, `none`, `start`, `target`, `stop`, `stroke`,
`pulse`, `tease`, `new_mood`, or accepted mood values.

This applies to neutral and adult prompt profiles. Adult/persona/anatomy prose
must retain the source tone and explicitness; only machine protocol remains
language-neutral.

## Rationale

An English prompt followed by “answer in Spanish” weakly primes smaller local
models. They may follow the surrounding English instructions instead,
especially after saved-memory blocks or a malformed-output repair.

Translating the whole schema creates the opposite failure: models begin
translating JSON keys and enum values. MagicHandy's parser is intentionally
strict, so a translated equivalent of `reply`, `motion`, `start`, or `stop` is
invalid.

Hybrid composition gives the model sustained target-language prose while
placing one stable English wire contract at the end.

## Composition Order

`composeSystem` produces this order:

```text
<localized built-in behavior/persona instructions>
<localized code-owned voice identity>
<English machine behavior constraints that define stable tokens>

<code-owned English JSON contract>
<code-owned pattern and motion capabilities>
<localized authoritative motion/conversation framing, when present>
<localized profile, anatomy, mood, and recent-reply framing>
  <user/model-authored values quoted verbatim>
<localized saved-memory header>
  <user-authored memories verbatim>
<localized final voice check>
<localized reply-language reminder for a built-in prompt set>
<code-owned final JSON output guard; always last>
```

Prompt sets cannot weaken or replace the parser contract. If a built-in set is
missing, composition falls back to the English built-in. Custom prompt sets do
not receive a built-in English language override: their own instructions remain
responsible for reply language, while the machine contract is still appended.

## Built-In Prompt Sets

| Language | Prompt set ID | Reply locale |
| --- | --- | --- |
| English | `magichandy_motion_v1` | `en` |
| Spanish | `magichandy_motion_v1_es` | `es` |
| Portuguese (Brazil) | `magichandy_motion_v1_pt_br` | `pt-BR` |
| Simplified Chinese | `magichandy_motion_v1_zh_hans` | `zh-Hans` |
| Japanese | `magichandy_motion_v1_ja` | `ja` |

The English ID remains the default for existing settings. The browser prompt
selector and source installer map native language choices to these IDs through
`config.PromptSetForLocale`.

## Dynamic Context

Code-owned framing is localized, but values are not:

- Persona descriptions and custom anatomy are quoted as user-authored text.
- Saved memories are included verbatim under a localized header.
- Recent assistant replies are canonical database text and remain verbatim.
- Mood and motion enums stay exact English protocol values even when surrounding
  prose is localized.
- Pattern IDs remain stable. Display names and descriptions may be localized in
  the future without changing the identifier sent in JSON.

This boundary avoids accidental translation of identity, intent, or historical
conversation content.

## Repair Prompts

`RepairPrompt` remains strict and protocol-focused. It includes a localized
instruction to preserve the selected built-in reply language while correcting
only JSON shape and accepted values. The final repair request still names the
English protocol tokens.

A repair must not:

- translate a correct user-facing reply into English;
- rewrite or sanitize user-authored context;
- accept localized JSON keys or enum values; or
- add markdown, commentary, or keys outside the contract.

## Automated Validation

`internal/chat/prompt_localization_test.go` covers all built-in locales and
asserts:

- localized behavior, voice, context, memory, anatomy, and final reminders are
  present;
- user-authored values remain byte-for-byte present;
- JSON keys and action/mood enums remain English;
- the code-owned output guard is the final prompt block;
- repair prompts preserve target language;
- custom prompt sets do not receive a built-in English language override.

The permanent live test is intentionally build-tagged because it requires a
running local model:

```powershell
go test -tags liveeval ./internal/chat -run TestLivePromptLocalizationGemma -v
```

Prerequisite: an OpenAI-compatible llama.cpp server at
`http://127.0.0.1:8080`. The test discovers the loaded model through
`/v1/models`. It creates no motion engine or transport and therefore cannot send
a device command.

## Live Gemma Evidence

On 2026-07-26,
`igorls-gemma-4-12b-it-qat-q4-0-unquantized-1bdd95189f67` passed the live
llama.cpp matrix in a final repeated 15.78-second run:

- Spanish, Brazilian Portuguese, Simplified Chinese, and Japanese;
- one chat-only request per language;
- one motion-intent request per language; and
- one deliberately malformed-response repair per language.

All 12 responses parsed as one strict JSON object. Every `reply` matched the
selected language heuristic, every motion request used the English `start`
enum, and every repair preserved the target reply language. This is evidence
for the tested Gemma build and prompt shape, not a guarantee for every local
model or quantization.

## Review Gate

For prompt changes, run unit tests first, then the live matrix against the
primary managed llama.cpp path. A release-facing prompt change passes only when:

- chat-only responses do not accidentally authorize motion;
- target-language replies are natural and not mixed with English instruction
  prose;
- motion and mood fields use only accepted English values;
- malformed-output repair preserves language and strict JSON;
- memories and profile values are not translated or sanitized; and
- the final output guard remains last.

Ollama remains supported, but managed llama.cpp with the installed Gemma model
is the primary acceptance path. Do not substitute an unrelated Ollama model for
this check without recording it as additional, not equivalent, evidence.