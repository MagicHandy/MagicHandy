# MagicHandy Risk Register

## Purpose

This register tracks rewrite risks that should survive between phases. A risk stays open until it is explicitly accepted, mitigated, or closed with evidence.

## Risk Levels

- High: can derail the rewrite or produce major user-facing regressions.
- Medium: likely to slow delivery or cause support burden.
- Low: manageable but worth tracking.

## R1: Real-Device Motion Validation Risk

Level: High

Description:
Simulated transport tests cannot fully reproduce Handy cloud REST latency, firmware buffering, HSP playback state, or physical feel. Source-only reasoning produced incorrect motion fixes in the old app.

Mitigation:

- validate motion retargeting on real hardware early
- capture trace exports for failed runs
- convert real traces into fixtures where possible
- keep diagnostics specific enough to distinguish planner behavior from device/API rejection

Exit evidence:

- real-device checklist passes for area focus, speed changes, stroke range changes, reverse changes, same-pattern updates, cross-pattern retargets, and emergency stop

## R2: Two-Codebase Drift

Level: High

Description:
StrokeGPT-ReVibed may continue changing while MagicHandy is being rewritten. Feature parity may drift, and agent time may be split across two architectures.

Mitigation:

- define parity milestones
- avoid porting every legacy behavior immediately
- preserve only important invariants/specs early
- decide when to freeze, continue, backport, or abandon

Exit evidence:

- documented parity/default-app decision

## R3: Motion Retargeting Complexity

Level: High

Description:
Smoothly changing active timed-point streams under variable command latency is the hardest part of the rewrite. If underspecified, MagicHandy can reproduce the same stop/start or hard-reset behavior.

Mitigation:

- maintain `docs/motion-retargeting.md`
- make retarget reasons traceable
- test same-pattern and cross-pattern handoffs
- use real-device validation before broad feature work

Exit evidence:

- real-device retarget tests pass without regular stop/go behavior

## R4: HSP v4 Contract Regression

Level: High

Description:
Known HSP schema and behavior constraints can be forgotten during a ground-up rewrite.

Mitigation:

- maintain `docs/hsp-v4-invariants.md`
- port invariants as executable tests before live transport
- review transport changes against those tests

Exit evidence:

- HSP invariant test suite exists and passes in CI

## R5: Bluetooth Implementation Risk

Level: Medium

Description:
Native Bluetooth on Windows may be costly or unreliable. Browser-owned Web Bluetooth requires an active tab and robust bridge state.

Mitigation:

- default to browser-owned Bluetooth early
- keep no-silent-fallback rule
- document bridge status clearly
- defer native Go Bluetooth until justified by a prototype

Exit evidence:

- Bluetooth ownership decision remains current and a working bridge passes manual checks

## R6: Optional Python Worker Complexity

Level: Medium

Description:
Moving ML dependencies out of the core app avoids core install failures but introduces IPC, process lifecycle, cancellation, and protocol-version complexity.

Mitigation:

- version the worker protocol
- app must run without workers
- implement stub workers before real ML providers
- surface worker status and crash diagnostics

Exit evidence:

- worker protocol tests pass and core app starts without Python workers

## R7: Packaging And Signing Risk

Level: Medium

Description:
Binary release expectations can expand to installers, code signing, auto-update, bundled optional workers, and bundled or downloadable llama.cpp runner variants.

Mitigation:

- start with portable zip
- document signing/auto-update decisions separately
- keep core-only release separate from voice-worker bundles

Exit evidence:

- repeatable GitHub release artifact exists and can run from a clean directory

## R8: User Migration Risk

Level: Medium

Description:
Users may have settings, memories, prompt sets, patterns, programs, and assets in StrokeGPT-ReVibed. A rewrite can lose or misinterpret those files.

Mitigation:

- non-destructive import
- dry-run compatibility report
- unsupported-field report
- representative migration fixtures

Exit evidence:

- migration tests pass and manual import produces a clear report

## R9: UI Regression Risk

Level: Medium

Description:
The current app has many UX learnings around settings organization, quick controls, visualizer mapping, and error visibility. A simpler UI should not lose critical controls or hide diagnostics.

Mitigation:

- preserve major settings mental model
- quick settings must apply immediately
- visualizer reads backend state
- browser tests once UI exists

Exit evidence:

- desktop/mobile visual checks and UI tests pass for settings, quick controls, stop, and diagnostics

## R10: Scope Creep Toward Legacy Parity

Level: High

Description:
Trying to port all legacy modes, pattern authoring, voice providers, and setup flows before the core is proven can stall the rewrite.

Mitigation:

- follow phase order
- keep explicit out-of-scope lists
- require real-device motion milestone before broad feature parity
- prefer small PRs with clear done criteria

Exit evidence:

- Phase 17 parity review recommends default/continue/freeze/backport with clear evidence

## R11: Rewrite Goals Left Unmeasured

Level: High

Description:
Maintainability, lower core memory, and shippable binaries are the stated
reasons for the rewrite, but they are easy to claim and easy to lose. Go does
not deliver them by itself: a god-package, a CGo dependency, or GC-held memory
can each defeat a goal silently. Without targets and enforcement, the rewrite
can complete without achieving its purpose.

Mitigation:

- maintain `docs/goals-and-guardrails.md` with measurable targets
- capture the Python core baseline and Go idle RSS in Phase 1
- enforce CI gates (lint, import boundaries, size norms, `CGO_ENABLED=0`)
- measure RSS at motion/app milestones, not just at idle

Exit evidence:

- recorded baseline plus Go numbers per milestone, and CI enforcing the gates

## R12: Frontend Debt Carryover

Level: Medium

Description:
The Go core owns only the backend. The current frontend is ~13k lines of vanilla
JS with a shared state/element god-registry. Porting it wholesale carries the
maintainability debt across and defeats the goal for half the codebase.

Mitigation:

- follow `docs/decisions/0004-frontend-strategy.md`: rebuild fresh, minimal-first,
  backend-state-driven; old JS is reference, not base
- apply the size/no-god-module norms to `web/`
- defer the heavy authoring UI rather than porting it early

Exit evidence:

- minimal UI built without a ported god-registry; `web/` respects size norms;
  parity review documents remaining UI gaps

## R13: llama.cpp Runner And Model Management Risk

Level: High

Description:
MagicHandy is intentionally making llama.cpp the quality-first Windows/NVIDIA LLM path. That improves control over model choice and runtime behavior, but it also makes runner packaging, CUDA compatibility, model downloads, GGUF metadata, disk usage, licenses, and hardware-fit reporting part of the product. A broken runner or unclear model manager can make the primary chat path harder to use than Ollama.

Mitigation:

- keep the Go core pure-Go and manage llama.cpp as an external `llama-server` process
- pin runner builds and record compatibility metadata
- start with a small curated GGUF catalog instead of an open-ended model zoo
- support importing a local GGUF without forcing a download
- require explicit download confirmation with visible size, license, checksum, and expected hardware fit
- verify downloads before install and move files atomically
- keep Ollama available as the secondary cross-platform provider
- surface runner stderr, health, model-load errors, and hardware-fit warnings in diagnostics

Exit evidence:

- Phase 9 can load and chat with a GGUF model on a supported Windows/NVIDIA setup
- Ollama still works as the secondary provider
- startup/status checks do not download models
- model install/import/load/unload paths are tested and documented
