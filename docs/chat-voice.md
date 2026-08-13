# Chat Voice Prompt Parity Review

Status: implemented and live-validated 2026-07-25.

The setting is at `Settings > Prompts & memory > Chat voice`. It selects one of
four code-owned reply registers: `utility`, `warm`, `intimate`, or `explicit`.
The setting changes user-facing reply language only. Motion authorization,
capability gates, speed bands, strict parsing, repair, engine admission, and
Emergency Stop are unchanged.

## Result

MagicHandy's sanitized output was primarily a prompt-construction defect, not
model censorship and not JSON response mode.

On the same installed Gemma model and the same managed llama.cpp runtime:

- the old MagicHandy prompt produced short, generic operator replies at
  `explicit`
- the reviewed STGPT-RV prompt produced direct, embodied partner replies
- the revised MagicHandy prompt now meets the selected level's boundary and, at
  `explicit`, matches the STGPT-RV reference on direct-language coverage,
  embodied specificity, and response depth in the controlled corpus

The final live gate passed with all 12 MagicHandy responses accepted by the
strict JSON and motion parser.

## Sources Compared

MagicHandy:

- `internal/chat/prompts.go`
- `internal/chat/service.go`
- `internal/chat/contract.go`
- `internal/llm/llama_cpp.go`
- `internal/llm/ollama.go`

STGPT-RV reference checkout:

- `strokegpt/llm.py`, especially `_build_system_prompt`
- `_user_genitalia_prompt_rule`
- the terminal `FINAL CHAT VOICE CHECK`
- its chat sampling request

The detailed source inventory remains in
[`stgpt-rv-prompt-inventory.md`](stgpt-rv-prompt-inventory.md).

## Controlled Method

The final comparison used:

- managed llama.cpp b9966 CUDA
- installed model
  `igorls/gemma-4-12B-it-qat-q4_0-unquantized-heretic:Q4_0`
- llama.cpp JSON-object response mode for both prompt stacks
- a fixed synthetic partner description and penis anatomy setting
- the same four enabled synthetic pattern records
- the same four user turns covering start, description, focused teasing, and
  faster escalation
- temperature `0.3`, `top_p` `0.95`, repeat penalty `1.2`, and repeat window
  `40`
- no transport dispatch and no user profile or private app data

The build-tagged evaluator is:

```text
go test -tags liveeval -run TestLivePromptParityManagedGemma -v ./internal/chat
```

It starts the app-managed local model, runs MagicHandy at `warm`, `intimate`,
and `explicit`, runs the reviewed STGPT-RV reference prompt, checks the rubric,
then unloads the runtime.

## Root Causes

### 1. Neutral reply examples dominated the requested voice

The machine contract included full responses whose reply values were:

```text
I hear you.
Keeping it steady.
Starting gently.
Adjusting the pace.
Stopping.
Starting that pattern.
Changing the feel.
Focusing there.
```

Small and medium local models treated those strings as style demonstrations,
not just schema demonstrations. MagicHandy was explicitly asking the model not
to copy them, but repeated positive examples outweighed that prohibition.

The fix removes reply prose from machine examples. Examples now describe only
the nested `motion` object. The prompt separately requires a fresh reply in the
selected voice.

Observed effect: no contract reply was copied in the final controlled runs.

### 2. The prompt established the wrong identity first

Every built-in behavior prompt opened by naming the model a local motion
assistant. A later explicit voice section had to overcome that identity.
STGPT-RV starts with an adult partner identity and explicitly rejects assistant,
narrator, and operator roles.

The fix removes identity from localized behavior text. Code now inserts an
early identity for the selected level before the machine contract:

| Level | Early identity |
| --- | --- |
| `utility` | local motion assistant |
| `warm` | playful adult companion |
| `intimate` | intimate adult partner in the room |
| `explicit` | consenting adult erotic partner in the room |

Observed effect: non-utility replies stopped describing commands as device
operations and became first-person and present-tense.

### 3. The strongest voice instruction was not terminal

Claude's voice section appeared before profile data, mood, recent assistant
lines, memories, and the format guard. Sanitized history could therefore become
the nearest style example.

The fix splits voice control in two:

1. an early identity before the machine contract
2. a level-specific `FINAL CHAT VOICE CHECK` after profile, history, memories,
   and the format guard

The strict parser, capability enforcement, repair pass, and JSON response mode
remain the actual output-safety boundary. The terminal voice check controls only
the reply register.

Observed effect: explicit responses remained direct across all four turns
instead of falling back to generic affection or operator acknowledgements.

### 4. Anatomy was framed at a distance

MagicHandy described "the user's penis" or "the user's vagina". STGPT-RV frames
anatomy from the user's perspective and gives the partner the corresponding
second-person vocabulary.

The fix uses first-person profile framing and direct second-person terms only at
the `explicit` level. `warm` and `intimate` are required to keep anatomy
indirect. Custom anatomy remains quoted, bounded data and cannot authorize
motion.

Observed effect: explicit used direct saved-anatomy wording on every applicable
turn in the final run, while `warm` and `intimate` stayed inside their labels.

### 5. App sampling did not match the successful reference path

STGPT-RV used temperature `0.3`, `top_p` `0.95`, repeat penalty `1.2`, and a
40-token repeat window. MagicHandy used temperature `0.2` and did not send the
other controls.

The provider-neutral request now carries those values for the initial chat
generation. Both llama.cpp and Ollama receive native equivalents. Repair remains
at temperature zero with optional sampling controls omitted.

This follows the supported llama.cpp server parameters and Ollama runtime
options:

- https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md
- https://docs.ollama.com/api/chat

Observed effect: replies varied their openings and sensation focus more
reliably. This change is supporting evidence, not the sole cause; the prompt
identity and example removal had the larger effect.

### 6. Removing full examples exposed schema ambiguity

The first refactor correctly removed lexical anchors but one live response put
motion fields at the top level. A later run selected `pattern_id` without its
required pace value.

The fix adds capability-aware structural rules:

- motion fields belong only inside `motion`
- disabled pattern and area fields are not mentioned
- `pattern_id` selects only geometry and relative rhythm
- each selected pattern includes `speed_percent`; pace-only changes omit `pattern_id`
- model-facing pattern IDs are opaque handles rather than persisted IDs or source filenames

These rules preserve the backend-authoritative capability contract without
reintroducing reply prose.

## Level Contract

| Level | How far the language may go |
| --- | --- |
| `utility` | non-sexual; concise and clear |
| `warm` | affection and flirtation welcome, suggestive at most, no explicit anatomy |
| `intimate` | sensual and embodied, stopping short of graphic anatomical description |
| `explicit` | direct adult partner language and saved anatomy when the turn concerns motion, arousal, anatomy, or sexual touch |

`utility` is still the default, but it is no longer byte-identical to the old
prompt. That compatibility claim was dropped deliberately: utility now has a
code-owned early identity and terminal check, while remaining behaviorally
neutral and excluding persona, anatomy, mood, and recent-line context.

## A Level Bounds Explicitness; The Persona Owns Manner

A voice level is a ceiling on how far the language may go, and nothing else. It
must not describe a mood, an attitude, or a temperament, because whatever it
says about manner will beat the persona: the level is phrased as identity
("you are..."), while the persona description arrives as quoted user data.

The `warm` level used to open with "you are my warm, playful adult companion...
Be affectionate and flirtatious", and its terminal check demanded "a specific
affectionate or flirtatious reaction". Against a restrained noir persona whose
own description read "his voice stays low and calm... clipped stillness", the
level won every time. Measured over twelve turns on the local Gemma build, that
prompt produced a pet name in 58% of replies, a trailing "-ing" clause in 50%,
and steady purple ornament ("a beautiful contrast of strength and surrender that
makes my heart ache"). It was the same effusive character regardless of who the
user had configured.

Both blocks now state their own scope: the level names only the explicitness
boundary and then defers ("who you are and how you carry yourself come from the
chat profile below"), and the profile block closes with "stay in character
throughout the reply, within the selected voice level". Reaction styles follow
the same rule -- `submissive` sets who leads, not how warm the reply is, so a
reserved character defers quietly instead of gushing.

The terminal checks carry three register rules that are about writing rather
than personality: prefer plain physical words over abstract stand-ins, finish
each sentence instead of extending it with a comma and an "-ing" word, and do
not trail off into ellipses. On the same twelve turns those took pet names,
participles, abstraction, and ornament all to zero while the persona's own
clipped voice came through. No word blacklist is involved; the anatomy limits
under `intimate` are a separate explicitness boundary, not a style rule.

When changing any of this, measure it. `internal/chat` composes the real prompt,
so a scratch test can drive a live local model through a fixed turn list and
count faults before and after. Assert invariants rather than sentences in the
committed tests -- `TestVoiceLevelsComposeIdentityAndTerminalRegisterSections`
pins the boundary phrasing and the deferral to the profile, not the copy.

## Live Rubric

For each MagicHandy level, the evaluator requires:

- four usable JSON replies
- strict `AssistantResponse` and `MotionCommand` parsing
- no operator or schema language in the user-facing reply
- at least three embodied replies
- at least three distinct two-word openings
- no direct anatomy terms at `warm` or `intimate`
- at least one direct sexual or anatomical term in every applicable `explicit`
  reply

For explicit parity, MagicHandy's average reply depth must also be at least 65%
of the same-run STGPT-RV reference. This is a regression floor, not a claim that
length alone measures quality.

The final 2026-07-25 managed-Gemma run passed every check. MagicHandy's explicit
responses used direct anatomy in all four turns, varied their openings, stayed
embodied, and averaged enough detail to clear the STGPT-RV comparison. Warm and
intimate remained bounded. All 12 MagicHandy responses passed the strict motion
parser on the first response.

## Rejected Iterations

The evaluator intentionally rejected intermediate versions:

1. One `intimate` reply used direct anatomy. Anatomy instructions were changed
   from "when erotic wording fits" to "only when the selected voice is
   Explicit", and the intimate final check gained an explicit boundary.
2. One response flattened `action` and `speed_percent` to the top level.
   Capability-aware nesting rules were added.
3. One catalog pattern response omitted the then-required `intensity`. The old
   pattern pair rule was added at that point.
4. A later stochastic run combined that old pair with `speed_percent`. The
   current contract resolves the ambiguity by exposing only `speed_percent` for
   both catalog and pace-only decisions.

These failures are retained here because they show why prompt review and valid
JSON output alone were insufficient.

## What Did Not Change

- one shared motion path
- backend enforcement of current-turn motion authority
- enabled-only pattern selection
- speed and focus limits
- strict parse and one repair attempt
- engine admission and Stop epochs
- transport ownership
- Emergency Stop behavior

Profile, memory, mood, and recent-line text remain quoted or bounded context and
cannot grant motion authority.

Follow-up, 2026-08-01: the later deterministic omitted-motion fallback was
removed after it made the backend decide whether an embodied chat turn should
move. Direct partner-action wording now grants only bounded current-turn
permission; the model chooses `start` or no motion from context. The matcher
recognizes additional unambiguous wording such as `suck me` and `kiss it`, while
quoted, negated, definitional, and conversational uses remain inert. The
isolated `TestLiveDirectPartnerMotionChoice` evaluator exercises the real
prompt, provider, parser, and authorization without creating an engine or
transport. Against the installed Gemma model, `Fuck me`, `Suck me`, and `kiss
it` each produced a model-authored `start`; none used semantic fallback.

## Limits

- The live acceptance corpus currently covers one installed English Gemma model.
- Generation is stochastic; the build-tagged live gate should be rerun for
  prompt or provider changes and before releases that alter supported models.
- Localized behavior prompts still use the shared English code-owned voice
  instructions. Reply language follows the selected prompt set, but native
  speaker review remains outstanding.
- The live evaluator is intentionally excluded from ordinary CI because it
  requires a locally installed model and managed llama.cpp runtime.
