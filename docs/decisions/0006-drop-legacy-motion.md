# ADR 0006: Drop Legacy Motion Pathways

## Status

Accepted for the rewrite plan. Dispatch-owner scope revised by ADR 0010
(2026-07-11): Intiface/Buttplug joins Cloud REST and browser Web Bluetooth as
a third dispatch owner of the same single motion backend and
transport-neutral frame. Everything listed under "Dropped entirely" stays
dropped; the recovery rule (no silent fallback — stop and report) applies to
every owner.

## Context

StrokeGPT-ReVibed carries three transports (HSP, HAMP, HDSP), three motion
backends (`continuous`, `position`/Flexible, `hamp`), and legacy scripted modes
(`auto`/`edging`/`milking` via `MotionScriptPlanner`/`ScriptStep`) alongside the
modern Continuous + Freestyle path.

Most of the motion complexity, conditional branching, and the recurring stop/go
regressions came from keeping the legacy paths working as fallbacks while
Continuous-over-HSP was being validated. A rewrite has no obligation to carry
them. The old repo remains for reference, so nothing is lost by dropping them.

## Decision

MagicHandy supports exactly one continuous motion backend and one
transport-neutral timed-point frame. Cloud REST and browser Web Bluetooth encode
that frame as HSP for firmware-v4/API-v3 Handy devices; Intiface maps it to
Buttplug `LinearCmd` for one selected linear actuator. These are dispatch owners,
not separate motion engines or fallback backends. Modes are thin planners on the
shared sampler. Everything in "Dropped" is removed, not ported.

### Dropped entirely

- HDSP direct-position transport and firmware v3 support (including the
  `hdsp_fallback` schema)
- HAMP stroke-window transport and the `hamp` motion backend
- the `position` / "Flexible position/script" finite-playback backend
- `MotionScriptPlanner` / `ScriptStep` scripted-step machinery
- the legacy `auto_mode_logic` (Legacy Auto) mode
- the legacy scripted `edging_mode_logic` / `milking_mode_logic` implementations
- the motion-backend selector UI (only one backend remains)

### Kept, rebuilt fresh (not ported)

- HSP over Cloud REST (primary) and HSP over browser Web Bluetooth (local) --
  same semantics, different dispatch owner (see `bluetooth-ownership.md`)
- Intiface/Buttplug `LinearCmd` for one selected linear actuator, behind the
  same transport contract and Stop semantics (ADR 0010)
- the continuous sampler and retargeting model (`docs/motion-retargeting.md`)
- Freestyle and plain-chat continuous motion as motion-engine clients
- fixed patterns, programs (funscripts), anchor loops, and area focus as content
- Edge/Milk only if wanted, rebuilt as continuous-engine planners (Phase 11),
  never as scripted `ScriptStep` ports

## Consequences

Positive:

- legacy command families collapse to one neutral frame; backends collapse from
  three to one; the backend selector disappears
- removes every `if backend == hamp`/`hdsp_fallback` branch, the slide-window
  math, finite-position playback, and the scripted stop/go planner -- the
  largest sources of motion complexity and the recurring stop/go-after-morph bug
- the HSP invariants and retargeting model no longer need "HAMP/HDSP when
  explicitly selected" caveats
- directly serves the maintainability and lower-complexity goals

Negative / deliberate trade-offs:

- Cloud REST and Browser Bluetooth require Handy firmware v4 plus API v3 access.
  Firmware v3 Handy hardware, or users blocked from API v3, are unsupported and
  stay on StrokeGPT-ReVibed. Intiface instead requires a user-run server and a
  supported linear actuator; non-Handy hardware acceptance remains open.
- there is no silent owner fallback: if the selected owner's prerequisites fail,
  the app reports an actionable error and does not move the device.
  Recovery is "stop and report," never a silent downgrade (see
  `docs/motion-retargeting.md`, Recovery Behavior).
- the app should ship and manage its own API v3 Application ID if Handy API terms
  allow it. Treat that ID as a public client identifier, not a secret. Keep a
  developer override for testing or future revocation. The Handy connection key
  remains the user's private credential.

## Supersedes

Earlier "HAMP/HDSP fallback only when explicitly selected" language in the plan
and HSP-specific frame wording. ADR 0010 extends the neutral frame to Intiface;
HAMP, HDSP, and firmware-v3 fallback remain dropped.
