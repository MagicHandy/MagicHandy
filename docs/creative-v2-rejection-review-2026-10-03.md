# Creative v2 response rejection investigation — October 3, 2026

## Report and current limits of the evidence

A user reports consistent Creative v2 failures while sending `hello`. Two
screenshots show `creative_v2 response rejected: creative v2 requires up to
eight edits ([] for no change) and a non-empty bounded reply`. Some earlier
turns succeeded. The user's app version, provider/model, composed prompt,
persona configuration, status report and failed raw response are not yet
available. This investigation does not establish their specific root cause.

On alpha.54 (`0e4102df`), that message combines four independent checks:
missing/null `edits`, more than eight edits, a missing/null/blank `reply`, or
a trimmed reply over 12,000 UTF-8 bytes. It does not mean a greeting needs
motion edits: `action:"none", edits:[]` is the expected no-change transaction.
The JSON decoder reached this check, so the error differs from invalid JSON,
an unknown control, a saved-limit violation, or a transport failure.

Creative v2 deliberately performs one generation without repair or an inferred
motion fallback. A rejected turn never reaches shared-engine dispatch. Existing
motion is not changed by that rejection; Emergency Stop remains independent.

## Confirmed hardening change

The Creative v2 response schema required `reply` to be a string but allowed
the empty string, which the runtime parser rejects. Add `minLength:1` and make
the non-empty requirement explicit in the Creative v2 prompt. Whitespace-only
text and the existing UTF-8 byte limit remain independently enforced by the
parser. The schema requirement survives recall augmentation and every live
action branch, including paused state and Autopilot's standing-wish field.

Replace the combined error with field-specific errors. Missing/null edits
still require an explicit array; excessive edits report the count; blank
replies identify `reply`; oversized replies report byte length. No raw reply
content is added to logs, diagnostics, history or exports. No malformed output
is accepted, no edit is inferred, and no retry or motion authority is added.

## Reproduction and verification

Regression tests fail against the previous generic error and empty-string
schema. After the change they cover each rejection, the exact byte boundary,
non-ASCII byte accounting, and unchanged scores. HTTP regressions verify that
even rejected proposals containing `action:"start"` and a speed edit do not
create a motion engine, command the fake transport, retry the model or persist
assistant output. The user line remains in canonical history.

Local live probes use fresh isolated data, the built-in prompt, utility voice,
reasoning off, 1,024 output tokens, Creative v2 selected and Autopilot off.
They send six consecutive `hello` messages per installed Ollama model:

| Model | Alpha.54 source | Hardened source |
| --- | --- | --- |
| `huihui_ai/granite4.1-abliterated:3b` | 6/6 valid; no motion | 6/6 valid; no motion |
| `igorls/gemma-4-12B-it-qat-q4_0-unquantized-heretic:Q4_0` | 6/6 valid; no motion | 6/6 valid; no motion |

Every probe retains its streamed raw output, errors, acceptance and engine
state under `.scratch/creative-v2-rejection-20261003/`. The original user
configuration has not been reproduced. These successful greetings establish
compatibility with those local endpoints, not a fix for the reported incident.
No accepted proposal changes motion, so there is no new motion curve to render.
The shared compiler, motion character, transport and Stop path are unchanged.

## Evidence to request for the reported incident

Settings → Diagnostics provides **Copy exact prompt** and **Copy report**.
Capture them while the failing chat/persona and Creative v2 are selected. The
prompt inspector composes the system prompt from current state; it does not
capture the entire failed request, output schema or history. Relevant-only lore
can change when the next message is sent.

For the decisive evidence, capture one failing `/api/chat/stream` response in
the browser Network panel, including `delta`, `malformed` and `error` events.
The concatenated `delta.data.text` values are the initial model output. The
stream also identifies provider/model and prompt set. Save that response before
refreshing or retrying; rejected raw JSON is not retained in assistant history
or the status report. Review its private conversation content before sharing;
the whole browser HAR is unnecessary and may contain unrelated requests.

Also record app version, whether the provider is managed/external llama.cpp or
Ollama, selected model, and whether the same greeting succeeds with **LLM
motion: Off** and in a fresh chat. Off is a temporary way to continue ordinary
chat; it does not diagnose which Creative v2 field failed. Preserve the failing
conversation/configuration until evidence is collected.
