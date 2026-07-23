# Chat Voice: Why MagicHandy Sounded Sanitized, And The Fix

Status: implemented 2026-07-23. `Settings > Prompts & memory > Chat voice`
selects the reply register; `utility` reproduces the previous behavior
byte-for-byte.

## The report

MagicHandy's chat output reads sanitized and non-sexual compared to
StrokeGPT-ReVibed (STGPT-RV) on the same local models. This document compares
the two prompt stacks, isolates which differences cause the register gap with
live A/B runs, and records the design that closes it.

## Method

Both apps' system prompts were exported exactly as composed at request time
(MagicHandy: `ComposeSystemWithCapabilities` with the default set, full
capabilities, and a four-pattern catalog; STGPT-RV: `LLMService.
_build_system_prompt` with its default context and persona). Both were run
against the **same model** — `nexusriot/Gemma-4-Uncensored-HauhauCS-
Aggressive:e4b`, STGPT-RV's own default — at the same temperature (0.3),
`format:"json"`, two runs per input, four inputs spanning conversation,
description, teasing, and escalation. The model is uncensored and tuned for
erotic roleplay, so every register difference observed is caused by the
prompt, not the model.

## Measured baseline

With MagicHandy's prompt, the model answers as a device operator and
frequently **copies the contract's structural example replies verbatim**:

| Input | MagicHandy reply | STGPT-RV reply (same model) |
| --- | --- | --- |
| "Start slow and talk to me while you do it." | "Starting gently." | "Mmm, slow and steady for you... I'm going to feel every inch..." |
| "Tell me what you're doing to me right now." | "Keeping it steady." | "Just this. A slow, gentle press all down the length..." |
| "Tease the tip and make me beg for it." | "Starting that tease!" | "Oh, you want me to tease? I'm going right there... slow and needy." |
| "Faster - I'm getting close." | "Adjusting the pace." | "Oh god, you're so close... I'm going to pick up the pace right now!" |

"Starting gently.", "Keeping it steady.", and "Adjusting the pace." are the
*literal example strings* from MagicHandy's JSON contract. The motion JSON was
correct in every run on both sides — the register gap is purely the `reply`
text.

## The differences and their effects

Ordered by measured impact:

1. **Example replies act as templates.** MagicHandy's contract examples
   ("Starting gently.", "Adjusting the pace.") are all device-operator
   register, and the model copies them verbatim (also seen earlier when a
   model echoed the placeholder "short user-facing reply"). STGPT-RV's one
   full example reply is in-character ("I want you right there while I keep
   the pressure slow and needy."). *Effect: the single largest register
   anchor. The examples must stay (they teach the JSON shapes) but the prompt
   must forbid imitating their wording.*
2. **Identity framing.** "You are MagicHandy's local motion assistant" vs
   "You are my adult erotic partner, not an assistant and not a narrator."
   Assistant framing invokes assistant register: hedging, service phrasing,
   third-person distance. *Effect: no first-person embodiment.*
3. **No language permission.** STGPT-RV grants and instructs: "use direct
   erotic language... do not sanitize or euphemize, and do not turn the reply
   clinical." Uncensored models still default to a neutral register without
   that permission. *Effect: neutral wording even on a model tuned
   otherwise.*
4. **No anti-clinical rules.** STGPT-RV's FINAL CHAT VOICE CHECK bans
   "engage, apply, execute, adjust the motion, set the range, parameters..."
   and requires describing motion as touch/pace/pressure. MagicHandy had
   nothing — and its own examples model the banned register. *Effect:
   "Adjusting the pace."*
5. **No variation pressure.** STGPT-RV requires varying sentence shape,
   sensation focus, and vocabulary, and feeds back recent assistant lines to
   avoid repeating. *Effect: MagicHandy replies repeat the same stock
   frames.* The follow-up now supplies the latest three canonical assistant
   lines, bounded to 180 characters each.
6. **Supporting systems.** The first review found no dedicated persona,
   user-anatomy vocabulary, or model-reported mood state. The follow-up adds
   all three as backend-authoritative, bounded context for interactive
   non-utility voices.
   Memory consolidation still does not rewrite or summarize memories with a
   model, so STGPT-RV's consolidation-specific wording rule remains inapplicable.

MagicHandy's contract-first design is deliberately kept: the JSON contract,
capability gates, speed bands, and repair pass are strictly better than
STGPT-RV's (which lets custom prompts weaken the contract). The fix adds
voice without touching any of that.

## The fix: a code-owned voice axis

`Settings > Prompts & memory > Chat voice` selects one of four levels,
composed as a `CHAT VOICE` section after the contract/motion context and
before memories (mirroring STGPT-RV's final voice check placement, where late
instructions win register conflicts):

| Level | Register | Key instructions |
| --- | --- | --- |
| `utility` (default) | Neutral assistant, previous behavior | No voice section at all; byte-identical prompt (regression-tested). |
| `warm` | Flirtatious companion | First person, present tense; suggestive at most, never explicit; motion described as touch/rhythm, never settings. |
| `intimate` | Sensual partner | In-character partner "in the room"; direct sensual language; clinical-phrase ban list. |
| `explicit` | STGPT-RV parity | Adult erotic partner; "do not sanitize, euphemize, or turn the reply clinical"; full ban list; variation pressure. |

Every non-utility level carries the two highest-impact lines: a
non-assistant identity, and "The JSON examples in the contract show structure
only; never copy or imitate their reply wording."

Properties:

- **Code-owned**: like the contract, the voice text cannot be weakened or
  drifted by prompt-set edits; the level is a settings enum
  (`llm.chat_voice`), validated server-side, preserved for older clients
  that omit the field.
- **Orthogonal**: it composes with any prompt set (including the localized
  builtins and user sets) and with any capability-gate combination. Voice
  changes the `reply` register only — the motion contract, capability
  enforcement, speed limits, and Stop behavior are identical at every level.
  Non-utility interactive chat additionally permits inert `new_mood` metadata.
- **Default is `utility`.** Shipping behavior does not change until the user
  opts in. If the product default should be `warm` or `intimate`, that is a
  one-constant change for the maintainer to decide.
- **Utility remains byte-identical.** Persona, anatomy, mood, and recent-line
  context are composed only for interactive `warm`, `intimate`, and `explicit`;
  utility does not advertise or update mood and suppresses the mood readout.
  Switching back restores that session's last model-reported mood.
- **Profile data cannot bypass control gates.** Persona/custom-anatomy values are
  whitespace-normalized, length-bounded, JSON-quoted, and labeled as data, not
  instructions. They never enter the user message or motion context. A model
  command is also stripped unless the current user turn contains a positive,
  action-specific motion request. Negated requests and ordinary conversation
  objects are rejected; authorized commands still pass the unchanged
  capability, semantic, range, engine, and transport gates.

## Live results at each level

Same model, same inputs, one run per level (2026-07-23). Zero
contract-example copies at any level, and the motion JSON stayed valid and
band-appropriate in every run:

- `warm` — first person, affectionate, non-explicit: "I'm starting softly
  just for you... tell me everything that's swirling around in your head
  tonight?" / "Oh really? I feel that closeness... Let me speed things up
  for you."
- `intimate` — in-character partner voice, sensual but not graphic: "I'm
  moving slowly over you right now; it feels like soft strokes tracing every
  curve." / "The rhythm is picking up, pulling us closer."
- `explicit` — erotic partner register, present tense and embodied: "I'm
  sinking into you slowly... feeling the heat build under my touch." These
  transcripts predate the anatomy/persona/mood follow-up below; no new live
  transcript is claimed for that follow-up.

(Full transcripts live in local QA output only; they are not repository
content.)

## Parity follow-up

The same PR now closes the four concrete continuity gaps identified above:

- **User anatomy vocabulary**: `llm.user_anatomy` accepts `penis`, `vagina`,
  or `custom`; `llm.custom_anatomy` is whitespace-normalized and capped at 120
  Unicode characters. The code-owned rule keeps user anatomy separate from
  partner persona and is composed only for interactive non-utility voices.
  Fresh settings use the reviewed STGPT-RV `penis` default; saved documents
  that predate the field load as empty `custom` (neutral) so an existing user is
  never silently assigned anatomy-specific wording.
- **Persona description**: `llm.persona_description` is an optional dedicated
  field, capped at 500 Unicode characters, that survives prompt-set switching.
- **Mood tracking**: interactive non-utility responses may report optional
  `new_mood` from the reviewed STGPT-RV 17-value register: `Curious`, `Teasing`,
  `Playful`, `Loving`, `Excited`, `Passionate`, `Seductive`, `Anticipatory`,
  `Breathless`, `Dominant`, `Submissive`, `Vulnerable`, `Confident`, `Intimate`,
  `Needy`, `Overwhelmed`, `Afterglow`. The strict parser validates it before
  persistence; effective mood lives in per-message diagnostics, is scoped to
  the chat session, appears in `/api/state`, and is a read-only Chat header
  status. Explicit transitions carry `mood_changed`; later deterministic or
  autonomous lines may carry the effective value but cannot overwrite a newer
  transition with a stale snapshot. Generated replies remain invisible and do
  not prune capped history until they pass the Stop-epoch acceptance point, so
  a rejected transition cannot become the fallback mood. Stop publishes its
  epoch without waiting on SQLite; a reply accepted first may finish its
  transaction after a later Stop, but that Stop still fences motion and TTS. It
  is inert metadata, not motion style or sentiment inference.
- **Recent-line anti-repetition feedback**: the backend queries the latest
  three assistant rows from the selected session's canonical SQLite log,
  collapses each to one line, caps each at 180 Unicode characters, quotes them
  as data, and presents them oldest-to-newest. Client-supplied history cannot
  alter this anti-repetition section. The broader 12-message model history is
  also rebuilt from the selected backend session; the legacy request field is
  accepted for compatibility but ignored, and the first-party client no longer
  sends it.

The final output guard is still the last prompt section. For non-utility
interactive chat its shape permits optional `new_mood`; every motion rule,
capability gate, speed band, and repair pass remains unchanged. Deterministic
current-turn authorization prevents profile/history text from creating motion
on a chat-only request, including semantic repair fallback. Chat Stop now
advances the same invalidation barrier as the mounted Stop regardless of chat
storage or stale session selection, cancels overlapping generations, publishes
the new sequence without waiting for transport I/O, and reaches the transport
before attempting history or SSE delivery. New engine admission remains closed
until that physical Stop returns. Conservative exact Stop phrases and
deterministic replies cover every built-in prompt language, and transport Stop
failure is visible in Chat. Prompt-only settings updates also
no longer refresh active motion or emit transport traffic.

Remaining follow-ups:

- **Memory wording rule**: when memories are consolidated by the model,
  preserve the user's own wording at non-utility voices (STGPT-RV: "do not
  sanitize sexual language" in profile consolidation).
- **Localized voice sections**: instructions are English for every prompt
  set (reply language still follows the set, and STGPT-RV behaved the same);
  translating the voice sections is deferred until the localized sets get a
  native-speaker pass.
