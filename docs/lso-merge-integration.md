# MagicHandy + LSO Integration Plan

## Status

Active, 2026-07-08. This plan describes combining **MagicHandy** and
**LSO (Local Stroke Orchestrator)** into one project. It is a living document;
the open decisions in the last section are tracked in
[lso-merge-alternatives.md](lso-merge-alternatives.md) and become ADRs as they
are settled.

## Context

The two projects are converging on the same goal — a local-first, LLM-driven
controller for The Handy — from different starting points:

- **MagicHandy** is the Go-first core established in this repo: a pure-Go
  (`CGO_ENABLED=0`) backend, a semantic motion engine with an HSP transport
  contract (Cloud REST + Browser Bluetooth), SQLite persistence, and a React
  UI built at build time and embedded in the binary. Its guiding constraints are
  efficiency (low RAM/CPU, small binary) and maintainability.
- **LSO** brings a broader feature set from its Python/TypeScript heritage:
  Intiface/Buttplug device support, a motion "block" library and player,
  personas, a richer component-driven UI, and multi-language localization.

The agreed direction is to build the merged product **on the MagicHandy Go
backend and architecture**, bringing LSO's capabilities onto it. Both teams'
priorities are kept: LSO's feature depth and MagicHandy's efficiency and safety
bar. Where those pull in different directions, features are adapted to fit the
budgets and invariants in [../AGENTS.md](../AGENTS.md), rather than the budgets
being relaxed to fit the features.

## How we collaborate

- `main` is the release line and holds the governing docs (this plan,
  `AGENTS.md`, the ADRs, the guardrails).
- Integration work happens on feature branches and reaches `main` by pull
  request with green CI and review. This lets contributors move fast in parallel
  without destabilizing `main`, and gives every change the same automated and
  human checks regardless of who or which tool wrote it.
- CI is the shared, impartial gate. It should grow to cover both stacks (Go core
  and the frontend), and its checks are strengthened, not weakened, as the
  surface grows (see `AGENTS.md` §6).

## Workstreams

Each item below is additive capability from LSO landing on the Go core. The
"must satisfy" notes are the existing project requirements applied to that area —
not new hurdles, just the same bar the rest of the code already meets.

### 1. Intiface / Buttplug transport

A websocket client speaking the Buttplug protocol, so the merged app can drive
Intiface-managed devices in addition to HSP.

- Must satisfy: implemented purely behind the `transport` interface as a dispatch
  owner (semantic 0–100 mapped at the boundary), pure-Go, covered by the motion
  safety gate (Stop, goroutine lifecycle), and honest about failure (no silent
  fallback — see ADR 0006's recovery rule).
- Decision needed: whether Intiface is a first-class dispatch owner alongside
  HSP or an opt-in/experimental one, and how it reconciles with the current
  HSP-only scope (ADR 0006). See alternatives, Decision 2. Record the outcome as
  a new ADR.

### 2. Motion library / blocks and the queue

LSO's saved motion "blocks," their editor/heatmap tooling, and a motion queue.

- Must satisfy: playback runs through the shared motion engine and the Phase 11
  arrangement contract — not a parallel block-playback engine (R14). Blocks are
  *content*; the engine is the single path that plays them.
- Decision needed: fold LSO blocks into the Phase 14 Pattern Library model, or
  keep a distinct library. See alternatives, Decision 4.

### 3. Personas

LSO's persona system (persona-driven chat plus motion bias).

- Must satisfy: personalization stays inspectable, resettable, and
  code-contract-safe (the motion JSON contract is appended by code and cannot be
  edited out — Phase 10 rule).
- Decision needed: merge personas with MagicHandy's prompt sets + long-term
  memory into one personalization model, or keep both. Overlapping systems drift;
  prefer one. See alternatives, Decision 3.

### 4. LSO data import and compatibility

One-time importers and compatibility endpoints so existing LSO users carry over
settings, personas, and library content.

- Must satisfy: non-destructive import (keep originals), a compatibility report,
  redacted secrets, and fixtures/tests — the same discipline as the
  StrokeGPT-import risk (R8) and the SQLite legacy import (ADR 0008). This is the
  natural home for the Phase 15 migration work.

### 5. Frontend

LSO brings a feature-rich React/TypeScript UI (block editors, heatmaps,
device-control surfaces, persona editors) and a localization system.

- Must satisfy: one canonical frontend ships and is embedded; it meets
  `docs/ui-design-guidelines.md` (visual system, safety invariants,
  accessibility) and holds the browser-side efficiency budget (`AGENTS.md` §3 —
  the in-browser cost was a first-order reason for the Go rewrite).
- Decision needed: which frontend is canonical and how the other is retired or
  folded in — the highest-leverage open decision. See alternatives, Decision 1.

### 6. Localization

Reconcile LSO's locale generation/tooling with the existing localization docs
(`docs/localization-wording.md`, `docs/prompt-localization-strategy.md`).

- Decision needed: one localization pipeline and one source of truth for
  translations. See alternatives, Decision 6.

### 7. Scripts and stack tooling

Start/stop and dependency-bootstrap scripts for the local stack.

- Must satisfy: keep them documented and, where they are user-facing, portable;
  do not commit runtime data or caches they generate (`AGENTS.md` §5).

## Cross-cutting requirements

These apply across every workstream and are covered in `AGENTS.md`; the ones most
relevant to a large merge:

- No committed runtime data (`*.db`/`-wal`/`-shm`), caches, `node_modules`, or
  duplicated large binaries; exactly one shipped UI `dist`.
- Split oversized files rather than raising their budgets by default.
- Re-measure RSS and binary size as the surface grows, and record it.
- Preserve the safety gate and the single-motion-path rule as new sources and a
  new transport arrive.

## Open decisions

The merge has several genuinely valid shapes. These are the decisions to make
deliberately — early, so they are chosen rather than defaulted-into by whichever
branch merged first — and each should end as an ADR:

1. Canonical frontend and how the other is retired/folded in.
2. Intiface/Buttplug transport scope (first-class vs opt-in) and HSP-only-scope
   reconciliation.
3. Personalization model: personas vs prompt sets + memory (merge vs keep both).
4. Motion content: LSO blocks vs the Phase 14 Pattern Library + Phase 11
   arrangement contract.
5. Repository/integration shape (single merged repo vs shared-backend split).
6. Localization pipeline and translation source of truth.

Each is laid out with trade-offs and a recommended default in
[lso-merge-alternatives.md](lso-merge-alternatives.md).

## Suggested sequencing

1. Land the additive, contract-respecting backend pieces first under review
   (transport behind the interface, LSO import, backend feature packages), each
   green against the safety gate and budgets.
2. Make the frontend-consolidation and feature-dedup decisions explicitly
   (Decisions 1, 3, 4) and record them before building further on either side.
3. Converge on one frontend, one personalization model, and one motion-content
   model; retire the duplicated paths.
4. Re-measure the efficiency budgets and update `docs/goal-scorecard.md` at each
   step so growth stays a tracked trend, not a surprise.
