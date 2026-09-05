# MagicHandy Risk Register

## Purpose

The replacement library is public under ADR 0020; ten continuous recipes replace
81 deprecated built-ins in model/default selection. Saved content remains
manually available. Exact derivative and wire checks cover the new path, while
an inert visual atlas exposes acceleration seams and model intent failures.
Granite still misses some valid movement requests despite schema constraints.
The [library review](motion-library-review-2026-09-04.md) records those failures,
current build evidence and the disconnected-device limit on physical acceptance.

Natural-motion follow-up: [the redesign review](motion-lab-redesign-2026-09-04.md)
records the user's rejection of directional timing, its near-ceiling commanded
peaks, and remaining model/schema failures. Continuous flow replaces the
experimental authoring framework while retaining one engine; final quantized
path checks preserve easing after duplicate removal. Optional Motion
Lab and LLM Lab keep previews inert, check saved limits before audition, expose
raw failures and cancel trials on Stop. Schemas constrain syntax, not intent;
successful compilation does not establish natural physical feel. Every release
now includes Labs behind a saved setting that defaults off (ADR 0021).
Backend gates reject disabled requests and auditions. Disabling cancels pending
lab work and stops lab motion; completed observations remain inert review data.
Real-device acceptance remains open.

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

Status 2026-07-14: Phase 7 and Cloud REST have current capped real-device
evidence. The 2026-07-02 Browser Bluetooth run moved and stopped the device but
predates the reverse-direction fix and lacks endurance evidence. Phase 14's
generated/imported curves pass automated safety checks, but routine-cycle feel
still needs a capped hardware check. The revised Intiface pacer also needs a
matched `motion_trace.v3` hardware run and subjective feel confirmation.

Status 2026-07-21: review of a 194-row Cloud run found three HSP adds accepted
after the preceding buffer tail, including a 958 ms request that crossed it by
540 ms. The engine now prebuffers and maintains the Cloud owner's 1.5-second
minimum using actual emitted coverage, and HTTP 200 API error envelopes no
longer masquerade as accepted points. The device reported
`DeviceNotConnected` during the follow-up, so a capped post-fix feel/trace run
remains required.

Status 2026-07-22: the retained paired-video run preserved the reported 1:17
source reversals exactly, but maintained only about 2.1 seconds of accepted HSP
coverage while issuing one append per second. Fixed media now uses a batched
10-second Cloud lead and four-second refill headroom; interactive targets keep
the 1.5-second retarget horizon. Loop-pattern reversal easing is confined to a
75 ms trapezoidal velocity ramp instead of the full stroke interval. Automated
timing, acceleration, wire-error, stationary-time, and chatter gates pass; the
capped post-fix subjective run remains open.

The first action in that capped follow-up jerked toward the stream's time-zero
point. The trace proved the old startup had no physical anchor: it applied the
new stroke window, buffered a semantic endpoint at `t=0`, and played without
reading slider position. Cloud Start/Resume now stop stale HSP, observe slider
and stroke state, use a speed-bounded HSP lead-in under a non-narrowing union
window, verify arrival, and only then begin semantic/media time. State failure
is fail-stopped and Stop cancels the lead-in. A corrected capped Cloud trace
then calibrated the window-relative API state through absolute endpoints,
verified stationary arrival inside the final window before main Play, completed
the validation sequence, and stopped successfully. A later replay showed that
API v3 extrapolates its relative `position` below zero when the stopped slider
is physically outside the active stroke window; finite extrapolation is now
accepted while absolute calibrated travel remains fail-closed. R1 remains High
pending the user's subjective confirmation that the first action no longer
feels abrupt.

A subsequent trace showed that scheduled HSP completion and physical arrival
are not equivalent: Stop was issued before the lagging slider reached the
target, freezing it short and making retries advance only incrementally. The
engine now observes arrival while the final lead-in target remains active,
with six cancelable reads, then Stops and verifies again. Exhaustion or drift
fails stopped without main playback. A capped 10% repeat covered an 88.93%-to-0%
lead-in, verified the stationary endpoint before main Play, and completed with
a successful safety Stop. All 128 concurrent state probes returned in 1-5 ms,
so the startup path did not block the core. R1 remains open for subjective
first-action and low-speed reversal feel rather than this objective startup
failure.

This correction is intentionally scoped to Cloud REST, the owner used by the
observed run and the one that exposes usable absolute slider/stroke state.
Browser Bluetooth currently avoids a state read because it can destabilize the
GATT session, while Intiface can time an initial `LinearCmd` but cannot observe
the actuator's starting position. Their unknown-position startup behavior
remains open under R1; closing it requires either reliable position feedback or
a conservative owner contract whose first command is bounded for worst-case
travel by the configured speed.

Status 2026-08-01: a read-only trace for a reported speed-limit regression
showed the target correctly clamped to the configured 35% maximum, but the
selected generated `curated-fast-drive-20` curve contained 40 ms reversals that
had bypassed catalog acceleration/reversal gates through a broad imported-curve
test exemption. The 171 generated clips remain active, but none receives that
exemption: 165 unsafe source curves are time-bucketed, all low-prominence
reversal chatter is removed, and every result passes the normal catalog fitter.
The existing feel envelope leaves 170 visibly experimental and one normally
labeled. Exact timing exceptions name only the two previously hardware-accepted
user patterns. Accepted calibration excursions also retain their raw observed
position for startup decisions, so clamping cannot skip the verified lead-in.
No post-fix hardware motion was run; capped Cloud REST startup and restored
pattern-feel evidence remain open.

Status 2026-08-13: pattern choice no longer determines global playback pace.
The prior catalog spanned roughly 57.6–436.4% travel/s at authored timing while
the full 1–100 speed control changed a selected loop by less than 2x. The shared
planner now measures each loop and targets `180 * speed_percent / 100` mean
semantic travel per second, then lengthens only where that curve's acceleration
or reversal floor requires it. The retained generated set is 59 of the original
171 (nine accepted, 50 experimental); together with six experimental designed
patterns, 31 of 87 built-ins are model-visible by default. Simulation covers
cross-pattern rate equality, proportional 20/40/80 pacing, focus compression
and expansion, soft anchoring, loop-seam reversals, and all-built-in safety at
100%. No post-change hardware motion was issued, so subjective
low-speed and reversal feel remain open under this risk.

Status 2026-08-20: selectable Dynamic LLM motion adds ephemeral center/span or
named-anchor geometry through the same planner and sampler. Tests bound span,
variation, reversal dwell, interior-anchor velocity, and active handoff
position/direction/velocity, but those checks do not establish physical feel.
Dynamic remains opt-in until a managed llama.cpp/Ollama prompt matrix and a
capped matched-device A/B run cover slow narrow loops, anchor pass-throughs,
reversals, conversational updates, Autopilot boundaries, and Stop.

Status 2026-08-20 calibration follow-up: qualitative device feedback reported
that Dynamic remained robotic and that a selected 73% felt slow relative to
other applications. The report did not include transport mode, latency, or a
`motion_trace.v3` export, so it establishes a feel regression but cannot isolate
transport timing. The shared planner's 100% reference was only 180% travel/s,
making 73% 131.4%/s (about 145 mm/s over a 110 mm reference stroke), and the
450 ms catalog authoring gap was also reused as a runtime floor for every
reversal. The engine now maps 1–100 through a selected Original / Handy 2
Standard / Handy 2 Pro profile using the published 110/125 mm travel and
32–400/450 mm/s normal envelopes. It evaluates a separate 7500%/s² and 100 ms
runtime envelope against the exact rendered curve, keeps
fractional authored phase, and gives nonzero Dynamic variation a long bounded
spatial-and-timing phrase. Ordered Chat settings are labeled set-point sliders;
categorical motion modes remain segmented choices, and the Connection menu
exposes the model calibration with its travel/top-speed evidence. No published
per-model acceleration limits were found, so no motor-RPM inference or Pro
overclock mode was added. Automated calibration,
acceleration, reversal, continuity, Dynamic determinism, UI, and Stop lifecycle
coverage is retained. No post-change hardware command was issued. A capped
matched run must still record selected model/profile, transport, latency,
trace, subjective continuity,
and Emergency Stop before physical acceptance is claimed. See
`docs/motion-calibration.md`.

Status 2026-08-20 range-envelope follow-up: Creative now separates outer
geometry, a model-selected 20%-or-wider span floor/profile, center/rhythm
variation, pace, and decision horizon. Breathe, wander, and contrast compile to
deterministic, loop-closed phrases lasting at least about 30 seconds at the
fastest supported normal profile; steady clears the envelope. Planner tests
cover bounds, acceleration, all three Handy profiles, trace identity, and
adaptive continuous retargets. The installed managed 12B Gemma model passed
25/25 first-response Creative cases with no repair and no engine or transport.
This reduces schema/prompt risk but does not establish physical feel: the
matched-device trace/Stop run above and the supported Ollama matrix remain open.

Status 2026-08-20 natural-turn follow-up: the latest installed Cloud REST trace
provided the missing diagnostic separation. Across 439 points and 85 legs, the
contrast envelope contracted from about 80% to about 32% and then spent more
than 30 reversals inside roughly 31–35% span. Most successful append
acknowledgements were about 330–341 ms; one network error occurred, and the
user's Stop succeeded in 330 ms. The trace therefore showed both a
stroke-length plateau and a piecewise-linear endpoint feel rather than
high-frequency sample jitter alone. Creative now gives wander/contrast local
seeded range choices at about two-cycle cadence and tests minimum exploration
in every circular four- and six-cycle window. Its true turns use whole-leg
shape-preserving PCHIP easing instead of compressing braking into the stored-
pattern guide, while the same exact 7500%/s² planner limit, sampler, transport,
and Stop path remain authoritative. Buffered 1%-resolution tests retain the
easing at the transport boundary.

The accompanying conversation review found reply/motion disagreement: a model
could claim tip or full-length coverage while producing no accepted update or
geometry that omitted an endpoint. Effective-window validation, elliptical
correction context, and position-only axis scoping now reject or normalize
those cases without a phrase-by-phrase prompt catalog. The reproduced sequence
passed 9/9 managed-model decisions and the broader matrix retained 25/25. No
new hardware command was issued, so the post-fix matched subjective run and
supported Ollama matrix remain open.

Status 2026-08-22 alpha.29 follow-up: the installed max-rate session showed the
scheduler did reconsider inside its 8–16 second window, while the model chose
six consecutive holds and preserved one semantic phrase for about 92 seconds.
Autopilot now reports semantic phrase age, reconsideration count, hold streak,
and current horizon without setting a forced-change threshold. The active Gemma
model retained 3/3 short-age holds, changed 3/3 otherwise identical long-age
cases, and completed a 12-turn transport-free run with one hold, 11 semantic
changes, 11 range envelopes, four horizons, and no repair/fallback. Creative is
now the product default by explicit choice; this model evidence does not close
the matched physical-feel gate.

The same installed profile had six disabled canonical built-ins and no disabled
user-authored rows. Alpha.29 retires those six, reducing the active catalog from
87 to 81 and the non-experimental model catalog from 31 to 25. Numeric catalog
gates remain necessary but cannot overrule direct physical curation. Startup
reconciliation deletes only built-in-origin rows, and no post-change hardware
motion was commanded.

Status 2026-08-22 alpha.30 output-fidelity follow-up: historical command
streams confirmed a speed plateau independent of the newer C2 change. The same
Creative geometry compiled to about 109.8%/s mean travel at selected speeds 42,
52, 62, and 72 because one unsafe interval globally slowed the entire phrase.
Creative now fits intervals locally against exact peak velocity, acceleration,
jerk, and reversal-gap extrema. The retained fixture's 42–72 mean rate rises
from 103.8 to 167.5%/s while its peak follows the selected Original-Handy
calibration from 167.6 to 269.0%/s. Breathe's additive/clamped shelf is replaced
by a smooth bounded blend, and compiled 12-second diversity floors cover every
span profile.

The installed Gemma provider then passed three repeated five-decision exact-
Autopilot runs: all 15 outputs used three sections, all compiled through the
shared planner, and least-varied windows measured 9.5–26.4% stroke CV with
15.9–57.1 points of length range. An explicit hold still won at max change rate
and a simulated 180-second phrase age. This closes the demonstrated commanded-
curve plateau and prompt/schema regressions, not subjective physical
acceptance. The installed app, controller, and device were not modified; a
matched capped Cloud run with current-source trace, latency, Stop, and felt
comparison remains required.

Status 2026-08-22 alpha.33 startup follow-up: the installed alpha.32 trace
isolated a separate Autopilot retry loop. All 15 model decisions produced valid
Creative targets, but all 15 starts stopped during position-aware preflight and
failed before main Play. The stationary Original Handy reported 111.33 mm while
its 0–100% stroke response reported 5.00–102.83 mm, mapping the park to 108.7%
of inferred travel; the existing 3% calibration-excursion allowance rejected
it. There were no trace drops, slider speed was zero, and failures recurred at
roughly 5–15 second intervals as LLM latency and the three-second retry backoff
combined. The shared engine now admits at most a 10% initial calibration
excursion into its existing speed-bounded acquisition lead-in, retaining the
raw distance and strict stationary 1% post-command arrival check. Larger
excursions remain rejected before Play and end only the exact failed mode
generation, preventing both an unchanged retry loop and a stale teardown of a
new run. Transient transport/model failures retain their retry path. Automated
tests replay the exact measured geometry, the tolerance boundaries, the
unsafe-state circuit, stale-generation protection, transient recovery, and
Stop lifecycle. No post-change physical motion was commanded; a capped
installed-device confirmation remains open under R1.

Status 2026-08-22 range/pause follow-up: the installed alpha.33 Cloud trace
contained 4,824 emitted points over 403.5 seconds. Only 1.46% of commanded time
was below position 10 and 0.45% above 90; the first 15 retained Creative phrases
clustered around center 48–52 and span 40–45 until explicit chat requested the
base. The same trace captured two user Pauses whose transport Stops succeeded,
followed about 2.2 seconds later by autonomous startup recovery and Play. Pause
had killed the engine loop before `paused=true` was published, so the mode loop
misread that transport-I/O gap as an unexpected idle engine.

The mode manager now latches Pause before transport I/O and cancels in-flight
autonomous work; repeated ticks over the exact `running=false, paused=false`
gap cannot decide, Start, or retarget, while explicit Resume preserves the mode
and phrase. Creative's motion-only system prompt now treats active Autopilot as
the user's ongoing bounded request, carries four recent compiled position bands,
and gives high-rate cosmetic updates one non-prescriptive semantic retry. The
active installed 12B Gemma completed three repeated six-turn evaluations during
tuning; the final qualitative prompt then completed an extended ten-turn
transport-free evaluation, moving from localized motion to materially different
base-reaching/near-full bands without giving the model a numeric broad-range
target or forcing a change. Wander's smooth high envelope phase also approaches
the model-selected outer span more often without reaching a forced endpoint.
The final explicit-direction run produced five valid three-section phrases and
still obeyed an explicit long-age hold. After one low-temperature Hei speech
sample overused first-person openings, an independent moderate speech
temperature passed three repeated four-line novelty runs; the motion slider
does not affect it.
No post-change physical motion was commanded; subjective feel, latency, Pause,
Resume, and Stop on the capped installed device remain open under R1.

Status 2026-08-23 alpha.35 fluidity follow-up: a read-only alpha.34 Cloud trace
reproduced the reported stop-and-shudder feel as whole-percent stationary edges.
Transition windows bypassed generated-motion quantization cleanup, retaining
36-80 ms reversal plateaus, and ordinary appends could repeat the immutable
previous wire endpoint. Post-Codex mode and intent changes did not alter the
steady sampler; they were not the source of this running-motion defect.

Generated-motion cleanup now applies to transition windows and compares each
new append with the previous wire tail while preserving authored transition
boundaries and fitted buffer coverage. Regressions replay the reported Wander
seed, 95/40 span envelope, and 38-to-32 speed handoff, plus an all-one-wire-step
transition. A user-driven current-source Cloud run at speed 65 / span 80 / floor
20 then emitted 195 active points over 10.395 seconds with strictly increasing
timestamps and no rounded duplicate edge. Ten of 11 appends succeeded at
328-379 ms (343.1 ms average); one failed with a 15 ms network error, and user
Stop completed in 328 ms. The trace was exported outside the repository. The
shared engine, semantic curve, transport boundary, and Stop path are unchanged.
Subjective feel was not reported, so matched physical acceptance remains open.

Status 2026-08-23 effective-pace follow-up: alpha.35 still interpreted the
selected percentage as the instantaneous crest of an eased stroke, so felt
mean pace remained substantially lower, and timing-resolved Creative loops
still inherited a generic 500 ms Pattern Library floor. Creative now requests
calibrated mean travel, uses the selected profile's 100% rate only as its hard
velocity ceiling, and derives one geometry-stable physical floor before
distributing authored timing. Short strokes retain a quiet rounded turn; long
strokes gain a cruise-like C2 body. Higher settings are monotonic and saturate
honestly against device velocity, the shared 7500%/s² acceleration budget, the
150000%/s³ smoothness budget, or 100 ms turn spacing. The backend publishes
effective/requested pace and limiter names, and Stop retains that run's
sanitized trace in a 128-row/1 MiB SQLite envelope exportable after restart.
This is compiler/diagnostic evidence only; no device command was issued, so the
matched capped subjective run remains open.

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
- preserve percentage units at the BLE encoder boundary
- invalidate stale command batches and deliver Stop directly from an already-connected browser when the backend is unavailable
- defer native Go Bluetooth until justified by a prototype

Exit evidence:

- Bluetooth ownership decision remains current and a working bridge passes manual checks

Status 2026-07-16: automated browser tests cover percentage encoding, malformed
protobuf rejection, direct Stop during backend loss, command-poll cancellation,
and Stop-before-disconnect teardown. The existing hardware run predates these
changes, the reverse-direction correction, and endurance testing, so R5 remains
open pending the documented capped real-device matrix.

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

Status 2026-07-11: schema v8 safely opens the divergent Rockfire v7 database,
preserves its settings/prompt data, and leaves its LSO-only tables untouched.
That is database-lineage compatibility, not the Phase 15 user importer; dry-run
mapping from StrokeGPT-ReVibed and LSO content remains open.

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

Status 2026-07-19: every top-level route, settings subsection, and Phase 14
Browse/Programs/Author/Training view passes rendered checks at 1440x900 and
390x844 with no horizontal overflow, unnamed controls, duplicate IDs, or nested
interactive elements. Route titles, heading progression, mobile navigation,
manual Speed, and speech-provider names have focused coverage. Backend preview
samples, not frontend interpolation, render every playback-preview curve. The
Import tab additionally passes 1280x800 and 390x844 rendered checks with fixed
trim targets and no overflow. Its client-side editor can only produce bounded
motion-content input for the normal validated import endpoint; it cannot start
motion or construct transport payloads.

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
The Go core owns only the backend. The canonical frontend is now React, but its
route lifecycle, asynchronous backend snapshots, optional worker surfaces, and
large integration harness can still recreate a shared-state god-module or hide
failed reads behind plausible empty UI. The unshipped legacy JavaScript is a
reference only and must not become a second implementation.

Mitigation:

- follow `docs/decisions/0004-frontend-strategy.md`: rebuild fresh, minimal-first,
  backend-state-driven; old JS is reference, not base
- apply the size/no-god-module norms to `web/`
- keep route/component state scoped, distinguish loading/error/data explicitly,
  and give independent mutation domains their own admission guards
- keep focused component tests beside the existing app integration harness

Exit evidence:

- canonical React UI built without a ported god-registry; `web/` respects size
  norms; full-route desktop/mobile checks and focused lifecycle tests pass;
  parity review documents remaining UI gaps

Status 2026-07-18: route lifetime, teardown writes, failed-read honesty,
cross-tab chat retry, mutation admission, mobile names, titles, and heading
progression have dedicated tests. All 141 frontend tests and the full rendered
route matrix pass; changed production files remain below the 800-line guideline.

## R13: llama.cpp Runner And Model Management Risk

Level: High

Description:
MagicHandy is intentionally making llama.cpp the quality-first Windows/NVIDIA LLM path. That improves control over model choice and runtime behavior, but it also makes runner packaging, CUDA compatibility, model downloads, GGUF metadata, disk usage, licenses, and hardware-fit reporting part of the product. A broken runner or unclear model manager can make the primary chat path harder to use than Ollama.

Mitigation:

- keep the Go core pure-Go and manage llama.cpp as an external `llama-server` process
- pin official runner releases, archive digests, and compatibility metadata
- start with a small curated GGUF catalog instead of an open-ended model zoo
- support importing a local GGUF without forcing a download
- keep model metadata in SQLite and model bytes in one private managed store;
  guard removal of the selected file
- treat Ollama's library as read-only: bounded manifest parsing, explicit copy,
  manifest SHA-256 verification, and clear rejection of split/auxiliary layers
- require explicit download confirmation with visible size, license, checksum, and expected hardware fit
- verify downloads before install and move files atomically
- keep Ollama available as the secondary cross-platform provider
- surface runner stderr, health, model-load errors, and hardware-fit warnings in diagnostics
- bound compact intent output, make hidden reasoning policy explicit, and
  separate cold load, prompt evaluation, reasoning, visible generation, and
  repair rate before attributing latency to the provider

Exit evidence:

- Phase 9 can load and chat with a GGUF model on a supported Windows/NVIDIA setup
- Ollama still works as the secondary provider
- startup/status checks do not download models
- model install/import/load/unload paths are tested and documented

Current evidence (2026-07-11): schema v9 inventory, standalone GGUF import,
Ollama daemon listing, configurable filesystem scan/import, atomic verified
copies, cancellation, deduplication, and selected-model removal protection are
implemented and fixture-tested. A live Windows library with 16 manifests scans
as 16 compatible models without starting a copy. Managed llama.cpp now pins
`b9966` / `c749cb0`, builds from source through an embedded controller-gated
helper, validates an app-owned manifest, resolves models by inventory ID, and
starts the runner offline with its UI disabled. A fresh Windows CPU build was
verified end to end (54.2 s, 18,432,916 installed bytes), as were idempotent
reuse and the Ollama-without-managed-runtime path. R13 remains High until CUDA
load/chat, curated downloads, and hardware-fit guidance have real-system
evidence.

Source-install mitigation (2026-07-13): `install.ps1` now provisions and verifies
missing Go/Git/CMake/MSVC/Windows SDK/CUDA/Ollama dependencies before a selected
build, while `update.ps1` reuses atomic non-secret choices unless the user opts
to revise them. Windows PowerShell 5.1 plan tests cover managed CUDA and
Ollama-only dependency graphs. This reduces manual setup drift but is not CUDA
load/chat evidence and does not lower R13 yet. A same-process CUDA environment
fix was then verified by building the pinned `b9966` runtime with CUDA 13.3 and
MSVC 19.51 and probing the installed `c749cb0` runner. This supplies CUDA build
evidence, but model load/chat remains unverified and R13 stays High.

Source-update lifecycle hardening (2026-07-13): rebuilds now send Emergency Stop
to a running checkout-owned app, terminate its process tree before replacing
executables, stage Go outputs, clean legacy `*.exe~` backups, and verify that the
new process owns the configured port before opening the browser. This prevents a
hidden bind failure from reopening an older embedded UI. It does not add CUDA
load/chat evidence or lower R13.

Latency-control mitigation (2026-07-13): requests use a reviewed output-token cap
(default 256), explicit automatic/off reasoning maps to provider-native fields,
repair temperature zero is serialized, and warm managed calls skip repeated
health/model-list preflights. A live managed Gemma 4 12B Q4 regression probe then
showed automatic reasoning consuming both 256- and 512-token limits with zero
visible JSON. Reasoning-off and a 128-token managed reasoning budget both
produced valid JSON for the same request. Reasoning now defaults off, the current
pinned managed automatic path receives half the total budget, length finishes are explicit,
and repair retains context while requesting reasoning off. This is one diagnostic case,
not broad fixed-model quality evidence; R13 remains High.

Live-provider follow-up (2026-07-20): the installed managed `llama.cpp b9966`
CUDA runner loaded the imported Gemma 4 11.9B Q4_0 model on an isolated port and
completed a 13-turn state-aware motion matrix with no repairs or malformed
responses. Start/relative/curated speeds stayed within the test's 20–40%
envelope; hold, area clear, and chat-only turns were correct; five repeated
variation requests selected five distinct patterns before an older choice was
eligible. The same final service completed the matrix against Ollama/Granite
4.1 3B, using the bounded repair path where that smaller model needed it. This
supplies real CUDA load/chat and secondary-provider evidence without dispatching
to hardware. Curated downloads and hardware-fit guidance remain open, so R13
stays High.

Distribution mitigation (2026-08-02): a fresh Windows install exposed that a
present CUDA Toolkit is not sufficient for a Visual Studio CMake build when
NVIDIA's VS build customizations are missing. The managed path now downloads
the official `b9966` CPU archive or CUDA 12.4 runner/runtime archives, verifies
fixed sizes and SHA-256 digests, rejects unsafe archive paths, stages atomically,
probes commit `c749cb0`, and requires CUDA device detection before activation.
It ships the pinned MIT license and provenance manifest, needs no Git/CMake/
MSVC/MSYS2/CUDA Toolkit, and preserves valid legacy source-built runtimes. Real
network CPU and CUDA installs completed locally; the CUDA runner detected the
RTX GPU. R13 remains High for curated model downloads and hardware-fit guidance.

Managed-endpoint hardening (2026-07-21): live diagnosis found NVIDIA Broadcast
occupying the former fixed managed port 8080 while the installed CUDA runner and
Gemma model loaded normally on an isolated loopback port. Managed mode now keeps
8080 when available, otherwise selects a free loopback port before constructing
its HTTP client, and reports an early runner exit with bounded stderr instead of
polling an unrelated service for the full load timeout. Collision and early-exit
paths have regression coverage. This removes one local-service conflict but does
not change the remaining curated-download and hardware-fit work, so R13 stays
High.

Latency-consistency follow-up (2026-08-01): direct warm production-prompt probes
were stable at roughly 128-141 ms to first token and 553-654 ms total while two
persisted app turns took 32-34 seconds. The tail correlated with a cold
Autopilot decision, shared-GPU TTS that continued after cancellation, and a
dead-parent managed runner retaining about 7.1 GB. Managed startup preload is
now the default with an on-demand memory-saving option; Chat preempts in-flight
autonomous inference; speech interruption is an explicit policy; Faster Qwen
closes canceled streaming generators; and Windows children use Job Object
containment. Exact-path duplicate detection blocks a second managed launch and
requires user confirmation plus backend path revalidation before termination.
Per-message phase timings make future attribution inspectable. Eight isolated
complete-route turns measured one 1.836-second cache-fill request followed by
seven 379-614 ms requests, with one provider call and no repair in every turn.
Live simultaneous-TTS cancellation acceptance remains required, so R13 stays
High.

Ollama-import compatibility follow-up (2026-08-03): one Ollama manifest exposed
a single model layer, but its GGUF embedded Gemma 4 audio and vision components;
Ollama loaded that fused artifact while the pinned stock llama.cpp text runner
failed with a tensor-count mismatch. Managed import and inventory now parse a
bounded GGUF metadata section, reject split shards and embedded audio, vision,
or projector components with an actionable Ollama/text-only explanation, and
cache compatibility until the file changes. The parser accepted the installed
12B text-only Gemma and rejected the reported fused model in 40 ms total. Chat
also keeps a visible first-pass draft stable while a repair streams, preventing
the repair pass from looking like repeated generation. Fixture and UI regression
tests cover both paths; R13 remains High for the existing open work.

## R14: Per-Source Motion Path Divergence

Status 2026-09-05 (Layered review): production chat, production Autopilot and
LLM Lab now carry cloned semantic Flow scores into the existing engine. There
is no independent motion loop or transport policy. Layered continuations
preserve the active score on failure and do not fall back to library patterns
or the legacy extra pace modulation. Fresh seeds change the realization;
captured seeds reproduce it. Shared-engine/fake-transport tests cover real model
responses, continuous retargeting, late mode changes and Stop; global Stop also
recognizes "Stop motion now" without waiting for inference. Planned motion and
captured wire output are reviewed separately from physical feel, which remains
open under R1. See [the Layered evidence](layered-motion-review-2026-09-05.md).

Level: High

Description:
StrokeGPT-ReVibed handled motion separately for chat, Freestyle, Edge/Milk,
trained patterns, and imported scripts. Protections (velocity caps, depth-jump
splitting, turn smoothing, stop/pause boundaries) added for one path did not
reach the others, which caused recurring mode-specific motion bugs.

Mitigation:

- one shared sampler/sanitizer for all sources (see `docs/motion-retargeting.md`,
  "Shared Sampling And Smoothing Protections")
- new sources produce semantic targets, never a parallel motion path
- import-boundary rules keep `modes`/`chat`/`llm` off `transport`

Exit evidence:

- a test asserts no motion source bypasses the shared path; protections are
  applied once and inherited by every caller

Status 2026-07-11: Phase 14 pattern and finite-program playback both construct
semantic `MotionTarget` content and enter the existing engine. API tests assert
engine ownership and disabled-pattern rejection; finite completion performs an
engine-owned Stop. Import-boundary tests still keep `patterns`, `chat`, and
`modes` away from transport internals. The audited Rockfire `manualqueue`
transport owner was deliberately not merged.

Status 2026-07-19: Chat Autopilot reuses Freestyle's mode lifecycle and emits
only resolved semantic pattern targets through `Engine.Start`/`ApplyTarget`.
Its control moved to Chat for information architecture, but the frontend still
sends only `mode:"autopilot"`; it does not construct motion or transport
payloads. Integration tests assert one continuous wire play across multiple
model-curated boundaries.

Status 2026-07-20: the full producer-to-owner audit still finds one dispatch
path. Buffered frames now merge authored knots with 25 ms probes under a 0.3%
error bound, loop seams preserve velocity when they are not reversals, and
retargets use a bounded C1 path blend. Import hygiene rejects <5% loop spans and
removes only rapid <=2% reversal chatter; slow subtle motion and finite programs
remain intact. Cloud's declared 1% endpoint resolution enables an engine-owned
0.8%-bounded fit that cuts catalog shallow-focus stationary wire time by 74%.
Unused raw Cloud/Bluetooth stroke/add/play HTTP routes were removed, so only the
engine calls mutating transport methods other than emergency Stop. Physical
feel and immediate stroke-envelope changes remain covered by R1/R22 rather than
being declared solved from simulation.

Status 2026-08-01: the bulk generated pattern regression did not create a
second dispatch path, but it exposed a parallel safety-policy exception in the
catalog tests. The active catalog includes those 171 clips through the ordinary
normalization and fitter path, while 170 remain behind the experimental-pattern
gate. The only exact-curve exemptions are the canonical `hard-and-regular` and
`playful-jerk` IDs carrying the `curated` tag. User-authored/imported content
still resolves to semantic targets and runs through the same engine; explicit
user library rows remain non-destructive data rather than being reclassified as
built-ins.

Status 2026-08-12: Autopilot's denser intra-segment speed texture remains a
same-pattern semantic `ApplyTarget` through the shared engine, so phase-preserving
handoffs and transport-independent bounds still apply. Timing-space guards keep
sampled waypoints at least six seconds apart with room for jitter, and the
worst measured boundary-plus-sway rate is 8.0 retargets/minute versus the
pre-change 9/minute ceiling. Pattern recency changes only the model-facing
allow-list; it does not create another motion or transport path.

Status 2026-08-13: normalized pattern speed is implemented inside the shared
motion planner, after semantic target admission and before the existing sampler
and transport owner. Chat, Autopilot, library audition, and programs still
produce `MotionTarget` values and do not gain a private clock or dispatch loop.
Video funscripts remain media-clock-locked and intentionally bypass loop-rate
normalization.

Status 2026-08-20: Dynamic LLM geometry also produces a semantic
`MotionTarget.Dynamic` and compiles to ordinary resolved loop content before
entering `Engine.Start`/`ApplyTarget`. It has no goroutine, transport import, or
dispatch API of its own. Interior anchors, slow variation, and velocity-aware
phase selection live in the shared planner; an API integration test verifies a
Dynamic start and update issue only one transport Play. Pattern and Dynamic
prompts are mutually exclusive, and an in-flight result is rejected if the
persisted mode changed before dispatch.

Status 2026-08-20 (alpha.25): a full-span Creative phrase at 96% variation
reproduced a process-ending empty-curve sample after floating-point endpoint
rounding reached `100.00000000000001`. The final semantic projection is now
clamped, plan compilation errors remain attached to the plan, and every Engine
admission/retarget path rejects them before transport work. Zero-value sampling
also fails stationary rather than panicking. Exhaustive variation/profile tests
cover the reduced case without adding a source-specific sampler or transport
path.

Status 2026-08-20 (alpha.26): explicit Creative span envelopes are compiled
inside the same `dynamicContent` resolver before ordinary curve validation.
They add no goroutine, transport method, device window mutation, or frontend
motion state. The trace records outer span, floor/profile, and backend-derived
phrase seed. Long-phrase phase search now scales with authored curve complexity
and scores the direction actually available after handoff; continuity tests
cover profile-to-profile retargets through the existing transition path.

Status 2026-08-22: multi-section Creative phrases remain one semantic
`MotionTarget.Dynamic` and one compiled loop; sections are not mode segments,
transport commands, goroutines, or queued playback owners. The Creative curve
profile is now C2 quintic Hermite with exact acceleration/jerk fitting, and its
whole-percent timing-aware simplifier still feeds the shared sampler and
declared transport resolution. Catalog/imported fitting is deliberately
unchanged. Focused tests cover C2 knot/seam continuity, monotonicity, all three
Handy profiles, quantized short-stroke timing, section reversal-length
diversity, one Play across retargets, and Stop-era lifecycle gates. No
post-change device command was issued, so R1/R22 subjective feel and transport
acceptance remain open.

## R15: Chat And Voice Delivery Ordering

Level: Medium

Description:
The old app sometimes spoke a reply the chat panel never displayed, and a
destructively drained global queue let one browser tab consume another's
messages.

Mitigation:

- lockstep chat-emit and TTS-enqueue; per-client cursors over a shared log;
  single-owner audio lease; model-error path kept out of history/TTS/motion
  (see ADR 0003, "Message And Audio Delivery Ordering")
- autonomous replies carry an ephemeral speech-request id on their canonical
  chat row, so the controller browser can play new lines without replaying
  initial history; an occupied TTS queue leaves later autonomous lines visible
  but does not deepen the speech backlog
- Phase 12 landed the substrate: versioned worker protocol with cancellation
  and queue-depth reporting, a core-owned serialized bounded queue, no-speech
  rejection (never an empty transcript into chat), and worker errors that
  terminate in the voice request log — never in history, TTS, or motion.
  The ordering trio itself (shared log + cursors, lockstep emit/enqueue,
  audio lease) is the first Phase 13 work item, before any real provider is
  wired to chat, because there is no audio playback to order against yet.

Exit evidence:

- tests cover spoken-equals-shown, multi-client cursor isolation, and the
  model-error path — **all three landed with the Phase 13 delivery-ordering
  foundation**: `TestSpokenReplyAlwaysMatchesDisplayedReply` (the enqueued
  TTS text is byte-identical to the logged reply, and only the controller
  can fetch the clip), `TestChatCursorsAreIsolatedAndMonotonicOverHTTP`, and
  `TestModelErrorsNeverEnterHistoryOrTTS`. The risk stays listed until a
  real provider (not the stub) has exercised the same path end to end.

Status 2026-07-19: PR #101 extends the same ordering to Chat Autopilot. Tests
cover browser-discoverable speech ids, no initial-history replay, and canceled
announcements staying out of the log. Real-provider and long-session queue
acceptance remain open.

## R16: Handy HSP Firmware v4 / API v3 Scope

Level: Medium

Description:
Dropping HAMP, HDSP, and firmware v3 (ADR 0006) means MagicHandy's Cloud REST
and Browser Bluetooth owners require Handy firmware v4 plus API v3 access and
have no fallback owner. Firmware v3 Handy hardware is unsupported. A missing,
revoked, or incompatible app Application ID also blocks Cloud REST HSP until
fixed, even if the user's connection key is valid. Intiface is a separate
transport-neutral owner for one selected `LinearCmd` actuator and does not
restore legacy Handy protocols.

Mitigation:

- ship and manage the app's own API v3 Application ID if Handy API terms allow;
  treat it as a public client identifier, not a secret, and keep a developer
  override for testing or future revocation
- the connection key stays the user's private credential
- detect and clearly report HSP-unavailable with concrete fix steps (Invariant 8)
- document the Handy-owner firmware v4 / API v3 requirement up front in
  README/setup, separately from Intiface requirements
- before Phase 16 packaging claims device support: review current Handy API
  docs for Handy 2 / Handy 2 Pro deltas (including the documented overclock
  mode) and expose per-device max-speed limits only from documentation —
  never guessed values (legacy notes item; see
  `docs/legacy-parity-sweep-2026-07.md` §D)
- keep StrokeGPT-ReVibed available for unsupported setups

Exit evidence:

- connect and HSP-unavailable paths give actionable guidance; the requirement is
  documented before first run; ordinary users do not have to find or paste an
  Application ID unless using the developer override

## R17: Local TTS Quality, Performance, And Module Lifecycle

Level: Medium

Description:
The NeuTTS experiment proved the optional worker boundary and fast persistent
GPU synthesis, but repeated listening still found slurring, inconsistent
articulation, and fragile reference conditioning. ADR 0012 retires that
runtime rather than shipping a large custom codec and native-runner surface
that did not meet release quality.

Local cloning now uses one bounded OpenAI-compatible Go adapter with optional
Faster Qwen3-TTS or Chatterbox server modules. Those modules improve the model
choices but add isolated Python/PyTorch environments, multi-gigabyte downloads,
hardware-specific wheels, and long first loads. Quality, warm latency, clean
cancellation, browser playback, and GPU coexistence with the chat LLM require
direct acceptance; protocol compatibility alone cannot prove them.

Mitigation:

- keep Python, Torch, CUDA, and model code in an optional child process behind
  ADR 0003; preserve the `CGO_ENABLED=0` core
- install each module only through the main decision tree or explicit module
  script; both show the pinned source revision, license, model, hardware
  target, install root, and expected disk impact before consent
- serialize Hugging Face model-file finalization on Windows and materialize
  Faster Qwen into ordinary app-owned files rather than runtime snapshot links;
  bind those files to the selected repository with a checked manifest, reject
  linked materialized paths, and retry with retained files and local metadata
  only for that same repository after failure
- resume source, environment, package, and model artifacts left before module
  state is committed; exclude only installer-generated package metadata from
  the managed checkout's integrity check, while tracked edits and unknown
  untracked files continue to block replacement
- keep Faster Qwen reference selection in Settings > Voice; command-line
  installation may finish without a reference, app status must distinguish
  that state from missing runtime files, and module updates must preserve
  GUI-owned reference values
- recommend Faster Qwen3-TTS only for NVIDIA/CUDA systems; use Chatterbox as
  the CPU/broader-hardware fallback and never advertise an unsupported Faster
  Qwen CPU mode
- bind managed servers to loopback, reject occupied ports, launch without a
  shell, suppress the Chatterbox standalone browser, and stop only a child the
  worker started
- bound request text, error bodies, response audio, queue depth, and deadlines;
  repair streamed WAV headers only after the bounded clip is retained; keep the
  playable core retention ceiling at 8 MiB per clip and nine clips (72 MiB
  worst case), independently of the worker's larger HTTP response ceiling
- default managed Faster Qwen to fixed seed `1337`, expose explicit New seed
  and Varied controls, and reseed Python, NumPy, and Torch inside the server's
  serialized inference lock; never add this extension to generic compatible
  providers; bound managed generation to a generous text-proportional window so
  a sampled failure to emit an end token cannot monopolize the queue
- carry Faster Qwen seed and tone controls on each speech request so normal
  delivery edits preserve the loaded model and reference cache; for actual
  process-configuration changes, reconfigure and restore the persisted
  auto-launch roles instead of leaving a stopped worker behind
- verify the owned model-server child during health checks and clear stale
  readiness when it exits, so an explicit Start can relaunch it rather than
  returning success from the still-running adapter process
- recommend a clean, exact-transcript, 3-to-10-second Faster Qwen reference and
  retest with a shorter excerpt before treating stochastic sampling as the sole
  cause of inconsistent cloning
- keep bearer credentials environment-only, redact provider errors, and clear
  adapter credentials before launching a managed model server
- migrate retired provider selections to voice output off instead of silently
  choosing a replacement with different privacy and hardware implications
- keep ElevenLabs as an independent cloud option while local acceptance is
  incomplete

Exit evidence:

- on representative clean reference WAVs, capped listening finds no persistent
  slurring, truncation, speaker drift, or unexpected pace changes
- cold load, time to first playable audio, warm completion, and repeat latency
  are recorded for supported hardware and remain usable beside the selected
  local LLM
- cancellation terminates the active request, the next request succeeds, and
  unload/app shutdown leave no owned server process
- Firefox and Chromium play retained WAV output; auto-launch off never stops an
  external service; occupied-port and startup-failure states are actionable
- installer plan-only and saved-choice update paths change no files or settings

Status 2026-07-31: NeuTTS code, settings, UI, runner, reference encoder, and
llama.cpp coupling are removed. The generic adapter, main-installer and
standalone Faster Qwen3-TTS/Chatterbox paths, provider-scoped settings,
auto-launch ownership, migration, and automated protocol/lifecycle tests are
implemented. The clean-host path repairs WinGet, installs Git/uv, provisions
module-compatible Python 3.10 or 3.11 plus PyTorch/model assets, and does not
require a preinstalled compiler. Faster Qwen installation now deliberately
defers its reference WAV and exact transcript to Settings > Voice without
blocking runtime installation or allowing updates to erase those values.
Windows model downloads serialize finalization, materialize the Faster Qwen
runtime outside cache snapshot links, and retry with retained files and local
metadata, avoiding `WinError 1314` and incomplete-link failures on ordinary
non-Developer-Mode accounts. Interrupted installs can resume before module
state exists without
discarding completed packages or treating installer-generated metadata as a
source edit, and nested module verification no longer invalidates the parent
installer: TTS module scripts now run in an isolated Windows PowerShell process
before launcher and state finalization continue. Qwen seed and tone saves now
apply per request without stopping the resident model; true runtime changes
restore auto-launch policy, and owned-child exits invalidate cached readiness.
A one-frame full-path warm-up reduced the measured local cold start from 14.82
to 13.06 seconds while retaining about 0.40 seconds to first audio. Live
listening, broader latency, browser, and VRAM
acceptance remains open. Historical NeuTTS measurements remain in
`docs/goal-scorecard.md` and `docs/perf-baseline.md`; they are not evidence for
the replacement modules.

## R18: LAN And Mobile Secure-Context Requirements

Level: Medium

Description:
Web Bluetooth and browser microphone capture only work in secure contexts.
`http://localhost` qualifies, so the default single-machine setup is fine, but
any LAN/mobile use of Bluetooth dispatch or voice input requires HTTPS on a
LAN address. StrokeGPT-ReVibed needed a generated local CA, an Android
certificate-helper endpoint, and exact-IP certificate SANs to make mobile
Chrome work — a large support surface that is easy to promise accidentally by
saying "works on your phone".

Mitigation:

- keep localhost HTTP as the default boundary with no surprise login or
  certificate requirement
- allow LAN only as ADR 0017's atomic boundary: one exact private/link-local
  address, TLS 1.2+, a currently valid matching IP SAN, an operator-trusted CA,
  and at least one enabled backend account; reject wildcard, hostname, public,
  plain-HTTP, and uninitialized-account configurations before serving
- keep automatic local CA creation, renewal, client trust installation, and an
  Android certificate helper out until a real mobile pass defines a supportable
  lifecycle
- retain exact same-origin/Host enforcement, login throttling, opaque secure
  sessions, the controller lease, and authentication-independent Emergency Stop
- never describe Bluetooth or voice features as LAN/mobile-capable before the
  secure-context story exists

Exit evidence:

- a recorded decision on LAN/mobile scope, and — if in scope — a working
  documented HTTPS flow verified from a real mobile browser

Status 2026-07-09: Phase 13 records **localhost-only** microphone support.
MagicHandy does not claim LAN/mobile voice input. HTTPS, local CA, exact-IP
SANs, and Android certificate support remain a Phase 16 exposure decision.

Status 2026-08-28: Phase 20 and ADR 0017 implement the fail-closed backend
foundation without changing the default. `localhost`, IPv4 loopback, and IPv6
loopback HTTP remain available. An opt-in LAN listener requires an exact
private/link-local IP, a valid certificate/key matching that IP, and an enabled
account; every private route is authenticated, browser origins are exact, and
expired sessions can still issue Stop. Wildcard/hostname/public/HTTP LAN binds
fail before serving. Account GUI, automatic CA/trust lifecycle, and the real
mobile exit run remain open, so MagicHandy still does not claim supported
LAN/mobile microphone or Web Bluetooth behavior.

## R19: Datastore Migration And Budget Risk

Level: Medium

Description:
Moving the three JSON stores (settings, memory, prompt sets) into a single
SQLite datastore (ADR 0008, `modernc.org/sqlite`) introduces a schema, a
migration surface, a one-time JSON→SQLite import, and a new dependency that adds
binary size and RSS. A botched import or migration could lose user data; an
unmeasured dependency could erode the memory and binary budgets that justify the
rewrite; and SQLite's single-writer model can surface `database is locked` if
concurrency is handled naively.

Mitigation:

- pure-Go driver only (`modernc.org/sqlite`), preserving `CGO_ENABLED=0` and
  free cross-builds; never a CGo driver
- non-destructive one-time import: keep the JSON file contents (renamed
  `*.migrated`) rather than deleting them; each legacy domain imports inside a
  SQLite transaction and archives only after commit, with settings import
  reported in load status
- forward-only migrations keyed on `PRAGMA user_version`, run transactionally at
  open; negative and newer-than-binary versions are clear errors, never an
  index panic or silent downgrade; current schemas are checked for required
  tables, columns, indexes, foreign-key enforcement, and referential integrity
- schema v8 reserves the divergent Rockfire v4-v7 lineage and reconciles its
  core settings/prompt shapes idempotently while preserving uninterpreted LSO
  tables for the explicit migration phase
- one process-owned connection pool shared by all logical domains, with WAL,
  per-connection pragmas, a bounded four-connection pool, one warm idle
  connection, `busy_timeout`, and one serialized writer
- re-measure binary size and idle/active RSS when Phase 11B lands and record in
  `docs/goal-scorecard.md`; the Phase 11B RSS miss is recorded as a waiver, not
  silently relaxed
- preserve the redaction contract: the connection key is never returned by
  reads, diagnostics, or exports; the `.db` file carries the same at-rest
  sensitivity as `settings.json` did
- corrupt-store startup: `quick_check(1)` identifies physical corruption,
  quarantines the exact DB/WAL/SHM files in a private recovery directory, starts
  a fresh current schema, and reports only the backup path; logical schema
  damage still fails clearly rather than discarding data
- schema v10 archives invalid or oversized active settings documents before
  defaults become active, caps recovery history at 20, and never exposes the
  preserved document through public state or diagnostics
- restrict the data directory to `0700` and database sidecars to `0600` on
  POSIX; Windows uses the current user's profile ACL

Exit evidence:

- Phase 11B: settings, memory, and prompt sets round-trip through SQLite with
  tests; the JSON import is covered by fixtures (present, absent, corrupt);
  binary size remains within target; RSS has a recorded waiver; redaction tests
  still pass
- Phase 14: patterns, programs, and reversible feedback round-trip through
  SQLite; synthetic main-v2 and Rockfire-v7 fixtures migrate to v8 without data
  loss; pure-Go build and size budget remain green
- 2026-07-18 persistence audit: physical-corruption quarantine, settings
  recovery history, negative/newer-version handling, current-schema and
  foreign-key validation, transaction-panic rollback, private POSIX modes, and
  one shared production pool have focused regression coverage; the stripped
  binary remains below 30 MB and current idle RSS remains within the SQLite
  waiver

Relates to R8 (user migration) and R11 (goals unmeasured).

## R20: MagicHandy + LSO Merge Integration Risk

Level: Medium

Description:
Merging LSO's feature set (Intiface/Buttplug transport, motion blocks/queue,
personas, a feature-rich frontend, localization) onto the Go core brings large,
fast-moving surface from a different lineage and different structure/style
preferences. Without shared, enforced standards a merge of this size can erode
the properties that justify the rewrite: a second motion path or a transport
that bypasses the engine (R14), duplicated personalization/content systems that
drift, a heavier browser footprint than the efficiency goal allows, oversized
files or weakened CI gates slipped in to "make it pass," and committed runtime
data or duplicated build artifacts. Two parallel frontends or two motion-content
models shipping at once is the concrete failure mode.

Mitigation:

- one shared floor for every contributor and agent (`AGENTS.md`), enforced by CI
  on every branch before it merges to `main`; gates are strengthened, not
  weakened, as the surface grows
- new transports (e.g., Intiface) implement the `transport` interface only and
  are covered by the motion safety gate; every motion source produces semantic
  targets for the shared engine (no parallel path, R14)
- converge duplicated systems: one canonical frontend, one personalization
  model, one motion-content model — decided deliberately and recorded as ADRs
  (`docs/lso-merge-integration.md`, `docs/lso-merge-alternatives.md`), not
  defaulted-into by merge order
- re-measure RSS, binary size, and browser bundle cost as capability lands, and
  record it in `docs/goal-scorecard.md`; heavy UI features must earn their weight
- repository hygiene: no committed `*.db`/`-wal`/`-shm`, caches, `node_modules`,
  `.scratch/`, or duplicated large binaries; split oversized files rather than
  raising the budget by default

Exit evidence:

- the merged app ships one frontend, one motion path, and one personalization
  model; CI (Go + frontend) is green with no weakened gates; budgets are
  re-measured and recorded; the open merge decisions are settled as ADRs

Relates to R14 (per-source motion divergence), R11 (goals unmeasured), R9 (UI
regression), and R8 (user migration).

## R21: Imported Motion Content Risk

Level: High

Description:
Pattern share files and third-party funscripts are untrusted inputs that can be
huge, malformed, nearly stationary, unexpectedly long, or physically harsh.
Treating media-timed scripts as repeatable patterns can also preserve long
inactive gaps or normalize an unusable span into misleading motion.

Mitigation:

- cap source files and request bodies at 8 MiB, browser-inspected sources at
  20,480 actions, selected/backend payloads at 4096 actions, durations at 24
  hours, and stored pattern/program capacities; reject unknown declared schemas,
  malformed metadata, missing positions, and non-finite/out-of-range values in
  both the browser inspection path and backend parser
- distinguish finite programs from repeatable patterns at schema, API, and
  engine levels; only explicit pattern import strips long stationary gaps and
  normalizes relative amplitude
- validate/simplify authoring input server-side and preview with the exact
  backend sampler; merge backend-owned knots into compact rendered previews so
  long-cycle reversals cannot disappear through uniform-sample aliasing; never
  execute raw file payloads or construct transport commands from imported data
- snap trims to source actions and display their exact duration; keep waveform,
  selection shading, pointer mapping, and fixed-size trim targets in one
  coordinate system; keep zoom/pan independent from selected and submitted
  content, provide direct proportional viewport scrolling, release wheel input
  at zoom limits, preserve every selected knot when importing a finite program,
  and reject loop selections above the 255 essential-knot storage bound without
  imposing a false 6.6-second maximum
- route all playback through the shared engine and user speed/stroke envelope;
  controller ownership, Pause, and global Stop remain unchanged
- for generated catalog loops, reject repeated fixed-endpoint micro-strokes and
  repeated same-span runs before the acceleration/reversal fitter; keep
  user-tested timing-preserved promotions visibly `curated` instead of implying
  that generated-motion budgets were applied

Exit evidence:

- malformed/bounds/inversion/gap fixtures pass; imported program completion and
  shared-engine ownership pass at the HTTP layer; a capped real-device sample
  confirms generated and imported content has no unexpected stop, step, or
  reversal behavior

Status 2026-07-19: browser/backend malformed-input tests pass. The Import tab
also passes focused zoomed-coordinate and payload tests plus rendered 1280x800
and 390x844 checks. Both trim targets measure 44 CSS pixels, fitted boundaries
remain inside the frame, vertical wheel input zooms around the cursor, horizontal
wheel input pans, and a proportional pointer/keyboard scrollbar moves the
viewport directly. An isolated backend persisted the trimmed fixture at the
displayed 4:15 duration with all 16 selected knots. Physical-feel evidence
remains open. Three real-world scratch funscripts (3,536-7,394 actions over
20-37 minutes) confirmed that valid 12-20 second loop slices preserve their
duration, while the old 74-point card preview omitted 102-190 stored knot times.
Compact previews now insert saved knots, and impossible dense selections are
blocked before upload by the backend's essential-knot contract.

Status 2026-07-20: live curation identified and retired six built-ins that
combined a fixed return point with 10-20% micro-strokes or repeated 30-40%
spans near the reversal floor. Six replacement source cycles now have a 30%
per-travel floor, at least four amplitude bands, bounded endpoint reuse, and no
long equal-amplitude run before the existing acceleration/reversal fit. Only
those six carry the experimental gate. `Hard and Regular` and `playful jerk`
preserve their accepted user timing under a `curated` tag. Automated hardware
motion was not run for the replacements; their below-40% physical-feel pass
remains open.

Status 2026-08-01: a later bulk generator placed 171 clips into the enabled
catalog without the generator motion budgets above; one traced clip reversed
20% of travel every 40 ms. All 171 remain active without a bulk exemption. A v3
manifest and tests enforce source identity, normal speed controls, complete
roughly 450 ms resampling for the 165 source curves outside hard budgets,
low-prominence reversal removal, and final acceleration/reversal compliance.
The established feel envelope labels 170 experimental and leaves only `Easy
Drive 4` normally labeled. The offline generator has no live-app posting path,
and the Go bulk importer requires explicit experimental acknowledgment. These
controls do not substitute for capped hardware acceptance.

Status 2026-08-13: the quality pass now ships 59 generated survivors rather
than all 171. Fifty remain experimental and nine clear the fitted-feel gate.
Model-facing metadata was rewritten around geometry and relative rhythm;
legacy pace-biased IDs remain only for persistence and are replaced by opaque
handles in prompts. A narrow seed migration updates untouched legacy default
names while preserving user renames, enablement, weights, and feedback. Runtime
speed normalization prevents authored travel rate from silently overriding the
LLM or user's requested pace. Capped physical acceptance remains required.

Status 2026-08-14: a retained Cloud trace separated a reported stop in `Hard
and Regular` from transport starvation. HSP point times stayed continuous and
accepted, but mean-rate normalization stretched its disproportionately long
26-point return into a roughly one-second sub-perceptual leg at 40%. A catalog
sweep found the same accidental runtime dwell in `Deep-Partial Sequence`.
Two-cycle sampling also found a 360 ms `Tease` slowdown split across its loop
seam. Their turning positions remain unchanged while the bad leg timing is
rebalanced. Planner and Cloud-resolution tests now cap continuous motion below
45% travel/s at 250 ms for default patterns without intentional holds. The
post-fix physical-feel check remains open; the failing scenario used Cloud REST
at 35-40%, roughly 322-379 ms request latency, with one later unrelated network
failure followed by a successful Stop.

Relates to R1 (real-device validation), R8 (migration), and R14 (one motion
path).

## R22: Third Dispatch Owner (Intiface) Surface Risk

Level: Medium

Description:
ADR 0010 adds Intiface/Buttplug as a third dispatch owner. Unlike the two
Handy owners, it is immediate-mode: a host-side pacer schedules every command
in wall time, with no device-side buffer, starving report, or stroke-window
projection. New failure modes (timer drift, underrun, missed stop-preemption,
double or missed window projection) could make the same motion feel different
per owner or, worse, weaken stop behavior on one path. Buttplug-side devices
also vary widely in actuator limits the Handy owners never see.

Mitigation:

- one owner-agnostic contract suite (Phase 14B.0): exactly-once window
  projection, exactly-once reverse mapping, stop preemption, honest health
  reporting, and no resampling — run against every owner including a fake
  Buttplug server
- motion-feel shaping (PCHIP, acceleration/reversal budgets, cycle and dwell
  floors) stays engine/generator-side so owners cannot diverge by design
- the pacer detects its own underrun and reports honest playback state; the
  stop-and-report rule (ADR 0006) applies — never a silent fallback
- absolute-deadline writes do not await the preceding ACK; an eight-command
  bounded ledger correlates responses under transport-owned deadlines, while
  missing/rejected ACKs invalidate the generation and force Stop without retry
- expired segments are discarded instead of replayed in a burst; live lateness
  has a 25% duration-compression bound, and per-wire timing/ACK telemetry is
  included in `motion_trace.v3`
- startup anchors the first neutral point before the shared playback clock
  starts; reverse mapping is fixed at append time while the min/max envelope
  retains the cross-owner immediate-update contract
- selected `DeviceMessageTimingGap` raises the shared sampler cadence and
  selected `StepCount` is exposed as an honest physical resolution limit
- Buttplug ping keepalive stays enabled so Intiface stops devices if
  MagicHandy dies
- live validation drives the same Handy through all three owners as a direct
  like-for-like consistency measurement before the owner is recommended

Exit evidence:

- contract suite green for all owners; Stop/owner-switch/goroutine-lifecycle
  gates extended to Intiface; a real-device session confirms matched feel on
  the same Handy over Cloud REST and Intiface and clean stop behavior on a
  non-Handy Buttplug device if available

Implementation status (updated 2026-07-13): the neutral-frame and shared Stop-preemption
suites plus fake-server handshake, keepalive, selection, underrun, rejection,
Stop, Close, HTTP runtime, and UI tests are implemented. Matched capped Handy
runs over Intiface and Cloud REST passed Start, Pause, phase-preserving Resume,
reverse quick refresh, active and repeated-idle Stop, and close-time Stop where
applicable, without starvation. Automated delayed/missing/rejected ACK,
deadline, coalescing, startup-anchor, timing-capability, concurrent Stop/Close,
and wire-telemetry cases now cover the immediate-mode deficiencies found in the
follow-up review. The risk remains Medium until the revised pacer receives a
matched subjective run; no non-Handy linear device was available for the
conditional run.

Review update 2026-07-20: adaptive buffered owners preserve authored knots,
while an Intiface frame deliberately stays on the selected device timing floor
and is tested not to inject a closer knot. Browser Bluetooth retains 0.1%
firmware point resolution; Cloud's integer API floor is documented rather than
hidden. A matched subjective run of shallow patterns and active envelope
changes is still required, so the risk remains Medium.

Review update 2026-07-21: selected Intiface `StepCount` now participates in the
shared quantization-aware reduction after scaling through the stroke window.
Cloud and Browser Bluetooth align the engine clock to accepted Play rather than
including setup/prebuffer time. A capped Cloud retarget checklist completed 15
commands without failure or starvation and ended in confirmed Stop; corrected
clocking reduced the run from 13 add batches / 125 points to 10 / 90. The risk
remains Medium pending the matched subjective Intiface run and a non-Handy
linear-device check.

Relates to R1 (real-device validation), R14 (one motion path), R16 (device
coverage), and R20 (LSO merge integration).

## R23: Emergency Stop Delivery Gaps

Level: Critical

Description:
The permanent Stop control is mounted outside routes. Active, paused,
repeated-idle, and no-engine requests cancel local work and attempt the selected
owner, with explicit errors when transport delivery fails. An unreachable
backend still cannot forward a Browser Bluetooth command, and no path may infer
physical delivery from local stopped state alone.

Implementation status (2026-07-12): active, paused, idle-engine, and no-engine
paths now attempt the selected transport; unavailable owners preserve local
stopped state while returning an explicit error. Intiface hardware produced
distinct successful active and repeated-idle Stop commands plus a recorded
close-time Stop. Browser-backend loss and current Cloud/Browser hardware retry
evidence remain open, so the risk stays Critical.

Implementation status (2026-07-27): all public owner Stop routes now enter one
global coordinator that advances the Stop epoch, invalidates media/chat/voice
work without waiting behind ordinary media lifecycle work, stops modes, and
then stops the engine or selected owner. Disconnect and owner-selection paths
use the same admission closure before owner-specific teardown. Automated race,
stale-media, canceled-dispatch, and shutdown-stream regressions cover the new
coordination; hardware retry evidence remains open.

Mitigation:

- retain regression coverage that every Stop request attempts the selected
  dispatch owner whenever available, including idle-engine and no-engine states
- preserve the current invariant that local planners and motion state stop even
  when transport delivery fails; surface the failure instead of claiming
  physical delivery
- complete current Cloud REST and Browser Bluetooth hardware checks for retry,
  owner-switch, and failed-delivery reporting; retain backend-loss coverage
- keep Stop mounted outside routes and controller ownership gates

Exit evidence:

- automated tests prove unconditional delivery attempts and local teardown for
  active, paused, idle, no-engine, read-only, owner-switch, and transport-error
  paths; capped hardware checks record Cloud REST and Browser Bluetooth results

Relates to R1 (real-device validation), R3 (transport behavior), and R9 (UI
safety regression).

## R24: Browser Microphone And Managed ASR Format Mismatch

Level: High

Description:
Browser voice input records WebM/Opus or Ogg, while the managed parakeet.cpp
path accepts WAV input. The original implementation forwarded compressed bytes
unchanged and was incompatible with the default managed microphone path. The UI
now decodes the recording, downmixes and resamples it to 16 kHz mono, and emits
real PCM16 WAV before submission; the managed API rejects non-WAV content.
The original control also acquired and destroyed the microphone for every
utterance, so speech begun during browser device/DSP startup was unrecoverable.
Its first "hands-free" revision merely recorded one fixed interval and stopped,
which did not satisfy the interaction contract.

Mitigation:

- run an end-to-end browser MediaRecorder sample through the pinned managed
  runner before claiming push-to-talk acceptance
- keep browser-side WAV conversion bounded; native audio dependencies must not
  enter the pure-Go core
- keep user-started hands-free capture active until manual stop; use bounded
  browser VAD with pre-roll, calibration, sensitivity/end-of-speech controls,
  and a three-phrase pending queue while the browser remains the
  permission/device owner
- upload raw audio and use a private process-session worker `audio_ref`; never
  log or diagnose captures, remove terminal work immediately, remove the owned
  session on shutdown, and reap stale crashed sessions after the bounded request
  window
- reject unsupported formats with a visible actionable error rather than
  forwarding bytes optimistically
- retain fixture tests for every accepted browser format and the WAV provider
  contract

Exit evidence:

- Chrome/Edge localhost push-to-talk and repeated hands-free phrases produce
  accurate transcripts through the pinned managed Parakeet install, with
  format/error tests and no core CGo dependency

Status 2026-07-15: the deterministic format mismatch, repeated cold-start path,
and fixed-interval pseudo-hands-free behavior are fixed. Hands-free now uses an
AudioWorklet, bounded VAD/pre-roll, sequential phrase submission, persisted
tuning controls, raw HTTP upload, session-scoped `audio_ref` staging, backend
Stop-generation fencing, and lifecycle/boundary regression tests. The engine
also rejects starts admitted before its latest Stop, covering delayed non-chat
motion requests. A production-boundary fixture run started the installed CPU
runner and pinned model, transcribed the official Dave WAV after canonical
16 kHz normalization, stopped the worker, and left no related process running.
A real Chrome/Edge run through the pinned runner/model remains required to close
the risk and quantify first-word accuracy and end-to-end latency.

Relates to R17 (voice dependency and latency risk) and R18 (browser security and
LAN microphone access).

## R25: Browser-Clock Media Sync Alignment

Level: High

Description:
Phase 18's synchronized funscript player uses the browser video as
its clock while device commands cross transports with different buffering and
wire latency. Poor anchoring, over-eager drift correction, a lost heartbeat, or
a seek race could make motion visibly late, jump phases, or continue after the
player is no longer authoritative.

Mitigation:

- keep media as a semantic client of the one motion engine; it never imports or
  dispatches to a transport directly
- make the video clock authoritative through explicit controller-gated
  play/pause/seek/heartbeat events, one bounded sync session, and generation
  fencing around seeks and replacement
- use app-owned paired-video controls with explicit scrub start/commit/cancel;
  hold the video at one captured timestamp until motion is armed instead of
  inferring intent from browser event timing
- fence every mounted player with a random session id and monotonic event
  sequence so late close/arm requests cannot cross session ownership
- Stop and explicitly re-arm on seek completion, rate change, confirmed/hard
  drift, or an effective video-speed-policy change; never phase-jump or rewrite
  an already buffered transport queue
- require future browser data before arming; actual `waiting` and decode failure
  Stop motion, while an advisory `stalled` event alone does not stop a player
  that still has buffered data
- stop after bounded heartbeat loss, preserve unconditional Stop, and expose
  anchor/drift/re-arm and media-speed-policy state in `motion_trace.v3`
- require fake-transport integration tests before device use, followed by a
  capped real-device alignment session across the supported owners

Exit evidence:

- M2 tests cover play readiness, seek, pause, resume, waiting/recovery, decode
  failure, ended, stale/reordered session events, calibrated/confirmed/hard
  drift, heartbeat loss, Stop, media speed-policy changes, and controller loss
  with one engine play path and no goroutine leaks
- M3 records trace-derived drift and subjective alignment on real hardware

Status 2026-07-22: M1-M2 are implemented. Exact-name scripts share one bounded
loader between the timeline and engine. The fake-transport path covers explicit
play after browser readiness, passive heartbeat, seek Stop/re-arm, pause,
waiting/canplay recovery, decode failure, end, heartbeat loss, Emergency Stop
generation fencing, closed-player request reordering, controller gates, and
clean invalidation when the effective video speed policy changes. Heartbeats
cannot start motion. The UI holds video while arming, labels read-only playback
as timeline-only, and leaves ordinary video usable when a script is invalid or
complete. R25 remains High until subjective alignment and supported-owner
evidence satisfy M3.

Review update 2026-07-22: the retained failing trace exposed both shallow Cloud
coverage and a later center-amplitude transform that was incorrectly labeled a
speed limit. Media now selects a 10-second Cloud minimum, prebuffers in
owner-capped batches, emits exact authored knots across engine chunks, and
preserves authored positions by default. The optional cap bounds each authored
segment's displacement without changing its timestamp or direction, and an
active policy change requires Stop/re-arm.

A capped post-fix Cloud run at 01:17 kept sixteen heartbeats `following` with
1 ms calibrated drift, 327-364 ms successful append latency, source-exact
reversals, and no starvation. A separate 01:16 to 01:30 seek re-armed once and
stayed healthy. R25 remains High for subjective continuity confirmation and
matched Browser Bluetooth/Intiface evidence.

Review update 2026-07-30: paired videos no longer expose native controls that
can advance the media clock before the application takes over. Play freezes at
the click timestamp until the backend confirms an active arm. Long pointer
scrubs preserve their start-time play intent without a timeout, issue one Stop,
and re-arm once at the committed timestamp; paused scrubs stay paused. Script
filter writes use the same freeze/stop/re-arm lifecycle. Focused browser tests
cover these paths, but no new real-device run was authorized for this pass, so
R25 remains High and M3 is unchanged.

Relates to R1 (real-device validation), R3 (transport behavior), R9 (UI safety
regression), R14 (one motion path), and R23 (Stop delivery).

## R26: Library Auto-Scan And Catalog Cleanup

Level: Medium

Description:
An opted-in startup scan performs bounded filesystem IO without a foreground
browser action. Automatic cleanup could also erase useful catalog state when a
removable/network location is offline, only partly readable, or truncated by a
scan bound. Follow-up thumbnail generation or conversion can amplify that work.

Mitigation:

- startup scanning is off by default, runs at most once per core start, and has
  no timer or watcher
- manual and startup triggers use one scanner, one polled backend state, one
  cancellation path, and one structured scan summary
- cleanup deletes catalog rows and owned cached thumbnails only, never source
  media, and only after the corresponding root was enumerated completely
- unavailable, cancelled, permission-failed, and file-limit-truncated roots
  preserve every unseen row regardless of the cleanup preference
- a cancelled or failed scan never starts thumbnail or conversion follow-up
  work; successful follow-ups remain bounded, cancellable, and visible in the
  shell notification center
- tests cover startup opt-in, cleanup on/off, complete-root deletion, partial
  root preservation, and legacy-setting defaults

Exit evidence:

- automated scanner/config/HTTP tests stay green and a temporary-root UI run
  shows startup activity, completion, warning, and cleanup summaries without
  touching source media

Status 2026-07-28: mitigations are implemented. The risk remains Medium pending
a Windows removable-drive and unavailable-network-root acceptance pass.

## R27: Portable Persona Archive Input Risk

Level: Low

Description:
Persona share files combine user-authored prompt text, lore, and an optional
image in a ZIP container. A malformed or hostile archive could attempt path
traversal, decompression amplification, oversized allocation, ID collision,
unsupported future semantics, or smuggling of settings and motion privileges.

Mitigation:

- accept exactly `persona.json` and an optionally declared `portrait.jpg`;
  reject directories, non-regular files, unknown/duplicate names, unknown JSON
  fields, and unsupported schema versions
- cap the compressed request at 4 MiB and independently cap declared and actual
  uncompressed manifest/portrait reads before allocation
- decode and dimension-check the JPEG, and apply the same persona/lore bounds
  used by direct editing before any database write
- generate fresh persona, lore, and custom behavior-profile IDs; resolve
  built-in profiles to the importing build's trusted local definition
- never carry sessions, timestamps, controller state, settings, memories,
  anatomy, capability gates, motion limits, or executable content
- require the active controller and the normal chat/Autopilot mutation lock for
  import; keep export read-only and serve it as a nosniff attachment

Exit evidence:

- store and HTTP tests cover round trips, fresh IDs, custom/built-in behavior
  profiles, lore/portrait preservation, runtime-metadata exclusion, traversal,
  unknown fields/files, asset mismatches, decompression bounds, controller
  ownership, and oversized bodies

Status 2026-07-30: mitigations and automated evidence are implemented. Keep the
risk open at Low until archives exported on one release are imported by a later
release during release-upgrade acceptance.

## R28: Windows Packaging And Optional Provisioning Trust

Level: High

Description:
The Windows setup shell installs an executable that can later launch
multi-gigabyte optional provisioning: managed runtime archives, Python
environments, GPU runtime packages, and model downloads. Unsigned development
artifacts also trigger Windows reputation warnings and provide no publisher
identity. A stale helper, unverified payload, accidental release publication,
or silent replay of old installer choices could change the machine or replace a
working runtime without informed consent.

Mitigation:

- build the setup EXE and portable ZIP from one staged payload with exact commit
  provenance, GPL source URL, per-file SHA-256 manifest, and outer checksums
- keep the pull-request workflow read-only and artifact-only; label its unsigned
  setup output `unsigned-ci`, retain it briefly, and give it no release path
- require `ReviewedUnsignedPublic` plus Microsoft's completed false-positive
  case ID and the explicitly approved alpha.8 through alpha.11 and alpha.13 through alpha.37 versions for unsigned setup
  publication; build into a dedicated public directory and lifecycle-test that
  exact setup before publishing three explicit paths
- retain `PortablePublic` as a fail-closed fallback: build no setup, verify the
  ZIP and one-entry checksum, scan the exact public directory, and publish only
  those two explicit paths
- keep Inno Setup thin: files, shortcuts, uninstall metadata, and launch only;
  all optional decisions and progress remain in the backend-authoritative GUI
- collect optional runtime/voice choices without executing them, show purpose,
  license, hardware, and disk cost first, then require one controller-gated GUI
  action to submit the reviewed installation plan
- keep that plan in one sequential cancellable backend queue with per-component
  state, bounded terminal output, process-tree teardown, and resumable partial
  downloads
- install managed llama.cpp only from official pinned CPU/CUDA archives with
  fixed size and SHA-256 checks, safe extraction, a bundled upstream license,
  staged commit/device probes, and atomic activation; do not provision a C++ or
  CUDA compiler toolchain for that path
- make source updates core-only so saved legacy installer state cannot silently
  rebuild llama.cpp, Parakeet, or a Python environment
- make uninstall data disposition explicit: recommend a bounded purge of the
  packaged `%APPDATA%\MagicHandy` root for clean reinstall, support explicit
  retention, and never infer or delete external/custom paths
- keep packaged defaults on loopback; Phase 20's LAN HTTPS/account flags remain
  explicit operator configuration outside the installer GUI until R18 mobile
  trust/renewal evidence exists. Automatic update application remains deferred
  until its signing, rollback, and motion-stop design exists
- keep release discovery read-only and opt-out: query only the canonical GitHub
  release list, select the highest backend-compatible stable or progressive
  prerelease version, cache and conditionally revalidate it, construct the
  release link locally, send no credentials, and never download or execute an artifact

Exit evidence:

- CI verifies payload/outer hashes and performs silent install, version, and
  uninstall smoke tests
- a clean standard Windows account installs, configures, updates, repairs an
  interrupted optional module, and uninstalls without preinstalled developer
  dependencies or undisclosed prompts
- a public setup release has valid, timestamped Authenticode on the installer
  and every payload executable through a documented protected process

Status 2026-08-03: the unsigned alpha.6 setup executable was classified at
launch as `Behavior:Win32/DefenseEvasion.A!ml` and its GitHub Release was
withdrawn. Exact checksum and CI provenance did not mitigate the absent
publisher identity or justify a security bypass. The file was submitted to
Microsoft for analysis. Microsoft completed case
`15c1e36d-fb35-4c5d-85de-83707169818a` with final determination `Not malware`,
reported no current cloud or client detection, and removed the detection.
ADR 0014 now separates pull-request `UnsignedCI`, version-bound reviewed alpha.8
through alpha.11 and alpha.13 through alpha.37, withdrawn portable-only alpha.12, and timestamped
`SignedPublic` policies. Release acceptance still
verifies every staged and outer hash, custom and Program Files installs,
shortcut/ARP metadata, active-process over-install, retained settings, explicit
data retention, bounded clean purge, and fresh state after reinstall. R28
remains High pending broader clean-machine voice acceptance and provisioned
production signing. The managed llama.cpp CPU and CUDA bundle paths have passed
real-network install, checksum, extraction, activation, and runner probes
without developer toolchains.

The exact alpha.6 setup hash showed 4 of 71 detections on VirusTotal. Its
sandbox summary reported no direct detection but attached generic obfuscation
and self-delete behavior tags to the unsigned Inno Setup overlay. Microsoft's
completed determination is authoritative for that submitted hash; alpha.6 stays
withdrawn and its immutable tag is not reused.

A controlled same-payload packaging comparison subsequently removed two
avoidable structural triggers from the CI setup: the 32-bit loader around an
amd64 payload and the solid `lzma2/ultra64` stream. The native-x64, non-solid
`zip/9` candidate passed PE-machine checks, the isolated installer lifecycle,
and a current Defender custom scan with no threats. Its larger approximately
17.9 MB size is an accepted transparency tradeoff. Alpha.8 through alpha.11 and
alpha.13 through alpha.37 may publish this hardened shape only through `ReviewedUnsignedPublic`,
bound to the completed case, alpha.9-and-later exact-artifact Defender scan, and
full lifecycle acceptance. This does not lower R28: each new hash is still
unsigned and trusted Authenticode remains the production exit evidence.

Alpha.12's portable-only GitHub Release was withdrawn and its immutable tag was
retained. Alpha.13 is the corrected distribution and explicitly restores the
three-artifact reviewed setup path without reusing or moving alpha.12.

## R29: Authenticated LAN Session And Credential Risk

Level: High

Description:
Opt-in LAN access moves private chat, personas, media metadata, model controls,
device configuration, and physical commands beyond the operating-system user's
loopback boundary. Weak passwords, credential stuffing, stolen cookies, an
over-broad bind, a certificate mismatch, cross-origin requests, or confused
controller/account authority could expose intimate data or command hardware.
Adding login identities does not by itself make the shared installation safe
for internet exposure or mutually untrusted tenants.

Mitigation:

- make LAN an atomic fail-closed startup mode: exact private/link-local address,
  TLS 1.2+, valid matching SAN, and at least one enabled account; reject public,
  wildcard, hostname, and plaintext bindings
- store salted Argon2id hashes with bounded parameters and generic failures;
  keep independent bounded per-IP and per-username login token buckets plus one
  cancelable global hash slot so a request burst cannot multiply memory cost
- enforce an eight-Unicode-character usability floor and a 1024-byte ceiling,
  show exact confirmation feedback without exposing the value, and continue to
  recommend a longer unique passphrase because the floor is not a strength claim
- store only random-session digests, use host-only HttpOnly Secure
  SameSite-Strict cookies, enforce idle/absolute/session-count bounds, and
  revoke sessions after password changes or disabling
- revoke the bootstrap session when protected first-run setup is saved so the
  administrator password is exercised immediately; normal reconfiguration does
  not silently revoke an independently authenticated session
- keep browser scheme/host/port exact and retain the controller lease as a
  second, separate authorization boundary; authentication never grants a new
  motion path or bypasses stop-first takeover
- leave the native host path picker loopback-only and keep raw passwords,
  session tokens, and private-key contents out of logs, settings, diagnostics,
  exports, and frontend storage
- keep Emergency Stop authentication-independent while every start, retarget,
  configuration write, private read, and takeover remains authenticated
- keep signed-in identity, per-session linked control context, and the
  controller lease separate; Self is the session default, inactive/unlinked
  targets are rejected, and no current motion API consumes the context
- bound profile images as decoded JPEGs, store them only under app-owned data,
  authorize reads to the owner/admin/active link, and reconcile interrupted or
  orphaned files at startup
- document LAN-only scope and forbid port forwarding, tunnels, public binds,
  reverse-proxy assumptions, and claims of per-user data isolation

Exit evidence:

- focused schema, credential, session, throttle, role, origin, startup, and Stop
  tests pass under the race and pure-Go gates
- a security review exercises CSRF, DNS/Host rebinding, JSON login/logout and
  local bootstrap, brute-force throttling, cookie theft/replay, session revocation,
  certificate expiration/renewal, and last-admin recovery
- a real second-device LAN run proves authenticated read-only/controller
  semantics and Stop without exposing the port outside the intended interface

Status 2026-08-28: ADR 0017's backend foundation and ADR 0018's account GUI are
implemented. Setup/Settings opt-in, React login/logout, session-expiry recovery,
administrator management, profile-image bounds/visibility, and Self-by-default
per-session linked context have focused automated coverage. Link invitations
and motion grants, MFA/recovery, automatic certificate lifecycle, formal
security review, race-suite evidence, and real second-device acceptance remain
open. The feature must continue to be described as opt-in LAN support, not
internet remote control or multi-tenant isolation.
