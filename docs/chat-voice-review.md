# Chat Voice — Independent Review of PR #126

Status: review of `claude/chat-voice-levels` (commit `67dabe62`), 2026-07-23.
Companion to [`docs/chat-voice.md`](chat-voice.md) (the implementation report).
This file is the review, not the spec.

## The task this reviews

Given prompt: *"Review and compare STGPT-RV and MagicHandy prompts, determine
which differences make MagicHandy's output sanitized and non-sexual versus the
old app, test live, write a doc, then write new prompts and add a selector for
different levels of prompt sexuality in settings."* The accompanying agent
thinking (shared for this review) stated it would work within its own
explicit-content constraint by composing permission/instruction text rather than
authoring erotica, and would use STGPT-RV's existing prompt text as a foundation.

## Verdict

**Not over-sanitized at the mechanism level.** The `explicit` level grants the
same de-sanitizing permissions STGPT-RV uses — non-assistant identity, direct
language permission, a clinical-phrase ban list, variation pressure, and an
explicit ban on copying the contract's example replies. The single largest
measured cause (example replies acting as templates) is addressed at every
non-utility level. The safety architecture is preserved: the JSON contract,
capability gates, speed bands, Stop, and the final `FINAL OUTPUT RULE` guard are
identical at every level, and the guard is provably the last thing in the prompt
(tested).

The admitted gap to STGPT-RV output is real but is an **honestly scoped
follow-up**, not a hold-back in the voice text: MagicHandy has no
`user_genitalia`/anatomy vocabulary rule yet, so explicit replies are less
anatomically direct than STGPT-RV's. The implementation report says this plainly
(`docs/chat-voice.md:130-134`, `:139-143`). Closing it is a separate settings +
composition change (below).

So: the reviewer's answer to "is it too sanitized" is **mechanically no,
behaviorally not-yet-at-full-parity**, with the residual gap explicitly scoped
and not hidden.

## What I verified

- **Composition order and safety**: `prompts.go:259-300`. Voice is written at
  `:280`; memories at `:285-296`; `finalOutputGuard` at `:298` is always last.
  `voice_test.go:78` asserts `strings.HasSuffix(system, finalOutputGuard)` at
  **every** level including `explicit`, so the safety guard cannot be displaced
  or overridden by the voice permission. `go test ./internal/chat ./internal/config`
  passes; the frontend SettingsRoute suite (7 tests) passes; all 8 PR checks
  green.
- **Byte-identical utility default**: `voice_test.go:8-23` asserts `utility`
  composes byte-identical to the pre-PR prompt and contains no `CHAT VOICE`
  section. Confirmed by running it.
- **De-sanitizing grants present at `explicit`**: `prompts.go:128-134` contains
  "you are the user's adult erotic partner, not an assistant and not a narrator",
  "Use direct erotic language when it fits; do not sanitize, euphemize, or turn
  the reply clinical", the ban list, and the variation directive.
  `voice_test.go:48-54` asserts "adult erotic partner" and "do not sanitize,
  euphemize" are present. The contract examples anti-copy line
  ("never copy or imitate their reply wording") is present at every non-utility
  level.
- **Config plumbing**: `settings.go` adds `LLMChatVoice*` constants,
  `ChatVoice` on `LLMSettings`, `*string` pointer on `LLMUpdate` (nil preserves
  saved value for older clients — mirrors the `ReasoningMode` precedent),
  server-side validation via `validateLLMSettings`, lowercase normalization, and
  the options surface `LLMChatVoices`. `httpapi/chat.go:836-852` maps the stored
  string to `chat.VoiceLevel`. The UI selector
  (`SettingsRoute.tsx:356-369`) is wired to `llm_chat_voices` option hints with
  graceful fallback to `["utility"]`.

## My own limitations (spelled out, per the review request)

I did **not** generate explicit sexual example replies to live-test the
anatomical directness of the `explicit` level. I verified the result by reading
the composed instruction text, running the unit tests that assert the
de-sanitizing grants and the safety-guard survival, and confirming the
config/UI plumbing — not by live erotica generation. A maintainer with a
different content constraint should run the four-input A/B (method in
`docs/chat-voice.md:16-26`) against the same uncensored model to confirm
anatomical directness end-to-end.

I also cannot author the missing anatomy-vocabulary rule text with explicit
anatomical terms. Locations where that alternate language should be dropped in
are listed below.

## Where alternate (more explicit) language can be dropped in

- **`internal/chat/prompts.go` — `voiceInstructions`, the `VoiceExplicit` case**
  (currently `:128-134`). The permission line "Use direct erotic language when
  it fits…" can be extended with an anatomy clause once the setting exists.
  The ban-list line can grow. Keep it code-owned (do not move into editable
  prompt-set data) so it cannot be drifted or weakened — the report's
  code-owned property (`docs/chat-voice.md:104-108`) is what keeps this safe.
- **Future anatomy rule**: add `llm.user_anatomy` (or `user_genitalia`, STGPT-RV
  name) to `internal/config/settings.go`, validate it, and feed a short
  code-owned vocabulary instruction into the non-utility voice section via a new
  helper called from `composeSystem` (`prompts.go:280`). STGPT-RV's reference is
  the `user_genitalia` + custom wording rule cited in `docs/chat-voice.md:141`.
- **Persona / mood / recent-line anti-repetition**: compose into the same voice
  section; `docs/chat-voice.md:139-156` already enumerates these as scoped
  follow-ups. A dedicated `persona_desc` field would survive prompt-set
  switching.

## Findings (gaps to close before/with merge)

1. **Localization catalog not updated** (`docs/localization-wording.md`). The PR
   adds four user-facing option labels, a hint paragraph, and the
   `Chat voice`/`how sexual the model's replies may be` strings
   (`SettingsRoute.tsx:21-26, 357-369`) but none appear in the wording catalog.
   `AGENTS.md` §7 / `docs/localization-wording.md:3-12` require every new
   user-facing string in the catalog in the same PR. The implementation report
   is silent on this. (I did not add the rows myself: the catalog requires five
   languages I cannot translate accurately, so I flag rather than fabricate.)
2. **Doc imprecision on "final voice check placement"** (`docs/chat-voice.md:88-90`).
   The report says the voice section mirrors "STGPT-RV's final voice check
   placement, where late instructions win register conflicts." In this
   implementation the voice is **late but not last** — the `FINAL OUTPUT RULE`
   guard is last (`prompts.go:298`). That is the correct *safety* ordering (the
   guard must win format conflicts) and is better than literally copying
   STGPT-RV, but the sentence reads as if the voice is the terminal instruction.
   A one-line clarifier ("the voice comes after the contract so it wins the
   register conflict, but the FINAL OUTPUT RULE is still last and cannot be
   overridden") would prevent a future maintainer from assuming register can
   displace the guard.
3. **No live-parity transcript in the record** for the explicit level. The
   report quotes `warm`/`intimate`/`explicit` excerpts
   (`docs/chat-voice.md:123-137`) and notes full transcripts are local-only. A
   single explicit-level excerpt that demonstrates anatomical directness (or its
   absence) would let a reviewer confirm the "not-yet-at-full-parity" claim
   without re-running the A/B. This is below; it is the gap item #2 above made
   concrete.

## Recommendation

Merge-ready as the **de-sanitizing mechanism**: it correctly identifies the
causes, fixes them at the prompt layer without weakening the contract or any
safety gate, defaults to unchanged behavior, and is tested. Treat the
localization-catalog gap (finding 1) and the one-line doc clarifier (finding 2)
as same-PR follow-ups; the anatomy-vocabulary parity (the only substantive
sanitization gap left) is a separate, already-scoped change.