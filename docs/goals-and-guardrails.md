# Goals And Guardrails

## Purpose

The rewrite is justified by maintainability, lower non-ML baseline memory, and
shippable binary releases. Those goals are easy to claim and easy to lose. This
document makes them measurable and enforceable so the project can prove it met
its purpose instead of just "rewrote it in Go."

Go does not deliver these goals by itself. A 4,800-line `motion.go` is as
unmaintainable as the Python `motion.py` it replaces; a CGo dependency destroys
the single-binary story; and a long-running loop can hold memory under GC. The
targets and gates below exist because the language choice is necessary but not
sufficient.

## Measurable Goal Targets

### Memory

Phase 1 must record a baseline before targets are trusted. Capture, on the same
machine, for StrokeGPT-ReVibed:

- core idle RSS: app loaded, UI served, no model, no motion, no voice
- core active RSS: continuous motion running, no ML worker

Record the actual numbers in Phase 1 notes. The Go targets are starting points
to refine against that baseline (all exclude Ollama, llama.cpp runner processes, CUDA, TTS, and ASR, which
run as separate processes):

- core idle RSS: target < 40 MB
- core active RSS (continuous motion + transport + SSE + one chat session, no
  ML worker): target < 80 MB
- sustained motion (1 hour continuous): RSS stable within +20% of active
  baseline after warmup; no unbounded growth
- if sustained RSS drifts, set `GOMEMLIMIT` and/or tune `GOGC` rather than
  declaring the goal met by idle numbers alone

### Measurement Procedure

Record measurements in `docs/perf-baseline.md` rather than only in PR notes. Each row should include:

- date and commit
- OS and architecture
- command used to start the process
- whether the browser UI was opened
- whether child worker/model processes were excluded
- steady RSS after warmup
- optional peak RSS during startup or first motion
- notes for unusual local state

On Windows, prefer process RSS from the app process itself (`WorkingSet64`) and record child processes separately. Do not include Ollama, llama.cpp runner processes, CUDA model workers, TTS, ASR, browser, or test runners in the core number. Take at least three samples after warmup and record the steady range, not a single lucky low number.

Targets are budgets, not proof. If a phase exceeds a budget, either fix it or record the reason and a follow-up risk. Do not silently relax targets in the same PR that misses them.

Exit evidence: `docs/perf-baseline.md` with measured Python baseline and measured Go numbers per milestone.

### Binary

- single static binary with embedded web assets (no source checkout, no Node
  runtime, no Python for the core)
- binary size: target < 30 MB
- cold start to serving UI: target < 500 ms
- See Phase 16 for packaging; these targets are checked there.

### Cross-Platform Scope

Decide explicitly rather than defaulting to Windows-only:

- primary release: `windows/amd64`
- best-effort builds: `linux/amd64`, `darwin/arm64`, `darwin/amd64`
- cross-builds must stay free, which requires the pure-Go rule below

### Local LLM Runtime Strategy

Quality comes first for the main local model path:

- primary LLM pathway: managed llama.cpp for Windows/NVIDIA systems
- secondary LLM pathway: Ollama for cross-platform compatibility and users who already manage models externally
- the Go core talks to llama.cpp as an external runner, not through CGo/libllama
- model downloads are explicit user actions with visible size, license, checksum, and disk-use information
- startup/status checks must not trigger multi-GB model downloads
- provider fallback is visible state, not a silent mid-session switch

Measure model-runtime memory separately from core RSS. A lower Go core memory number is not evidence that a loaded GGUF, Ollama model, or CUDA context is smaller.

## Maintainability Guardrails

These are enforced in CI from Phase 1, not aspirational.

### CI gates (every PR)

- `gofmt`/`gofumpt` formatting check
- `go vet ./...`
- `golangci-lint run` (must include `staticcheck`, `gocyclo`/`funlen`,
  `depguard` for import boundaries)
- `go test ./...`
- `go test -race ./...`
- `CGO_ENABLED=0 go build ./cmd/magichandy`

### Pure-Go rule

- the core binary builds with `CGO_ENABLED=0`
- no CGo dependency in `internal/` core paths
- prefer pure-Go libraries (e.g., if a datastore is ever needed, a pure-Go one,
  not a CGo SQLite driver)
- anything that genuinely needs native code (BLE, native audio) lives behind the
  browser bridge or a worker process, never in the core binary

### Import boundaries (enforced, structural form of ADR 0002)

- `motion` may depend on a transport interface, not `transport` internals
- `modes` depend on the `motion` engine, not on `transport`
- `llm` and `chat` produce semantic targets only and must not import `transport`
- `httpapi` depends inward; nothing depends on `httpapi`
- enforce with `depguard` and/or a `go list`-based boundary test

### Size and complexity norms

- soft cap: no core source file over ~600-800 lines; split before exceeding
- no single struct acting as a god-object (the `motion.py` failure mode)
- function complexity within the configured `gocyclo`/`funlen` thresholds
- these surface as lint findings to track, not silent drift

### Frontend

The same size/no-god-module norms apply to `web/`. See
`docs/decisions/0004-frontend-strategy.md`.

## Safety Gate: Motion Goroutine Lifecycle

A leaked or zombie goroutine that keeps commanding the Handy after stop is a
physical-safety issue, not a memory nit. This gate is required, not advisory.

- exactly one goroutine owns device command dispatch for an active stream
- the motion loop is started and stopped through a `context.Context`
- emergency stop cancels that context, cancels/drains in-flight transport
  requests, and marks the engine stopped even if the transport stop call fails
  (see ADR 0002 Emergency Stop Contract)

Test expectation:

- motion and transport packages use `go.uber.org/goleak` (or equivalent) in
  `TestMain`
- a test asserts that emergency stop during an active retarget leaves zero
  active motion goroutines and either sends a transport stop or records a stop
  failure in diagnostics
- `go test -race` is green for these packages

Exit evidence: leak + stop-teardown tests exist and pass in CI before any phase
builds features on top of the motion loop.

## Relationship To Other Docs

- ADR 0001: why Go, and what Go will and will not improve
- ADR 0002: semantic/transport contract and emergency stop contract
- ADR 0004: frontend strategy
- `docs/goal-scorecard.md`: the living status of every target in this file —
  scored per phase with evidence links; budget misses are recorded there in
  the PR that misses them
- `docs/risk-register.md`: R11 (goals unmeasured) and R12 (frontend debt) track
  the risks these guardrails mitigate
