# ADR 0040: Optional asynchronous review of local motion

Date: 2026-10-11

Status: Implemented on `codex/asynchronous-motion-review`; separate from the release line.

## Context

Local chat is responsive, but a valid motion edit can miss a compound request.
Autopilot can also drift from a continuing user direction. Making every local
choice wait for a cloud model would discard local inference's latency advantage.
The user requested review of both chat and Autopilot, with immediate local action.

## Decision

Add an off-by-default **Motion second opinion** role using an existing named
ChatGPT, OpenAI, OpenRouter, or compatible connection. Local chat and accepted
local Autopilot choices enter the existing engine before review is scheduled.
Hosted primary choices are not reviewed. Manual, media, library playback, and
Freestyle targets do not acquire review authority merely because chat is open.

The reviewer gets before/applied semantic state, saved limits, bounded motion
history, and up to eight recent user messages (the existing 2,000-character
per-message context bound). It does not receive personas, memories, assistant
replies, or autonomous speech. Message sharing is disclosed separately from
the primary Chat and Autopilot roles. Existing primary local prompts do not change.

One call returns `keep` or a high-confidence material correction in the selected
motion grammar. A preference for a different reasonable pattern is insufficient;
seed-only changes are also rejected deterministically. Output cannot start,
stop, resume, change global limits, alter a standing hold, or insert dialogue.
Direct semantic correction won the tested comparison with asking Gemma to act
on the review feedback. No prompt injection or second local generation is used.

The server owns one lazy worker and one replaceable pending job. New work cancels
and replaces an older review, and the deadline is ten seconds from admission.
There is no reviewer goroutine, call, or prompt suffix while disabled. The worker
uses the connection's existing request lane, leaving the local lane available.
Errors and refusals leave the accepted local motion running without repair/retry.

Applying a correction requires the same settings, credentials revision, controller
generation, control-command sequence, Stop epoch, active conversation revision,
personalization revision, mode generation, chat activity, segment, and engine
plan. The mode handoff cancels superseded planning, while the engine's existing
`ApplyTargetIfCurrent` performs the atomic plan check. Stop does not wait for the
reviewer's network call. Shutdown cancels and joins the worker before closing
credentials/storage. There is no second motion engine, sampler, or transport path.

Accepted corrections update keepalive and Autopilot's authoritative segment.
Autopilot records `second_opinion` as its decision source and retains its cadence,
with at least three seconds to observe the correction. It then sees the corrected
state on its next local planning turn. A correction does not review itself.

## Evidence and limits

See [the experiment](../motion-second-opinion-review-2026-10-11.md). This is an
intent-following aid, not a safety controller or a physical-comfort score.
Confidence is a conservative model category, not a calibrated probability.
The live comparison used GPT-6 Sol / Low and Gemma 12B, primarily Creative v2;
other providers use their existing adapters but have not all been live-benchmarked.

The UI stays compact: a switch, a reviewer selector, an explicit sharing sentence,
and collapsed implementation details. Results come from backend state and use
the existing dismissible notice system. Removing its connection turns review off.
