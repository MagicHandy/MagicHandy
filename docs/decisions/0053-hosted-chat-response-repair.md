# ADR 0053: Bounded repair of rejected hosted chat responses

Status: Proposed for review, 2026-10-23. Extends ADRs 0039 and 0041.

## Context

A hosted model can understand a request but produce a response that violates
the structured contract or motion validator. Layered's counted phrases exposed
this distinction: a model-selected combination could contain an invalid join
even though the user's requested regional movement is possible. Rejected output
must not partially change motion, speech, mood, or conversation history. Users
also need a way to request correction without writing the same message again.

## Decision

Add `llm.hosted_response_repair_enabled`, defaulting on when absent, for
interactive hosted chat. The service retains its original request, response
schema and validation closure. A structural or semantic JSON rejection can
make exactly one additional request to the same provider/model with the
original conversation and user request, the complete rejected object, and the
exact validator feedback. The replacement passes the same parser, admission,
authorization and compiler contract. No inferred motion, semantic fallback,
keyword rule or relaxation of the original constraints is allowed.

Eligibility requires object-shaped JSON output of at most 64 KiB. Plain prose,
empty output, typed provider refusals, authentication/network failures and
provider truncation are not repair candidates. Repair input still passes the
provider's normal context budget: its original user turn, rejected response and
feedback are protected as a three-message tail, and an over-budget essential
request fails explicitly. No history or user text is silently rewritten to
force admission. A refusal during repair cannot chain into the separate local
refusal fallback and create a third model request.

When automatic repair is disabled, the current chat error can offer **Repair
response**. `POST /api/chat/repair` accepts the canonical session ID, retained
failure ID and current video motion owner, with ordinary controller and Stop
headers. It does not
accept a replacement prompt, model response or motion payload. The server keeps
one process-local continuation for the latest failed turn, expiring after five
minutes. It consumes that continuation after atomic workspace admission and
before provider work; a failed or canceled repair cannot be clicked again. A
click racing the original stream's cleanup cannot consume an unadmitted repair.
Reloading does not resurrect it.
Failure diagnostics remain available independently of this temporary action.

Admission requires the same active session and latest committed user turn,
workspace revision, persona/profile/memories, saved provider/model/motion
settings, credential revision, controller identity/generation, Stop sequence,
mode and engine generation/plan. New turns and session switches advance a
workspace revision even if no reply was committed. Generation reuses the
ordinary interactive provider lane and cancellation lifetime. A repaired
retarget uses the shared engine's atomic `ApplyTargetIfCurrent` check, so it
cannot replace motion changed by Autopilot or another control. Starts still
use the existing engine admission and Stop generation. There is no second
motion loop or dispatch path.

The same chat SSE stream reports `repairing_response`, then the accepted reply
or final error. A successful correction is one assistant reply after the
original user row; it never appends a duplicate user row. The existing bounded
failure log records the rejected output, exact initial validator error,
replacement output, input contract and timing, including successful repairs.
Secret redaction precedes truncation and persistence. Remote controllers see
only a safe repair action and shared conversation status; raw failures and
provider diagnostics remain administrator-only.

Stop, request cancellation and stale work still retain the already rejected
attempt under an independent 500 ms diagnostic-write deadline. This cannot
publish a reply, speak or dispatch motion. `repair_outcome` distinguishes a
successful replacement, failed correction, Stop, cancellation and stale work;
a late cancellation updates only that existing record's non-secret outcome.
The captured original provider/mode remains the diagnostic provenance even
when cancellation was caused by a later settings change.

Capturing the optional repair context happens before composition. If that
capture fails, repair admission closes while the ordinary request can still
produce a valid reply; repair bookkeeping cannot make otherwise usable chat
unavailable. Failure metadata includes the actual canonical user sequence,
independently of the context revision used for stale-work checks.

## Consequences

Valid hosted replies retain their single generation. Invalid replies can incur
one additional model call and its cost/latency. The UI explains that tradeoff
and keeps the setting alongside the selected external chat connection.
Automatic and manual repair use identical validation. The hosted mechanism
does not change local or autonomous repair policy. The accompanying
fresh-motion-request clarification separately replaces compact interactive
lifecycle wording; it does not grant an old or canceled repair permission to
restart motion.

## Local continuous-chat adaptation

The local follow-up separately extends the existing one-correction local policy
to interactive Layered and Creative v2. In Gemma trials, replaying rejected JSON
encouraged the model to repeat invalid choices or omit part of a compound
request. Local correction therefore regenerates from the original conversation,
contract and latest user request plus exact validator feedback; the bad draft
is retained in diagnostics but is not added to the next generation's context.
The schema and parser are unchanged between attempts. A successful first reply
still uses one call. Single-attempt retries and autonomous decisions cannot
enter this path, and provider errors, refusals, truncation and plain prose are
not retried.

The correction is automatic and uses the existing local policy; the external
setting and manual continuation remain external-only. Before making the extra
call, the shared budget preserves the full system/profile and original latest
request. If essential input does not fit, the original failure remains visible
and no extra call is made. Stop and cancellation prevent publication or motion
from the replacement. Local progress uses the same compact chat status, with
no pending manual continuation or new background work.

An expired correction requires a new user request against current state. This
is intentional: replaying a technically valid but stale score is not a repair
of the current conversation. The service does not claim that every rejected
response is recoverable or that successful schema validation proves subjective
motion quality. See the [installed-session and evaluation review](../hosted-response-repair-review-2026-10-23.md).
