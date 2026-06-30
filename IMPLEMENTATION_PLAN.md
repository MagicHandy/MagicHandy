# MagicHandy Go Implementation Plan

## Core Direction

MagicHandy is a Go-first ground-up rewrite of StrokeGPT-ReVibed.

The rewrite is justified by maintainability, cleaner architecture, future binary releases, lower non-ML baseline overhead, simpler long-running concurrency, and fewer Python environment failures in the core install path. Go alone will not fix Handy cloud latency, local LLM memory, CUDA memory, or all motion smoothness bugs. Motion quality must come from a better motion model, transport scheduler, retargeting algorithm, diagnostics, and real-device validation.

Python may still exist behind optional worker boundaries for Chatterbox, faster-whisper, Parakeet, Torch, CUDA, or other ML-heavy features. Those dependencies should not define the core app install path.

Local LLM support is quality-first. The primary MagicHandy LLM path is a managed llama.cpp runtime for Windows/NVIDIA systems, using curated GGUF models and explicit model management. Ollama remains supported as the secondary pathway for cross-platform compatibility, users who already manage models through Ollama, and non-Windows/non-NVIDIA systems. See `docs/decisions/0005-local-llm-runtime.md` and `docs/model-management.md`.

## Rewrite Guardrails

- Do not start by porting every feature.
- Build a better motion and transport foundation first.
- Preserve hard-won StrokeGPT-ReVibed HSP constraints as tests.
- Keep semantic motion intent separate from physical transport output.
- Keep modes as clients of the motion engine, not alternate motion engines.
- Make real-device validation a first-class milestone.
- Keep the Go core sidecar-compatible even if the end goal is a full Go app.
- Define parity and kill milestones so the parallel rewrite does not run forever.
- Track the rewrite goals as measurable targets, not claims: memory, binary
  size, and startup are budgeted and checked (see `docs/goals-and-guardrails.md`).
- Keep the core pure-Go (`CGO_ENABLED=0`); native-only needs such as BLE or
  native audio stay behind the browser bridge or a worker, never in the core.
- Enforce maintainability in CI from Phase 1 (lint, import boundaries, size
  norms) so Go does not grow its own god-modules.
- Rebuild the frontend fresh and minimal instead of porting the old JS (see
  `docs/decisions/0004-frontend-strategy.md`).
- Treat the motion goroutine lifecycle as a safety gate (leak and stop-teardown
  tests), because a goroutine that commands the device after stop is unsafe.

## Goal-Ready Phase Workflow

Each phase below is written so a future `/goal` can complete it end-to-end. A phase should end with:

- code committed and pushed to a scoped branch
- tests passing for the phase
- documentation updated when behavior or architecture changes
- a PR opened unless the phase is explicitly local-only planning
- clear notes about what was intentionally not implemented

Use branch prefix `codex/` unless a different branch is requested.

## Target Architecture

```text
MagicHandy/
  cmd/magichandy/          app entrypoint
  internal/config/         settings, migrations, defaults
  internal/httpapi/        REST, SSE, and WebSocket routes
  internal/chat/           chat sessions, streaming, malformed response handling
  internal/llm/            provider interface, llama.cpp runner, Ollama adapter, prompts, JSON repair
  internal/motion/         semantic targets, pattern engine, sampler, retargeting
  internal/transport/      Handy cloud REST + browser Bluetooth, HSP-only, bridge contract
  internal/modes/          freestyle, auto, edging, milking successors
  internal/diagnostics/    traces, setup checks, latency probes, bug-report bundles
  internal/audio/          voice-output queue, external TTS worker client
  internal/asr/            voice-input worker client
  internal/workers/        external worker lifecycle and protocol helpers
  web/                     frontend assets
  docs/
  scripts/
```

# Phase 0: Planning Specs And Risk Register

## Suggested `/goal`

`/goal Complete MagicHandy Phase 0: write the decision records, measurable goals/guardrails, risk register, HSP invariants, motion retargeting spec, Bluetooth ownership decision, frontend strategy/UI design, local LLM runtime strategy, legacy motion-path removal decision, and worker boundary spec. Do not implement app code yet.`

## Objective

Capture the architectural decisions and hard-won constraints before implementation starts, so the rewrite does not rediscover StrokeGPT-ReVibed's known failure modes.

## Scope

Create planning docs only.

Required docs:

- `docs/decisions/0001-go-first-core.md`
- `docs/decisions/0002-motion-transport-contract.md`
- `docs/decisions/0003-voice-worker-boundary.md`
- `docs/decisions/0004-frontend-strategy.md`
- `docs/decisions/0005-local-llm-runtime.md`
- `docs/decisions/0006-drop-legacy-motion.md`
- `docs/goals-and-guardrails.md`
- `docs/ui-design.md`
- `docs/motion-retargeting.md`
- `docs/hsp-v4-invariants.md`
- `docs/bluetooth-ownership.md`
- `docs/risk-register.md`
- `docs/model-management.md`

## Required Content

`0001-go-first-core.md`:

- why Go-first
- why not Rust-first for the whole app
- what Go should improve
- what Go will not improve
- why optional Python workers are acceptable

`0002-motion-transport-contract.md`:

- semantic intent vs physical transport
- speed intent vs physical velocity/timed spacing
- stroke range as physical envelope
- reverse direction at transport boundary
- emergency stop contract

`0003-voice-worker-boundary.md`:

- worker protocol versioning
- worker lifecycle
- missing-worker behavior
- cancellation and timeout expectations

`0004-frontend-strategy.md`:

- fresh frontend strategy rather than porting old JavaScript wholesale
- backend-state-driven visualizer and motion UI
- no global mutable client god-registry
- command/state model and active-controller/read-only-client rules
- no required runtime build step for users

`0005-local-llm-runtime.md`:

- llama.cpp as the quality-first Windows/NVIDIA local LLM path
- Ollama as the secondary cross-platform compatibility path
- external `llama-server` process instead of CGo/libllama in the core
- provider contract, runner lifecycle, and explicit model management
- no surprise model downloads or hidden runtime fallback

`0006-drop-legacy-motion.md`:

- HSP-only transport family for MagicHandy
- Cloud REST and browser Bluetooth as dispatch owners, not separate motion engines
- no HAMP, HDSP, firmware v3, or finite-position fallback backend
- no Legacy Auto / scripted Edge / scripted Milk ports
- clear unsupported-device and HSP-unavailable behavior

`goals-and-guardrails.md`:

- measurable memory, binary, startup, and packaging targets
- baseline measurement procedure
- pure-Go core rule (`CGO_ENABLED=0`)
- CI gates and lint/import-boundary expectations
- motion goroutine lifecycle safety gate

`ui-design.md`:

- persistent Stop and device-state bar
- single authoritative visualizer
- immediate-apply quick controls
- routed settings/navigation model
- accessibility and responsive layout rules
- explicit old-app UI flaws to avoid

`motion-retargeting.md`:

- active stream representation
- required future buffer lead
- command-latency compensation
- handoff time selection
- phase preservation
- avoiding hard resets
- avoiding stationary bridge holds
- avoiding direction-opposing handoffs
- starvation/paused recovery

`hsp-v4-invariants.md`:

- HSP points are `0..100`, not `0..1000`
- do not pre-apply local stroke-depth calibration to every HSP point
- stroke range uses transport stroke-window command
- reverse direction happens at transport boundary
- do not rewrite semantic speed intent into physical speed feedback
- HSP timed-point spacing is the speed contract
- same-pattern updates preserve phase
- new-pattern replacements choose low-jump handoff phase
- HSP unavailable is a clear error; no fallback transport exists (see ADR 0006)
- active speed/stroke/direction settings refresh active motion immediately

`bluetooth-ownership.md`:

- compare browser-owned Web Bluetooth vs native Go Bluetooth
- default to browser-owned BLE bridge for early implementation
- define what the Go server owns and what the browser owns

`risk-register.md`:

- real-device validation risk
- two-codebase drift
- Bluetooth implementation risk
- user migration risk
- feature parity risk
- packaging/signing risk
- unmeasured rewrite-goal risk
- frontend debt carryover risk
- llama.cpp runner and model-management risk
- per-source motion path divergence risk
- chat/voice delivery ordering risk
- firmware v4 / API v3-only support risk

`model-management.md`:

- curated model catalog and GGUF metadata
- explicit download/import/load/unload flow
- llama.cpp runner compatibility and hardware-fit checks
- Ollama model handling as secondary provider support
- disk usage, checksums, licenses, and failure recovery

## Validation

- Markdown files exist and are internally consistent.
- No app implementation is started.
- `README.md` links to the new planning docs if the repo has a README by then.

## Done Criteria

- All required docs exist.
- The docs explicitly distinguish rewrite motivations from motion-smoothness fixes.
- Rewrite goals are measurable and have a baseline/perf evidence path.
- The frontend strategy avoids carrying over old JS debt by default.
- The HSP invariants are concrete enough to become tests in Phase 2.
- The retargeting spec is concrete enough to guide Phase 4.

## Out Of Scope

- Go module scaffolding.
- UI implementation.
- real Handy API calls.
- voice workers.

# Phase 1: Repo Scaffold And App Shell

## Suggested `/goal`

`/goal Complete MagicHandy Phase 1: create the Go module, GPLv3 repo scaffold, CI, embedded single-page app shell, health endpoint, structured logging, and baseline tests.`

## Objective

Create a buildable, testable Go application skeleton that can be packaged later and can serve a minimal browser UI.

## Scope

Implement:

- Go module
- GPLv3 license
- README
- `.gitignore`
- basic app entrypoint in `cmd/magichandy`
- embedded static assets in `web/`
- HTTP server with health/status route
- structured logging
- graceful shutdown
- basic CI with the gates in `docs/goals-and-guardrails.md` (`go vet`,
  `golangci-lint` incl. `staticcheck`/`gocyclo`/`funlen`/`depguard`, `go test`,
  `go test -race`, and a `CGO_ENABLED=0` build)
- `.golangci.yml` with explicit thresholds and depguard boundaries, even if
  early thresholds are intentionally lenient
- import-boundary scaffold (depguard/test for motion/transport/llm/httpapi rules)
- goroutine-leak test harness (`go.uber.org/goleak`) ready for motion/transport
- record the StrokeGPT-ReVibed memory baseline and the Go core idle RSS in
  `docs/perf-baseline.md` using the procedure in `docs/goals-and-guardrails.md`

Suggested initial layout:

```text
cmd/magichandy/main.go
internal/httpapi/server.go
internal/logging/logging.go
web/index.html
web/app.css
web/app.js
.github/workflows/test.yml
```

## Validation

Run locally:

```powershell
go test ./...
go test -race ./...
go run ./cmd/magichandy
```

Manual check:

- open local app URL
- confirm page loads
- confirm `/healthz` or equivalent returns OK

## Done Criteria

- App starts and serves the embedded UI.
- Health endpoint works.
- CI runs `go test ./...` and `go test -race ./...`.
- CI also runs `go vet`, `golangci-lint`, and a `CGO_ENABLED=0` build.
- `.golangci.yml` exists and encodes the initial import-boundary and size norms.
- The StrokeGPT-ReVibed memory baseline and the Go core idle RSS are recorded
  (see `docs/goals-and-guardrails.md`).
- README explains how to run from source.
- No motion, Handy, chat, or voice feature is faked in the UI beyond placeholder status.

## Out Of Scope

- settings persistence
- real Handy transport
- motion engine
- local LLM chat
- voice workers

# Phase 2: Config, Settings, And App State

## Suggested `/goal`

`/goal Complete MagicHandy Phase 2: implement versioned settings, settings API, app state snapshot, settings tests, and a minimal settings UI without adding motion or chat behavior.`

## Objective

Establish durable settings and state foundations before motion and transport are added.

## Scope

Implement:

- settings struct with version field
- default settings
- JSON load/save
- migration hook structure
- app data directory selection
- settings API routes
- app state snapshot route
- minimal settings UI for core fields

Initial settings should include:

- server port
- HSP dispatch owner placeholder (`cloud_rest` first, `browser_bluetooth` later)
- firmware v4 / API v3 requirement state, not a v3/v4 selector
- API v3 Application ID source (`bundled_app_id` by default, optional developer override)
- Handy connection key field
- motion speed min/max
- stroke range min/max
- reverse direction
- diagnostics verbosity

## Validation

```powershell
go test ./...
go test -race ./...
go run ./cmd/magichandy
```

Manual check:

- save settings
- restart app
- confirm settings persist
- corrupt/missing settings file recovers safely

## Done Criteria

- Settings load from disk and save atomically.
- Defaults are applied for missing fields.
- Migrations have a testable structure even if only v1 exists.
- UI can view/save the initial settings.
- Secrets are not printed in logs or diagnostics.

## Out Of Scope

- Handy connection attempts
- motion commands
- local LLM providers
- voice
- migration from StrokeGPT-ReVibed settings

# Phase 3: Transport Interface, Fake Handy, And Trace Schema

## Suggested `/goal`

`/goal Complete MagicHandy Phase 3: define the transport interface, fake Handy simulator, transport diagnostics, motion trace schema, trace export, and golden tests for command shape. Do not call the real Handy API yet.`

## Objective

Build deterministic transport and trace foundations before real device integration.

## Scope

Implement:

- `internal/transport` interface
- fake transport implementation
- command result type
- transport diagnostics snapshot
- motion trace row schema
- trace ring buffer
- trace export endpoint
- golden tests for command shape

Suggested types:

- `Transport`
- `CommandResult`
- `TransportDiagnostics`
- `TimedPoint`
- `MotionTraceRow`

## Validation

```powershell
go test ./...
go test -race ./...
```

Tests should cover:

- fake command recording
- command result diagnostics
- trace row serialization
- trace ring capacity
- no secret leakage in diagnostics

## Done Criteria

- Fake transport can record stop, stroke-window, and HSP add/play-like commands.
- Trace export returns stable JSON.
- Golden tests make transport command schema explicit.
- No real network calls exist in this phase.

## Out Of Scope

- real Handy API
- motion sampler
- UI visualizer beyond trace/diagnostic display if convenient

# Phase 4: HSP v4 Invariant Tests And Cloud Command Shaping

## Suggested `/goal`

`/goal Complete MagicHandy Phase 4: add HSP v4 invariant tests and command-shaping code for Handy cloud REST without sending real API calls by default.`

## Objective

Convert known HSP v4 landmines into executable tests before implementing live transport.

## Scope

Implement command builders for:

- firmware v4/API v3 auth metadata
- HSP stroke-window command
- HSP timed-point add/play command shape
- HSP stop/resume/sync placeholders if planned

Port invariant tests from `docs/hsp-v4-invariants.md`.

## Validation

```powershell
go test ./...
go test -race ./...
```

Tests must prove:

- HSP `x` values remain `0..100`
- stroke-depth settings are not baked into every point
- reverse orientation maps at transport boundary
- HSP unavailable is reported as a clear error; no fallback transport exists
- speed intent is not fed back as physical velocity
- command payloads omit secrets in trace rows

## Done Criteria

- HSP command builders exist and are fully tested.
- No default live network calls occur.
- The code refuses invalid v4/API v3 prerequisites with clear diagnostics.

## Out Of Scope

- live Handy connection
- long-running motion loop
- chat/modes

# Phase 5: Real Handy Cloud Transport

## Suggested `/goal`

`/goal Complete MagicHandy Phase 5: implement real Handy cloud REST transport, connection checks, HSP state/SSE listener where available, diagnostics, and manual transport test endpoints.`

## Objective

Make the app able to talk to a real Handy through Cloud REST, with honest diagnostics and no silent fallback behavior.

## Scope

Implement:

- API v3 Application ID handling through a bundled public app identifier, with an optional developer override
- Handy connection key handling
- firmware v4 / API v3 requirement checks
- connection check endpoint
- real command dispatch client
- HSP add/play/stop/stroke-window calls
- HSP state/SSE listener if available and practical
- latency measurement
- command history diagnostics

## Validation

```powershell
go test ./...
go test -race ./...
```

Manual real-device checklist:

- invalid connection key reports a connection-key problem
- missing/invalid bundled or override Application ID reports an API auth problem
- ordinary users are not asked for an Application ID unless they choose a developer override
- valid app identifier plus valid connection key connects
- stop command works
- stroke-window command works
- basic HSP timed points move device
- diagnostics show last command path/status/elapsed/error

## Done Criteria

- Cloud REST HSP dispatch works with the app Application ID and user connection key.
- Device/API errors are visible and specific.
- No secret values are logged or exported.
- Emergency stop works through the real transport.

## Out Of Scope

- full motion engine
- LLM chat
- modes
- Bluetooth

# Phase 5B: Browser Bluetooth HSP Dispatch Owner

## Suggested `/goal`

`/goal Complete MagicHandy Phase 5B: implement browser-owned Web Bluetooth as a second HSP dispatch owner, using the same transport interface, command schema, diagnostics, no-cloud-fallback behavior, and stop contract as Cloud REST.`

## Objective

Add the local Bluetooth dispatch owner without creating another motion backend or bypassing the HSP-only transport contract.

## Scope

Implement:

- browser-owned Web Bluetooth bridge protocol
- HSP command dispatch over the browser bridge
- bridge connection/status events
- stale-tab detection
- explicit no-cloud-fallback behavior while Bluetooth is selected
- diagnostics parity with Cloud REST command results
- UI visibility rules from `docs/bluetooth-ownership.md`

## Validation

```powershell
go test ./...
go test -race ./...
```

Manual real-device checklist:

- Bluetooth controls are hidden until the dispatch owner is enabled/selected
- browser permission flow is user-driven
- disconnect/stale-tab state is visible
- HSP timed points move the device over Bluetooth
- emergency stop works over Bluetooth
- when Bluetooth is selected and unavailable, Cloud REST is not used silently

## Done Criteria

- Browser Bluetooth is a dispatch owner for the same HSP command family as Cloud REST.
- Bluetooth does not introduce a second sampler, backend, or fallback motion path.
- Diagnostics make bridge, browser, and device failures distinguishable.

## Out Of Scope

- native Go Bluetooth
- HAMP/HDSP/Flexible Position over Bluetooth
- making Bluetooth the default before Cloud REST and diagnostics are stable

# Phase 6: Motion Engine MVP

## Suggested `/goal`

`/goal Complete MagicHandy Phase 6: implement the motion engine MVP with semantic targets, continuous plans, sampler, long-lived motion loop, stop, settings refresh, fake transport playback, traces, tests, race tests, and soak tests.`

## Objective

Build the core motion engine independently from LLM chat and modes.

## Scope

Implement:

- `MotionTarget`
- `MotionPlan`
- `ActiveMotionState`
- continuous sampler
- fixed pattern support
- area focus support
- soft anchor loop support if not too large; otherwise document as Phase 7
- long-lived motion loop
- stop/cancel handling
- speed limit updates while moving
- stroke range updates while moving
- reverse direction updates while moving
- trace annotations for every applied target and settings refresh

## Validation

```powershell
go test ./...
go test -race ./...
```

Tests should cover:

- target clamping
- same-pattern phase preservation
- settings refresh while active
- stop interrupts active loop
- fake transport receives continuous points
- trace rows describe active motion
- no goroutine leaks in a short soak test

## Done Criteria

- Motion can run continuously against fake transport until stopped.
- Settings changes apply to active fake-transport motion immediately.
- No regular stop/start occurs in fake playback traces.
- Race tests pass.

## Out Of Scope

- real-device retarget proof
- local LLM providers
- UI beyond minimal manual controls if helpful

# Phase 7: Motion Retargeting And Real-Device Validation

## Suggested `/goal`

`/goal Complete MagicHandy Phase 7: implement motion retargeting per the spec and validate on real hardware with trace exports for area changes, speed changes, stroke changes, direction changes, same-pattern changes, and cross-pattern changes.`

## Objective

Prove the highest-risk motion behavior on real hardware before broad app features are built on top.

## Scope

Implement/refine:

- active stream retargeting
- latency-aware buffer lead
- handoff time selection
- phase-preserving same-pattern changes
- low-jump cross-pattern handoff
- starvation/paused recovery behavior
- real-device trace export workflow

## Validation

```powershell
go test ./...
go test -race ./...
```

Manual real-device checklist:

- manual continuous motion starts
- area focus changes while already moving
- speed limit changes while moving
- stroke range changes while moving
- reverse direction changes while moving
- same-pattern speed changes preserve phase
- cross-pattern retargets do not hard reset to a fixed position
- Cloud REST latency spike behavior is visible in diagnostics
- emergency stop works during retargets

## Done Criteria

- Active motion does not stop after routine retargets.
- Area changes do not jump to a hard reset position.
- Diagnostics explain transport/API failures.
- Failed real-device runs produce trace files that can become fixtures.
- Known unresolved motion limitations are documented.

## Out Of Scope

- LLM chat
- modes
- voice

# Phase 8: Minimal Frontend Motion UI And Visualizer

## Suggested `/goal`

`/goal Complete MagicHandy Phase 8: implement the minimal browser UI for device connection, manual motion controls, quick settings, emergency stop, diagnostics, trace export, and a visualizer driven by motion engine state.`

## Objective

Build a usable motion-control UI around the validated motion engine. Follow
`docs/ui-design.md` for layout, the single visualizer, the persistent Stop,
immediate-apply controls, feedback, and accessibility.

## Scope

Implement UI for:

- Handy credentials and connection status
- HSP dispatch owner selection/status (Cloud REST / browser Bluetooth) and firmware/API requirement status
- manual start/stop motion
- speed/stroke/direction quick settings
- emergency stop
- transport diagnostics
- trace export
- visualizer driven by engine state, not guessed UI state

## Validation

```powershell
go test ./...
go test -race ./...
```

Browser/manual checks:

- save settings
- connect/disconnect feedback is clear
- quick settings apply immediately to active motion
- stop button remains visible and works
- visualizer state matches engine trace reasonably
- no layout overlap on desktop and mobile widths

## Done Criteria

- User can control motion without chat.
- Quick settings change active motion immediately.
- Visualizer reads backend state.
- Diagnostics are visible enough for bug reports.

## Out Of Scope

- local LLM chat
- voice
- full settings parity

# Phase 9: Local LLM Provider Integration

## Suggested `/goal`

`/goal Complete MagicHandy Phase 9: implement the local LLM provider layer with llama.cpp as the primary Windows/NVIDIA path, Ollama as the secondary cross-platform path, streaming chat, strict JSON contract, repair pass, malformed-response UI indicator, prompt sets, and chat-driven motion through the motion engine only.`

## Objective

Add chat without letting the LLM bypass deterministic motion control. Prioritize response quality and runtime control by making managed llama.cpp the first-class local model path on Windows/NVIDIA systems, while keeping Ollama as the compatibility provider for users and platforms where llama.cpp packaging is not the right default.

## Scope

Implement:

- LLM provider interface shared by llama.cpp and Ollama
- managed llama.cpp `llama-server` runner for Windows/NVIDIA
- pinned runner metadata and compatibility checks
- GGUF model registry with curated quality-first defaults
- explicit model download/import/load/unload flow
- model size, license, checksum, context, quantization, and hardware-fit metadata
- OpenAI-compatible streaming chat client for llama.cpp
- Ollama adapter as the secondary provider path
- provider availability/status endpoint
- response schema
- JSON repair pass
- malformed response warning metadata
- prompt templates
- prompt set structure
- chat history
- chat-driven motion target application

## Validation

```powershell
go test ./...
go test -race ./...
```

Tests should cover:

- provider contract behavior for llama.cpp and Ollama adapters
- managed runner launch/health/error-state handling with a fake runner
- valid response parsing
- malformed response handling
- repair pass behavior
- `move: null` allowed for conversational turns
- malformed text remains visible with warning metadata
- chat motion calls only motion engine API
- no surprise model downloads during startup or provider status checks

Manual checks:

- llama.cpp unavailable is clear and actionable
- a supported Windows/NVIDIA llama.cpp setup can load a GGUF model and chat
- Ollama unavailable is clear when Ollama is selected
- valid Ollama model can chat as the secondary path
- malformed response shows visible warning instead of replacing the message entirely
- motion starts on first appropriate motion request

## Done Criteria

- Chat can drive motion through deterministic targets.
- llama.cpp is the primary documented local LLM path for Windows/NVIDIA.
- Ollama remains available as the secondary cross-platform provider.
- LLM cannot issue raw transport commands.
- Model downloads are explicit and user-confirmed.
- Malformed responses remain visible with a warning indicator.
- Model errors do not enter chat history as assistant dialogue.

## Out Of Scope

- long-term memory
- modes
- voice
- bundling every llama.cpp acceleration backend in the first pass

# Phase 10: Memory And Prompt Management

## Suggested `/goal`

`/goal Complete MagicHandy Phase 10: implement long-term memory, individual memory removal, prompt sets, prompt library UI, memory import/export basics, and tests.`

## Objective

Add model personalization and prompt management in a maintainable way.

## Scope

Implement:

- memory store
- enable/disable saved memories
- individual memory removal
- memory clear all
- prompt sets
- prompt set create/edit/delete/select
- prompt anatomy/persona fields if not already present
- UI for model/prompt/memory settings

## Validation

```powershell
go test ./...
go test -race ./...
```

Manual checks:

- add memory
- remove individual memory
- clear memories
- disable memories and verify prompt excludes them
- prompt set selection persists

## Done Criteria

- Memory is transparent and manageable.
- Prompt sets are editable without modifying code.
- Defaults are protected from accidental destructive edits.

## Out Of Scope

- voice
- advanced mode planners

# Phase 11: Modes As Motion Clients

## Suggested `/goal`

`/goal Complete MagicHandy Phase 11: implement Freestyle and normal chat continuous mode as clients of the motion engine, with traceable planner decisions and no separate motion pathway.`

## Objective

Introduce autonomous motion behavior without recreating the old split motion architecture.

## Scope

Implement:

- Freestyle MVP
- normal chat continuous keep-moving behavior
- planner decision type
- planner trace rows
- mode start/stop API
- mode UI controls
- mode feedback path if scoped

Rules:

- modes produce semantic targets/plans
- modes do not call transport directly
- modes do not replace streams every few seconds without retargeting safeguards
- no legacy morph behavior unless explicitly redesigned and validated

## Validation

```powershell
go test ./...
go test -race ./...
```

Manual checks:

- Freestyle starts and keeps moving
- chat mode keeps moving until stop
- mode changes are traceable
- stop interrupts modes
- settings changes apply during modes

## Done Criteria

- Modes use only the motion engine API.
- Continuous mode does not stop between chat turns.
- Freestyle does not hard reset on routine changes.
- Planner decisions are visible in diagnostics.

## Out Of Scope

- Edge/Milk legacy parity unless explicitly requested
- pattern library authoring UI
- voice

# Phase 12: Voice Worker Boundary

## Suggested `/goal`

`/goal Complete MagicHandy Phase 12: implement the optional voice worker protocol, worker lifecycle management, voice status UI, and a stub worker; do not bundle heavy ML models yet.`

## Objective

Add voice architecture without pulling Python ML dependency instability into the Go core.

## Scope

Implement:

- worker protocol version
- worker process lifecycle
- health/status messages
- request/response envelope
- cancellation
- timeout handling
- queue depth status
- crash status
- stub TTS worker
- stub ASR worker
- UI status for missing/unloaded workers

## Validation

```powershell
go test ./...
go test -race ./...
```

Manual checks:

- app runs without workers
- stub worker can be started/stopped
- worker crash is visible
- cancellation works for stub request

## Done Criteria

- Voice is optional.
- Worker protocol is versioned and tested.
- Core app remains usable without Python.

## Out Of Scope

- Chatterbox implementation
- faster-whisper implementation
- Parakeet implementation
- CUDA setup scripts

# Phase 13: Voice Feature Implementations

## Suggested `/goal`

`/goal Complete MagicHandy Phase 13: implement one real voice worker path at a time, starting with the lowest-risk provider, while keeping the core app functional without voice installed.`

## Objective

Add real voice capabilities incrementally behind the worker boundary.

## Scope

Pick one provider per PR/subphase:

- hosted/ElevenLabs-style TTS client if desired
- local Chatterbox worker
- faster-whisper worker
- Parakeet worker

Each provider must include:

- setup documentation
- load/unload behavior
- status diagnostics
- queue/cancellation behavior
- failure messages that do not crash the core app

## Validation

Provider-specific tests plus:

```powershell
go test ./...
go test -race ./...
```

Manual checks:

- missing dependency is reported clearly
- provider loads when installed
- generation/transcription can be cancelled
- queue depth is visible
- app still works when provider fails

## Done Criteria

- At least one real voice provider works behind the protocol.
- No Python ML dependency is required for the core app to start.

## Out Of Scope

- making all voice providers parity-complete in one phase

# Phase 14: Pattern Library, Programs, And Authoring

## Suggested `/goal`

`/goal Complete MagicHandy Phase 14: implement motion pattern library, program/funscript import, pattern playback through the motion engine, and a simplified pattern authoring UI with sane point simplification/interpolation.`

## Objective

Bring authored content into the new motion architecture without recreating the old pattern playback pitfalls.

## Scope

Implement:

- built-in pattern data format
- user pattern registry
- program/funscript registry
- import/export
- pattern playback through motion engine
- authoring canvas
- drawing simplification
- interpolation controls
- preview based on backend sampler

## Validation

```powershell
go test ./...
go test -race ./...
```

Manual checks:

- import pattern/program
- play pattern
- draw simple pattern
- simplification does not flatten pattern unexpectedly
- drawing is clipped or constrained visibly
- preview matches playback semantics

## Done Criteria

- Authored content routes through shared motion engine.
- Drawn patterns are sparse, editable, and interpolated.
- Programs are not confused with short loop patterns.

## Out Of Scope

- LLM-curated pattern sequencing unless explicitly included

# Phase 15: Migration From StrokeGPT-ReVibed

## Suggested `/goal`

`/goal Complete MagicHandy Phase 15: implement import tools for StrokeGPT-ReVibed settings, memories, prompt sets, motion patterns, and programs, with a compatibility report and tests.`

## Objective

Let users migrate without manually copying files or guessing what carried over.

## Scope

Implement import for:

- `my_settings.json`
- memories
- prompt sets
- motion patterns
- programs/funscripts
- selected assets if safe

Include:

- dry-run mode
- compatibility report
- unsupported-field report
- rollback or non-destructive import behavior

## Validation

```powershell
go test ./...
go test -race ./...
```

Manual checks:

- import sample old settings
- imported settings appear in UI
- unsupported fields are reported
- secrets are handled safely

## Done Criteria

- Migration is non-destructive.
- Users get a clear report.
- Tests cover representative old settings files.

## Out Of Scope

- exact behavioral parity for every legacy setting

# Phase 16: Packaging And Release Pipeline

## Suggested `/goal`

`/goal Complete MagicHandy Phase 16: create Windows release packaging, portable zip output, version metadata, release docs, and a decision record for signing and auto-update.`

## Objective

Make MagicHandy distributable as a core binary app.

## Scope

Implement:

- Windows binary build
- portable zip
- embedded web assets
- default config/data directory behavior
- version command/endpoint
- release GitHub Actions workflow
- release notes template
- signing decision doc
- auto-update decision doc
- worker bundle strategy doc

## Validation

```powershell
go test ./...
go test -race ./...
go build ./cmd/magichandy
```

Manual checks:

- unzip release
- run app from clean directory
- app creates config/data directories correctly
- no source checkout required

## Done Criteria

- A user can download and run the core app without Python.
- Release artifact includes license and README.
- Optional voice worker setup is documented separately.

## Out Of Scope

- production code signing unless credentials/process already exist
- auto-update implementation unless explicitly approved

# Phase 17: Parity Review And Default-App Decision

## Suggested `/goal`

`/goal Complete MagicHandy Phase 17: compare MagicHandy against StrokeGPT-ReVibed, document remaining gaps, run real-device and packaging checks, and recommend whether MagicHandy is ready to become the default app.`

## Objective

Make an explicit product decision instead of allowing the rewrite to drift indefinitely.

## Scope

Review:

- motion reliability
- transport diagnostics
- chat behavior
- mode behavior
- voice status
- settings coverage
- migration coverage
- packaging quality
- known gaps

Produce:

- `docs/parity-review.md`
- `docs/default-app-readiness.md`
- GitHub issues for remaining gaps
- recommendation: default, continue parallel, freeze, or backport/abandon

## Validation

Run full project validation plus real-device checklist.

## Done Criteria

- Gaps are explicit.
- Recommendation is concrete.
- If not ready, next milestones are clear.
- If ready, criteria for freezing StrokeGPT-ReVibed are documented.

## Out Of Scope

- fixing every gap discovered during review

# Cross-Phase Testing Requirements

Every implementation phase should run:

```powershell
go test ./...
go test -race ./...
```

Every phase also runs `go vet`, `golangci-lint`, and a `CGO_ENABLED=0` build of
the core binary (see `docs/goals-and-guardrails.md`).

When motion or transport behavior is touched, also run the goroutine-leak and
emergency-stop-teardown tests (the motion safety gate).

When frontend is touched, also run browser/UI tests once they exist.

When transport or motion behavior is touched, update trace/golden tests.

When real-device behavior is touched, capture:

- exact scenario
- transport mode
- firmware/API mode
- command latency summary
- trace export
- observed behavior
- what was intentionally not changed

# Parity And Kill Milestones

## Motion Core Milestone

MagicHandy motion becomes eligible to replace StrokeGPT-ReVibed motion when:

- manual motion works on real hardware
- area focus works while already moving
- settings changes apply immediately while moving
- no regular stop/go behavior occurs during retargets
- trace diagnostics explain failures

## App Default Milestone

MagicHandy becomes eligible as the recommended app when:

- chat plus motion works end-to-end
- settings import works
- emergency stop is reliable
- basic diagnostics are present
- packaging produces a usable Windows binary

## Freeze Decision

Once MagicHandy reaches the app default milestone, decide whether to:

- freeze StrokeGPT-ReVibed except for critical fixes
- continue both temporarily
- backport the motion core idea and abandon the full rewrite

# Implementation Rule

Do not start by porting every feature. Start with a better motion and transport foundation, preserve the hard-won HSP constraints as tests, validate on real hardware early, then make chat and modes call into the new core.
