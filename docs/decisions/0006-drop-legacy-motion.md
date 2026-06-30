# ADR 0006: Drop Legacy Motion Pathways

## Status

Accepted for the rewrite plan.

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

MagicHandy supports exactly one transport family -- HSP (Handy firmware v4 /
API v3) -- over two dispatch owners: Cloud REST and browser Web Bluetooth. These
are dispatch owners for the same command family, not separate motion engines or
fallback backends. There is one motion backend: the continuous sampler. Modes are
thin planners on that engine. Everything in "Dropped" is removed, not ported.

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
- the continuous sampler and retargeting model (`docs/motion-retargeting.md`)
- Freestyle and plain-chat continuous motion as motion-engine clients
- fixed patterns, programs (funscripts), anchor loops, and area focus as content
- Edge/Milk only if wanted, rebuilt as continuous-engine planners (Phase 11),
  never as scripted `ScriptStep` ports

## Consequences

Positive:

- transports collapse from three to one; backends from three to one; the backend
  selector disappears
- removes every `if backend == hamp`/`hdsp_fallback` branch, the slide-window
  math, finite-position playback, and the scripted stop/go planner -- the
  largest sources of motion complexity and the recurring stop/go-after-morph bug
- the HSP invariants and retargeting model no longer need "HAMP/HDSP when
  explicitly selected" caveats
- directly serves the maintainability and lower-complexity goals

Negative / deliberate trade-offs:

- MagicHandy requires Handy firmware v4 plus API v3 access. Firmware v3
  hardware, or users blocked from API v3, are unsupported and stay on
  StrokeGPT-ReVibed.
- there is no fallback transport: if HSP prerequisites fail, the app reports HSP
  unavailable with a clear, actionable error and does not move the device.
  Recovery is "stop and report," never a silent downgrade (see
  `docs/motion-retargeting.md`, Recovery Behavior).
- the app should ship and manage its own API v3 Application ID if Handy API terms
  allow it. Treat that ID as a public client identifier, not a secret. Keep a
  developer override for testing or future revocation. The Handy connection key
  remains the user's private credential.

## Supersedes

Earlier "HAMP/HDSP fallback only when explicitly selected" language in the plan
and the HSP invariants. Those references are updated to HSP-only.
