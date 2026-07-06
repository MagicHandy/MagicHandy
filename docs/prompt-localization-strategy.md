# Prompt Localization Strategy

## Decision

Use hybrid localized prompts for local models:

- Localize behavior, persona, anatomy, memory, and voice-output prose into the
  target language.
- Explicitly tell the model which language to use for the user-facing `reply`
  value.
- Keep JSON keys, enum values, pattern IDs, and the code-owned JSON contract in
  English.
- Do not translate protocol tokens such as `reply`, `motion`, `action`,
  `pattern_id`, `speed_percent`, `none`, `start`, `target`, `stop`, `stroke`,
  `pulse`, or `tease`.

This is the rule for both neutral prompt packs and future explicit/adult prompt
packs. Adult/persona/anatomy prose should be translated at the same tone and
explicitness as the source; only machine protocol stays stable.

## Why Not English-Only Prompts

An English prompt with one instruction like "answer in Spanish" usually protects
JSON compliance, but it gives small local models weak language priming. These
models can over-weight the surrounding English instructions and produce English
or mixed-language `reply` text, especially after repair prompts or saved-memory
blocks.

## Why Not Fully Translated Prompts

Fully translating the entire system prompt, including the JSON schema and enum
descriptions, gives stronger language priming but creates protocol risk. Small
local models are more likely to translate JSON keys or enum values when the
schema explanation is translated, for example emitting localized equivalents of
`reply`, `motion`, `start`, or `stop`. MagicHandy's parser is intentionally
strict, so translated protocol tokens become malformed responses.

## Hybrid Shape

The composed prompt should have this shape:

```text
<localized behavior/persona instructions>
<localized instruction: write the user-facing `reply` in the target language>
<English instruction: keep JSON keys and enum values exactly as defined>

<code-owned English JSON contract>

<localized saved-memory header, if memories are present>
- <user-authored memory text, verbatim>
```

The prompt contract remains appended by code, not stored inside prompt sets. That
keeps editable/user-imported prompts from weakening the parser contract while
still letting built-in and custom prompt sets control persona and language.

## Current Built-In Prompt Sets

`internal/chat/prompts.go` defines these built-ins:

| Language | Prompt set ID | Reply language |
| --- | --- | --- |
| English | `magichandy_motion_v1` | English |
| Spanish | `magichandy_motion_v1_es` | Spanish |
| Portuguese (Brazil) | `magichandy_motion_v1_pt_br` | Brazilian Portuguese |
| Simplified Chinese | `magichandy_motion_v1_zh_hans` | Simplified Chinese |
| Japanese | `magichandy_motion_v1_ja` | Japanese |

The existing English ID remains the default so saved settings and old traces do
not change behavior unexpectedly. A future locale setting can select the matching
built-in prompt set by default for new users.

## Repair Prompts

Repair prompts should stay strict and protocol-focused. They may be English, but
must include the selected prompt set and should tell the model to preserve the
reply language required by that prompt set. The repair pass should fix JSON
shape, not translate or rewrite the user's preferred language.

## Validation Guidance

When validating prompt changes against Ollama, use the local models that are
strong at this task, such as:

- `igorls/gemma-4-12B-it-qat-q4_0-unquantized-heretic:Q4_0`
- `draganis/vanessa`

For each supported language, test both chat-only and motion-request turns. A pass
requires all of the following:

- The response parses as one JSON object with no markdown or extra keys.
- The `reply` value is in the selected language.
- JSON keys and enum values remain exact English protocol tokens.
- Motion requests use only permitted `action`, `pattern_id`, and
  `speed_percent` values.
- User-authored memories are referenced naturally and are not silently sanitized
  or translated unless the user requested translation.

Suggested smoke prompts:

```text
hello, talk to me briefly
start slow motion
make it a little faster
stop now
```

Run the same intent in the target language too. The model may receive user input
in any language, but the selected prompt set determines the default language for
the `reply` value unless the user explicitly requests a different language.
