# ADR 0042: Bounded conversation windows per external connection

Status: Accepted; included in the alpha.61 release candidate.
Date: 2026-10-11.

## Context

The application discarded history before external models could use their larger
contexts: storage retained 200 messages, composition retained 64 messages / 24 KB,
and Responses always admitted requests against a conservative 32K window.
Long-running stories and phase-based motion requests consequently lost their
earlier agreements. Raising only the provider limit would not fix that.

## Decision

Save `context_window` on each named external connection, independently of local
runtime settings. Zero and 32768 retain the previous request policy. Larger
choices apply to chat and conversation-aware Autopilot using that connection.

| UI choice | Maximum history messages | Maximum history text | Conservative request window |
|---|---:|---:|---:|
| Recent | 64 | 24,000 UTF-8 bytes | 32,768 |
| Long | 256 | 96,000 UTF-8 bytes | 131,072 |
| Extended | 512 | 192,000 UTF-8 bytes | 262,144 |

Request admission counts one UTF-8 byte per input token, then reserves output,
declared reasoning, schema and template space. These are application budgets,
not tokenizer counts or a promise that a remote server supports that capacity.
Instructions and the current request are preserved; oldest whole history messages
are removed if necessary. Included canonical dialogue is never summarized,
rewritten, keyword-filtered or partially clipped.

Stored conversations retain 1,000 committed messages independently of the
200-message display window and bounded history-page responses. Browser reads
request only the newest 200 rows on first load or reset, anchored to the page
snapshot, rather than
downloading all retained context and discarding it in the UI. The default API
still permits bounded pagination through the full retained history. A dedicated
hosted prompt reader materializes at most the chosen history budget plus one
bounded look-ahead message. Counts, contents and omissions share one SQLite
statement snapshot. Incremental recovery delivers every retained revision in
bounded pages, even after a hidden tab accumulates over 200 messages; the display
window must not silently skip unseen changes. Pending replies remain invisible.
Pruning finds an indexed
sequence cutoff and updates the existing revision marker atomically with deletion;
this preserves late-commit and recovery behavior without repeated exclusion sets.
Already deleted history cannot be restored.

Existing external connections keep Recent until changed. On a newly selected
ChatGPT/OpenAI/OpenRouter catalog model, the editor suggests Long only when the
provider advertises at least 131072 context. Unknown/manual models and generic
compatible endpoints start with Recent to preserve local-server performance. Explicit choices
remain editable, and catalog choices beyond advertised capacity are disabled.
Settings and Setup use the same compact history field and collapsed explanation.

Local Ollama and managed/external llama.cpp keep their existing prompt limits,
sampling, model-context configuration and call count. Local speech history is
trimmed before copying/unwrapping, even if a caller has a much larger transcript.
Technical-only planning receives no conversation; the optional second opinion
continues to receive only its bounded motion context and recent requests.

The existing motion contract, shared engine, limits, controller admission,
generation fencing, pause and Stop behavior remain authoritative. Longer history
helps the model select the next semantic edit; it does not enqueue a long
sequence of future device commands or add another planning loop.

Responses now reports actual input/output/cached token counts when supplied.
History omission diagnostics account for database selection, composition and
provider admission. Typed context-capacity failures suggest reducing the setting;
they do not trigger refusal fallback, rewrite dialogue or expose remote error text.

## Alternatives and evidence

The [live evaluation](../long-context-review-2026-10-11.md) compared Recent, Long
and Extended on two account-visible models. Longer windows retained early story
facts and revised phase instructions, with useful response times. A tested
history-first prompt rearrangement did not show consistent cache or latency
benefits and was discarded. The existing prompt order remains.

Automatic summarization and provider-side conversation storage are deferred.
They add a separate continuity representation, latency and failure modes that
were unnecessary for the measured sessions. ChatGPT plan HTTP requests require
full needed input and `store:false`; they do not accept `previous_response_id`
for persistent conversation state. See the official
[Sign in with ChatGPT preview requirements](https://developers.openai.com/siwc/token-sharing-open-source/preview-limitations).

Histories remain finite, and capacity is not guaranteed recall. Users can choose
Long for normal extended sessions and Extended when a longer story needs it,
subject to provider support, latency, usage limits and API charges.
