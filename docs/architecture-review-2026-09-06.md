# Architecture and lifecycle review — September 6, 2026

Reviewed baseline: `df3d4a73` on `main`. This review combines repository-wide
inventory and automated gates with manual tracing of the principal application
boundaries. It is not a claim that every line or every hardware scenario is
defect-free. The inventory contains 33 Go packages and 601 tracked Go/TypeScript
source and test files under `cmd`, `internal`, and `web/src` before these changes.

## Assessment

The shared semantic engine, transport interface, pure-Go datastore, external
voice/model workers, and single embedded frontend remain sound architectural
choices. Import-boundary tests cover those relationships. The main maintenance
risk is coordination accumulating at the HTTP edge and in the mode manager:
request admission, cancellation, persistence, model leases, and long-lived
loops have different lifetimes but often share a large function or object.
Passing baseline tests did not cover the cancellation gaps found below.

The correction keeps one engine and one backend source of truth. Mode startup
now makes admission checks visible and delegates initial run-state reset to a
focused helper. Browser observations are scoped to their subscription lifecycle.
The memory domain provides the compact read model the status endpoint needs.

## Coverage

| Boundary | Inspected paths and checks | Assessment |
| --- | --- | --- |
| Startup and HTTP orchestration | `cmd/magichandy`, `httpapi/server.go`, controller, shutdown, motion, chat, LLM scheduler | Shared shutdown/Stop epochs exist; canceled mode and model admission required fixes. The HTTP server remains the largest cross-domain coordinator. |
| Motion and autonomous modes | Engine admission/start/Stop, manager control gates and startup, transport command gate and Intiface queue | Existing single-engine and Stop contracts retained. New tests use fake engines and preserve the goleak gate. No sampler, curve, transport payload, or physical timing changes. |
| Chat and LLM | Canonical chat log/history, bounded continuous history, provider streaming and model inventory, scheduler | Bounded histories and serialized inference remain useful. Dead interactive work must not cancel live autonomous inference. Model inventory read cost remains a follow-up. |
| Persistence and content | Shared SQLite writer, settings snapshots, memory, pattern summaries, media catalog/scanner, persona APIs | One process-owned DB is retained. Memory status unnecessarily loaded every text record. Writer admission cancellation deserves a separate change. |
| Browser state and controls | App-state polling/SSE, API client, controller identity, permanent Stop, existing UI safety suite | Backend snapshots remain authoritative. Poll/subscription ownership and reconciliation needed regressions. Broad context updates remain a performance concern. |
| Voice workers | Supervisor Stop/shutdown and request admission, tracking, invalidation, retained audio | Separate worker lifecycle and bounded request/audio retention are preserved; baseline and final suites cover their contracts. No live audio-worker acceptance was performed. |
| Accounts and hygiene | Bounded Argon2 parameters, HTTP authentication boundary, redacted settings, ignore rules | No credential format or authentication policy change. SQLite ignore globs were missing despite the repository rule; no tracked DB/sidecars were found. This is not a penetration-test attestation. |
| Delivery | CI definitions, import/size gates, frontend embed, Windows installer checks, stripped build | Existing hard gates retained. Only the canonical `web/dist` is regenerated. No dependency added. |

## Confirmed findings and corrections

| Priority | Finding and consequence | Correction and evidence |
| --- | --- | --- |
| P1 | `Manager.Start` detached from an already-canceled request. A start canceled before admission, behind a user control, or during previous-loop teardown could still activate a persistent mode. | Check cancellation before admission, after waiting for control gates, and after draining the old loop. Three regressions failed on baseline and now pass; a fourth preserves the accepted mode's independent lifetime. Run-state initialization moves to `manager_start.go`. |
| P2 | The LLM coordinator registered/preempted before checking cancellation and could grant an available slot to a canceled caller. A dead interactive request could interrupt valid autonomous inference. | Check cancellation under the coordinator lock on every admission attempt, including wakeups. Remove waiter registration in that same cancellation branch. Two new regressions failed before the fix; existing priority/preemption/waiter-removal tests remain green. |
| P2 | An explicit browser refresh waiting on a poll could restart fetching after the provider was disabled. An aborted request could also occupy `inFlight` across immediate re-enabling. | Fence waiting refreshes by subscription lifecycle, clear the abandoned slot on cleanup, and reject superseded responses. Disabled/re-enabled lifecycle tests failed on baseline and now pass. |
| P2 | Queued callbacks from a closed motion EventSource could repopulate disabled state or clear a newer stream. | Ignore callbacks after that subscription closes. Regression covers both a late motion event and a late error. |
| P2 | `liveMotion` always took precedence over polling, despite the documented reconciliation contract. An old running event could mask a later stopped snapshot. | A completed poll supersedes events observed before that poll began; events received during the request are retained because their server ordering is unknown. Tests cover both cases. No motion state is synthesized in the browser. |
| P2 | The regular memory status poll loaded and allocated all private memory text only to count records. | Add `memory.Store.Summary(ctx)` using aggregate SQL; the HTTP path passes its request context. Mutation/global-switch and cancellation tests cover behavior; benchmark below measures the change. |
| P3 | The ignore file protected known data directories but did not cover SQLite files created elsewhere in the checkout. | Add `*.db`, `*.db-wal`, and `*.db-shm`. No runtime artifacts are committed. |

The unchanged-code baseline passed `go test ./...` and 473 frontend tests.
The newly added negative regressions were run before their corresponding fixes
and failed for the behaviors above. Final frontend coverage is 478 tests in 65
files. The backend adds eight regression tests and a comparative benchmark.

## Remaining architectural work

These are scoped follow-ups, not claims of additional reproduced hardware bugs.
They are recorded so this repair does not imply that the codebase's broader
maintenance debt is gone.

1. **P2 — Extract application orchestration from `httpapi.Server`.** It owns
   settings transitions, chat/session mutation, model scheduling, voice work,
   media synchronization, and engine admission. Move one use case at a time
   behind narrow application interfaces, beginning with chat lifecycle, while
   keeping HTTP decoding and responses at the edge. Preserve Stop ordering and
   the existing import rules; a wholesale rewrite would be hard to review.
   **Implemented first use case:** `chatapp.Workspace` now owns conversation
   session policy, admission/cancellation, observations and Stop history. Other
   domains remain follow-ups; see the [implementation review](chat-mode-boundaries-2026-09-06.md).
2. **P2 — Reduce the mode manager's state surface.** Even after the startup
   extraction it still combines user-control admission, chat keepalive, motion
   cadence, speech cadence, arc state, and variation history. Group lifecycle
   state by owner and separate scheduling decisions from state transitions.
   Existing control-order and teardown tests should accompany each extraction.
   **Implemented:** explicit state records, focused control/chat/status modules,
   pure work selection and owned pause-clock updates; all share the original
   manager mutex. The same follow-up fixes stale chat completion ownership.
3. **P2 — Make datastore writer admission cancellable.** `DB.WithTx(ctx)` waits
   on `writeMu.Lock()` before reaching `BeginTx(ctx)`. The context cannot end
   that wait. Several persistence APIs also replace caller lifetimes with
   `context.Background()`. A follow-up should introduce cancellable admission
   without losing one serialized writer and explicitly distinguish durable
   commits from cancelable request reads. Do not merely remove serialization.
4. **P2 — Isolate high-frequency browser subscriptions.** The 125 ms motion
   stream updates the same React context consumed for settings, controller
   status, and notifications. Per-tab streams also repeat backend snapshot
   work. Profile a large chat/library view, then consider separate motion and
   slower app-state subscriptions or selectors. Preserve backend authority;
   this review does not claim a measured frame-rate defect.
5. **P3 — Give model status a compact inventory query.** `llmState` obtains a
   full model-manager snapshot on each `/api/state` poll; `List` checks model
   files as it reads rows. A summary should count inventory/imports and inspect
   only the selected model, with explicit invalidation when files change.
   Measure with multiple installed models and a slower disk before choosing a
   cache policy.
6. **P3 — Split large files at behavioral boundaries as they change.** Baseline
   examples include settings (1,445 lines), HTTP voice (1,408), HTTP chat
   (1,375), mode manager (1,324), chat service (1,280), and synchronized video
   UI (1,144). File length is a warning about responsibility and lock scope,
   not itself a bug. No advisory or hard threshold is changed here.

## Validation and performance

The memory benchmark uses 200 entries with 2,000 ASCII characters each and the
same process/toolchain for both read methods. Three runs on Windows/amd64,
Ryzen 9 9950X3D:

| Read method | Time per read | Allocated bytes/read | Allocations/read |
| --- | --- | --- | --- |
| Previous full snapshot | 467–505 microseconds | 475,987–475,992 | 1,463 |
| Aggregate summary | 33.2–34.4 microseconds | 1,232 | 36 |

That is about 14 times faster and 99.7% fewer allocated bytes for this saturated
memory fixture. It is not a claim of a 14-times-faster whole application. Query
results, including item-enabled counts while the global switch is off, retain
their existing meanings.

Same-toolchain stripped binary and stopped-simulator observations are recorded
in [the performance baseline](perf-baseline.md#2026-09-06--architecture-and-lifecycle-audit).
The pure-Go binary grows by 3,584 bytes. The established SQLite RSS waiver and
cold-start risk remain; this change does not redefine their budgets.

Required checks: Go tests, race tests, vet, golangci-lint v2.12.2, import
boundaries/goleak, TypeScript, frontend tests/build, pure-Go build, and Windows
PowerShell installer tests. Local logs and scratch fixtures stay under
`.scratch/architecture-audit/` and out of Git. CI rechecks the pushed revision.

## Review app and limits

The isolated review app uses the in-process motion simulator and a local
Ollama model, `huihui_ai/granite4.1-abliterated:3b`. LLM motion is Off. The
required `scripts/check-review-llm.ps1` real-generation probe passes. A
text-only request through the actual Chat UI returns `Chat is ready.` with one
provider call and no repair/fallback; motion remains idle. The final handoff
URL is `http://127.0.0.1:49931/#/chat`. The final-build chat turn took 1,359 ms
overall (1,307 ms to first token; 1,358 ms provider generation). These are local
model observations, not transport latency. The browser shows the reply, Motion
simulator, core ok, idle motion, LLM motion Off, and the permanent Stop.

No user app data or credentials were copied. No physical device was connected
or commanded. The cancellation scenarios use fake engines; the stopped review
trace is retained locally. Sampler/sanitizer behavior, motion character,
transport mapping, and Stop delivery semantics are intentionally unchanged.
Physical transport latency and feel are not validated by this work.
