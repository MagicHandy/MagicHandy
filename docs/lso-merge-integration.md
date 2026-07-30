# MagicHandy + LSO Integration Plan

## Status

Active, updated 2026-07-24. This plan describes combining **MagicHandy** and
**LSO (Local Stroke Orchestrator)** into one project. It is a living document;
the open decisions in the last section are tracked in
[lso-merge-alternatives.md](lso-merge-alternatives.md) and become ADRs as they
are settled.

## Context

The two projects are converging on the same goal — a local-first, LLM-driven
controller for The Handy — from different starting points:

- **MagicHandy** is the Go-first core established in this repo: a pure-Go
  (`CGO_ENABLED=0`) backend, a semantic motion engine with a transport-neutral
  frame (Cloud REST, Browser Bluetooth, and Intiface owners), SQLite persistence,
  a pattern/program library, and a React UI built at build time and embedded in
  the binary. Its guiding constraints are efficiency (low RAM/CPU, small binary)
  and maintainability.
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

## Current next steps (snapshot, 2026-07-24)

The phase table in [../IMPLEMENTATION_PLAN.md](../IMPLEMENTATION_PLAN.md) tracks
depth and [feature-ideas.md](feature-ideas.md) catalogs possibilities. Neither
answers "what is worth picking up next", which is the question that comes up
between maintainers working different hours. This is that list. It is a
**snapshot and it goes stale** — trust the linked docs over it whenever they
disagree.

**Recently landed, so it does not need rebuilding:** the chat
voice/persona/anatomy/mood axes (#126, #127), automatic named-area pattern
projection, and the acceleration-budget reversal ramp (#128). Anything starting
from "make the replies feel in character" or "motion pauses at the turn" should read
[chat-voice.md](chat-voice.md) and the
[2026-07-24 motion review](motion-pathway-review-2026-07-20.md#follow-up-review---2026-07-24)
first.

### Content and frontend

- **Reaction-style axis** (submissive / dominant / playful / teasing) — the one
  personalization dimension the shipped axes do not cover. Build it as an enum
  axis composed in code beside `chat_voice`, not as a second personalization
  system; the shape and the reasoning are in
  [lso-merge-alternatives.md](lso-merge-alternatives.md), Decision 3.
- **Replace the weak built-in patterns.** Most of the catalog is generated from
  parameter specs; only two entries are promoted from curves that were used and
  accepted in practice (`internal/motion/content_curated.go`). Which generated
  ones are worth replacing is a judgment call best made by whoever has a large
  funscript collection to draw from. Content work, no architecture risk;
  [pattern-library.md](pattern-library.md) covers import and promotion.
- **Enter-to-send in chat** — still Ctrl+Enter in
  `web/src/components/ChatPanel.tsx`. Keep a modifier for newline.
- **Mouse-tracked authoring**, scoped as record-then-play. See the design note
  in [feature-ideas.md](feature-ideas.md) §C.

### Motion core and backend

- **Hardware acceptance backlog.** Several changes are correct by test and
  unverified by hand: reversal feel at 20-40% speed and a focused pattern at the
  20-point minimum (#128), Stop during a chat turn with TTS speaking (#127),
  Phase 18 M3 media alignment re-recorded against `expected_media_time_ms`, and
  Browser Bluetooth endurance beyond short sessions. This is the largest honest
  gap in the project and it cannot be closed by more code.
- **Live-input behavior per dispatch owner** — one instrumented capture each,
  which the authoring-by-demonstration work depends on
  ([motion pathway review](motion-pathway-review-2026-07-20.md#open-question-live-input-over-hsp-unmeasured)).
- **Autopilot arrangements and session authority** - independent cadence and
  speech-motion authority are specified in
  [autopilot-cadence.md](autopilot-cadence.md); freeform arrangements and
  model-triggered session entry/exit remain open (`llm-control-surface.md`
  ideas E/F).
- **Live motion log and in-chat feedback buttons** — both are "strong
  candidate" in the ideas catalog and both are small.

### Waiting on a decision, not on work

- **Offline pre-transcode** — open questions recorded in
  [video-playback.md](video-playback.md).
- **Merge Decisions 1, 3, 4, 5, 6** below. Decision 3 now has shipped code
  under it, so it is the one most likely to be decided by accident if someone
  starts building.
- **Whether to adopt an issue tracker.** This section exists partly because
  small claimed items and open questions have nowhere else to live between
  planning-doc revisions.

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
- ADR 0010 resolved this as a first-class, user-selected dispatch owner. Its
  immediate-mode delivery uses absolute deadlines, bounded asynchronous ACK
  correlation, generation-invalidating Stop, and wire-level diagnostics while
  remaining behind the shared neutral-frame contract.

A 2026-07-13 source review of LSO commit `206d468a` retained its useful absolute
monotonic deadline and adjacent-keyframe-duration ideas, but rejected its
unbounded whole-script transport queue, ACK-less success reporting,
flush-before-Stop ordering, direct-control bypass, and dequeued-command-after-
Stop race. None of those execution paths are migration inputs.

### 2. Motion library / blocks and the queue

LSO's saved motion "blocks," their editor/heatmap tooling, and a motion queue.

- Must satisfy: playback runs through the shared motion engine and the Phase 11
  arrangement contract — not a parallel block-playback engine (R14). Blocks are
  *content*; the engine is the single path that plays them.
- Phase 14 now establishes the canonical Pattern/Program model. LSO blocks and
  queues must be imported into that library plus the Phase 11 arrangement
  contract; a distinct playback path is not an admissible option. The remaining
  decision is field mapping and which block shapes are repeatable patterns vs
  finite programs. See alternatives, Decision 4.

### 3. Personas

LSO's persona system (persona-driven chat plus motion bias).

- Must satisfy: personalization stays inspectable, resettable, and
  code-contract-safe (the motion JSON contract is appended by code and cannot be
  edited out — Phase 10 rule).
- Decision needed: merge personas with MagicHandy's prompt sets + long-term
  memory into one personalization model, or keep both. Overlapping systems drift;
  prefer one. See alternatives, Decision 3.
- As of #126/#127 the Go core composes prompts from independent code-owned axes
  (prompt set, reply register, partner identity, user anatomy, model-reported
  mood). That is most of the recommended option already built, and it narrows
  the remaining gap to a **reaction-style axis** plus saveable presets. Decision
  3 in the alternatives doc carries the table and the composition rule; read it
  before adding a persona surface, or the two systems will duplicate.

### 4. LSO data import and compatibility

One-time importers and compatibility endpoints so existing LSO users carry over
settings, personas, and library content.

- Must satisfy: non-destructive import (keep originals), a compatibility report,
  redacted secrets, and fixtures/tests — the same discipline as the
  StrokeGPT-import risk (R8) and the SQLite legacy import (ADR 0008). This is the
  natural home for the Phase 15 migration work.

#### Rockfire branch audit (2026-07-11)

The remote `Rockfire` branch was audited rather than merged. Its database
lineage reached `PRAGMA user_version=7` with useful LSO rows, but the branch also
contained committed runtime databases, duplicate datastore/frontend trees,
stale build assets, and a `manualqueue` package that owned transport dispatch.
Those source/runtime artifacts are not migration inputs and were not copied.

The Intiface client at audited branch commit `50614411` is also excluded. It
bursts future neutral points without honoring timestamps, ignores movement
ACKs, retains queued movement after Stop, has no ping keepalive, and participates
in several private motion-owner paths. The current owner is an independent
implementation of the accepted shared-engine and Stop contract.

Schema v8 can open that database non-destructively. It repairs the canonical
settings/prompt shapes and preserves these Rockfire tables for Phase 15:

- `funscript_files` and `motion_blocks`: candidate Program/Pattern source data;
  action timing, source relationships, ratings, favorites, blocked state, and
  usage/success metadata must appear in the dry-run compatibility report
- `saved_queues`: candidate Phase 11 arrangement data; never a second queue
  player
- `personas`: candidate prompt-set/memory/personalization data
- `ui_preferences` and `app_state`: locale, active persona, and operation-mode
  preferences; import only where a canonical setting exists, otherwise report
  them as deferred/unsupported

No preserved row is exposed to the app until an explicit importer maps it. This
prevents an automatic schema migration from silently changing motion meaning.

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
- Preserve the safety gate and the single-motion-path rule as new sources arrive
  and the implemented Intiface owner evolves.

## Open decisions

The merge has several genuinely valid shapes. These are the decisions to make
deliberately — early, so they are chosen rather than defaulted-into by whichever
branch merged first — and each should end as an ADR:

- **Decision 1:** canonical frontend and how the other is retired/folded in.
- **Decision 3:** personalization model: personas vs prompt sets + memory.
- **Decision 4:** motion-content field mapping into the Phase 14
  Pattern/Program library and Phase 11 arrangement contract (the shared engine
  target is settled).
- **Decision 5:** repository/integration shape (single merged repo vs
  shared-backend split).
- **Decision 6:** localization pipeline and translation source of truth.

The former Intiface/Buttplug scope decision is closed: ADR 0010 selected and
Phase 14B implemented a first-class dispatch owner for one linear actuator.

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
