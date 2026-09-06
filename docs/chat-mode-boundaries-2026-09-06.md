# Chat application and mode state boundaries — September 6, 2026

This implements the first two priorities from the
[architecture audit](architecture-review-2026-09-06.md#remaining-architectural-work).
The parent revision is `a44281be` (PR #253). It is a focused application boundary
and mode-state refactor, with one reproduced Autopilot coordination bug fixed
and additional canceled-admission coverage.

## Conversation application service

`internal/chatapp.Workspace` owns session lifecycle policy and interactive turn
admission through a persistence interface. Session handlers retain HTTP parsing,
controller checks and response encoding. Coherent status reads and deterministic
Stop history move into the same application boundary. Persona/mode mutations
retain their serialization through explicit scoped leases. The composition root
continues to own SQLite, model services, the mode manager and the shared engine.
See [ADR 0026](decisions/0026-conversation-application-boundary.md) for dependencies
and lock ordering.

Stop cancellation does not acquire the session gate or wait for a database read.
An epoch rejects admissions overtaken by Stop, and request/process cancellation
is rechecked after persistence returns. Cleanup is idempotent and releases only
its own turn. History is still recorded after physical Stop has completed or
timed out; a stale session ID never writes into the replacement conversation.

The HTTP chat file decreases from 1,375 to 1,272 lines. The larger gain is that
session policy and cancellation can now be tested without the HTTP server.
Model generation, streaming and speech coordination remain future use cases.
Datastore admission itself remains non-cancellable, as recorded in follow-up 3.

## Mode manager responsibilities

The manager now names seven state records explicitly: mode-loop lifecycle,
user controls, chat recovery, motion scheduling, speech scheduling, motion
history, and event status. All remain protected by the same `Manager.mu`, so
cross-record transitions retain their atomicity and no extra locks or loops
are introduced. Control handling, chat recovery and status move to focused
files; `manager.go` decreases from 1,297 to 766 lines.

Motion scheduling now has a pure work-selection function. It preserves the
existing retry, queued-boundary, planning, then motion/speech maintenance
priority. The manager executes the selected transition with the existing
generation checks. Pause clock shifts live with their state records and preserve
zero clocks and relative timing. Seeded random streams and their consumption
order, motion targets, samplers, curves and transport dispatch are unchanged.

The extraction reproduced a real ownership bug: after an old chat was canceled
and a new chat began, completion of the old request cleared the shared activity
boolean. Autopilot could then plan during the new reply. Activity IDs now require
the completing request to own the current activity before it can release planning.
The regression failed with the previous completion API and passes with this fix.

## Validation

- Full `go test ./...`, `go test -race ./...`, `go vet ./...`, golangci-lint
  v2.12.2, import boundaries, source budgets, and goleak gates pass locally.
- Pure-Go stripped Windows build with `CGO_ENABLED=0` passes. The frontend
  typecheck, 478 tests in 65 files, production build, and PowerShell installer
  suite pass. The canonical `web/dist` rebuild is identical; dependencies and
  all existing gate thresholds are unchanged.
- Seven new test functions cover session/Autopilot policy, replacement-turn
  ownership, Stop/request/shutdown during a blocked persistence read, stale
  Stop history, stale chat completion, scheduling priority at boundaries, and
  pause clock preservation. Existing control ordering, mode teardown, seeded
  cadence/sway and API regression suites remain green.
- An isolated current build at `http://127.0.0.1:49935` passed session create,
  save, activation and deletion. In-process simulator Freestyle start, pause,
  resume and Stop passed; it remained stopped after a 1.5-second observation.
  HTTP round trips were 3.24, 0.96, 2.26 and 1.13 ms respectively. These are
  simulator control observations, not hardware wire-latency measurements.
- The simulator trace contains 14 rows, eight successful `fake_handy` transport
  results, no dropped rows and no real-device calls. Local export:
  `.scratch/chat-mode-boundaries/simulator-trace.json`. Numerical motion behavior
  and seeded tests were retained; this change does not alter motion character
  or model-to-motion mapping, so no new pattern/proposal atlas was required.
- `scripts/check-review-llm.ps1` passed against the exact review process using
  local Ollama `huihui_ai/granite4.1-abliterated:3b`. The visible app chat returned
  **“Chat is ready.”**, one provider call, 99 ms request / 54 ms first token,
  with no repair, fallback or motion. LLM motion remains Off and the simulator
  is idle. The previous review process/tab at port 49931 is preserved.

The release binary grows 17,408 bytes to 19,097,600 bytes. See the
[performance observations](perf-baseline.md#2026-09-06--chat-and-mode-state-boundaries)
for all stopped-server samples and the existing RSS/startup limitations.
Scratch evidence is intentionally not committed. Hardware acceptance and the
other audit follow-ups remain open.
