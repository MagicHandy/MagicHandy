# Parity With StrokeGPT-ReVibed — Capability Overview

MagicHandy is a ground-up rewrite of StrokeGPT-ReVibed (STGPT-RV), so "parity"
is a recurring question. Parity evidence was previously spread across three
documents at three different altitudes. This page is the **index and the
capability-level view** that ties them together — it does not restate them.

## The three existing parity records (still authoritative)

1. **UI/UX behavior** — [ui-design.md](ui-design.md) "Functional Parity
   Baseline": the nine hard-won UI behaviors (backend-loss handling, scrollback
   stickiness, connection verification, estimate honesty, pause/resume, copyable
   diagnostics, reset-to-defaults, Stop-shortcut visibility, chat continuity).
   **All nine are closed.**
2. **Notes files + June 2026 hardware PRs** —
   [legacy-parity-sweep-2026-07.md](legacy-parity-sweep-2026-07.md): motion
   quality lessons and the KNOWN_PROBLEMS/ROADMAP items, dispositioned
   Adopted / Covered / Deferred / Rejected.
3. **Full PR history (#21–#333)** —
   [legacy-lessons-sweep-2026-07-11.md](legacy-lessons-sweep-2026-07-11.md): the
   ~300 older PRs mined for lessons the first two sweeps only sampled, read
   skeptically (many STGPT-RV "lessons" compensate for its architecture, not
   reality).

Those cover *behaviors* and *lessons* well. What was missing was a single
**feature-capability matrix** mapping STGPT-RV's own advertised surface (its
`README.md` "What it does" and "Settings tour") to MagicHandy's status. That is
below.

## Disposition key

Same vocabulary as the sweeps, plus one:

- **Covered** — MagicHandy does this (evidence linked).
- **Exceeds** — MagicHandy does this and more.
- **Partial** — some of it exists; the gap is named.
- **Deferred** — recorded, deliberately not scheduled yet.
- **Rejected** — conflicts with MagicHandy design, with reason.
- **Undecided** — not yet decided whether MagicHandy will build it at all.

## Capability matrix

| STGPT-RV capability | MagicHandy status | Where / gap |
| --- | --- | --- |
| Natural-language Handy control via local LLM | **Covered** | Streaming chat → engine, strict JSON contract |
| Cloud REST transport | **Covered** | Phase 5 |
| Web Bluetooth (local) transport | **Covered** | Phase 5B; endurance still a watch item |
| Intiface/Buttplug transport | **Exceeds** | ADR 0010 / Phase 14B — STGPT-RV has no Intiface owner |
| Deterministic motion safety (speed limits, smoothing, stop) | **Covered** | Sampler/sanitizer + transport caps; goleak/race gated |
| Named patterns, curated by the LLM | **Covered** | Phase 14; enabled-only curation |
| Programs (funscripts), separate from loops | **Covered** | Phase 14 library |
| Motion Pattern Studio (import, draw, crop, preview, save) | **Covered** | Phase 14 authoring |
| Adaptive **Freestyle** | **Covered** | Phase 11 planner on the shared engine |
| **Preset modes: Auto / Edge / Milk** | **Rejected (as ports)** | ADR 0006 drops legacy scripted modes; Edge/Milk may return only as continuous-engine planners if wanted, never as `ScriptStep` ports |
| **LLM stroke-region control (tip/shaft/base)** | **Partial** | Engine `AreaFocus` exists; **not exposed in the chat contract** — see [llm-control-surface.md](llm-control-surface.md) idea A |
| **LLM program/script selection** | **Partial** | Engine `ProgramID` exists; chat contract exposes only `pattern_id` — idea B |
| **Soft-anchor loops (tip/upper/mid/lower/base)** | **Partial** | Engine `SoftAnchor` exists; no authoring UI or model access — idea G |
| **LLM-driven autonomous mode (Autopilot)** | **Implemented (2026-07-19)** | LLM-curated segments over the Freestyle loop with deterministic planner fallback and chat-log/TTS say-lines; live-model acceptance open. Shape follows [llm-control-surface.md](llm-control-surface.md) ideas E/F |
| Voice output (cloud + local cloning) | **Covered / differs** | ElevenLabs + NeuTTS Air (STGPT-RV used Chatterbox); ADR 0007 |
| Voice input (ASR) | **Covered / differs** | Managed Parakeet (STGPT-RV also had faster-whisper); ADR 0007 |
| Persona prompt + memory | **Covered** | Phase 10 prompt sets + inspectable memory |
| User identity / interest selector | **Deferred** | Recorded in the Phase 15 personalization notes |
| Reset to defaults | **Covered** | Parity row 7 |
| Stroke-range limit + range test | **Covered** | Live limits + manual-motion test |
| **Tip/base calibration (beyond stroke range)** | **Deferred** | STGPT-RV backlog #12; benefit unconfirmed vs current stroke-range behavior |
| Model management + GPU/VRAM sizing | **Covered / exceeds** | Managed llama.cpp lifecycle + Ollama; curated download + VRAM-fit advice is Phase 16 |
| Diagnostics + transport capture/trace | **Covered** | Diagnostics panel, trace export |
| LAN / mobile HTTPS | **Deferred** | Risk R18; explicit Phase 16 decision |
| **Migration/import from STGPT-RV** | **Undecided** | See below and Phase 15 |
| Story Mode (scripted/voiced scenes) | **Deferred** | STGPT-RV backlog #15; net-new, depends on reliable voice + arrangement |
| Single-operator local app (no account/tracking) | **Covered** | By design |

## What "needed for parity" actually means now

The **UI/behavior parity baseline is complete** and the transport/motion-safety
surface meets or exceeds STGPT-RV. The remaining parity-relevant work clusters
into four groups, none of which is a simple port:

1. **LLM control depth.** The reference app let the model reach stroke regions,
   soft-anchor loops, and (in its item #16 direction) authored programs.
   MagicHandy's *engine* already supports all three; the gap is a safe,
   versioned **contract exposure**, catalogued in
   [llm-control-surface.md](llm-control-surface.md). This is the largest genuine
   parity gap and the highest-leverage because the motion capability already
   exists.
2. **Autonomous behavior.** Autopilot (LLM-driven mode) is unstarted; its shape
   is ideas E/F in the control-surface doc.
3. **Deliberately-not-ported modes.** Auto/Edge/Milk are dropped as scripted
   ports (ADR 0006). If they return, they return as continuous-engine planners.
   This is a *design choice*, not an open gap.
4. **Deferred conveniences.** Tip/base calibration, identity selector, Story
   Mode, and LAN/HTTPS are recorded and deliberately unscheduled.

## Migration importer — Undecided

STGPT-RV data import (settings, memories, prompt sets, patterns, programs) is
scoped as Phase 15 but is now **explicitly undecided** — it may not be built at
all. The reasons it is not simply "planned":

- It is a **one-time convenience**, not core value; a user can hand-copy a
  handful of settings, and patterns/programs already import through the normal
  library import path.
- STGPT-RV is a **live upstream** with evolving formats; an importer pinned to
  today's shapes carries ongoing maintenance for a shrinking audience.
- The **LSO merge** ([lso-merge-integration.md](lso-merge-integration.md)) may
  change what "migration" even means (its Rockfire tables are already preserved
  uninterpreted by schema v8), so committing an STGPT-RV-specific importer now
  could be wasted or redone.
- Its dependent, the Phase 16.3 in-wizard "porting step", inherits the same
  undecided status.

Recording it as Undecided (not "Not started") keeps the plan honest: the
absence is a pending **decision**, not merely pending **work**. If the decision
lands as "build it", the Phase 15 scope in
[IMPLEMENTATION_PLAN.md](../IMPLEMENTATION_PLAN.md) is ready to execute; if it
lands as "won't build", this row and Phase 15 should be closed as Rejected with
that reason.

## Related

- [IMPLEMENTATION_PLAN.md](../IMPLEMENTATION_PLAN.md) — phases and the Known Gaps list
- [goal-scorecard.md](goal-scorecard.md) — goal/budget evidence
- [llm-control-surface.md](llm-control-surface.md) — the LLM-control gap and ideas
- [feature-ideas.md](feature-ideas.md) — the 2026-07-19 deep review: STGPT-RV
  features itemized at the settings/UI level (candidate rows for this matrix's
  next refresh), ideas mined from the Codex history, and net-new ideas
- [decisions/0006-drop-legacy-motion.md](decisions/0006-drop-legacy-motion.md) — why Auto/Edge/Milk are not ported
