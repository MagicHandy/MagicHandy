# Datastore and observation follow-up — September 6, 2026

Parent: `56c49ec9`, the conversation/mode-state extraction in PR #254, itself
based on architecture audit PR #253. This implements audit priorities 3–5 and
checks the combined application boundaries again. The size guideline remains
advisory; ongoing file splitting is maintenance work, not another new subsystem.

## Implemented boundaries

**Cancellable writer admission.** `DB.WithTx` uses one capacity-one admission
channel instead of an uncancellable mutex. It checks context cancellation both
before waiting and after acquiring the writer; cancellation does not invoke a
queued callback or interrupt a different committing transaction. Existing
rollback-on-error/panic behavior remains, and the gate releases on every exit.
Two negative tests reproduced indefinite waiting behind a held writer on the
old implementation. Concurrent successful/failed writes still serialize and
commit only the intended rows.

Request-aware chat read methods live in `log_reads.go`; session status, history,
cursor and prompt preparation use request/turn contexts. Persona binding, lore,
prompt-set and memory reads follow that lifetime. Compatibility methods and
accepted durable settings/session/transcript mutations retain their existing
non-request lifetime. Database schema, shared writer ownership and dependencies
are unchanged. This does not convert every domain mutex into a cancellable gate.

**Separate browser subscriptions.** The app context now carries slow snapshots,
connection state and controller identity. Live backend motion has a separate
context/hook. The slow value is memoized, and Chat's visualizer/library playback
subscribe below their routes. Existing poll/event reconciliation, disabled-state
cleanup, permanent Stop and controller heartbeat behavior remain. The mode,
transport and frontend motion models are not duplicated. Per-tab SSE connections
and the backend work they perform remain; no browser leader election is added.

**Compact model status.** A count query and bounded import-job count replace
full inventory snapshots in `/api/state`. Only a selected managed model with
an installed runtime needs file validation. Empty/unknown selections keep the
inventory available and readiness false. Each selected-file check observes
current existence/size/mtime and uses the existing compatibility validation;
there is no new TTL or summary cache. The full inventory UI retains detailed
records, but closes SQL rows before filesystem inspection to free the pooled
connection. Model metadata parsing now receives the request context.

## Additional reproduced bugs and combined review

1. **Shutdown during chat pre-registration storage.** PR #254 linked the
   application lifetime only after active-session validation. A blocked read
   before registration could outlive shutdown. The new regression failed before
   correction. The lifetime now covers admission, preflight session resolution
   and observations; tests cover all three. Stop epochs, replacement-turn IDs,
   session/persona guards and the earlier stale Autopilot activity fix remain.
2. **Lost cancellation classification in inventory errors.** The inventory
   wrapper formatted the underlying error with `%v`, so `errors.Is` could not
   distinguish cancellation from a storage failure. Its failing regression now
   passes with both the inventory sentinel and cancellation cause preserved.

The review traced writer release/rollback, request versus durable lifetimes,
chat admission/Stop cleanup, selected-file invalidation, API status meanings,
subscription cleanup and the actual Chat/library consumer boundaries. It also
reruns the previous mode control-order, generation, shutdown, goleak and browser
reconciliation regressions. It is not a claim that all code or hardware scenarios
are defect-free. Motion targets, samplers, curves, seeded cadence and transport
payload construction are unchanged.

## Performance evidence

A React Profiler fixture contains 500 conversation rows and 500 library rows,
each consuming slow app state. Eighty separate
motion events caused **80 additional renders per slow view**, 80 commits and
270.54 ms of React render work before the split. Afterward the slow views have
**zero additional renders/commits**. The final regression also mounts a live
observer and verifies it receives all 80 updates.
This measures render propagation in jsdom, not a browser frame-rate guarantee;
ordinary two-second app polls still update slow consumers. The regression and
optional `VITE_PROFILE_SUBSCRIPTIONS=1` output are in
`app-state.subscriptions.test.tsx`.

Same-process Windows/amd64 model benchmark, 128 small valid managed GGUF files,
three runs, Go 1.26.4:

| Status method | Time/read | Bytes/read | Allocations/read |
| --- | ---: | ---: | ---: |
| Full inventory snapshot | 2.46–4.87 ms | 288,035–329,022 | 2,883–2,903 |
| Selected-model summary | 41.5–41.9 microseconds | 4,558–4,560 | 101 |
| Counts only | 5.10–5.20 microseconds | 972–973 | 24 |

Against the two warmer full-inventory runs, selected status is about 59 times
faster and allocates about 98.4% fewer bytes. The first full run includes colder
inspection work. These are query measurements on local storage, not end-to-end
model latency or a slower-disk benchmark. No new cache policy depends on them.
Selected same-size replacement with changed mtime, restoration, deletion,
unknown selection and inventory mutation have separate regressions.

The stripped binary and browser asset deltas, startup observations and memory
limits are recorded in [the performance baseline](perf-baseline.md#2026-09-06--datastore-and-observation-efficiency).

## Validation and review app

Full Go tests, race tests, vet, golangci-lint v2.12.2, import/size/goleak gates,
pure-Go build, frontend typecheck/build, all **479 frontend tests in 66 files**,
and the PowerShell installer suite pass locally. Race checks for chatapp and
HTTP are repeated after the final preflight/status lifetime extension. Existing
gate thresholds and dependencies are unchanged; only canonical `web/dist` ships.

The isolated review app is at `http://127.0.0.1:49939/#/chat`, using the simulator
and local Ollama `huihui_ai/granite4.1-abliterated:3b`. The real provider readiness
probe passes. Visible text-only chat returned **“Chat is ready.”** with one
provider call, no repair/fallback or motion (95 ms request, 52 ms first token
in the initial acceptance run). LLM motion is Off.

Session create/save/activate/delete and simulator Freestyle start/pause/resume/
Stop pass. Their HTTP round trips were 3.78/0.98/1.94/1.26 ms; motion stayed
stopped after a 1.5-second observation. A second run through the actual UI
confirmed live motion in the status-bar and Chat visualizers, and the permanent
Stop returned both to idle. The combined UI trace contains 33 rows, 23 successful
`fake_handy` results and zero drops. Trace exports and probe/profile/build logs remain
under ignored `.scratch/data-observation/`. All transport results are simulator
results; no hardware acceptance or motion-character change is claimed.
