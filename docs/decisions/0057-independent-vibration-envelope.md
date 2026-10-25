# ADR 0057: Independent vibration in the shared engine

Status: implemented with automated/simulated acceptance, 2026-10-25. Physical vibrator acceptance remains open.

The owner requested independent model control of vibration alongside The Handy.
ADR 0047's conversion profiles cannot provide that: changing their source changes
the stroker too. Device-aware wording alone does not add independent control.

The existing motion engine owns a second semantic envelope on its existing
clock and lifecycle. A bounded vocabulary (follow, off, steady, wave, pulse)
defines relative drive within the intensity the human has armed. The engine
validates and samples it; AppendPoints carries those samples beside the linear
frame. The vibrator transport only interpolates, applies fade/quantization and
enforces the live intensity and ceiling. No model can set a device address, raw
command, safety limit, arming state or individual motor output. There is no new
motion goroutine or direct chat-to-transport path.

An envelope-only change does not retarget or resample the Handy's plan. Starting
vibration from idle starts a vibration-only run; the linked dispatcher must not
start or reposition an included but idle Handy. Starting linear playback later
is a new, ordinarily prepared run. Stop/Pause, loss of coverage and linked
failure policy still stop the appropriate outputs through the common lifecycle.

Missing vibration edits preserve the current envelope. Explicit follow returns
to the saved conversion profile; off silences vibration without stopping the
linear pattern. Stop always remains global. New starts without an envelope use
the saved follow profile. All motors in the selected vibrator still move together.

Model capability is evidence-driven. Compare awareness and independent control
on available local and external models using the real production parser, retain
all responses and accepted targets, and render compiled outputs. Local prompt
and schema changes are admitted only when they do not harm ordinary motion
requests. Device facts come from backend snapshots, never user claims.

Validation must cover mixed output isolation, vibration-only startup, Stop and
late-response races, arming/ceiling boundaries, malformed envelopes, coverage,
pause/resume, diagnostics, model latency and intent, and UI state. Physical
acceptance remains separate; no real vibrator is connected by this work.

The selected wire format keeps the existing linear action/edits and adds an
explicit nullable vibration object. GPT's grouped alternative did not produce
consistently better patterns. Local independent control remains disabled after
Gemma's misleading unarmed-output replies; local prompts/schemas stay unchanged.
The external minimum context is 64K, with the existing compact history cap of
64 messages/24 KB. Full guidance is retained; the limit does not pad requests.
See [evaluation, review and acceptance](../intiface-vibrator-review-2026-10-25.md).

Follow-up: [ADR 0058](0058-model-capability-levels.md) makes the model contract
an explicit local capability choice. Simple remains compact; Intermediate adds
tested guidance/awareness, and Advanced can opt into this independent envelope.
The tested 12B model is not recommended for Advanced. Human arming is unchanged.
