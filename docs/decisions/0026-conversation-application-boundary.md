# ADR 0026: Conversation application lifecycle boundary

- Date: 2026-09-06
- Status: Proposed for review
- Builds on: ADR 0002 and the September 6 architecture audit

## Context

The HTTP server owns conversation admission, session mutation, request
cancellation, persona coordination, and transcript handling alongside unrelated
transport and settings work. Those lifetimes are difficult to test independently.
Moving only the mutexes to another HTTP file would retain the same ownership.
Stop must cancel pending work even while a session operation waits on storage.

## Decision

Introduce `internal/chatapp.Workspace`, an application service above the existing
`chat` persistence domain. It accepts a session persistence port, the process
lifetime, and a read-only Autopilot-status function. It owns canonical session
admission, session create/activate/save/delete policy, interactive turn ownership,
coherent chat status observations, and best-effort deterministic Stop history.
The process still owns and closes the shared datastore.

HTTP handlers decode requests, enforce authentication/controller policy, invoke
these use cases, and translate their results to the existing API. Generation,
SSE, model scheduling, and speech dispatch remain at their current boundaries
for subsequent focused changes. The new service does not own an engine or a
transport and does not generate device payloads.

The service's session gate serializes admission against conversation changes.
An explicit lease lets mode/persona transitions participate in that gate. When
both are needed, acquire the session lease before the persona mutation lock.
Mode transitions retain their existing internal control/lifecycle ordering.
Turn cancellation has a separate short lock that never spans persistence,
mode operations, or the session gate. Stop advances an admission epoch and
cancels the current turn without waiting for storage; an admission overtaken by
Stop is rejected when it returns from storage. Parent and process cancellation
are checked at admission, and accepted turns inherit both lifetimes.

Finishing a turn releases only that turn's ID. The mode manager likewise returns
an activity ID when chat postpones autonomous planning; only that ID can release
the activity. This prevents cleanup from a canceled request from releasing its
replacement. Physical Stop completes or times out before transcript recording.

Core domains cannot import `chatapp`; the import tests and depguard enforce this
direction. Existing bans on importing `httpapi` or dispatching transport from
semantic clients remain enforced. No new dependency, goroutine loop, or lock
around device Stop is introduced.

## Consequences

Conversation policy can now be tested without constructing an HTTP server,
model runtime, or engine. A persistence test double can deliberately stall a
read and prove that Stop remains independent of it. Existing API integration
tests continue to cover wire behavior and controller/persona contracts.

The [subsequent datastore/observation follow-up](../data-observation-review-2026-09-06.md)
adds request contexts to the persistence read port and cancellable datastore
writer admission. Preflight resolution, status reads, and turn admission link
the process lifetime before touching SQL. Durable mutations and their post-commit
session lists retain their existing lifetime. The session/persona coordination
leases still serialize accepted mutations; they are not cancellable mutex waits.
This remains the first application use case extracted from the HTTP server, not a claim that
all cross-domain orchestration has moved. The explicit mode/persona leases are
an incremental composition seam, not a public general-purpose locking API.

See [implementation and validation](../chat-mode-boundaries-2026-09-06.md).
