# ADR 0025: Model-interpreted intent for continuous chat

- Date: 2026-09-06
- Status: Proposed for review
- Extends: ADRs 0023 and 0024

## Context

Live users reported valid Creative v2 and Layered replies being rejected as
unrequested changes. The host used overlapping English word lists, global
negation detection, and a few phrase-specific exceptions to classify requests.
Gemma 12B produced the exact requested edit in eleven independent cases that
those checks rejected. A restriction on speed could block a width edit;
asking for an explanation could cancel an accompanying instruction. These
rules also treated some translations and pace paraphrases as unauthorized.

Adding more recognized phrases would preserve the same failure mode. The user
explicitly requested interpretation without deterministic chat-input rules.

## Decision

Interactive Creative v2 and Layered responses declare a top-level `action`:
`none`, `start`, or `update`, alongside their existing partial `edits` and
`reply`. The model interprets the entire request and conversation. Backend
authorization does not classify chat words or infer motion from reply prose.

The backend constructs the output grammar from its authoritative snapshot:

- Running: `none` with empty edits, or `update` with the mode's bounded edits.
- Stopped: `none` with empty edits, or an explicit `start` decision. An update
  cannot implicitly start motion. Starting with unchanged settings is valid.
- Paused: only `none` with empty edits. A generated response cannot resume it.

Each action is a separate schema branch. Choosing `none` constrains the edits
to an empty object or array before generation. Runtime checks independently
reject contradictory actions, invalid state transitions and invalid scores,
including when a provider ignores the schema. Only the validated semantic
`MotionCommand` reaches the existing shared engine. Controller ownership,
Stop cancellation, run/mode fences, saved limits, sampling and sanitization
remain authoritative. No second motion path or inference repair loop is added.

Remove the old continuous-mode word and coverage classifiers, including the
Creative v2 Lab experiment's duplicate scope check. Experimental score parsers
retain their compact edits/reply format; they may decode the additive action
field. Backend-generated Autopilot decisions retain their existing scheduling
authority and continuation contract. This decision does not broaden their
speed/range or exact-hold permissions.

## Consequences

The host guarantees state and device constraints; semantic request following
is a model-quality property evaluated with live replies. A correctly formed
response can still misunderstand a request. Tests must therefore record both
structural acceptance and requested-control/preservation accuracy, retaining
rejected proposals and accepted mismatches. Removing an error alone is not
evidence of better motion mapping.

The selected harness uses one generation and one prompt across the evaluated
models. Shortening it hurt E4B. Single-pass summaries, a separate intent pass,
an extra units explanation, and extra reasoning were evaluated and not selected.
They did not justify more complexity or improved acceptance at the expense of
understanding. See [the live review](../continuous-request-review-2026-09-06.md)
for measurements, remaining failures, and reproduction.
