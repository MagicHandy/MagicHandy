# Contributing to MagicHandy (humans and agents)

This file is the shared floor for **everyone** who changes this repository —
people and AI agents alike, whatever editor or model they use. It applies to
every branch. `main` is the release line; changes reach it by pull request with
green CI and review. Different contributors bring different tastes in structure
and styling, and that is welcome — but the requirements below are the invariants
that keep MagicHandy safe, efficient, and maintainable, and they are not
negotiable per-branch.

If a change cannot meet a requirement here, that is a discussion to have in the
PR (and possibly a recorded decision), not a gate to quietly relax.

## 1. Device safety is the first requirement

MagicHandy drives real hardware. These hold for any change that touches motion,
transport, or the control UI:

- **One motion engine, one motion path.** Every motion source (chat, modes,
  freestyle, library/blocks, imported scripts) produces *semantic targets* for
  the shared engine. No feature gets its own private motion loop or bypasses the
  shared sampler/sanitizer (this is the recurring StrokeGPT failure — see
  `docs/risk-register.md` R14).
- **Transports implement the transport interface, nothing more.** A new device
  backend (Cloud REST, Browser Bluetooth, Intiface/Buttplug, …) is a *dispatch
  owner* behind the `transport` interface (`Stop`, `SetStrokeWindow`, `AddHSP`,
  `PlayHSP`, `Diagnostics`). It maps the engine's semantic 0–100 to the device
  at the boundary; it never reaches back into the engine or invents a second
  motion model (ADR 0002).
- **Stop always works.** Emergency Stop is always mounted, reachable by
  read-only and backend-offline clients, cancels in-flight work, and marks the
  engine stopped even if the transport stop call fails. It is never gated behind
  a route, an overlay, or the active controller.
- **The goroutine-lifecycle gate is mandatory.** Motion/transport packages keep
  their `goleak` `TestMain` and stop-teardown tests green. A goroutine that can
  command the device after Stop is a safety bug, not a nit.

## 2. Backend-authoritative UI

- The frontend renders backend snapshots and events. It must not keep a parallel
  model of motion, controller ownership, settings, or transport state, and must
  never construct raw device/transport payloads.
- Follow `docs/ui-design.md` and `docs/ui-design-guidelines.md`: status-only top
  bar with compact readouts (no oversized round "pills"), immediate-apply live
  controls, honest commanded-estimate labeling, one steel-azure interactive hue,
  green only for go/running, red only for Stop, and `prefers-reduced-motion`
  respected. No purple or blue-green decorative tones, no glow effects.

## 3. Keep the core lean (the reason this is a Go rewrite)

The rewrite exists to be efficient and shippable. Protect that:

- **Pure-Go core, `CGO_ENABLED=0`.** No CGo dependency in `internal/` paths.
  Native-only needs (BLE, native audio) live behind the browser bridge or a
  worker process, never in the core binary. New third-party dependencies must be
  pure-Go, licensed compatibly (GPL-3.0-only project), and justified in the PR.
- **Budgets are real.** Idle/active RSS, binary size, and cold start are tracked
  in `docs/goal-scorecard.md`. A change that adds weight — Go dependency,
  browser payload, embedded asset — re-measures the affected budget and records
  it in the same PR. Browser-side weight counts: keep the shipped UI bundle lean
  and make heavy UI features (large component trees, big i18n tables, canvases)
  earn their cost.

## 4. Maintainability

- **Split before you exceed the size norm.** The soft cap is ~800 lines per core
  source file. When a file grows past it, **split it**; raising a file's
  line-budget override is a last resort that needs a stated reason and reviewer
  sign-off, not a default way to make the check pass.
- **Import boundaries are enforced** (`depguard` + `internal/architecture`
  tests): `chat`/`llm`/`modes` never import `transport`; `motion` depends on the
  transport interface, not internals; nothing depends on `httpapi`. Keep them
  green.
- **No god-modules.** Packages match the target architecture in
  `IMPLEMENTATION_PLAN.md`. One struct owning unrelated state is the failure mode
  this project is rewriting away from.

## 5. Repository hygiene

- **Never commit generated or runtime files:** SQLite databases and their
  sidecars (`*.db`, `*.db-wal`, `*.db-shm`), logs, traces, `node_modules`,
  build/tool caches, `.scratch/`, or local data directories. Add them to
  `.gitignore`; if one was committed by accident, remove it (it is runtime state,
  not source).
- **Commit build output only where the project deliberately embeds it** — the
  single shipping UI `dist` that the Go binary serves — and keep exactly one
  canonical copy. Do not accumulate stale hashed bundles or multiple parallel
  build directories.
- **One canonical frontend.** The binary embeds and serves one UI. Additional
  UI trees do not ship in parallel; retire or fold them rather than shipping two.
- **Secrets never land in git, logs, diagnostics, or exports.** The Handy
  connection key and any API keys are private credentials (redacted views only).
- **Large binary assets** (logos, images) live in one place, referenced — not
  duplicated across several directories.

## 6. CI and review

- **CI gates stay green and are not weakened to pass a change.** The gates are
  `gofmt`/`go vet`/`golangci-lint`, `go test ./...`, `go test -race ./...`, the
  `CGO_ENABLED=0` build, the import-boundary and line-budget tests, and the
  frontend build/test. If a gate blocks you, fix the code or record an explicit,
  reviewed exception — do not silently disable, downgrade, or skip a gate.
- **Docs move with the code.** A change to behavior or architecture updates the
  relevant doc in the same PR (`IMPLEMENTATION_PLAN.md`, ADRs under
  `docs/decisions/`, `docs/ui-design*.md`, `docs/risk-register.md`), and a change
  that moves a budget re-scores `docs/goal-scorecard.md`.
- **Scope per branch; merge by PR.** Use a clear branch name (agents/tools use
  their own prefix; feature branches are fine). Every branch — regardless of who
  or what authored it — meets this same bar before it merges to `main`.
- When real-device behavior is touched, capture the scenario, transport mode,
  latency summary, trace export, and what was intentionally left unchanged.

## 7. Where the details live

- `docs/goals-and-guardrails.md` — measurable targets and CI gates.
- `docs/decisions/` — architecture decisions (Go-first core, motion/transport
  contract, HSP scope, frontend strategy, SQLite, …). Add an ADR for a new
  cross-cutting decision instead of letting it calcify by accident.
- `docs/ui-design.md`, `docs/ui-design-guidelines.md` — UI safety and visual
  system.
- `docs/risk-register.md` — open risks and their mitigations.
- `docs/lso-merge-integration.md` — the plan for combining MagicHandy and LSO,
  and `docs/lso-merge-alternatives.md` for the open decisions.
- `IMPLEMENTATION_PLAN.md` — phases, status, and the target architecture.
