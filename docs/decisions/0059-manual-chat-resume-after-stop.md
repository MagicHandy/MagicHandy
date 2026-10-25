# ADR 0059: Fresh manual chat after Stop

Status: implemented, 2026-10-25.

An earlier Stop cancels older work; it must not prevent a new motion request.
The Handy already supports this lifecycle. Vibrator Emergency Stop also resets
live intensity to zero, which previously prevented subsequent model actions
from producing vibration.

## Decision

Keep Emergency Stop's immediate zero and protocol Stop behavior. Remember the
previous human-selected vibrator intensity privately on the same selected owner.
A fresh controller-admitted chat request with LLM motion enabled may capture an
opaque, one-use permission bound to Stop sequence, selection epoch, control
generation and level revision. It does not arm or play output by itself.

The normal model action decides whether the request asks for motion. Dispatch
restores the prior intensity, capped by the current ceiling, then uses the
shared engine start path. Questions, chat-only replies, vibration-off actions,
Autopilot and reviewers cannot restore it. No language classifier or model-selected
arming field is added. COYOTE strength is unchanged.

Another Stop invalidates the in-flight permission. Explicit intensity changes
(including zero when already zero), reconnection, reselection and faults clear
it. Failed dispatch rolls intensity back to zero without overwriting a newer
human control. Repair of the same admitted manual request may retain permission;
a pre-Stop or otherwise stale repair cannot acquire it.

The backend exposes the pending level for a short stopped-state explanation.
**Keep vibration off** explicitly sets zero and clears permission. The Add device
notice explains independent control: Advanced models, such as hosted ones,
support it in Layered/Creative v2; other models use movement-follow vibration
(capability is per model since the ADR 0058 amendment).

## Evidence

Transport/HTTP tests cover repeated cycles, every capability tier, questions,
background dispatch, cancellation, Stop during generation and at the writer
barrier, zero/reselection/failure, a lower ceiling, rollback, and fresh versus
revoked repair. Existing Handy lifecycle tests remain green.

Live GPT 6.1 Sol (low) passed 8/8 first-attempt checks through HTTP and simulated
Buttplug. Gemma 12B Intermediate initially confused old playing replies with the
stopped backend state. A short backend state message immediately before the new
request corrected that in mixed and vibrator-only sessions. It is limited to
eligible stopped Intermediate turns; Simple and Advanced are unchanged. The
Intermediate JSON reminder now agrees with its grouped-array contract.

The final Gemma run applied all four motion requests and kept all four questions
idle. Seven of eight turns completed on the first attempt; one geometry/center
conflict used the existing bounded repair successfully. The trial cue run was
8/8 first-attempt. These small samples are not reliability guarantees. GPT median
request time was 6,067 ms (4,346–9,915); Gemma was 881 ms (600–2,168, including
repair). The opt-in quality harness retains this first-attempt failure.

Evidence stays ignored in `.scratch/manual-vibration-resume-2026-10-25/`:
initial/revised captures, full SSE and backend traces, current-build text-only
chat check, and the visual atlas. The atlas retains 64 evaluated turns, including
failed selections and inert questions, with 22 distinct plots and two inspected
overview sheets. Detailed final Gemma base-mixture and GPT drift plots showed
smooth planned reversals, zero velocity/acceleration knot jumps, and wire points
following the same curves. This does not establish physical smoothness. Sampler,
curves, profile conversion, limits and transport cadence are unchanged. No real
hardware was connected.

Validation: full Go tests; race tests for chat, HTTP and transport; vet/lint;
997 frontend tests, localization/type checks and build; pure-Go Windows build.
